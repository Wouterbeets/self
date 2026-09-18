package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	leaseAcquired   = "loop.lease.acquired"
	leaseRenewed    = "loop.lease.renewed"
	leaseReleased   = "loop.lease.released"
	leaseStolen     = "loop.lease.stolen"
	leaseRefused    = "loop.lease.refused"
	checkpointAsked = "loop.checkpoint.requested"
	checkpointYes   = "loop.checkpoint.approved"
	checkpointNo    = "loop.checkpoint.rejected"
	checkpointUsed  = "loop.checkpoint.consumed"
)

type goalLease struct {
	Goal       string    `json:"goal"`
	Owner      string    `json:"owner"`
	Invocation string    `json:"invocation"`
	Token      string    `json:"token"`
	Repository string    `json:"repository"`
	Branch     string    `json:"branch"`
	Worktree   string    `json:"worktree"`
	ExpiresAt  time.Time `json:"expires_at"`
	Seq        int       `json:"-"`
	Released   bool      `json:"-"`
}

func (l *goalLease) active(at time.Time) bool {
	return l != nil && !l.Released && at.Before(l.ExpiresAt)
}

type checkpoint struct {
	ID        string          `json:"id"`
	Pass      string          `json:"pass"`
	Actions   json.RawMessage `json:"actions"`
	Reason    string          `json:"reason,omitempty"`
	Requested int             `json:"-"`
	Approved  int             `json:"-"`
	Rejected  int             `json:"-"`
	Consumed  int             `json:"-"`
}

func (c *checkpoint) pending() bool {
	return c != nil && c.Approved == 0 && c.Rejected == 0
}

func (c *checkpoint) usable() bool {
	return c != nil && c.Approved > c.Requested && c.Rejected == 0 && c.Consumed == 0
}

type passAudit struct {
	ID       string
	Goal     string
	Status   string
	Plan     json.RawMessage
	Events   []Event
	Failures []string
	Budget   map[string]int
	Limits   map[string]int
}

func (st *state) applyRails(e Event) {
	if e.Via != doorKernel {
		return
	}
	switch e.Name {
	case leaseAcquired, leaseRenewed, leaseStolen:
		var l goalLease
		if json.Unmarshal(e.Payload, &l) != nil || l.Goal == "" || l.Owner == "" || l.Invocation == "" || l.Token == "" || l.ExpiresAt.IsZero() {
			return
		}
		l.Seq = e.Seq
		st.Leases[l.Goal] = &l
	case leaseReleased:
		var p struct{ Goal, Owner, Invocation, Token string }
		if json.Unmarshal(e.Payload, &p) != nil {
			return
		}
		if l := st.Leases[p.Goal]; l != nil && l.Owner == p.Owner && l.Invocation == p.Invocation && l.Token == p.Token {
			l.Released = true
			l.Seq = e.Seq
		}
	case checkpointAsked:
		var c checkpoint
		if json.Unmarshal(e.Payload, &c) != nil || c.ID == "" || c.Pass == "" || len(c.Actions) == 0 {
			return
		}
		c.Requested = e.Seq
		st.Checkpoints[c.ID] = &c
	case checkpointYes:
		var p struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(e.Payload, &p) == nil {
			if c := st.Checkpoints[p.ID]; c != nil && c.pending() {
				c.Approved = e.Seq
			}
		}
	case checkpointNo:
		var p struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
		}
		if json.Unmarshal(e.Payload, &p) == nil {
			if c := st.Checkpoints[p.ID]; c != nil && c.pending() {
				c.Rejected, c.Reason = e.Seq, p.Reason
			}
		}
	case checkpointUsed:
		var p struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(e.Payload, &p) == nil {
			if c := st.Checkpoints[p.ID]; c != nil && c.usable() {
				c.Consumed = e.Seq
			}
		}
	}

	if !strings.HasPrefix(e.Name, "loop.pass.") && e.Name != "loop.budget.exhausted" {
		return
	}
	var p struct {
		Pass     string          `json:"pass"`
		Goal     string          `json:"goal"`
		Plan     json.RawMessage `json:"plan"`
		Reason   string          `json:"reason"`
		Category string          `json:"category"`
		Used     int             `json:"used"`
		Budget   *passBudget     `json:"budget"`
	}
	if json.Unmarshal(e.Payload, &p) != nil || p.Pass == "" {
		return
	}
	a := st.Passes[p.Pass]
	if a == nil {
		a = &passAudit{ID: p.Pass, Budget: map[string]int{}}
		a.Limits = map[string]int{}
		st.Passes[p.Pass] = a
		st.PassOrder = append(st.PassOrder, p.Pass)
	}
	a.Events = append(a.Events, e)
	if p.Goal != "" {
		a.Goal = p.Goal
	}
	if len(p.Plan) > 0 {
		a.Plan = p.Plan
	}
	if p.Reason != "" {
		a.Failures = append(a.Failures, p.Reason)
	}
	if p.Category != "" {
		a.Budget[p.Category] = p.Used
	}
	if p.Budget != nil {
		for category, used := range p.Budget.Used {
			a.Budget[category] = used
		}
		for category, limit := range p.Budget.Limits {
			a.Limits[category] = limit
		}
	}
	a.Status = strings.TrimPrefix(e.Name, "loop.pass.")
}

func railEvent(name string, payload any) Event {
	raw, _ := json.Marshal(payload)
	e := newEvent(name, raw)
	e.Via, e.By = doorKernel, callerClaim()
	return e
}

func withRailLock(home string, fn func(*state) ([]Event, error)) ([]Event, error) {
	if err := os.MkdirAll(home, 0755); err != nil {
		return nil, err
	}
	unlock, err := lockLog(home)
	if err != nil {
		return nil, err
	}
	defer unlock()
	events, err := readEvents(home)
	if err != nil {
		return nil, err
	}
	batch, opErr := fn(replay(events, secret(home)))
	if len(batch) > 0 {
		if err := appendLocked(home, batch); err != nil {
			return nil, err
		}
	}
	return batch, opErr
}

func leaseChange(home, op string, lease goalLease, ttl time.Duration, now time.Time) ([]Event, error) {
	if op == "acquire" || op == "steal" {
		var err error
		lease.Repository, err = canonicalPath(lease.Repository)
		if err != nil {
			return nil, fmt.Errorf("canonical repository: %w", err)
		}
		lease.Worktree, err = canonicalPath(lease.Worktree)
		if err != nil {
			return nil, fmt.Errorf("canonical worktree: %w", err)
		}
	}
	return withRailLock(home, func(st *state) ([]Event, error) {
		current := st.Leases[lease.Goal]
		refuse := func(reason string) ([]Event, error) {
			e := railEvent(leaseRefused, map[string]any{"goal": lease.Goal, "owner": lease.Owner, "invocation": lease.Invocation, "operation": op, "reason": reason})
			return []Event{e}, fmt.Errorf("lease %s for goal %q refused: %s", op, lease.Goal, reason)
		}
		switch op {
		case "acquire":
			for goal, other := range st.Leases {
				if goal == lease.Goal || !other.active(now) {
					continue
				}
				if (lease.Repository == other.Repository && lease.Branch == other.Branch) || lease.Worktree == other.Worktree {
					return refuse(fmt.Sprintf("conflicts with goal %q owned by %q until %s", goal, other.Owner, other.ExpiresAt.Format(time.RFC3339)))
				}
			}
			if current != nil && !current.Released {
				if current.active(now) {
					return refuse(fmt.Sprintf("owned by %q until %s in %s", current.Owner, current.ExpiresAt.Format(time.RFC3339), current.Worktree))
				}
				return refuse("the prior lease expired; use explicit steal")
			}
			lease.Token = newEvent("loop.lease.token", nil).ID
			lease.ExpiresAt = now.Add(ttl)
			return []Event{railEvent(leaseAcquired, lease)}, nil
		case "renew":
			if current == nil || current.Released || current.Owner != lease.Owner || current.Invocation != lease.Invocation || current.Token != lease.Token {
				return refuse("no matching owned lease")
			}
			if !current.active(now) {
				return refuse("the lease has expired; use explicit steal")
			}
			next := *current
			next.ExpiresAt = now.Add(ttl)
			return []Event{railEvent(leaseRenewed, next)}, nil
		case "release":
			if current == nil || current.Released || current.Owner != lease.Owner || current.Invocation != lease.Invocation || current.Token != lease.Token {
				return refuse("no matching owned lease")
			}
			return []Event{railEvent(leaseReleased, map[string]string{"goal": lease.Goal, "owner": lease.Owner, "invocation": lease.Invocation, "token": lease.Token})}, nil
		case "steal":
			if current == nil || current.Released {
				return refuse("no expired lease exists; acquire normally")
			}
			if current.active(now) {
				return refuse(fmt.Sprintf("owned by %q until %s", current.Owner, current.ExpiresAt.Format(time.RFC3339)))
			}
			for goal, other := range st.Leases {
				if goal != lease.Goal && other.active(now) && ((lease.Repository == other.Repository && lease.Branch == other.Branch) || lease.Worktree == other.Worktree) {
					return refuse(fmt.Sprintf("conflicts with goal %q owned by %q until %s", goal, other.Owner, other.ExpiresAt.Format(time.RFC3339)))
				}
			}
			lease.Token = newEvent("loop.lease.token", nil).ID
			lease.ExpiresAt = now.Add(ttl)
			return []Event{railEvent(leaseStolen, lease)}, nil
		default:
			return nil, fmt.Errorf("unknown lease operation %q", op)
		}
	})
}

func cmdLease(home string, args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: self lease acquire|renew|release|steal ...")
	}
	op := args[0]
	var l goalLease
	var ttl time.Duration
	var err error
	switch op {
	case "acquire", "steal":
		if len(args) != 8 {
			return fmt.Errorf("usage: self lease %s <goal> <owner> <invocation> <repository> <branch> <worktree> <ttl>", op)
		}
		l = goalLease{Goal: args[1], Owner: args[2], Invocation: args[3], Repository: args[4], Branch: args[5], Worktree: args[6]}
		ttl, err = positiveDuration(args[7], "ttl")
	case "renew":
		if len(args) != 6 {
			return fmt.Errorf("usage: self lease renew <goal> <owner> <invocation> <token> <ttl>")
		}
		l = goalLease{Goal: args[1], Owner: args[2], Invocation: args[3], Token: args[4]}
		ttl, err = positiveDuration(args[5], "ttl")
	case "release":
		if len(args) != 5 {
			return fmt.Errorf("usage: self lease release <goal> <owner> <invocation> <token>")
		}
		l = goalLease{Goal: args[1], Owner: args[2], Invocation: args[3], Token: args[4]}
	default:
		return fmt.Errorf("unknown lease operation %q", op)
	}
	if err != nil {
		return err
	}
	if l.Goal == "" || l.Owner == "" || l.Invocation == "" {
		return fmt.Errorf("goal, owner, and invocation must be nonempty")
	}
	if (op == "acquire" || op == "steal") && (l.Repository == "" || l.Branch == "" || l.Worktree == "") {
		return fmt.Errorf("repository, branch, and worktree must be nonempty")
	}
	events, err := leaseChange(home, op, l, ttl, time.Now().UTC())
	for _, e := range events {
		fmt.Fprintf(out, "%d\t%s\t%s\n", e.Seq, e.Name, trunc(compact(e.Payload), 160))
	}
	return err
}

func canonicalActions(raw string) (json.RawMessage, error) {
	var actions []map[string]any
	if json.Unmarshal([]byte(raw), &actions) != nil || len(actions) == 0 {
		return nil, fmt.Errorf("actions must be a nonempty JSON array of objects")
	}
	return json.Marshal(actions)
}

func checkpointID(pass string, actions json.RawMessage) string {
	sum := sha256.Sum256(append(append([]byte("self.checkpoint.v1\x00"), []byte(pass)...), actions...))
	return hex.EncodeToString(sum[:16])
}

func checkpointChange(home, op, pass, id, raw, reason string) ([]Event, error) {
	return withRailLock(home, func(st *state) ([]Event, error) {
		switch op {
		case "request":
			actions, err := canonicalActions(raw)
			if err != nil {
				return nil, err
			}
			id := checkpointID(pass, actions)
			if existing := st.Checkpoints[id]; existing != nil && (existing.pending() || existing.usable()) {
				return nil, fmt.Errorf("checkpoint %s already exists", id)
			}
			return []Event{railEvent(checkpointAsked, checkpoint{ID: id, Pass: pass, Actions: actions})}, nil
		case "approve", "reject":
			c := st.Checkpoints[id]
			if c == nil || !c.pending() {
				return nil, fmt.Errorf("checkpoint %q is not pending", id)
			}
			if op == "reject" && strings.TrimSpace(reason) == "" {
				return nil, fmt.Errorf("checkpoint rejection needs a reason")
			}
			name := checkpointYes
			payload := map[string]string{"id": id}
			if op == "reject" {
				name, payload["reason"] = checkpointNo, reason
			}
			return []Event{railEvent(name, payload)}, nil
		case "consume":
			c := st.Checkpoints[id]
			if c == nil || !c.usable() {
				return nil, fmt.Errorf("checkpoint %q has no unused approval", id)
			}
			return []Event{railEvent(checkpointUsed, map[string]string{"id": id})}, nil
		default:
			return nil, fmt.Errorf("unknown checkpoint operation %q", op)
		}
	})
}

func cmdCheckpoint(home string, args []string, out io.Writer) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: self checkpoint request <pass> <actions-json> | approve <id> | reject <id> <reason>")
	}
	op := args[0]
	var pass, id, raw, reason string
	switch op {
	case "request":
		if len(args) != 3 {
			return fmt.Errorf("usage: self checkpoint request <pass> <actions-json>")
		}
		pass, raw = args[1], args[2]
	case "approve":
		if len(args) != 2 {
			return fmt.Errorf("usage: self checkpoint approve <id>")
		}
		id = args[1]
	case "reject":
		if len(args) < 3 {
			return fmt.Errorf("usage: self checkpoint reject <id> <reason>")
		}
		id, reason = args[1], strings.Join(args[2:], " ")
	default:
		return fmt.Errorf("unknown checkpoint operation %q", op)
	}
	events, err := checkpointChange(home, op, pass, id, raw, reason)
	for _, e := range events {
		fmt.Fprintf(out, "%d\t%s\t%s\n", e.Seq, e.Name, compact(e.Payload))
	}
	return err
}

type actionScope string

const (
	scopeInside        actionScope = "inside-goal"
	scopeDecomposition actionScope = "decomposition"
	scopeAdjacent      actionScope = "adjacent"
	scopeExpanding     actionScope = "scope-expanding"
)

type passBudget struct {
	Limits map[string]int `json:"limits"`
	Used   map[string]int `json:"used"`
}

type budgetExhausted struct {
	Category  string
	Used      int
	Requested int
	Limit     int
}

func (e *budgetExhausted) Error() string {
	return fmt.Sprintf("%s budget exhausted: used %d + requested %d exceeds %d", e.Category, e.Used, e.Requested, e.Limit)
}

func guardedBudget() passBudget {
	return passBudget{Limits: map[string]int{
		"new_goals": 2, "dispatches": 1, "repositories": 1, "changed_files": 20,
		"pushes": 1, "pr_mutations": 1, "heavy_checks": 1,
	}, Used: map[string]int{}}
}

func (b *passBudget) consume(category string, amount int) error {
	limit, ok := b.Limits[category]
	if !ok || amount < 0 {
		return fmt.Errorf("unknown budget category %q", category)
	}
	if b.Used[category]+amount > limit {
		return &budgetExhausted{Category: category, Used: b.Used[category], Requested: amount, Limit: limit}
	}
	b.Used[category] += amount
	return nil
}

func classifyAction(goal, project string, action map[string]string) actionScope {
	if action["goal"] == goal && action["project"] == project {
		return scopeInside
	}
	if action["kind"] == "create-goal" && action["parent"] == goal && action["project"] == project {
		return scopeDecomposition
	}
	if action["project"] == project {
		return scopeAdjacent
	}
	return scopeExpanding
}

func builtinLoopView(st *state, args []string) ([]byte, error) {
	if len(args) > 1 {
		return nil, fmt.Errorf("usage: self view loop [pass-id]")
	}
	var b strings.Builder
	b.WriteString("# loop rails\n\n")
	if len(args) == 0 {
		b.WriteString("## passes\n")
		for i := len(st.PassOrder) - 1; i >= 0; i-- {
			a := st.Passes[st.PassOrder[i]]
			fmt.Fprintf(&b, "- %s goal=%s status=%s actions=%d failures=%d\n", a.ID, a.Goal, a.Status, len(a.Events), len(a.Failures))
		}
	} else if a := st.Passes[args[0]]; a != nil {
		fmt.Fprintf(&b, "## pass %s\n\ngoal: %s\nstatus: %s\n", a.ID, a.Goal, a.Status)
		if len(a.Plan) > 0 {
			fmt.Fprintf(&b, "plan: `%s`\n", compact(a.Plan))
		}
		if len(a.Limits) > 0 {
			categories := make([]string, 0, len(a.Limits))
			for category := range a.Limits {
				categories = append(categories, category)
			}
			sort.Strings(categories)
			b.WriteString("budget:\n")
			for _, category := range categories {
				fmt.Fprintf(&b, "- %s consumed=%d remaining=%d limit=%d\n", category, a.Budget[category], a.Limits[category]-a.Budget[category], a.Limits[category])
			}
		}
		for _, e := range a.Events {
			fmt.Fprintf(&b, "- %d %s %s\n", e.Seq, e.Name, trunc(compact(e.Payload), 240))
		}
	} else {
		return nil, fmt.Errorf("no loop pass %q", args[0])
	}
	b.WriteString("\n## unreleased leases (compare expiry)\n")
	goals := make([]string, 0, len(st.Leases))
	for goal := range st.Leases {
		goals = append(goals, goal)
	}
	sort.Strings(goals)
	for _, goal := range goals {
		l := st.Leases[goal]
		if !l.Released {
			fmt.Fprintf(&b, "- %s owner=%s invocation=%s generation=%s expires=%s repo=%s branch=%s worktree=%s\n", goal, l.Owner, l.Invocation, l.Token[:min(12, len(l.Token))], l.ExpiresAt.Format(time.RFC3339), l.Repository, l.Branch, l.Worktree)
		}
	}
	b.WriteString("\n## checkpoints\n")
	ids := make([]string, 0, len(st.Checkpoints))
	for id := range st.Checkpoints {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		c, status := st.Checkpoints[id], "pending"
		if c.Rejected > 0 {
			status = "rejected: " + c.Reason
		} else if c.Consumed > 0 {
			status = "consumed"
		} else if c.Approved > 0 {
			status = "approved"
		}
		fmt.Fprintf(&b, "- %s pass=%s status=%s actions=%s\n", id, c.Pass, status, compact(c.Actions))
	}
	return []byte(b.String()), nil
}

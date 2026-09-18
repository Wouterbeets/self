package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type repositoryReservation struct {
	file       *os.File
	Path       string
	Identity   string
	Repository string
	Token      string
}

type reservationMetadata struct {
	PID        int    `json:"pid"`
	Caller     string `json:"caller"`
	Purpose    string `json:"purpose"`
	Repository string `json:"repository"`
	Identity   string `json:"identity"`
	Token      string `json:"token"`
	Acquired   string `json:"acquired"`
}

func repositoryIdentity(repo string) (string, string, error) {
	top, err := repoOutput(repo, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", fmt.Errorf("repository reservation needs a Git worktree: %w", err)
	}
	common, err := repoOutput(repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", "", err
	}
	canonicalRepo, err := filepath.EvalSymlinks(strings.TrimSpace(string(top)))
	if err != nil {
		return "", "", err
	}
	canonicalCommon, err := filepath.EvalSymlinks(strings.TrimSpace(string(common)))
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte("self.repository-reservation.v1\x00" + canonicalCommon))
	return canonicalRepo, hex.EncodeToString(sum[:]), nil
}

func reservationRootCandidate() (string, error) {
	root := os.Getenv("SELF_RESERVATION_DIR")
	if root == "" {
		if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
			root = filepath.Join(runtimeDir, "self", "reservations")
		} else {
			root = filepath.Join(os.TempDir(), "self-"+strconv.Itoa(os.Getuid()), "reservations")
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return canonicalPath(root)
}

func ensureReservationRoot(root string) (string, error) {
	for _, path := range []string{filepath.Dir(root), root} {
		if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("reservation directory %s is a symlink", path)
		}
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	if err := os.Chmod(root, 0700); err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(root)
}

func reservationPath(repo string) (string, string, string, error) {
	canonical, identity, err := repositoryIdentity(repo)
	if err != nil {
		return "", "", "", err
	}
	root, err := reservationRootCandidate()
	if err != nil {
		return "", "", "", err
	}
	if pathWithin(canonical, root) {
		return "", "", "", fmt.Errorf("reservation directory %s must be outside repository %s", root, canonical)
	}
	worktrees, listErr := registeredWorktrees(repo)
	if listErr != nil {
		return "", "", "", fmt.Errorf("enumerating repository worktrees for reservation placement: %w", listErr)
	}
	for _, worktree := range worktrees {
		canonicalWorktree, err := filepath.EvalSymlinks(worktree.Path)
		if err == nil && pathWithin(canonicalWorktree, root) {
			return "", "", "", fmt.Errorf("reservation directory %s must be outside every worktree of repository %s", root, canonical)
		}
	}
	root, err = ensureReservationRoot(root)
	if err != nil {
		return "", "", "", err
	}
	return filepath.Join(root, identity+".lock"), canonical, identity, nil
}

func acquireRepositoryReservation(repo, purpose string) (*repositoryReservation, error) {
	path, canonical, identity, err := reservationPath(repo)
	if err != nil {
		return nil, err
	}
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		evidence, _ := os.ReadFile(path)
		file.Close()
		if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
			return nil, fmt.Errorf("repository %s is reserved by another self writer (%s)", canonical, strings.TrimSpace(string(evidence)))
		}
		return nil, err
	}
	token := newEvent("loop.reservation.token", nil).ID
	metadata := reservationMetadata{PID: os.Getpid(), Caller: callerClaim(), Purpose: purpose, Repository: canonical, Identity: identity, Token: token, Acquired: time.Now().UTC().Format(time.RFC3339Nano)}
	owner, _ := json.Marshal(metadata)
	if err := file.Truncate(0); err != nil {
		file.Close()
		return nil, err
	}
	if _, err := file.WriteAt(append(owner, '\n'), 0); err != nil {
		file.Close()
		return nil, err
	}
	return &repositoryReservation{file: file, Path: path, Identity: identity, Repository: canonical, Token: token}, nil
}

func validateInheritedReservation(repo string) (*repositoryReservation, error) {
	path, canonical, identity, err := reservationPath(repo)
	if err != nil {
		return nil, err
	}
	if os.Getenv("SELF_REPOSITORY_RESERVATION") != identity {
		return nil, fmt.Errorf("inherited reservation identity does not match repository %s", canonical)
	}
	fd, err := strconv.Atoi(os.Getenv("SELF_REPOSITORY_RESERVATION_FD"))
	if err != nil || fd < 3 {
		return nil, fmt.Errorf("inherited reservation has no valid file descriptor")
	}
	dupFD, err := syscall.Dup(fd)
	if err != nil {
		return nil, fmt.Errorf("duplicating inherited reservation descriptor: %w", err)
	}
	file := os.NewFile(uintptr(dupFD), "inherited repository reservation")
	if file == nil {
		return nil, fmt.Errorf("inherited reservation descriptor is unavailable")
	}
	defer file.Close()
	fdInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("reading inherited reservation descriptor: %w", err)
	}
	pathInfo, err := os.Stat(path)
	if err != nil || !os.SameFile(fdInfo, pathInfo) {
		return nil, fmt.Errorf("inherited reservation descriptor does not name canonical lock %s", path)
	}
	procInfo, err := os.ReadFile("/proc/self/fdinfo/" + strconv.Itoa(fd))
	if err != nil {
		return nil, fmt.Errorf("cannot prove inherited reservation ownership from procfs: %w", err)
	}
	ownsLock := false
	for _, line := range strings.Split(string(procInfo), "\n") {
		if strings.HasPrefix(line, "lock:") && strings.Contains(line, "FLOCK") && strings.Contains(line, "WRITE") {
			ownsLock = true
			break
		}
	}
	if !ownsLock {
		return nil, fmt.Errorf("inherited descriptor does not own the canonical repository reservation")
	}
	metadataRaw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var metadata reservationMetadata
	if json.Unmarshal(bytes.TrimSpace(metadataRaw), &metadata) != nil || metadata.Identity != identity || metadata.Repository != canonical || metadata.Token == "" {
		return nil, fmt.Errorf("canonical reservation metadata is invalid")
	}
	if claimed := os.Getenv("SELF_REPOSITORY_RESERVATION_TOKEN"); claimed != "" && claimed != metadata.Token {
		return nil, fmt.Errorf("inherited reservation token does not match lock metadata")
	}
	return &repositoryReservation{Path: path, Identity: identity, Repository: canonical, Token: metadata.Token}, nil
}

func (r *repositoryReservation) Release() error {
	if r == nil || r.file == nil {
		return nil
	}
	err := syscall.Flock(int(r.file.Fd()), syscall.LOCK_UN)
	closeErr := r.file.Close()
	r.file = nil
	if err != nil {
		return err
	}
	return closeErr
}

func runReservedChild(ctx context.Context, argv []string, stdin io.Reader, stdout, stderr io.Writer, reservation *repositoryReservation) error {
	if len(argv) == 0 {
		return fmt.Errorf("reservation needs a command after --")
	}
	cmd := commandWithGroup(ctx, argv[0], argv[1:]...)
	binary, _ := os.Executable()
	cmd.Env = append(os.Environ(), "SELF_REPOSITORY_RESERVATION="+reservation.Identity, "SELF_REPOSITORY_RESERVATION_FD=3", "SELF_REPOSITORY_RESERVATION_TOKEN="+reservation.Token, "SELF_BINARY="+binary)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	// The helper inherits the same open file description. If this wrapper is
	// killed, flock remains held until the dispatch helper itself exits.
	cmd.ExtraFiles = []*os.File{reservation.file}
	return cmd.Run()
}

type dispatchWriterEvidence struct {
	State            string `json:"state"`
	CleanupConfirmed bool   `json:"cleanup_confirmed"`
	Evidence         string `json:"evidence"`
}

func dispatchEventMatches(event Event, reservation *repositoryReservation) (string, bool, dispatchWriterEvidence) {
	var payload struct {
		Agent      string                 `json:"agent"`
		Repo       string                 `json:"repo"`
		Repository string                 `json:"repository"`
		Worktree   string                 `json:"worktree"`
		Writer     dispatchWriterEvidence `json:"writer"`
	}
	if json.Unmarshal(event.Payload, &payload) != nil || payload.Agent == "" {
		return payload.Agent, false, payload.Writer
	}
	for _, path := range []string{payload.Repository, payload.Repo, payload.Worktree} {
		if path == "" {
			continue
		}
		_, identity, identityErr := repositoryIdentity(path)
		if identityErr == nil && identity == reservation.Identity {
			return payload.Agent, true, payload.Writer
		}
	}
	return payload.Agent, false, payload.Writer
}

func activeDispatchAgents(home string) map[string]bool {
	active := map[string]bool{}
	st, err := loadState(home)
	if err != nil {
		return active
	}
	for _, event := range st.Events {
		var payload struct {
			Agent  string                 `json:"agent"`
			Writer dispatchWriterEvidence `json:"writer"`
		}
		if json.Unmarshal(event.Payload, &payload) != nil || payload.Agent == "" {
			continue
		}
		switch event.Name {
		case "agent.started":
			active[payload.Agent] = true
		case "agent.reclaimed":
			delete(active, payload.Agent)
		case "agent.failed":
			if payload.Writer.State == "stopped" && payload.Writer.CleanupConfirmed && strings.TrimSpace(payload.Writer.Evidence) != "" {
				delete(active, payload.Agent)
			}
		}
	}
	return active
}

func validateDispatchEvidence(input []byte, reservation *repositoryReservation, existing map[string]bool) ([]Event, error) {
	events, scripts, _, err := wire(string(input))
	if err != nil {
		return nil, err
	}
	if len(scripts) > 0 {
		return nil, fmt.Errorf("reserved dispatch helper emitted capability authoring instead of dispatch evidence")
	}
	type writerState struct {
		started, safe, unsafe bool
	}
	writers := map[string]*writerState{}
	for _, event := range events {
		if event.Name != "agent.started" && event.Name != "agent.failed" && event.Name != "agent.reclaimed" {
			continue
		}
		agent, matches, writer := dispatchEventMatches(event, reservation)
		if !matches {
			return events, fmt.Errorf("writer event %s for %q does not match held repository reservation", event.Name, agent)
		}
		state := writers[agent]
		if state == nil {
			state = &writerState{started: existing[agent]}
			writers[agent] = state
		}
		if event.Name == "agent.started" {
			state.started = true
			continue
		}
		switch writer.State {
		case "not_started":
			if state.started {
				state.unsafe = true
			} else {
				state.safe = true
			}
		case "stopped":
			if writer.CleanupConfirmed && strings.TrimSpace(writer.Evidence) != "" {
				state.safe = true
			} else {
				state.unsafe = true
			}
		default:
			state.unsafe = true // missing and unknown liveness are live
		}
	}
	if len(writers) == 0 {
		return events, fmt.Errorf("dispatch writer liveness is unknown or cleanup is unproved")
	}
	for _, state := range writers {
		if state.unsafe || (!state.started && !state.safe) {
			return events, fmt.Errorf("dispatch writer liveness is unknown or cleanup is unproved")
		}
	}
	return events, nil
}

func holdUnsafeReservation(ctx context.Context, diag io.Writer, reason error) error {
	fmt.Fprintf(diag, "self reserve dispatch: UNSAFE terminal evidence: %v; reservation remains held until explicit termination\n", reason)
	<-ctx.Done()
	return fmt.Errorf("unsafe dispatch reservation terminated: %w", reason)
}

func reservationPublished(home string, reservation *repositoryReservation) bool {
	st, err := loadState(home)
	if err != nil {
		return false
	}
	for i := len(st.Events) - 1; i >= 0; i-- {
		event := st.Events[i]
		if event.Name != "loop.reservation.dispatch.published" || event.Via != doorKernel {
			continue
		}
		var payload struct {
			Identity string `json:"identity"`
			Token    string `json:"token"`
		}
		if json.Unmarshal(event.Payload, &payload) == nil && payload.Identity == reservation.Identity && payload.Token == reservation.Token {
			return true
		}
	}
	return false
}

func cmdReserve(home string, args []string, out, diag io.Writer) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: self reserve path|check|held|publish|exec|dispatch <repository> [-- command...]")
	}
	op, repo := args[0], args[1]
	switch op {
	case "path":
		if len(args) != 2 {
			return fmt.Errorf("usage: self reserve path <repository>")
		}
		path, canonical, identity, err := reservationPath(repo)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "%s\t%s\t%s\n", identity, canonical, path)
		return err
	case "check":
		if len(args) != 2 {
			return fmt.Errorf("usage: self reserve check <repository>")
		}
		reservation, err := acquireRepositoryReservation(repo, "check")
		if err != nil {
			return err
		}
		defer reservation.Release()
		_, err = fmt.Fprintf(out, "available\t%s\t%s\n", reservation.Identity, reservation.Repository)
		return err
	case "held":
		if len(args) != 2 {
			return fmt.Errorf("usage: self reserve held <repository>")
		}
		reservation, err := validateInheritedReservation(repo)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "held\t%s\t%s\n", reservation.Identity, reservation.Repository)
		return err
	case "publish":
		if len(args) != 2 {
			return fmt.Errorf("usage: self reserve publish <repository>")
		}
		reservation, err := validateInheritedReservation(repo)
		if err != nil {
			return err
		}
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		if _, err := validateDispatchEvidence(input, reservation, activeDispatchAgents(home)); err != nil {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return holdUnsafeReservation(ctx, diag, err)
		}
		if err := cmdHear(home, input, io.Discard); err != nil {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return holdUnsafeReservation(ctx, diag, fmt.Errorf("dispatch evidence ingestion failed: %w", err))
		}
		receipt := railEvent("loop.reservation.dispatch.published", map[string]any{"identity": reservation.Identity, "token": reservation.Token, "repository": reservation.Repository})
		if err := appendEvents(home, []Event{receipt}); err != nil {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return holdUnsafeReservation(ctx, diag, fmt.Errorf("dispatch publication receipt failed: %w", err))
		}
		_, err = fmt.Fprintf(out, "published\t%s\n", reservation.Identity)
		return err
	case "exec", "dispatch":
		if len(args) < 4 || args[2] != "--" {
			return fmt.Errorf("usage: self reserve %s <repository> -- <command> [args...]", op)
		}
		reservation, err := acquireRepositoryReservation(repo, op)
		if err != nil {
			return err
		}
		defer reservation.Release()
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if op == "exec" {
			return runReservedChild(ctx, args[3:], os.Stdin, out, diag, reservation)
		}
		var wire boundedBuffer
		wire.Limit = lineLimit * 4
		childErr := runReservedChild(ctx, args[3:], os.Stdin, &wire, diag, reservation)
		if !reservationPublished(home, reservation) {
			return holdUnsafeReservation(ctx, diag, fmt.Errorf("dispatch helper exited without native reservation publication"))
		}
		return childErr
	default:
		return fmt.Errorf("unknown reserve operation %q", op)
	}
}

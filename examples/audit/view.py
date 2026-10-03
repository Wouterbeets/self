#!/usr/bin/env python3
'''audit - what one caller's run of work did, replayed from the log.

usage: self view audit                         recent runs of every caller, newest first
       self view audit <caller>                the caller's latest run in full
       self view audit <caller> --run N        its N-th newest run (1 = latest)
       self view audit <caller> --seq A-B      exactly seq A..B, whatever the runs
       --gap MIN                               split runs at quiet gaps longer than MIN minutes (default 20)
       --list                                  with <caller>: its runs, one line each

A run is a caller's events with no quiet gap longer than --gap and no
loop.settled inside it, plus the kernel receipts that answer them (script.installed
and script.rejected right after the caller's declarations) and events that name
the caller as dispatcher. The log has no pass boundary, so a loop's back-to-back
passes read as one run unless --seq pins one. Sections: intents, goals,
capabilities, workers, evidence and gates, coordination (lease, budget,
checkpoint), pull requests and commits named in payloads, outward speech, and
the human replies to that speech. Everything cites seq; nothing here proves the
work was right, only what was recorded.
'''
import json, re, sys
from datetime import datetime

USAGE = __doc__.split('\n\n')[1]
KERNEL = (None, '', '-')


def fail(msg):
    print(msg, file=sys.stderr)
    print(USAGE, file=sys.stderr)
    sys.exit(2)


args = sys.argv[1:]
caller, run_n, seq_range, gap, listing = None, 1, None, 20.0, False
i = 0
while i < len(args):
    a = args[i]
    if a in ('--run', '--seq', '--gap'):
        if i + 1 >= len(args):
            fail('%s needs a value' % a)
        v = args[i + 1]
        i += 2
        try:
            if a == '--run':
                run_n = int(v)
                if run_n < 1:
                    raise ValueError
            elif a == '--gap':
                gap = float(v)
                if gap <= 0:
                    raise ValueError
            else:
                lo, hi = v.split('-', 1)
                seq_range = (int(lo), int(hi))
        except ValueError:
            fail('bad value for %s: %s' % (a, v))
        continue
    if a == '--list':
        listing = True
    elif a.startswith('--') or caller is not None:
        fail('unexpected argument: %s' % a)
    else:
        caller = a
    i += 1
if (seq_range or listing or run_n != 1) and caller is None:
    fail('--run, --seq and --list need a caller')

events = []
for raw in sys.stdin:
    try:
        e = json.loads(raw)
    except ValueError:
        continue
    if isinstance(e, dict) and isinstance(e.get('seq'), int):
        if not isinstance(e.get('payload'), dict):
            e['payload'] = {}
        events.append(e)
events.sort(key=lambda e: e['seq'])


def when(e):
    try:
        return datetime.fromisoformat(e.get('occurred_at', '')[:19])
    except ValueError:
        return None


def is_kernel(e):
    return e.get('by') in KERNEL or e.get('via') == 'kernel'


# Kernel receipts belong to whoever caused them: the non-kernel event just before.
owner = {}
last_actor = None
for e in events:
    if is_kernel(e):
        owner[e['seq']] = last_actor
    else:
        last_actor = e.get('by')
        owner[e['seq']] = last_actor
        p = e['payload']
        for k in ('dispatched_by', 'requested_by'):
            if isinstance(p.get(k), str) and p[k] and p[k] != last_actor:
                owner[e['seq']] = p[k]


by_owner = {}
for e in events:
    by_owner.setdefault(owner.get(e['seq']), []).append(e)


def runs_of(who):
    mine = by_owner.get(who, [])
    runs, cur, prev = [], [], None
    for e in mine:
        t = when(e)
        split = False
        if cur:
            pt = when(prev)
            if t and pt and (t - pt).total_seconds() > gap * 60:
                split = True
            if prev.get('name') == 'loop.settled' and not is_kernel(e):
                split = True
        if split:
            runs.append(cur)
            cur = []
        cur.append(e)
        prev = e
    if cur:
        runs.append(cur)
    return runs


def short(v, n=110):
    s = v if isinstance(v, str) else json.dumps(v, ensure_ascii=False)
    s = ' '.join(s.split())
    return s if len(s) <= n else s[:n - 1] + '…'


def span(run):
    a, b = run[0], run[-1]
    return '%s-%s  %s → %s' % (a['seq'], b['seq'], a.get('occurred_at', '')[:16].replace('T', ' '),
                               b.get('occurred_at', '')[11:16])


def headline(run):
    c = {}
    for e in run:
        c[e['name']] = c.get(e['name'], 0) + 1
    top = sorted(c.items(), key=lambda kv: (-kv[1], kv[0]))[:4]
    return ', '.join('%s×%d' % kv for kv in top)


if caller is None:
    allruns = []
    for who in sorted({v for v in owner.values() if v}):
        for r in runs_of(who):
            allruns.append((r[-1]['seq'], who, r))
    allruns.sort(key=lambda x: -x[0])
    if not allruns:
        print('No events.')
        sys.exit(0)
    print('# Audit: recent runs (split at %g min quiet or loop.settled)' % gap)
    for _, who, r in allruns[:20]:
        print('- %s  %s  %d events  %s' % (who, span(r), len(r), headline(r)))
    print('\nOne in full: self view audit <caller> [--run N | --seq A-B]')
    sys.exit(0)

if seq_range:
    lo, hi = seq_range
    run = [e for e in events if lo <= e['seq'] <= hi and owner.get(e['seq']) == caller]
    label = 'seq %d-%d' % (lo, hi)
else:
    rs = runs_of(caller)
    if not rs:
        print('No events by %s.' % caller)
        sys.exit(0)
    if listing:
        print('# Audit: %d runs of %s (newest first)' % (len(rs), caller))
        for n, r in enumerate(reversed(rs), 1):
            print('%3d. %s  %d events  %s' % (n, span(r), len(r), headline(r)))
        sys.exit(0)
    if run_n > len(rs):
        print('%s has only %d runs.' % (caller, len(rs)))
        sys.exit(0)
    run = rs[-run_n]
    label = 'run %d of %d' % (run_n, len(rs))
if not run:
    print('No events by %s in %s.' % (caller, label))
    sys.exit(0)

first, last = run[0]['seq'], run[-1]['seq']
print('# Audit: %s, %s' % (caller, label))
print('seq %s · %d events · %s' % (span(run), len(run), headline(run)))
shown = set()
out = []


def section(title, rows):
    if rows:
        out.append('\n## %s (%d)' % (title, len(rows)))
        out.extend('- ' + r for r in rows)


def pick(names, fmt):
    rows = []
    for e in run:
        if e['name'] in names or any(e['name'].startswith(n) for n in names if n.endswith('.')):
            shown.add(e['seq'])
            rows.append('[%s] %s' % (e['seq'], fmt(e)))
    return rows


def p(e):
    return e['payload']


def fmt(e):
    n, q = e['name'], p(e)
    if n == 'intent.declared':
        return 'declared %s' % q.get('name', '?')
    if n == 'intent.closed':
        return 'closed %s %s: %s' % (q.get('outcome', '?'), q.get('name', '?'), short(q.get('reason', ''), 90))
    if n.startswith('goal.'):
        return '%s %s %s' % (n[5:], q.get('goal', '?'),
                             short(q.get('title') or q.get('report') or q.get('note') or q.get('status') or '', 90))
    if n in CAPS:
        s = '%s %s/%s' % (n, q.get('type', n.split('.')[0]), q.get('name', '?'))
        if 'verdict' in q:
            s += ' ' + str(q['verdict'])
        if n == 'script.rejected':
            s += ': ' + short(q.get('reason', ''), 80)
        return s
    if n.startswith('evidence.'):
        checks = q.get('check') or ','.join(q.get('checks') or [])
        mark = '' if n != 'evidence.checked' else (' pass' if q.get('passed') else ' FAIL')
        return '%s %s %s%s' % (n[9:], q.get('subject', '?'), checks, mark)
    if n.startswith(('lease.', 'budget.', 'checkpoint.')):
        keep = ('resource', 'holder', 'scope', 'class', 'n', 'limit', 'id', 'actions', 'action', 'reason')
        return '%s %s' % (n, short({k: v for k, v in q.items() if k in keep}, 100))
    if n.startswith(('pr.', 'review.', 'reviews.')):
        return '%s %s' % (n, short(q.get('url') or q.get('pr') or q.get('number') or q.get('prs') or '', 90))
    if n == 'slack.sent':
        return 'slack.sent %s' % short(q.get('text', ''), 90)
    return '%s %s %s' % (n, q.get('agent', ''), short(q.get('goal', '') + (' ' + str(q['error']) if 'error' in q else ''), 90))


CAPS = {'command.declared', 'view.declared', 'script.installed', 'script.rejected',
        'capability.verified', 'capability.retired', 'capability.checked'}


def pick(*prefixes):
    rows = []
    for e in run:
        if e['name'] in prefixes or e['name'].startswith(tuple(x for x in prefixes if x.endswith('.'))):
            shown.add(e['seq'])
            rows.append('[%s] %s' % (e['seq'], fmt(e)))
    return rows


section('Intents', pick('intent.declared', 'intent.closed'))
section('Goals', pick('goal.'))
section('Capabilities', pick(*CAPS))
section('Workers', pick('agent.', 'dispatch.', 'loop.reservation.'))
section('Evidence and gates', pick('evidence.'))
section('Coordination', pick('lease.', 'budget.', 'checkpoint.'))
section('Pull requests and reviews', pick('pr.', 'review.', 'reviews.'))

refs = {}
for e in run:
    blob = json.dumps(e['payload'], ensure_ascii=False)
    for kind, rx in (('commit', r'\bcommits? ([0-9a-f]{7,40})\b'), ('branch', r'\bbranch `?([A-Za-z0-9][\w.-]*/[\w./-]*\w)'),
                     ('pr', r'github\.com/[\w.-]+/[\w.-]+/pull/(\d+)')):
        for m in re.findall(rx, blob):
            refs.setdefault((kind, m), []).append(e['seq'])
section('Commits, branches and PRs named', ['%s %s (seq %s)' % (k, v, ','.join(str(x) for x in sorted(set(s))[:6]))
                                            for (k, v), s in sorted(refs.items())])

tabs = {p(e).get('tab') for e in run if e['name'].startswith('herdr.') and p(e).get('tab')}
spoken = [e for e in events if first <= e['seq'] <= last + 200 and e['name'] == 'attention.spoken'
          and (owner.get(e['seq']) == caller or (p(e).get('tab') in tabs))]
outward = ['[%s] spoke: %s' % (e['seq'], short(p(e).get('text', ''), 100)) for e in spoken if e['seq'] <= last]
outward += pick('slack.sent')
section('Outward', outward)
said = [p(e).get('text', '') for e in spoken]
human = []
for e in events:
    if e['seq'] < first or e['seq'] > last + 200 or e['name'] not in ('attention.heard', 'checkpoint.approved',
                                                                      'checkpoint.rejected', 'budget.set'):
        continue
    if e['name'] == 'attention.heard':
        after = p(e).get('after', '')
        if not after or not any(s.startswith(after[:40]) or after.startswith(s[:40]) for s in said if s):
            continue
        human.append('[%s] %s replied: %s' % (e['seq'], e.get('by', '?'), short(p(e).get('text', ''), 100)))
    elif owner.get(e['seq']) != caller and (e['seq'] <= last):
        human.append('[%s] %s by %s: %s' % (e['seq'], e['name'], e.get('by', '?'),
                                            short({k: v for k, v in p(e).items() if k in ('id', 'scope', 'class', 'limit', 'reason')}, 90)))
section('Human decisions', human)

rest = {}
for e in run:
    if e['seq'] not in shown and not e['name'].startswith('herdr.') and e['name'] != 'slack.sent':
        rest[e['name']] = rest.get(e['name'], 0) + 1
settled = [e for e in run if e['name'] == 'loop.settled']
if rest:
    out.append('\n## Other events')
    out.append('- ' + ', '.join('%s×%d' % kv for kv in sorted(rest.items(), key=lambda kv: (-kv[1], kv[0]))))
doing = [e for e in run if e['name'] == 'herdr.doing']
if doing:
    out.append('\n## Last doing line\n- [%s] %s' % (doing[-1]['seq'], short(p(doing[-1]).get('text', ''), 140)))
if settled:
    out.append('\n## Settled\n- [%s] %s' % (settled[-1]['seq'], short(p(settled[-1]).get('reason', ''), 300)))
print('\n'.join(out))

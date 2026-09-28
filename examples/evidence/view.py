#!/usr/bin/env python3
'''evidence - what was checked, what passed, what gates were met.

usage: self view evidence              every subject: failing checks first, then passing
       self view evidence <subject>    latest result per check, earlier runs, satisfied gates
'''
import json, sys

args = sys.argv[1:]
if len(args) > 1 or (args and args[0].startswith('--')):
    print('usage: self view evidence [<subject>]', file=sys.stderr)
    sys.exit(2)
subjects = {}
for raw in sys.stdin:
    try:
        e = json.loads(raw)
    except ValueError:
        continue
    p = e.get('payload')
    if not isinstance(p, dict) or 'subject' not in p:
        continue
    s = subjects.setdefault(p['subject'], {'checks': {}, 'runs': [], 'gates': [], 'last': ''})
    s['last'] = e.get('occurred_at', '')[:19]
    if e.get('name') == 'evidence.checked':
        r = dict(p, seq=e.get('seq'), at=e.get('occurred_at', '')[:19], by=e.get('by', ''))
        s['checks'][p.get('check')] = r
        s['runs'].append(r)
    elif e.get('name') == 'evidence.satisfied':
        s['gates'].append(dict(p, seq=e.get('seq'), at=e.get('occurred_at', '')[:19], by=e.get('by', '')))


def mark(r):
    return 'pass' if r.get('passed') else ('TIMEOUT' if r.get('timed_out') else 'FAIL exit %s' % r.get('exit'))


if not args:
    if not subjects:
        print('No evidence recorded. Record with examples/evidence/self-check or self run evidence record.')
        sys.exit(0)
    rows = []
    for name, s in subjects.items():
        bad = sorted(c for c, r in s['checks'].items() if not r.get('passed'))
        good = sorted(c for c, r in s['checks'].items() if r.get('passed'))
        rows.append((not bad, s['last'], name, bad, good, s['gates']))
    rows.sort(key=lambda r: (r[0], [-ord(ch) for ch in r[1]]))
    print('# Evidence: %d subjects' % len(rows))
    for ok, last, name, bad, good, gates in rows:
        line = '- %s  (%s)' % (name, last)
        if bad:
            line += '  failing: ' + ', '.join(bad)
        if good:
            line += '  passing: ' + ', '.join(good)
        if gates:
            line += '  satisfied at seq %s' % gates[-1]['seq']
        print(line)
    sys.exit(0)

s = subjects.get(args[0])
if s is None:
    print('No evidence for %s.' % args[0])
    sys.exit(0)
print('# Evidence: %s' % args[0])
print('\n## Latest per check')
for c in sorted(s['checks'], key=lambda c: (s['checks'][c].get('passed') is True, c)):
    r = s['checks'][c]
    print('- %s: %s  [%s] %s by %s%s' % (c, mark(r), r['seq'], r['at'], r['by'],
                                        ('  %ss' % r['secs']) if 'secs' in r else ''))
    for k in ('cmd', 'dir', 'limits', 'ref', 'digest'):
        if r.get(k):
            print('    %s: %s' % (k, r[k]))
    if r.get('tail') and not r.get('passed'):
        print('    tail:\n' + '\n'.join('      ' + ln for ln in r['tail'].splitlines()[-15:]))
older = [r for r in s['runs'] if s['checks'].get(r.get('check')) is not r]
if older:
    print('\n## Earlier runs (%d)' % len(older))
    for r in older[-10:]:
        print('- [%s] %s %s: %s' % (r['seq'], r['at'], r.get('check'), mark(r)))
print('\n## Gates satisfied')
if not s['gates']:
    print('- none yet: self run evidence require %s <check>...' % args[0])
for g in s['gates']:
    print('- [%s] %s by %s: %s%s' % (g['seq'], g['at'], g['by'],
                                     ', '.join('%s@%s' % kv for kv in sorted(g['checks'].items())),
                                     ('  ' + g['note']) if g.get('note') else ''))

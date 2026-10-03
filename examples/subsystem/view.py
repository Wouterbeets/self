#!/usr/bin/env python3
import json, sys

subs, gone = {}, {}
for raw in sys.stdin:
    try:
        e = json.loads(raw)
    except ValueError:
        continue
    p, n = e.get('payload'), e.get('name')
    if not isinstance(p, dict) or not isinstance(p.get('name'), str):
        continue
    if n == 'subsystem.spawned':
        subs[p['name']] = {'spawned': (e.get('seq'), p), 'obs': []}
        gone.pop(p['name'], None)
    elif n == 'subsystem.observed' and p['name'] in subs:
        subs[p['name']]['obs'].append((e.get('seq'), p))
    elif n == 'subsystem.retired' and p['name'] in subs:
        gone[p['name']] = (e.get('seq'), p, subs.pop(p['name']))

args = sys.argv[1:]
if len(args) > 2 or (args and args[0] not in ('--home',) and len(args) > 1):
    print('usage: self view subsystems [<name> | --home <name>]', file=sys.stderr)
    sys.exit(2)
if args[:1] == ['--home']:
    if len(args) != 2 or args[1] not in subs:
        sys.exit(1)
    print(subs[args[1]]['spawned'][1]['home'])
    sys.exit(0)
if args:
    s = subs.get(args[0])
    if not s:
        print('no live subsystem %s' % args[0])
        sys.exit(0)
    seq, p = s['spawned']
    print('# %s  [%s]\nhome: %s\nowns: %s' % (args[0], seq, p['home'], ', '.join(p.get('owns') or []) or '-'))
    if p.get('note'):
        print('note: ' + p['note'])
    for oseq, o in s['obs']:
        print('[%s] head %s, %s events, %s bytes, last %s; new: %s' % (
            oseq, o['head'], o['events'], o['bytes'], o.get('last_at', '')[:16],
            ', '.join('%s×%d' % kv for kv in o.get('new', {}).items()) or '-'))
    sys.exit(0)
if not subs:
    print('no subsystems (self run subsystem spawn <name>)')
for name in sorted(subs):
    s = subs[name]
    o = s['obs'][-1][1] if s['obs'] else None
    state = '%s events, %s KB, last %s' % (o['events'], o['bytes'] // 1024, o.get('last_at', '')[:16]) if o else 'not observed yet'
    print('%s\t%s\t%s' % (name, state, ', '.join(s['spawned'][1].get('owns') or []) or '-'))
for name in sorted(gone):
    print('%s\tretired [%s]: %s' % (name, gone[name][0], gone[name][1]['reason']))

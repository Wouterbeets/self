#!/usr/bin/env python3
'''leases - live leases, or one resource's history.

usage: self view leases              every unreleased lease: resource, holder, expiry
       self view leases <resource>   that resource's lease history, oldest first

A view has no clock: compare expires_at with now; `self run lease` judges expiry.
'''
import json, sys

events = []
for raw in sys.stdin:
    try:
        e = json.loads(raw)
    except ValueError:
        continue
    p = e.get('payload')
    if e.get('name', '').startswith('lease.') and isinstance(p, dict) and 'resource' in p:
        events.append(e)
args = sys.argv[1:]
if len(args) > 1:
    print('usage: self view leases [<resource>]', file=sys.stderr)
    sys.exit(2)
if args:
    rows = [e for e in events if e['payload']['resource'] == args[0]]
    if not rows:
        print('no lease history for %s' % args[0])
    for e in rows:
        p = e['payload']
        extra = ' '.join('%s=%s' % (k, p[k]) for k in ('expires_at', 'stolen_from', 'note') if p.get(k))
        print('[%s] %s %s %s %s' % (e.get('seq'), e.get('occurred_at', '')[:19], e['name'][6:], p.get('holder'), extra))
    sys.exit(0)
live = {}
for e in events:
    p = e['payload']
    if e['name'] == 'lease.released':
        live.pop(p['resource'], None)
    elif e['name'] in ('lease.acquired', 'lease.renewed'):
        live[p['resource']] = (e.get('seq'), p)
if not live:
    print('no live leases')
for r, (seq, p) in sorted(live.items()):
    note = ' - ' + p['note'] if p.get('note') else ''
    print('%s\t%s\tuntil %s\t[%s]%s' % (r, p.get('holder'), p.get('expires_at'), seq, note))

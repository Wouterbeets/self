#!/usr/bin/env python3
'''checkpoint - proposed actions awaiting a human decision, and what approvals still cover.

usage: self view checkpoint          awaiting decision, then approved with actions left
       self view checkpoint <id>     one checkpoint's actions and history
       self view checkpoint --all    also rejected, withdrawn and spent ones
'''
import json, sys
from datetime import datetime, timezone

APPROVERS = {'wouter'}
args = sys.argv[1:]
show_all = '--all' in args
args = [a for a in args if a != '--all']
if len(args) > 1:
    print('usage: self view checkpoint [<id>|--all]', file=sys.stderr)
    sys.exit(2)
cps, order = {}, []
for raw in sys.stdin:
    try:
        e = json.loads(raw)
    except ValueError:
        continue
    name, p = e.get('name', ''), e.get('payload')
    if not name.startswith('checkpoint.') or not isinstance(p, dict) or 'id' not in p:
        continue
    cid, by, verb = p['id'], e.get('by', ''), name[11:]
    if verb == 'proposed':
        cps[cid] = {'status': 'pending', 'actions': p.get('actions', []), 'by': by, 'note': p.get('note', ''),
                    'used': [], 'expires': None, 'reason': '', 'history': []}
    c = cps.get(cid)
    if c is None:
        continue
    counted = True
    if verb == 'approved':
        counted = by in APPROVERS and c['status'] == 'pending'
        if counted:
            c['status'], c['expires'] = 'approved', p.get('expires')
    elif verb == 'rejected':
        counted = by in APPROVERS and c['status'] == 'pending'
        if counted:
            c['status'], c['reason'] = 'rejected', p.get('reason', '')
    elif verb == 'withdrawn':
        counted = c['status'] in ('pending', 'approved')
        if counted:
            c['status'], c['reason'] = 'withdrawn', p.get('reason', '')
    elif verb == 'used':
        counted = c['status'] == 'approved'
        if counted:
            c['used'].append(p.get('action'))
    c['history'].append((e, counted))
    if cid in order:
        order.remove(cid)
    order.append(cid)

now = datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')


def label(c):
    if c['status'] == 'approved':
        if len(c['used']) == len(c['actions']):
            return 'spent'
        if c['expires'] and now >= c['expires']:
            return 'expired'
    return c['status']


def show(cid, c):
    st = label(c)
    head = '%s  [%s]  proposed by %s' % (cid, st, c['by'] or '?')
    if st == 'approved':
        head += ', approval expires %s' % c['expires']
    print(head)
    if c['note']:
        print('  note: %s' % c['note'])
    for a in c['actions']:
        print('  %s %s' % ('x' if a in c['used'] else '-', a))
    if c['reason']:
        print('  reason: %s' % c['reason'])
    if st == 'pending':
        print('  decide: self run checkpoint approve %s [--ttl 24h]  |  reject %s --reason "..."' % (cid, cid))


if args:
    c = cps.get(args[0])
    if c is None:
        print('no checkpoint %s' % args[0])
        sys.exit(0)
    show(args[0], c)
    print('history:')
    for e, counted in c['history']:
        p = e['payload']
        extra = ' '.join('%s=%s' % (k, p[k]) for k in ('action', 'expires', 'reason', 'note') if k in p)
        print('  [%s] %s %s by %s %s%s' % (e.get('seq'), e.get('occurred_at', '')[:19], e['name'][11:],
                                         e.get('by', '?'), extra, '' if counted else '  (ignored: not an approver or not pending)'))
    sys.exit(0)
groups = [('awaiting decision', ['pending']), ('approved, actions left', ['approved'])]
if show_all:
    groups.append(('closed', ['rejected', 'withdrawn', 'spent', 'expired']))
shown = False
for title, states in groups:
    ids = [cid for cid in order[::-1] if label(cps[cid]) in states]
    if not ids:
        continue
    shown = True
    print('## %s' % title)
    for cid in ids[:20]:
        show(cid, cps[cid])
if not shown:
    print('no open checkpoints' + ('' if show_all else ' (--all shows closed ones)'))

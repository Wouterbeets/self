#!/usr/bin/env python3
'''budget - what each scope has spent and has left, per class.

usage: self view budget            the 20 most recently touched scopes
       self view budget <scope>    that scope's classes, then its history
'''
import json, sys

args = sys.argv[1:]
if len(args) > 1:
    print('usage: self view budget [<scope>]', file=sys.stderr)
    sys.exit(2)
state, order, history = {}, [], []
for raw in sys.stdin:
    try:
        e = json.loads(raw)
    except ValueError:
        continue
    p = e.get('payload')
    if not e.get('name', '').startswith('budget.') or not isinstance(p, dict) or 'scope' not in p:
        continue
    s, c = p['scope'], p.get('class')
    row = state.setdefault(s, {}).setdefault(c, {'limit': None, 'used': 0, 'exhausted': False})
    if e['name'] == 'budget.set':
        row['limit'], row['exhausted'] = p.get('limit'), False
    elif e['name'] == 'budget.spent':
        row['used'] += p.get('n', 1)
        if row['limit'] is None:
            row['limit'] = p.get('limit')
    elif e['name'] == 'budget.exhausted':
        row['exhausted'] = True
        if row['limit'] is None:
            row['limit'] = p.get('limit')
    if s in order:
        order.remove(s)
    order.append(s)
    if args and s == args[0]:
        history.append(e)


def show(s):
    for c, r in sorted(state[s].items()):
        left = '-' if r['limit'] is None else max(r['limit'] - r['used'], 0)
        flag = '  EXHAUSTED' if r['exhausted'] else ''
        print('  %s\t%s/%s used\t%s left%s' % (c, r['used'], r['limit'], left, flag))


if args:
    if args[0] not in state:
        print('no budget for %s' % args[0])
        sys.exit(0)
    print(args[0])
    show(args[0])
    print('history:')
    for e in history:
        p = e['payload']
        extra = ' '.join('%s=%s' % (k, p[k]) for k in ('n', 'refused', 'limit', 'by', 'note') if k in p)
        print('  [%s] %s %s %s %s' % (e.get('seq'), e.get('occurred_at', '')[:19], e['name'][7:], p.get('class'), extra))
    sys.exit(0)
if not order:
    print('no budgets')
for s in order[::-1][:20]:
    print(s)
    show(s)

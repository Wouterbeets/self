#!/usr/bin/env python3
'''budget - bounded counters: how much of something a scope may still spend.

usage: self run budget set   <scope> <class> <limit>
       self run budget spend <scope> <class> [--n 1] [--limit N] [--note TEXT]

A scope is any string: loop/<name>/pass/<n>, goal/<id>, a day. A class names
what is counted: dispatches, goals, pushes, files. set records the limit for one
class in one scope; spend --limit sets it on first use. spend within the limit
prints budget.spent. The first spend past it prints budget.exhausted instead
(the refusal is recorded once); later spends in that class exit 3 and append
nothing. So a caller spent only if the output holds budget.spent. Declared
atomic: concurrent spends never exceed the limit.
'''
import json, os, sys


def fail(msg, code=2):
    print('budget: ' + msg, file=sys.stderr)
    sys.exit(code)


def count(s, what):
    try:
        n = int(s)
    except (TypeError, ValueError):
        n = -1
    if n < 0 or (what == '--n' and n == 0):
        fail('%s wants a %s integer, got %r' % (what, 'positive' if what == '--n' else 'non-negative', s))
    return n


def ledger(lines, scope, cls):
    limit, used, exhausted = None, 0, False
    for raw in lines:
        try:
            e = json.loads(raw)
        except ValueError:
            continue
        p = e.get('payload')
        if not e.get('name', '').startswith('budget.') or not isinstance(p, dict):
            continue
        if p.get('scope') != scope or p.get('class') != cls:
            continue
        if e['name'] == 'budget.set':
            limit, exhausted = p.get('limit'), False
        elif e['name'] == 'budget.spent':
            used += p.get('n', 1)
            if limit is None and isinstance(p.get('limit'), int):
                limit = p['limit']
        elif e['name'] == 'budget.exhausted':
            exhausted = True
    return limit, used, exhausted


def main(argv):
    if len(argv) < 3 or argv[0] not in ('set', 'spend'):
        fail(__doc__.split('\n\n')[1])
    verb, scope, cls, rest = argv[0], argv[1], argv[2], argv[3:]
    if not scope.strip() or not cls.strip():
        fail('empty scope or class')
    limit, used, exhausted = ledger(sys.stdin, scope, cls)
    by = os.environ.get('SELF_CALLER', '')
    if verb == 'set':
        if len(rest) != 1:
            fail('usage: self run budget set <scope> <class> <limit>')
        new = count(rest[0], 'limit')
        if new == limit:
            return
        p = {'scope': scope, 'class': cls, 'limit': new, 'used': used}
        print(json.dumps({'name': 'budget.set', 'payload': p}, sort_keys=True))
        return
    opts = {'--n': '1', '--limit': None, '--note': None}
    i = 0
    while i < len(rest):
        if rest[i] in opts and i + 1 < len(rest):
            opts[rest[i]] = rest[i + 1]
            i += 2
        else:
            fail('unexpected argument %r' % rest[i])
    n = count(opts['--n'], '--n')
    given = count(opts['--limit'], 'limit') if opts['--limit'] is not None else None
    if limit is None and given is None:
        fail('no limit for %s in %s: budget set it or pass --limit' % (cls, scope))
    if limit is not None and given is not None and given != limit:
        fail('%s in %s has limit %d, not %d; budget set changes it' % (cls, scope, limit, given))
    limit = limit if limit is not None else given
    p = {'scope': scope, 'class': cls, 'n': n, 'used': used + n, 'limit': limit}
    if opts['--note']:
        p['note'] = opts['--note']
    if by:
        p['by'] = by
    if used + n <= limit:
        print(json.dumps({'name': 'budget.spent', 'payload': p}, sort_keys=True))
        return
    msg = '%s in %s exhausted: %d of %d used, %d more refused' % (cls, scope, used, limit, n)
    if exhausted:
        fail(msg, 3)
    print('budget: ' + msg, file=sys.stderr)
    p = dict(p, used=used, refused=n)
    del p['n']
    print(json.dumps({'name': 'budget.exhausted', 'payload': p}, sort_keys=True))


main(sys.argv[1:])

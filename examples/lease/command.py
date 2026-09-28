#!/usr/bin/env python3
'''lease - exclusive, expiring ownership of any named resource.

usage: self run lease acquire <resource> [--ttl 30m] [--holder H] [--note TEXT] [--steal]
       self run lease renew   <resource> [--ttl 30m] [--holder H]
       self run lease release <resource> [--holder H]

A resource is any string: goal/<id>, repo:<path>#<branch>, a worktree path.
The holder defaults to SELF_CALLER. acquire fails while another holder's lease
is unexpired; after expiry it needs --steal, which records the previous holder.
acquire by the current holder renews. Declared atomic: the kernel commits the
decision only against the log it was made from, so concurrent acquires have
exactly one winner. Refusals exit 3 with the holder and expiry on stderr.
'''
import json, os, re, sys
from datetime import datetime, timedelta, timezone

UNITS = {'s': 'seconds', 'm': 'minutes', 'h': 'hours', 'd': 'days'}


def fail(msg, code=2):
    print('lease: ' + msg, file=sys.stderr)
    sys.exit(code)


def ttl(s):
    m = re.fullmatch(r'(\d+)([smhd])', s or '')
    if not m or int(m.group(1)) <= 0:
        fail('--ttl wants a positive <n>s|m|h|d, got %r' % s)
    return timedelta(**{UNITS[m.group(2)]: int(m.group(1))})


def ts(t):
    return t.strftime('%Y-%m-%dT%H:%M:%SZ')


def parse_ts(s):
    return datetime.strptime(s, '%Y-%m-%dT%H:%M:%SZ').replace(tzinfo=timezone.utc)


def leases(lines):
    live = {}
    for raw in lines:
        try:
            e = json.loads(raw)
        except ValueError:
            continue
        name, p = e.get('name', ''), e.get('payload')
        if not name.startswith('lease.') or not isinstance(p, dict) or 'resource' not in p:
            continue
        if name in ('lease.acquired', 'lease.renewed'):
            live[p['resource']] = p
        elif name == 'lease.released':
            live.pop(p['resource'], None)
    return live


def main(argv):
    if len(argv) < 2 or argv[0] not in ('acquire', 'renew', 'release'):
        fail(__doc__.split('\n\n')[1])
    verb, resource, rest = argv[0], argv[1], argv[2:]
    if not resource.strip():
        fail('empty resource')
    opts = {'--ttl': '30m', '--holder': os.environ.get('SELF_CALLER', ''), '--note': None}
    steal = False
    i = 0
    while i < len(rest):
        a = rest[i]
        if a == '--steal' and verb == 'acquire':
            steal = True
        elif a in opts and i + 1 < len(rest) and (a != '--note' or verb == 'acquire') and (a != '--ttl' or verb != 'release'):
            opts[a] = rest[i + 1]
            i += 1
        else:
            fail('unexpected argument %r' % a)
        i += 1
    holder = opts['--holder']
    if not holder:
        fail('no holder: set SELF_CALLER or pass --holder')
    now = datetime.now(timezone.utc).replace(microsecond=0)
    cur = leases(sys.stdin).get(resource)
    expired = cur is not None and parse_ts(cur['expires_at']) <= now
    mine = cur is not None and cur.get('holder') == holder
    held = '%s held by %s until %s' % (resource, cur.get('holder'), cur.get('expires_at')) if cur else ''

    if verb == 'release':
        if not mine:
            fail(held or resource + ' is not leased', 3)
        out = ('lease.released', {'resource': resource, 'holder': holder})
    elif verb == 'renew':
        if not mine:
            fail(held or resource + ' is not leased', 3)
        if expired:
            fail(held + ' has expired; acquire it again', 3)
        out = ('lease.renewed', {'resource': resource, 'holder': holder,
                                 'expires_at': ts(now + ttl(opts['--ttl'])), 'since': cur.get('since')})
    else:
        p = {'resource': resource, 'holder': holder, 'expires_at': ts(now + ttl(opts['--ttl'])), 'since': ts(now)}
        if opts['--note']:
            p['note'] = opts['--note']
        if cur and mine and not expired:
            out = ('lease.renewed', dict(p, since=cur.get('since')))
        elif cur and not mine and not expired:
            fail(held, 3)
        elif cur and not mine and not steal:
            fail(held + ' has expired; pass --steal to take it', 3)
        else:
            if cur and not mine:
                p['stolen_from'] = cur.get('holder')
            out = ('lease.acquired', p)
    print(json.dumps({'name': out[0], 'payload': out[1]}, sort_keys=True))


main(sys.argv[1:])

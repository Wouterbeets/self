#!/usr/bin/env python3
import json, os, re, shutil, subprocess, sys
from datetime import datetime, timezone

USAGE = '''usage: self run subsystem spawn <name> [--owns name.prefix,...] [--note TEXT]
       self run subsystem observe [<name>]
       self run subsystem retire <name> <reason…>'''

NAME = re.compile(r'[a-z0-9][a-z0-9._-]{0,63}')


def fail(msg, code=2):
    print('subsystem: ' + msg, file=sys.stderr)
    sys.exit(code)


def self_bin():
    for c in (os.environ.get('SELF_BIN'), shutil.which('self'), '/home/wouter/go/bin/self'):
        if c and os.access(c, os.X_OK):
            return c
    fail('no self binary: set SELF_BIN')


def emit(name, payload):
    print(json.dumps({'name': name, 'payload': payload}, sort_keys=True))


def known(lines):
    subs = {}
    for raw in lines:
        try:
            e = json.loads(raw)
        except ValueError:
            continue
        p = e.get('payload')
        if not isinstance(p, dict) or not isinstance(p.get('name'), str):
            continue
        n = e.get('name')
        if n == 'subsystem.spawned':
            subs[p['name']] = dict(p, observed=None)
        elif n == 'subsystem.observed' and p['name'] in subs:
            subs[p['name']]['observed'] = p
        elif n == 'subsystem.retired':
            subs.pop(p['name'], None)
    return subs


def tally(home, after):
    path = os.path.join(home, 'events.jsonl')
    head, at, total, size, fresh = 0, '', 0, 0, {}
    try:
        size = os.path.getsize(path)
        with open(path, encoding='utf-8') as f:
            for raw in f:
                try:
                    e = json.loads(raw)
                except ValueError:
                    continue
                total += 1
                head, at = e.get('seq', head), e.get('occurred_at', at)
                if head > after:
                    fresh[e.get('name', '?')] = fresh.get(e.get('name', '?'), 0) + 1
    except FileNotFoundError:
        pass
    return head, at, total, size, fresh


def main(argv):
    subs = known(sys.stdin)
    if not argv or argv[0] not in ('spawn', 'observe', 'retire'):
        fail(USAGE)
    verb, rest = argv[0], argv[1:]
    root = os.path.join(os.environ.get('SELF_HOME') or fail('SELF_HOME unset'), 'sub')

    if verb == 'spawn':
        if not rest or not NAME.fullmatch(rest[0]):
            fail('spawn wants a name of lowercase letters, digits, dots, dashes')
        name, opts, i = rest[0], {'--owns': '', '--note': ''}, 1
        while i < len(rest):
            if rest[i] in opts and i + 1 < len(rest):
                opts[rest[i]] = rest[i + 1]
                i += 2
            else:
                fail('unexpected argument %r' % rest[i])
        if name in subs:
            fail('%s already spawned at %s' % (name, subs[name]['home']), 3)
        home = os.path.join(root, name)
        if os.path.exists(os.path.join(home, 'events.jsonl')):
            fail('%s already holds a log; retire or move it first' % home, 3)
        os.makedirs(home, exist_ok=True)
        born = {'name': 'subsystem.born', 'payload': {'name': name, 'parent': os.environ['SELF_HOME']}}
        env = dict(os.environ, SELF_HOME=home)
        r = subprocess.run([self_bin(), 'hear'], input=json.dumps(born) + '\n', env=env, text=True, capture_output=True)
        if r.returncode != 0:
            fail('could not start the child log: ' + r.stderr.strip(), 1)
        for tree in ('view', 'bin'):
            os.makedirs(os.path.join(home, tree), exist_ok=True)
        owns = [o for o in opts['--owns'].split(',') if o]
        emit('subsystem.spawned', {'name': name, 'home': home, 'owns': owns, 'note': opts['--note'],
                                   'by': os.environ.get('SELF_CALLER', '')})
        return

    if verb == 'retire':
        if len(rest) < 2 or rest[0] not in subs:
            fail('retire wants a spawned name and a reason')
        emit('subsystem.retired', {'name': rest[0], 'reason': ' '.join(rest[1:])})
        return

    if len(rest) > 1 or (rest and rest[0] not in subs):
        fail('observe wants no argument or a spawned name')
    for name in rest or sorted(subs):
        s = subs[name]
        last = s['observed'] or {}
        head, at, total, size, fresh = tally(s['home'], last.get('head', 0))
        if head == last.get('head') and size == last.get('bytes'):
            continue
        top = dict(sorted(fresh.items(), key=lambda kv: (-kv[1], kv[0]))[:8])
        emit('subsystem.observed', {'name': name, 'head': head, 'last_at': at, 'events': total,
                                    'bytes': size, 'since': last.get('head', 0), 'new': top})


main(sys.argv[1:])

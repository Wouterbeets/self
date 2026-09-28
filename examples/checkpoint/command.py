#!/usr/bin/env python3
'''checkpoint - a human decision on an exact set of proposed actions, used once.

usage: self run checkpoint propose  <id> <action>... [--note TEXT]
       self run checkpoint approve  <id> [--ttl 24h] [--note TEXT]
       self run checkpoint reject   <id> --reason TEXT
       self run checkpoint withdraw <id> [--reason TEXT]
       self run checkpoint use      <id> <action>

An action is any exact string the proposer will later act on: "open PR from
self/budget", "push goal/x to origin". approve and reject belong to an approver
(APPROVERS below; changing it is a script.authored, so it stays in history).
An approval covers only the recorded actions, each usable once, until --ttl
runs out. use prints checkpoint.used when the action may go ahead; otherwise it
exits 3 and appends nothing, so a caller acts only if the output holds
checkpoint.used. use trusts an approval only when the kernel stamped it with an
approver's caller, so a mind cannot approve itself through self hear under its
own name. Declared atomic: two uses of one action never both succeed.
'''
import json, os, re, sys
from datetime import datetime, timedelta, timezone

APPROVERS = {'wouter'}
UNITS = {'m': 'minutes', 'h': 'hours', 'd': 'days'}


def fail(msg, code=2):
    print('checkpoint: ' + msg, file=sys.stderr)
    sys.exit(code)


def now():
    return datetime.now(timezone.utc).replace(microsecond=0)


def ts(t):
    return t.strftime('%Y-%m-%dT%H:%M:%SZ')


def ttl(s):
    m = re.fullmatch(r'(\d+)([mhd])', s or '')
    if not m or int(m.group(1)) <= 0:
        fail('--ttl wants a positive <n>m|h|d, got %r' % s)
    return timedelta(**{UNITS[m.group(2)]: int(m.group(1))})


def state(lines):
    cps = {}
    for raw in lines:
        try:
            e = json.loads(raw)
        except ValueError:
            continue
        name, p = e.get('name', ''), e.get('payload')
        if not name.startswith('checkpoint.') or not isinstance(p, dict) or 'id' not in p:
            continue
        by, cid = e.get('by', ''), p['id']
        if name == 'checkpoint.proposed':
            cps[cid] = {'status': 'pending', 'actions': list(p.get('actions', [])), 'by': by,
                        'used': set(), 'expires': None}
            continue
        c = cps.get(cid)
        if c is None:
            continue
        if name == 'checkpoint.approved' and by in APPROVERS and c['status'] == 'pending':
            c['status'], c['expires'] = 'approved', p.get('expires')
        elif name == 'checkpoint.rejected' and by in APPROVERS and c['status'] == 'pending':
            c['status'] = 'rejected'
        elif name == 'checkpoint.withdrawn' and c['status'] in ('pending', 'approved'):
            c['status'] = 'withdrawn'
        elif name == 'checkpoint.used' and c['status'] == 'approved':
            c['used'].add(p.get('action'))
    return cps


def opts(rest, allowed):
    got, pos, i = {}, [], 0
    while i < len(rest):
        if rest[i] in allowed:
            if i + 1 >= len(rest):
                fail('%s wants a value' % rest[i])
            got[rest[i]] = rest[i + 1]
            i += 2
        elif rest[i].startswith('--'):
            fail('unknown option %r' % rest[i])
        else:
            pos.append(rest[i])
            i += 1
    return got, pos


def emit(name, p):
    print(json.dumps({'name': 'checkpoint.' + name, 'payload': p}, sort_keys=True))


def main(argv):
    verbs = ('propose', 'approve', 'reject', 'withdraw', 'use')
    if len(argv) < 2 or argv[0] not in verbs:
        fail(__doc__.split('\n\n')[1])
    verb, cid = argv[0], argv[1]
    if not cid.strip() or cid.startswith('--'):
        fail('missing checkpoint id')
    cps = state(sys.stdin)
    c = cps.get(cid)
    me = os.environ.get('SELF_CALLER', '')
    if verb == 'propose':
        o, actions = opts(argv[2:], ('--note',))
        actions = [a.strip() for a in actions]
        if not actions or any(not a for a in actions):
            fail('propose wants at least one nonempty action')
        if len(set(actions)) != len(actions):
            fail('duplicate action')
        if c is not None:
            if c['status'] == 'pending' and c['actions'] == actions:
                return
            fail('checkpoint %s already exists (%s); propose a new id' % (cid, c['status']))
        p = {'id': cid, 'actions': actions}
        if '--note' in o:
            p['note'] = o['--note']
        emit('proposed', p)
        return
    if c is None:
        fail('no checkpoint %s' % cid)
    if verb in ('approve', 'reject'):
        o, pos = opts(argv[2:], ('--ttl', '--note') if verb == 'approve' else ('--reason',))
        if pos:
            fail('unexpected argument %r' % pos[0])
        if me not in APPROVERS:
            fail('%s is for an approver (%s), not %r' % (verb, ', '.join(sorted(APPROVERS)), me), 3)
        if c['status'] != 'pending':
            fail('checkpoint %s is %s, not pending' % (cid, c['status']), 3)
        if verb == 'reject':
            if not o.get('--reason', '').strip():
                fail('reject wants --reason')
            emit('rejected', {'id': cid, 'reason': o['--reason'], 'actions': c['actions']})
            return
        p = {'id': cid, 'actions': c['actions'], 'expires': ts(now() + ttl(o.get('--ttl', '24h')))}
        if '--note' in o:
            p['note'] = o['--note']
        emit('approved', p)
        return
    if verb == 'withdraw':
        o, pos = opts(argv[2:], ('--reason',))
        if pos:
            fail('unexpected argument %r' % pos[0])
        if me != c['by'] and me not in APPROVERS:
            fail('only the proposer %r or an approver may withdraw %s' % (c['by'], cid), 3)
        if c['status'] not in ('pending', 'approved'):
            fail('checkpoint %s is already %s' % (cid, c['status']), 3)
        p = {'id': cid}
        if '--reason' in o:
            p['reason'] = o['--reason']
        emit('withdrawn', p)
        return
    o, pos = opts(argv[2:], ())
    if len(pos) != 1:
        fail('usage: self run checkpoint use <id> <action>')
    action = pos[0].strip()
    if c['status'] != 'approved':
        fail('checkpoint %s is %s, not approved' % (cid, c['status']), 3)
    if action not in c['actions']:
        fail('%r is not among the approved actions of %s: %s' % (action, cid, '; '.join(c['actions'])), 3)
    if action in c['used']:
        fail('%r was already used under %s' % (action, cid), 3)
    if c['expires'] and ts(now()) >= c['expires']:
        fail('approval of %s expired at %s' % (cid, c['expires']), 3)
    left = [a for a in c['actions'] if a not in c['used'] and a != action]
    emit('used', {'id': cid, 'action': action, 'left': len(left)})


main(sys.argv[1:])

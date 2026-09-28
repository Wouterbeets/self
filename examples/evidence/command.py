#!/usr/bin/env python3
'''evidence - record check results against a subject; require them before calling work done.

usage: self run evidence record  <subject> <check> --exit N [--secs S] [--cmd TEXT]
                                  [--dir DIR] [--digest HEX] [--tail TEXT] [--ref URL]
                                  [--limits TEXT] [--timed-out]
       self run evidence require <subject> <check>... [--max-age 24h] [--note TEXT]

A subject is any string the work is about: goal/<id>, loop/<name>/pass/<n>,
repo#branch. A check is a name for one mechanical question: tests, clean,
pushed, diff-within. record keeps one result (passed iff exit is 0; failures
are evidence too); examples/evidence/self-check runs a command and records it.
require prints evidence.satisfied only when the latest result of every named
check passed and, with --max-age, is recent enough; otherwise it names what is
missing, failing or stale, exits 3 and appends nothing. A caller marks work
done only if the output holds evidence.satisfied. A result is the recording
caller's claim; the kernel stamps who recorded it.
'''
import json, re, sys
from datetime import datetime, timedelta, timezone

UNITS = {'m': 'minutes', 'h': 'hours', 'd': 'days'}


def fail(msg, code=2):
    print('evidence: ' + msg, file=sys.stderr)
    sys.exit(code)


def age(s):
    m = re.fullmatch(r'(\d+)([mhd])', s or '')
    if not m or int(m.group(1)) <= 0:
        fail('--max-age wants a positive <n>m|h|d, got %r' % s)
    return timedelta(**{UNITS[m.group(2)]: int(m.group(1))})


def when(s):
    try:
        return datetime.fromisoformat(s[:19]).replace(tzinfo=timezone.utc)
    except (TypeError, ValueError):
        return None


def latest(lines, subject):
    got = {}
    for raw in lines:
        try:
            e = json.loads(raw)
        except ValueError:
            continue
        p = e.get('payload')
        if e.get('name') == 'evidence.checked' and isinstance(p, dict) and p.get('subject') == subject:
            got[p.get('check')] = {'passed': p.get('passed') is True, 'seq': e.get('seq'),
                                   'at': e.get('occurred_at', ''), 'by': e.get('by', ''), 'exit': p.get('exit')}
    return got


def opts(rest, valued, flags=()):
    got, pos, i = {}, [], 0
    while i < len(rest):
        a = rest[i]
        if a in flags:
            got[a] = True
            i += 1
        elif a in valued:
            if i + 1 >= len(rest):
                fail('%s wants a value' % a)
            got[a] = rest[i + 1]
            i += 2
        elif a.startswith('--'):
            fail('unknown option %r' % a)
        else:
            pos.append(a)
            i += 1
    return got, pos


def emit(name, p):
    print(json.dumps({'name': 'evidence.' + name, 'payload': p}, sort_keys=True))


def main(argv):
    if not argv or argv[0] not in ('record', 'require'):
        fail(__doc__.split('\n\n')[1])
    if argv[0] == 'record':
        o, pos = opts(argv[1:], ('--exit', '--secs', '--cmd', '--dir', '--digest', '--tail', '--ref', '--limits'),
                      ('--timed-out',))
        if len(pos) != 2 or not all(x.strip() for x in pos):
            fail('usage: self run evidence record <subject> <check> --exit N [...]')
        try:
            code = int(o['--exit'])
        except (KeyError, ValueError):
            fail('record wants --exit N (the check\'s exit status)')
        p = {'subject': pos[0], 'check': pos[1], 'exit': code, 'passed': code == 0}
        if '--secs' in o:
            try:
                p['secs'] = round(float(o['--secs']), 2)
            except ValueError:
                fail('--secs wants a number')
        for k in ('cmd', 'dir', 'digest', 'ref', 'limits'):
            if o.get('--' + k):
                p[k] = o['--' + k]
        if o.get('--tail'):
            p['tail'] = o['--tail'][-1500:]
        if o.get('--timed-out'):
            p['timed_out'] = True
        emit('checked', p)
        return
    o, pos = opts(argv[1:], ('--max-age', '--note'))
    if len(pos) < 2:
        fail('usage: self run evidence require <subject> <check>... [--max-age 24h]')
    subject, checks = pos[0], list(dict.fromkeys(pos[1:]))
    oldest = datetime.now(timezone.utc) - age(o['--max-age']) if '--max-age' in o else None
    got, gaps = latest(sys.stdin, subject), []
    for c in checks:
        r = got.get(c)
        if r is None:
            gaps.append('%s: never recorded' % c)
        elif not r['passed']:
            gaps.append('%s: failed (exit %s, seq %s)' % (c, r['exit'], r['seq']))
        elif oldest and (when(r['at']) or oldest) <= oldest:
            gaps.append('%s: passed at %s, older than --max-age %s' % (c, r['at'][:19], o['--max-age']))
    if gaps:
        fail('%s is not satisfied:\n  %s' % (subject, '\n  '.join(gaps)), 3)
    p = {'subject': subject, 'checks': {c: got[c]['seq'] for c in checks}}
    if '--max-age' in o:
        p['max_age'] = o['--max-age']
    if o.get('--note'):
        p['note'] = o['--note']
    emit('satisfied', p)


main(sys.argv[1:])

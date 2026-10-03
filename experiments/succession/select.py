#!/usr/bin/env python3
import json, os, sys

USAGE = 'usage: select.py <parent events.jsonl> <out dir>'


def load(path):
    out = []
    with open(path, encoding='utf-8') as f:
        for raw in f:
            try:
                out.append(json.loads(raw))
            except ValueError:
                pass
    return out


def account(root, name, events, why):
    d = os.path.join(root, name)
    os.makedirs(d, exist_ok=True)
    keep = ('name', 'payload', 'occurred_at', 'by')
    rec = ''.join(json.dumps({k: e[k] for k in keep if k in e}, sort_keys=True) + '\n'
                  for e in sorted(events, key=lambda e: e.get('seq', 0)))
    with open(os.path.join(d, 'record.jsonl'), 'w', encoding='utf-8') as f:
        f.write(rec)
    with open(os.path.join(d, 'intent.md'), 'w', encoding='utf-8') as f:
        f.write('# succession: %s\n\n%s\n' % (name, why))
    return len(events), len(rec)


def howtos(ev):
    live = {}
    for e in ev:
        p = e.get('payload') or {}
        if e['name'] == 'howto.saved':
            live[p.get('howto')] = e
        elif e['name'] == 'howto.retired':
            live.pop(p.get('howto'), None)
    return list(live.values())


def metis(ev):
    cards, conf = {}, {}
    for e in ev:
        c = (e.get('payload') or {}).get('card')
        if e['name'] == 'metis.noted':
            cards[c], conf[c] = e, []
        elif e['name'] == 'metis.confirmed' and c in cards:
            conf[c].append(e)
        elif e['name'] == 'metis.retired':
            cards.pop(c, None)
            conf.pop(c, None)
    return [e for c in cards for e in [cards[c]] + conf[c]]


def goals(ev):
    status, by_goal, created = {}, {}, {}
    for e in ev:
        if not e['name'].startswith('goal.'):
            continue
        p = e.get('payload') or {}
        g = p.get('goal')
        by_goal.setdefault(g, []).append(e)
        if e['name'] == 'goal.created':
            created[g] = p
            status.setdefault(g, p.get('status') or 'active')
        elif e['name'] == 'goal.updated' and p.get('status'):
            status[g] = p['status']
        elif e['name'] == 'goal.removed':
            status[g] = 'removed'
    live = {g for g, s in status.items() if s not in ('closed', 'removed')}
    full = set(live)
    for g in live:
        p = created.get(g) or {}
        if p.get('parent'):
            full.add(p['parent'])
    ends = set()
    for g in live:
        for d in (created.get(g) or {}).get('depends_on') or []:
            if d not in full:
                ends.add(d)
    for e in ev:
        if e['name'] == 'goal.updated':
            p = e.get('payload') or {}
            if p.get('goal') in full and p.get('depends_on') is not None:
                for d in p['depends_on']:
                    if d not in full:
                        ends.add(d)
    picked = [e for g in full for e in by_goal.get(g, [])]
    for g in ends:
        picked += [e for e in by_goal.get(g, []) if e['name'] == 'goal.created'
                   or (e['name'] == 'goal.updated' and (e.get('payload') or {}).get('status'))]
    projects = [e for e in ev if e['name'].startswith('project.')]
    return picked + projects, len(live), len(ends)


def intents(ev):
    decl, closed = {}, set()
    for e in ev:
        n = (e.get('payload') or {}).get('name')
        if not isinstance(n, str):
            continue
        if e['name'] == 'intent.declared':
            decl[n] = e
            closed.discard(n)
        elif e['name'] == 'intent.closed':
            closed.add(n)
    return [decl[n]['payload'] for n in decl if n not in closed]


def main(argv):
    if len(argv) != 2:
        print(USAGE, file=sys.stderr)
        return 2
    ev = load(argv[0])
    out = argv[1]
    os.makedirs(out, exist_ok=True)
    total = sum(len(json.dumps(e)) for e in ev)
    print('parent: %d events, %d KB' % (len(ev), total // 1024))
    n, b = account(out, 'howto', howtos(ev), 'Live how-tos, latest revision only.')
    print('howto: %d events, %d KB' % (n, b // 1024))
    n, b = account(out, 'metis', metis(ev), 'Live metis cards with confirmations since their latest note.')
    print('metis: %d events, %d KB' % (n, b // 1024))
    picked, live, ends = goals(ev)
    n, b = account(out, 'goals', picked, 'Open goals with full history, their parents, and the terminal status of every goal they depend on.')
    print('goals: %d open, %d dependency ends, %d events, %d KB' % (live, ends, n, b // 1024))
    oi = intents(ev)
    with open(os.path.join(out, 'open-intents.jsonl'), 'w', encoding='utf-8') as f:
        for p in oi:
            f.write(json.dumps({'name': 'intent.declared', 'payload': p}, sort_keys=True) + '\n')
    print('intents: %d open' % len(oi))
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))

#!/usr/bin/env python3
'''identity - who this body says it is, and who its siblings say they are.

Identity is data, not kernel: the newest self.identity event wins, and the older
ones stay in the log as the history of who this instance said it was.

With more than one body, "newest wins" alone is a bug. A sibling's identity
arrives here as ordinary testimony through a learn:<peer> door, and on a bad day
it is newer than this body's own — so this body would introduce itself as the
other one. The door is the fix: only locally witnessed testimony can say who
THIS body is, and a sibling's is shown as theirs.
'''
import json
import sys


def main():
    if sys.argv[1:]:
        print('usage: self view identity   # takes no arguments', file=sys.stderr)
        return 2

    local, foreign = [], {}
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            event = json.loads(line)
        except ValueError:
            continue
        text = (event.get('payload') or {}).get('text')
        if not (isinstance(text, str) and text.strip()):
            continue
        via = str(event.get('via') or '')
        if via.startswith('learn:'):
            foreign[via[len('learn:'):]] = text.rstrip()
        else:
            local.append(text.rstrip())

    if not local:
        print('no identity yet.')
        print()
        print('  a mind sets one by appending a self.identity event carrying its text')
    else:
        print(local[-1])
        older = len(local) - 1
        if older:
            print()
            print('(%d superseded identity event(s) still in the log)' % older)

    for name in sorted(foreign):
        print()
        print('--- as %s describes itself (arrived through learn:%s, not this body) ---' % (name, name))
        print(foreign[name])
    return 0


if __name__ == '__main__':
    sys.exit(main())

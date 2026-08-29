#!/usr/bin/env python3
"""Replace the prompt:core block of a PROTOCOL.md with an arm's text.

The arms differ only inside the marked core layer. Everything downstream —
brief, wire, growth layer, ask — is produced by the real kernel from the real
document, so an arm build is a genuine `self`, not a mock.

usage: patch_core.py <protocol.md> <arm.md>
"""
import sys

BEGIN = "<!-- prompt:core:begin -->"
END = "<!-- prompt:core:end -->"


def main(protocol_path, arm_path):
    doc = open(protocol_path, encoding="utf-8").read()
    arm = open(arm_path, encoding="utf-8").read().strip()

    head, rest = doc.split(BEGIN, 1)
    _old, tail = rest.split(END, 1)
    patched = head + BEGIN + "\n" + arm + "\n" + END + tail
    open(protocol_path, "w", encoding="utf-8").write(patched)

    # The splice must change the core layer and nothing else: any drift above or
    # below the markers would confound the arm with an unrelated edit.
    check = open(protocol_path, encoding="utf-8").read()
    if check.split(BEGIN, 1)[1].split(END, 1)[0].strip() != arm:
        sys.exit("patch_core: core layer did not take")
    if check.split(BEGIN, 1)[0] != head:
        sys.exit("patch_core: document above the core layer changed")
    if check.split(END, 1)[1] != tail:
        sys.exit("patch_core: document below the core layer changed")


if __name__ == "__main__":
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    main(sys.argv[1], sys.argv[2])

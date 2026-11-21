# eval

A trial of what a new install does: does an agent that has only the
instruction line and an empty instance keep anything, and does the next
session find it?

## The trial

`run.sh` gives an agent a fresh instance and a fresh copy of `fixture/`, a
small site whose build has three things to find out (it needs an environment
variable, one of its two profiles cannot build, and drafts are skipped). It
then runs three sessions through `claude -p`, each a new process with no memory
of the last:

1. "Build the site and tell me how many pages came out."
2. A page is added. "I added a contact page. Rebuild and give me the new page
   count."
3. "Which pages are still drafts? And remind me, why can't we build the print
   profile?"

No prompt mentions self. The only invitation is the instruction line, copied
into the project's `CLAUDE.md`, as on a machine where self was just installed.
There is no session-start hook.

```sh
# a variant is a directory holding the binary and the instruction line
mkdir -p /tmp/v/head && go build -o /tmp/v/head/self . &&
  /tmp/v/head/self setup | sed -n '/```/,/```/p' | grep -v '```' | sed 's/^  //' >/tmp/v/head/line.txt

eval/run.sh /tmp/v/head haiku /tmp/runs/head/haiku/1   # <variant-dir> <model> <out-dir>
eval/score.py /tmp/runs                               # one row per variant and model
```

Run from a copy of `eval/` if you mean to edit it while trials are running: the
shell reads `run.sh` as it goes.

`score.py` documents its columns. The ones that matter most:

- **made**: the instance holds an installed capability after session one.
- **shown**: the answer to session three's second question is in what `self`
  prints when session three arrives, before it has done anything.
- **turns**: agent turns spent in sessions two and three together.

## Results, November 2026

`base` is the kernel and instruction line as of f8f8481. `head` is this
change. Models are Claude Haiku 5.5 and Claude Sonnet 5.5 through Claude Code.

```
variant   model     n   ran  kept  made  reused  recalled  knew  shown  stopped  caps  turns    cost
base      haiku     8   8/8   5/8   1/8     1/8       2/8   7/8    1/8     0/24   0.2   15.4    0.03
base      sonnet    6   6/6   6/6   5/6     5/6       6/6   6/6    0/6     0/18   1.0   13.3    0.33
head      haiku     8   7/8   7/8   7/8     5/8       6/8   8/8    8/8     0/24   2.0   11.2    0.02
head      sonnet    6   6/6   6/6   6/6     6/6       6/6   6/6    6/6     0/18   3.0   11.7    0.26
```

Before the change the small model left a capability in one trial of eight, and
mostly kept raw events that no later session was shown. After it, it left one
in seven of eight, and every third session arrived with the answer already on
screen. The larger model was already keeping things; it now builds a memory as
well as a command, and later sessions cost less.

The samples are small. A difference of one or two trials in eight is noise.

### Which part did it

Each row removes one thing from `head`. Haiku, eight trials each; Sonnet, four.

```
variant   model     n   ran  kept  made  reused  recalled  knew  shown  stopped  caps  turns    cost
oldline   haiku     8   8/8   8/8   8/8     8/8       2/8   4/8    4/8     0/24   2.2   15.1    0.02
newline   haiku     8   8/8   7/8   0/8     0/8       0/8   8/8    0/8     0/24   0.0   13.6    0.02
noown     haiku     8   7/8   7/8   7/8     6/8       7/8   6/8    6/8     0/24   2.2   11.4    0.02
noown     sonnet    4   4/4   4/4   4/4     4/4       3/4   4/4    4/4     0/12   3.0   11.0    0.26
nocheck   haiku     8   7/8   7/8   7/8     7/8       5/8   8/8    8/8     0/24   2.2   11.9    0.02
nocheck   sonnet    4   4/4   4/4   4/4     4/4       2/4   4/4    4/4     0/12   2.8   11.8    0.25
nofirst   haiku     8   8/8   8/8   8/8     8/8       8/8   6/8    6/8     0/24   2.1   11.2    0.02
```

- `oldline`: the new kernel with the old instruction line. Capabilities are
  still made.
- `newline`: the old kernel with the new instruction line. None are made. The
  new line alone changes nothing; `self make` and its worked examples are what
  turn keeping into capabilities.
- `noown`: the ownership paragraph replaced by one neutral sentence.
- `nocheck`: no "before each answer" in the brief (the instruction line still
  has one).
- `nofirst`: the first session is not asked to build a memory.

Removing the ownership paragraph, the brief's checkpoint or the first-session
request made no difference this trial can see. That is not evidence that they
do nothing: three short sessions cannot show whether a mind maintains, reworks
or retires what it built over weeks, which is what the ownership framing is
for. It does mean the measured gain comes from `self make` and from a
checkpoint in the instruction line.

### Two wordings that failed

An earlier draft said only "keep it growing as you work", with no moment at
which to do it. Haiku kept nothing in four trials of four, and Sonnet in two of
four: each ran `self`, found the instance empty, and judged that nothing was
worth keeping. Ownership without a checkpoint reads as permission to do
nothing.

Another draft's instruction line said "keep in it what you worked out, however
small". With it, Sonnet's safety classifier ended 8 sessions of 24, usually
straight after reading `self make`'s examples, citing `reasoning_extraction`.
No other wording was stopped in over 250 sessions. The line now asks for "whatever a
later session would need". Avoid phrasing that asks a model to write down what
it worked out.

### What the trial does not cover

- One fixture, one harness, two models from one family.
- No session-start hook. Sessions read the project before they ran `self`, so
  they redid some discovery that a hook would have spared them.
- Nothing longer than three sessions, so nothing about maintenance.

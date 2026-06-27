# home/ — your board, ready to run

A real, daily-use surface grown on `self`: a **Now / This week / Waiting / Done**
board. Capture anything into one box; move tasks between lanes with one click.
Everything is a plain HTML form (no JavaScript), and the brain can drive it too
("add task: call the plumber" → it lands in Now).

This directory is a ready-to-run body — `events.jsonl` plus the home's two keys.
You don't need an LLM to *use* it (only to grow new capabilities); `self
rehydrate` rebuilds the board from the log.

## run it daily

```sh
go build -o self .

# point a home at this body (use a stable path so your tasks persist)
export SELF_HOME=$HOME/.self
cp home/events.jsonl home/.secret home/.identity "$SELF_HOME"/

./self            # rehydrates, then serves the live garden
# open http://localhost:7777/board  — capture, click to move, done.
```

Or from the terminal:

```sh
./self run capture "buy milk"
./self run move 12 this week      # 12 is the task's id (shown on its card as #12)
./self show board                 # render it
```

Your tasks are events in `$SELF_HOME/events.jsonl` — the only truth. Nothing is
ever lost; "done" is just another event, and the board is a pure replay of the
log. Back up (or sync) that one file and you've backed up your board.

## the capabilities

| | what it does |
| --- | --- |
| `capture <text>` | drop a task onto the board (starts in **Now**) → `task.captured` |
| `move <id> <lane>` | send a task to `now` / `this_week` / `waiting` / `done` (friendly spellings like "this week" work) → `task.moved` |
| `board` | the Now / This week / Waiting / Done view, with a capture box and one-click move buttons |

## make it yours

This is `self` — you grow it by talking to it. Wire a brain (see
`../brain/README.md`) and ask, in plain language, for what your life needs:
*"add a notes board", "track which tasks I finished this week", "add a
'someday' lane".* The brain declares the capability and the kernel compiles it
live. The board you start with is just the first sentence, not the whole story.

The seed this was grown from is `../seeds/home/` — it ships reference
implementations and **examples**, so every capability is verified against its
contract before it installs (the compile even caught a real bug in `move`'s
multi-word lane parsing before it could ship).

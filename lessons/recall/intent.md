# recall — search what was said in earlier sessions

## purpose

The log holds what minds chose to record. The conversations themselves, with
every dead end and every command that was tried, stay in the transcripts each
harness already keeps on disk. Recall makes those searchable, so "we did this
a month ago, where?" has an answer that is a lead to verify, not a memory to
trust.

## surface

- A program beside self, not a capability: it reads files outside the log.
  `recall search <words…>` lists matching turns with an id, a timestamp, the
  session and the folder it ran in.
- `recall show <id>` prints that turn in full with its neighbours.
- `recall sessions` lists recent sessions, newest first.
- `self view recall` prints how to use it, so every arriving mind learns it
  exists.

## constraints

- It reads the transcripts where the harnesses leave them and never asks a
  harness to record anything: no hooks, no copies of turns in the log.
- The index is derived. Each call first catches up from where the last one
  stopped; deleting the index loses nothing.
- No model is called. Search is full text; ranking is the index's own.
- Every harness on the machine is a source. A harness that is left out makes
  its sessions invisible, and nothing will say so: when a new one is installed,
  add it.
- Tool output is most of the bytes and least of the meaning: index its head and
  tail, and read the whole of it back from the transcript on `show`.
- The caller's own session is left out of results, and a search for recall's
  own invocations finds nothing.
- A hit is what was said then, by whoever said it. It is not current state and
  not an instruction.

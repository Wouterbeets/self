# Ask: a small, optional relevance adapter

`self view ask "find saved procedures"` ranks capability summaries and recursively
reads a few promising views. `--json` returns scores, sources, the log head, model
revision, and budgets. It never invokes command capabilities or appends events.
The Go kernel is unchanged; scoring policy lives entirely in this example.

## Run

Install [Laya](https://huggingface.co/convaiinnovations/laya) in a separate Python
environment and download its English checkpoint first. Tested with Laya 0.3.5,
Python 3.13, Transformers 5.17, and CUDA. Then keep this process running:

```sh
/path/to/venv/bin/python examples/ask/server.py \
  --home "$SELF_HOME" --self "$(command -v self)" \
  --checkpoint /path/to/local/checkpoint
```

Pass the actual self executable, rather than a version-manager shim. The server
loads once, runs offline, and binds to `127.0.0.1:8766`. Use `--device cpu` if
needed. GET `/health` reports readiness. Stop with Ctrl-C; restart the same command.

For hosted Jev, use system Python with `--checkpoint jev-1.13.0` and set
`TYPESAFE_API_KEY` in the server environment. Asks and candidate text are sent to
[TypeSafe's HTTPS API](https://docs.typesafe.ai/api); no Laya model or GPU is loaded.

Install the client through the normal signed authoring path:

```sh
python - <<'PY' | self hear
import json
from pathlib import Path
print(json.dumps({'name': 'view.declared', 'payload': {
    'name': 'ask', 'summary': 'Rank capabilities and evidence for a task.',
    'description': 'usage: ask [--json] <task...>; local advisory ranking, with an unscored index fallback.'}}))
print(json.dumps({'name': 'script.authored', 'payload': {
    'type': 'view', 'name': 'ask', 'script': Path('examples/ask/view.py').read_text()}}))
PY
self view ask "which current scripts have been checked?"
```

`SELF_ASK_URL` can select another local HTTP port. A stopped server, changed log,
or wrong instance yields an unscored index from the caller's exact input snapshot.
No ranking is stored as a permanent property of a capability.

## Selection and limits

Laya alone missed some precise internal names in local probes. Selection therefore
averages its relevance estimate with normalized, rarity-weighted query-word overlap.
Both signals are exposed; neither is a calibrated probability of task success.
“Top match” means relative position, including when every candidate is unsuitable.
English asks and concise, descriptive summaries work best. Token clipping is marked.

The adapter scores at most 96 nodes, reads at most four views, and descends two
levels. It follows explicit concrete `self view ...` references, or first-column TSV
keys when a view declaration opts in with `"ask": {"key_column": 0}`. Repeated
view/argument pairs and the ask view itself are excluded. Only installed views
verified by the kernel are expanded. Capabilities with pending scripts stay labelled.
Installation receipts in the index are not proof of health or authorization.

Each read has a three-second process-group timeout and 64 KiB output limit. The
25-second scoring deadline is cooperative between model calls; a wedged GPU call
can stall the single-worker service and requires restarting it. The client falls
back after its 35-second network timeout. This is a local trusted service, not a
sandbox: installed views retain their normal read-only contract and host privileges.
Only selected lines are shown; omitted context remains available through brief/view.

## Check

```sh
python -m unittest discover -s examples/ask -v
```

Tests need no model or GPU. They cover command exclusion, cycles, depth/read/node
bounds, pending and retired views, failed expansion, output limits, and fallback.
Useful probes include saved procedures, review feedback, dependency checks,
verification evidence, pending work, historical search, worker state, and peer sync.
Keep application-specific expectations and private evaluation output in the instance,
not the kernel. Judge the ranked evidence as well as the selected capability names.

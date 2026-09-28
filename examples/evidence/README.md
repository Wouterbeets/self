# Evidence: record checks, require them before calling work done

A mind that says work is done should be able to point at the checks that make
it so. `self-check` runs one check command in the caller's environment and
records its result against a subject; `require` passes only when the latest
result of every named check passed:

```sh
C=examples/evidence/self-check
$C goal/x tests  --dir ~/src/app --timeout 20m --exclusive heavy-build --mem 6G -- make test
$C goal/x clean  --dir ~/src/app -- sh -c 'test -z "$(git status --porcelain)"'
self run evidence require goal/x tests clean --max-age 2h | grep -q evidence.satisfied && self run goal ...
self view evidence goal/x        # latest per check, failures with their last lines, gates met
```

A subject is any string (`goal/<id>`, `loop/<name>/pass/<n>`, `repo#branch`); a
check is a name for one mechanical question. Failures are recorded too, so a
pass shows what it tried. `require` exits 3 naming what is missing, failing or
stale and appends nothing; which checks a kind of work needs, and refusing to
close without `evidence.satisfied`, is the caller's policy.

`self-check` runs the check outside the kernel because commands get a clean
HOME and PATH. `--timeout` kills the check's whole process group, so no child
outlives it; `--exclusive NAME` waits on a shared flock so only one heavyweight
build runs at a time; `--mem`/`--cpu` run it in a transient systemd user scope
with MemoryMax/CPUQuota. It records the command, directory, limits, duration,
exit, a sha256 of the output and, on failure, the last 15 lines. A result is
the recording caller's claim; the kernel stamps who recorded it, and
`self run evidence record` also takes results from elsewhere (a CI run, with
`--ref <url>`).

Git checks that make completion mechanical (`$R` a repository, `$B` a branch):

```sh
sh -c 'test -z "$(git -C $R status --porcelain)"'                         # clean
sh -c 'git -C $R fetch -q origin $B && test "$(git -C $R rev-parse origin/$B)" = "$(git -C $R rev-parse HEAD)"'  # pushed
sh -c '! git -C $R diff --name-only origin/main...HEAD | grep -v "^services/app/"'  # diff within a path
sh -c 'git -C $R merge-base --is-ancestor $OLD origin/$B'                 # no force-push since $OLD
sh -c 'test $(df --output=avail -k /tmp | tail -1) -gt 2000000'            # 2 GB free in /tmp
```

## Install

```sh
jq -nc '{name:"command.declared",payload:{name:"evidence",summary:"Record check results against a subject; require them before calling work done",description:"usage: see examples/evidence/command.py; run checks with examples/evidence/self-check"}}
        ,{name:"view.declared",payload:{name:"evidence",summary:"What was checked, what passed, what gates were met",description:"usage: see examples/evidence/view.py",consumes:["evidence.checked","evidence.satisfied"]}}' | self hear
jq -nc --rawfile c examples/evidence/command.py --rawfile v examples/evidence/view.py \
  '{name:"script.authored",payload:{type:"command",name:"evidence",script:$c}},{name:"script.authored",payload:{type:"view",name:"evidence",script:$v}}' | self hear
```

`self-check` is a plain script; put it on PATH or call it by path.

## Test

```sh
sh examples/evidence/test.sh
```

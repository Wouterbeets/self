# Kernel minimization audit

Scope: reduce the runtime's implementation and policy while retaining its
append-only state, local signed installation, and ordinary process/pipe model.
This records the implementation review and behavioral verification; line count
alone is not the completion criterion.

## Current measurements

Compared with repository HEAD, the seven root production Go files total
2,755 lines instead of 2,955: 200 fewer (6.8%). This includes comments and the
existing worktree changes; it excludes tests, documentation, examples, and
browser sidecars. Moving code to another file would not count as a reduction.
`go.mod` declares no external dependencies.

## Verified mechanisms

| Requirement | Implementation and evidence |
| --- | --- |
| Orientation reads, never appends or consumes stdin | `dispatch`, `cmdSituate`; `TestReadsNeverWrite`, `TestSituateIgnoresStdin` |
| Shared interpretation during writes and cold starts | `state.apply`; `TestIncrementalReplayMatchesColdReplay` compares each lifecycle prefix |
| Local receipts govern executable bytes | `install`, `verifyReceipt`, `materialize`; undeclared, forged, tampered and replayed-receipt tests |
| Scripts compose through argv, stdin, stdout and exit status | `runCommand`, `runView`; atomic event-batch, opaque-byte, failed-output and silent-command tests |
| Attribution survives verbatim | `callerClaim`, signed receipt fields; `TestCallerAttributionIsVerbatim` |
| Views receive signed inputs and no instance location | `consumed`, `scriptEnv`; receipt-versus-declaration and environment tests |
| Concurrent appends retain distinct sequence numbers | log lock and append path; `TestConcurrentAppendsDoNotCollide` |
| Crash recovery preserves complete records | `dropFragment`, `lastSeq`; torn/complete tails, including 2 MB records, and invalid-append tests |
| Reconstruction reports failure and protects live bytes | `rehydrate`; permission failure, failed link, stale symlink and reconciliation tests |
| Account imports cannot activate kernel vocabulary | `readAccount`, frozen `refused` set; membership tests plus deposit rejection for every forbidden name |
| Curation and key creation survive competing writers | shared staged publication using `os.Link`; concurrent export and key tests |
| Loop policy is explicit and domain-independent | `cmdLoop`, standard flag parsing; quiet/change/refusal, environment, nudge and timeout tests |
| Timeout terminates signal-ignoring descendants in the process group | `cmd.Cancel`; normal and SIGTERM-ignoring process regression |
| Shell completion delegates domain knowledge | `runCompleter`; delegated arguments, name discovery, deadline and read-only tests |

The offline demo additionally exercises the public executable through account
learning, signed installation, command/view execution, tamper repair, rebuilding
from log plus key, and account exchange. No real model or external service is
needed for this evidence.

## Sidecar integration

The new `TestHTTPWithRealKernel` builds and invokes the actual kernel, then
checks the HTTP brief, view index and a nested view with an argument. It exposed
a regression missed by stub-based tests: plain capability names lost hyperlinks
and nested routing in self-serve. The sidecar now accepts both plain and legacy
bold names for display. Routing uses `self __complete view|run ""`, the existing
machine interface, instead of interpreting brief sections. This removes eight
sidecar production lines and avoids adding a second capability-list API.

## Boundary assessment

| Candidate split | Dependencies inspected | Decision |
| --- | --- | --- |
| HTTP and browser presentation | `cmd/self-serve` calls the CLI for opaque view output, command effects and completion names; browser launcher owns HTTP lifecycle | Keep external. The real-kernel HTTP test verifies the boundary, including nested names. |
| Provider adapters | `examples/mind-*` supply a normal process to `cmdLoop` | Keep external. Provider-specific session formats, credentials and options remain outside the kernel. |
| Completion | Candidate enumeration uses `state.list`, pending receipts, built-in log shadowing and signed `executeView`; shell shims only forward words | Keep enumeration beside replay. External enumeration would duplicate that interpreter or require a new equivalent API. Domain candidates already run as ordinary views; shims are printed data, not a second interpreter. |
| Account learning | `readAccount` validates the frozen vocabulary before `cmdLearn` stamps moments, provenance and an attestation in one batch | Keep the trust boundary together. Sending imported records through ordinary `hear` loses their original moments and speaker, and treating learned declarations as local events changes authority. |
| Account giving | Capability selectors verify historical receipts; lineage renaming shares the frozen import vocabulary; publication shares the atomic file primitive | Keep the small exporter beside those rules. A shell export of the formatted built-in log would truncate payloads; an independent exporter would need raw-event, signature and vocabulary semantics. |
| Fixed-point loop | `stateRevision` observes committed events; `situate` shares protocol layers; typed `errRefused` distinguishes recoverable authoring rejection from I/O failure | Keep the generic driver beside these primitives. An external wrapper over current CLI status cannot distinguish rejection from a failed append, and hashing the physical log counts torn writes. Splitting requires additional machine primitives or a duplicate interpretation of state. |

The criterion is total implementation and semantic duplication, not merely the
number of source files or executables. No domain model, resident AI, provider
client, network server, database or plugin loader was added to the kernel.
Regular and completion views now share one `executeView`; log replay and live
installation share `state.apply`; publication and reconstruction each have one
implementation. This completes the identified boundary and duplication review.

## Verification and limits

The final checks are the full Go tests (including the actual-kernel HTTP test),
race detector, vet, package builds, diff whitespace checks, and offline demo.
The evidence table above states what each behavioral check establishes; a green
suite alone is not proof of arbitrary stronger guarantees.

The runtime still makes no fsync durability claim, provides no whole-account
filesystem transaction or script sandbox, leaves the log unbounded, and cannot
kill descendants that deliberately leave their process group. These are
explicit boundaries, not guarantees inferred from successful tests.

No deployment or commit is implied by this audit. Unrelated worktree files are
outside this kernel change.

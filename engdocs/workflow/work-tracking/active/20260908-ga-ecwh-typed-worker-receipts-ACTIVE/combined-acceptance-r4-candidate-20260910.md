# ga-tmgr combined acceptance R4 — source-only handoff
Date: 2026-09-10. Existing READY ga-ecwh parent, attached owned ga-tmgr.
Core: /home/loucmane/gascity-core-worktrees/ga-ecwh-typed-worker-receipts
HEAD: 96e0ed0423c77ecc44237b4f9c3e667648c74a48
Owner: external-coordinator.v1:038188a07df973a7a976387e7af363f2acab2472642d9ed3844944553e26a707
Scoped direct-source authority retained; full parent goal active/incomplete.
SOURCE READY FOR INDEPENDENT REVIEW; R4 runtime NOT EXECUTED or accepted.

## Consolidated review inputs
All paths in this section are under /tmp/ga-tmgr-combined-candidate-r4-20260910/.
- source.tar — complete combinedprobe source/docs and exact owning decoder test.
  SHA256 202b69434056f897f55cc4d8639d98a201cbc1053de3aec51ce52fd8af1643b6
- source.sha256 — individual source hashes.
  SHA256 ba2edd31b8bb094752ee3b7326804160f0dfffdea272c203dbb9c824b9244d64
- r3-to-r4-complete.diff — full unified delta, including new files.
  SHA256 4b5644a43230f1e0ffe495d401479245bee297b90aac33f02f2f10dcec380125
- execution-plan.json — exact cases/argv template, namespaces, dependency pins.
  SHA256 3482db51643d02cc07168c0f92d8aae842b179c163a6338e60b28023d8435b64
- combinedprobe — static reviewed-input candidate, NOT runtime-tested.
  SHA256 0bb7076b25c060b20c6867a9c26014be3997a8fd640d5765ce6e4fdee58abbc1
- impostor — same version text, separately linked imageVariant=r4-impostor.
  SHA256 d9fd9dd8611adc7c28ca0a66c01ba52a09a62fbd875c917116b9019c734401dc
- old-decoders.test — actual owning-package decoder test executable.
  SHA256 cd2b75151423b35de9abe3e0525f6872ce7de53bf78f4550dd5d62398c0f7df9
- decoder-inputs-complete.sha256 / decoder-inputs-complete.tar — 8478 exact
  compiler/test/module/embed inputs; unchanged original decoder bodies included.
  Manifest SHA256 6bab7149db530d4383d265d86d0d8103767bb8d8007f5aacf28089d66ba01da1
  Archive SHA256 5ee22a81be8f01ee64a4570fd2ebd9d91dbb2a5c6ee162916c76406686b5de0f

## Six groups and deliberate failure cases
1. Real fixture exit/restart/exec before/after commit: exact phase, pidfd terminal,
   replacement PID/instance and teardown, or same PID/start + actual exec/new
   startup instance. No brute-force PID reuse or claim to exhaust ABA schedules.
2. Same-build endpoint impersonation and distinct executable bytes/path: actual
   second disposable listener/process; old host binding refuses before W starts.
   The inherited expected-version negative is not counted as wrong-image proof.
3. Before/after rename: bounded real deadline wait, writer exit23, channel EOF,
   and real O_PATH-directory fsync EBADF. The fsync case proves a deliberate
   syscall adapter error, NOT real disk EIO or crash-persistence durability.
   Expiry exercises V's deadline refusal at the phase; W's existing deadline
   checks remain. No atomic kernel clock/rename guarantee is claimed.
4. Four actual readers at valid-old/mixed/valid-new phases; a held receipt read
   crosses final rename and must refuse. Another real transaction is refused by
   the occupied lease. No second writer commit or stochastic race exhaustion.
5. Exact unchanged Core LoadManifest/decodeReceipt compile and execute, with
   valid old-format positives and synthetic-new/version-changed refusals.
   This does not stand in for final integration of every production consumer.
6. Fixture access/default-descendant ACL controls, confined denials and full
   unchanged cache stat/hash/xattr inventory. Unsupported positives stop
   INCONCLUSIVE; they are never converted to denial PASS.
All postcommit faults retain the committed pair as committed_evidence_incomplete.
No rollback, replay, repair, or acceptance from a result file alone.

## Bounds, effects, inherited evidence
The ordered plan has22 cases; whole run/fixture360s, V25s, W23s, supervisor35s,
request20s, lease22s, HTTP2s. All loops have contexts; no unbounded output.
Per case: one disposable supervisor, one V, at most one W/bwrap and its existing
mutation child; lifecycle/identity cases may add one replacement supervisor.
All targets are owned fixture PIDs. Only fixture loopback HTTP is exposed to V;
W retains its private network/proc and exact minimal mounts/pipes.
Only the fresh runroot contains runtime writes; every fixture/result is retained.
No installed metadata/cache, service, rig, credential or host configuration writes.
fdInventory remains byte-identical to R3:
8c2f3cc0cf309cb8141e9a3454e800966addd820240fd76b5857cfe78e69c7a4.
Same namespace/transport/denial algorithms and subscription review gates retained.
Plan argv is one exact template plus closed ordered cases to retain16KiB bounds.
M5/R3 observations are inherited reasoning only; changed R4 helper/input requires
its own review and input-bound runtime proof. Consumed M5/R2/R3 are never replayed.
Original20/index and R3 preedit archive remain preserved. R2 INCONCLUSIVE and
Opus R3 HOLD plus separate Astra adjudication remain unchanged historical evidence.
ga-oz9e remains deferred; Core -> gct-13ku/gct-10pg -> useful Claude -> both handovers.

## Offline evidence and review questions
focused-bound-final.txt: focused pure combinedprobe suite PASS (0.022s).
vet-bound-final.txt: focused vet exit0. Static builds exit0.
old-decoders-final.txt: exact compiled owning test PASS; no decoder reimplementation.
red.txt retains intended missing-case RED; earlier iterations remain preserved.
Initial non-escalated go-list lacked cache write access and produced an incomplete
2-input archive. It is preserved but NOT accepted; only *-complete inputs above bind
decoder provenance. No runtime probe or model inference occurred this checkpoint.
Review generation/endpoint attribution, pipe EOF/exit handling, exact post-rename
classification, reader barriers, ACL support refusal, deadlines/teardown and frame
bounds. Pure fixture observations in tests are validator tests, NOT live results.

## Next command — ONLY after independent exact-candidate source review
Actual UID1000 in the six frozen genuine host namespaces; R4 runroot must be absent.
No execution permission is inferred from this document.
```sh
/tmp/ga-tmgr-combined-candidate-r4-20260910/combinedprobe execute-reviewed /tmp/ga-tmgr-combined-candidate-r4-20260910/execution-plan.json 3482db51643d02cc07168c0f92d8aae842b179c163a6338e60b28023d8435b64
```
Require genuine outer exit0, then separately:
```sh
/tmp/ga-tmgr-combined-candidate-r4-20260910/combinedprobe validate-result /tmp/ga-tmgr-combined-run-r4-20260910/result.frames 3482db51643d02cc07168c0f92d8aae842b179c163a6338e60b28023d8435b64 0
```
Do not use literal0 if execution actually failed. Preserve refusal; no automatic retry.

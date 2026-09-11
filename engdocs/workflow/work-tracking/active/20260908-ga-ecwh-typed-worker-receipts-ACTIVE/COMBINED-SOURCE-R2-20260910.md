# ga-tmgr combined R2 — four-finding source correction

SOURCE CANDIDATE; independent delta review and all combined runtime remain HOLD.
No combined R1/R2 run, M5 rerun, inference, live operation or goal change occurred.

## Decision and preserved review

Retain the existing V/W architecture. Exploration ended with M5 feasibility PASS.
Boundary decision: /tmp/ga-tmgr-boundary-checkpoint-20260910-1701.md
SHA256 dd5fa3f0987d738ec196da5129132f2536237c679ada12e2f61584be504bf3dc.
R1 actual Opus SOURCE HOLD: /tmp/ga-tmgr-combined-independent-review-r1-20260910/verdict.md
SHA256 0cf2e5b8ec4d192ff70e14fb4052e57d726b3a1c11296b26d1324a30837dddac.
verified.json SHA256 64928ac2a0bc77846a2ed3455cc4f506445fd28644d9adbb771333b082d8c714.
These hashes were reverified; old verdicts and runtime/source histories unchanged.

## Exactly the consolidated correction

1. Writer terminal evidence now binds actual child PID, Wait ProcessState PID,
   observed Wait/ExitError, and pidfd POLLIN. Missing witnesses refuse.
   Unsupported/version-mismatch use writer_not_started, exit -1 and no fake reap.
2. R1 DID execute version. R2 executes literal /proc/observed-PID/exe version,
   rechecks genuine process/API identity around it, and compares a separately
   frozen plan/request ExpectedVersion. New version-mismatch case independently
   varies that expectation and requires exact pre-writer refusal evidence.
3. Unsupported lease routes are actually absent from the HTTP mux. Exact404
   path/status/body survive query and post-query host identity revalidation.
   200 application errors, other statuses, routes or details do not satisfy it.
4. Every process attempt binds PID/boot/start/image, address, FD and marker/memory
   digests. Retained fixture pidfd/generation checks surround fresh host positive
   controls after EACH negative set. Wrong targets, unavailable positives and
   out-of-order/older-than10s refresh windows are INCONCLUSIVE.

Rich target witnesses exceeded the old single16KiB observation record in a pure
regression (32995 indented /21965 compact bytes). To retain the unchanged16KiB
per-frame guard, result.frames has exactly six existing length-prefixed records:
one header plus five ordered cases. Count/order/truncation/trailing/unknown/
duplicate/oversize checks remain closed. This is evidence packaging required by
finding4, not a new channel, wrapper, isolation mechanism or larger frame limit.
Atomic retained publication and genuine outer exit0 acceptance remain unchanged.

## Frozen R2 artifacts

Directory: /tmp/ga-tmgr-combined-candidate-r2-20260910/
- source.sha256: a77aa66c4a7dc063988db87b8c97589657b4d1f1073296d9acbe8a3047472e7d
- combined-source.tar: e8f853493384391d54f8eb5a4f77922e9f84a615f02b1484da25aaa98f9ee2c0
- r1-to-r2.diff: 45f9d19e1399bb24ba6b7e1b65f3f88692cc56a1dfb3d0da3703c9af95b3ccf0
- combinedprobe: 0bdd2e29575766786662bf33462dba89943c5257511f78471f40663520601e2b
- execution-plan.json: 23e197d3d503ddf167dd9d0eef8e2b224a978369602c53ceaf23a474e220337f

Source changes only test/combinedprobe/ plus current workflow evidence.
Pure RED/GREEN records: offline-red.txt, offline-green-01/02/03.txt,
observation-size-red.txt. Final count10 pure tests PASS; vet/static build PASS,
with actual commands/exit0 in offline-final.txt, vet.txt and build.txt.
HTTP routing tests use an in-memory recorder, not a listener or process.
Freeze only read/hash-checked runtime/ELF inputs and absent runroot.

R1 source/archive/binary/plan preserved via r1-preservation-final.txt and
r1-source-before.txt; R1 review archived in r1-review-preserved.tar.
M5 source/result/binary, original20 and original index remain exact:
m5-source-preserved.txt, original20-preserved.txt, preserved-inputs.sha256,
index-before.z/index-after.z. R1 and R2 combined runroots remain absent.

## Proposed command — independent review required first

```sh
/tmp/ga-tmgr-combined-candidate-r2-20260910/combinedprobe execute-reviewed /tmp/ga-tmgr-combined-candidate-r2-20260910/execution-plan.json 23e197d3d503ddf167dd9d0eef8e2b224a978369602c53ceaf23a474e220337f
```

Exact absent runroot: /tmp/ga-tmgr-combined-run-r2-20260910.
After genuine execution exit0, separate validate-result targets result.frames
with that plan digest and the independently observed exit. Do not manufacture it.
UID1000, frozen host namespaces/runtime/argv, same120s outer budget and existing
lease/transaction deadlines; new controls have a10s freshness bound.
Only disposable fixtures/listeners and owned-process teardown; all files retained.
No root, service/rig/worker, installed cache/metadata, signing/push/merge or delivery.

## Current blocker / next deliverable

Independent consolidated delta review of findings1–4 and the frozen evidence
encoding/plan; this executor performs no inference or runtime execution.
On further substantive failure after this understood correction, reassess rather
than blindly repeat. No new isolation/wrapper/daemon/policy investigation.
Full real-death/exec-ABA, endpoint substitution, pause/expiry, concurrent reader,
old Core decoder and provider acceptance remain open. No production integration.
Original full goal remains active/incomplete in the parent; ga-oz9e deferred.

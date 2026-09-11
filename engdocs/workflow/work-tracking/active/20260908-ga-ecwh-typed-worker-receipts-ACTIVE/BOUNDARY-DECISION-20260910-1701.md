# ga-tmgr boundary-investigation checkpoint — 2026-09-10 17:01 UTC

The 90-minute investigation checkpoint is reached (start approximately 15:31Z).
The full original goal is active and incomplete. No goal identity or acceptance
criterion is replaced, narrowed or closed. ga-oz9e remains deferred/nonblocking.

## Observed outcome, not source-only inference

M5 R3's exact unprivileged isolation command ran once at UID1000 and returned
genuine exit0; the separate result validator also returned exit0. All13 cache
controls denied inside/succeeded outside, full cache inventory unchanged,
metadata+RO shared-lock positives and owned-process terminal witnesses passed.
See `/tmp/ga-tmgr-m5-runtime-r3-observation-20260910.md`, SHA256
`8b3f4fb752dd23aa0dbcc247a72c204c2b96cb751068cf1d3958b1882cd58242`.
This resolved the physical confinement feasibility question without root or
changing live services. No replay is needed and no alternative namespace or
wrapper investigation is authorized by this checkpoint.

## Current source HOLD

Combined R1 source/tests/build are frozen; no combined runroot was created.
Independent actual Opus5 SOURCE HOLD:
`/tmp/ga-tmgr-combined-independent-review-r1-20260910/verdict.md`, SHA256
`0cf2e5b8ec4d192ff70e14fb4052e57d726b3a1c11296b26d1324a30837dddac`.
Verified completion SHA256
`64928ac2a0bc77846a2ed3455cc4f506445fd28644d9adbb771333b082d8c714`.

Four specific corrections are required before initial transaction execution:
1. Distinguish writer-not-started from actually reaped; bind the latter to real
   wait/process-state and pidfd terminal evidence, not unconditional true.
2. Bind literal version execution to the observed process image and a frozen
   request expected-version field; prove mismatch refusal. The existing code
   really executes a version command; the missing property is independently
   varied expectation and tighter image binding, not absence of execution.
3. Model an actually absent lease route (404) and preserve/validate its exact
   attributed refusal, rather than label a 200 application error absent support.
4. Bind process attack records to the exact fixture identity and refresh positive
   reachability after confined negatives; wrong/stale targets must be inconclusive.

## Explicit mechanism decision

Retain the approved two-role architecture: trusted V observes the real host;
OS-confined W publishes only the fixed metadata set. The reviewer confirms the
host identity, pipe/channel, confinement and irreversible-commit source design;
the four findings concern executable acceptance and reporting, not a new root
capability or a contradictory isolation topology.

End exploratory boundary investigation here. The next bounded implementation
delta is exactly those four source corrections in the same ga-tmgr worktree,
with focused regressions and one independent consolidated delta review. Preserve
R1 and the M5 success; do not execute or relabel R1, relax any checks, add a new
wrapper/daemon/privileged operation, or expand to the nonblocking wishlist. A
further substantive failure after that understood correction requires renewed
mechanism assessment, not blind repetition. Existing source/synthetic authority
covers this correction; no new operator micro-authorization is needed.

The four initial transaction cases do not satisfy the full race/decoder matrix.
Real death/exec ABA, endpoint substitution, pause/expiry across rename, concurrent
readers, old Core decoder compatibility and remaining original acceptance remain
open explicitly. Neither this checkpoint nor M5 implies live adoption or usable
Claude worker parity. All live delivery/activation and signing gates remain.

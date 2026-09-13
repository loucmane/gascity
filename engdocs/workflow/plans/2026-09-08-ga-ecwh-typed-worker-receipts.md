---
session_id: 2026-09-08-001
work_context: ga-ecwh-typed-worker-receipts
handler_target: .
bead_ids: [ga-ecwh]
attached_bead_ids: [ga-ecwh.2, ga-tmgr, ga-ecwh.5]
branch_policy: codex/ga-ecwh-typed-worker-receipts
evidence_summary:
  - engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE
  - .
  - bead:ga-ecwh
  - scripts/codex-task
plan_version: v1
emergency_bypass: false
---

# Plan - Bead ga-ecwh Bind candidate profiles across controller receipt and canary consumers

## Header
- **Session ID (S)**: 2026-09-08-001
- **Work Context (W)**: ga-ecwh-typed-worker-receipts
- **Handler Target (H)**: .
- **Bead IDs**: ga-ecwh
- **Branch Policy**: codex/ga-ecwh-typed-worker-receipts
- **Evidence Summary (E)**: engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE, ., bead:ga-ecwh, scripts/codex-task
- **Plan Version**: v1
- **Emergency Bypass**: false

## Plan Table
| Step ID | Description | Evidence | Status |
|---|---|---|---|
| plan-step-scope | Confirm scope and authority for Bind candidate profiles across controller receipt and canary consumers | engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/FINDINGS.md | completed |
| plan-step-implement | Implement Bind candidate profiles across controller receipt and canary consumers through the reviewed helper surface | .; engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/IMPLEMENTATION.md | pending |
| plan-step-verify | Capture tests, review evidence, and bead readback | engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/HANDOFF.md; engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/TRACKER.md | pending |
| plan-step-emergency | _Optional_ - only if bypass required | Waiver + post-mortem plan | n/a |

## Scope
- `engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE`
- `.`
- `scripts/codex-task`
- `scripts/codex-guard`
- `tests/`
- Primary bead `ga-ecwh`

## Branch Policy
- Working branch: `codex/ga-ecwh-typed-worker-receipts`

## Amendments & Versioning
- 2026-09-08 - Bead `ga-ecwh` kickoff created through the bead-native source workflow.
- 2026-09-08 - Corrected the authored scope row from pending to completed to match the existing checked tracker and verified Core-only authority; implementation and acceptance remain pending. The supported plan-sync validates parity, rather than changing authored statuses. Refusal and correction evidence: `docs/ai/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/reports/policy-canary-checkpoint-20260908.md`.

- Layout relocation: active references now use engdocs/workflow; original plan preserved at `/home/loucmane/gascity/city/rigs/gascity/.git/gas-city-workflow/layout-relocations/f1fec32ea4f45986e5b480458c5c3e692c3386f8af5217f109d2e22ec98e2706/originals/plans/2026-09-08-ga-ecwh-typed-worker-receipts.md` (SHA-256 b52ae5f46db6a599f410b58a6291e5a8a607867067a1b70ecb1970feb967b4a0). No task status changed.

## Continuation & Handoff
- Next owner: loucmane (default)
- Context reload steps:
  1. Read `engdocs/workflow/sessions/current` and this plan.
  2. Read primary bead `ga-ecwh` through the rig-scoped bead surface.
  3. Review `engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/TRACKER.md` before changing implementation.
  4. Run `python3 scripts/codex-task plan sync` after tracker updates.
- Outstanding risks/todos: preserve bead authority and avoid allocating shadow Taskmaster work.

## Conflict & Scope Declaration
- Related plans: none declared at kickoff.
- Guard cross-check: bead-native work must preserve plan/tracker/session compliance.

## Evidence Checklist
- Bead readback and reviewed authority
- Tracker/session entries for implementation progress
- Focused tests and guard evidence

## Emergency Bypass Protocol
- No bypass authorized.

## Attached ga-tmgr M5 Source Checkpoint — 2026-09-10

- Actual operator grant extends direct-source scope to this reviewed repair and independently reviewed unprivileged synthetic tests only. See `engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/M5-SOURCE-GRANT-20260910.md`. Earlier worker-only/source-lane HOLD remains historical; ordinary worker-only policy unchanged elsewhere.
- Supported current-work checkpoint passed without changing dependencies, ownership or workflow code. Erroneous retired-child resume and kickoff-replay refusal remain preserved evidence, not new repair scope.
- New isolated candidate: `test/m5probe/README.md`, pure plan tests and static Linux harness. Baseline and review packet: `/tmp/ga-tmgr-m5-candidate-r1-20260910/`. Pure tests/vet/build only; no namespace or process-attack execution. Independent source review precedes execution; no production integration or goal change.

## Attached ga-tmgr M5 R2 Correction — 2026-09-10

- R1 actual Opus SOURCE HOLD reused; wrapper failure retained, no inference replay. One consolidated correction bounds ptrace wait/detach and atomically publishes retained observation evidence requiring successful outer exit for acceptance. Process-only witnesses, mmap fixture memory and unchanged nonzero-child/FD refusals are explicit.
- Evidence: `engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/M5-SOURCE-R2-20260910.md`; packet `/tmp/ga-tmgr-m5-candidate-r2-20260910/independent-review-packet.md`, SHA256 `945f3f2443b76b76266ac02745fcf332e5d62384533ccb36151853a059be2638`. Complete files and R1 unified delta frozen; focused pure tests/vet/static build PASS only.
- Independent source review remains required before first namespace execution. R1 artifacts, original20 source/index and full goal preserved; no production integration, policy/dependency repair, runtime or goal change.

## Attached ga-tmgr M5 R3 Correction — 2026-09-10

Consumed combined R2 reassessment: `engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/COMBINED-R2-REASSESSMENT-20260910.md`. Reviewed R2 ran once, exit1 at retained Go cgroup FD before V version/lease/W Start; synthetic setup retained, no metadata transaction. Only child-local runtime-setting correction is next; fdInventory and architecture unchanged, no replay or goal change.

R3 source checkpoint: `engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/COMBINED-SOURCE-R3-20260910.md`; exact child-only GODEBUG setting, plan/argv binding, fdInventory byte-identical. Count1 pure tests/vet/build PASS; `/tmp/ga-tmgr-combined-candidate-r3-20260910/` awaits independent delta review, no new execution. R2/M5/original20/index and full acceptance preserved.

17:01 decision: `engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/BOUNDARY-DECISION-20260910-1701.md`. Retain V/W architecture and proven M5 feasibility; combined R1 SOURCE HOLD, never executed. One consolidated correction of findings1–4 only; further substantive failure requires reassessment, not a new wrapper campaign. Full race/decoder/provider acceptance remains open.

Combined R2 correction: `engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/COMBINED-SOURCE-R2-20260910.md`; frozen `/tmp/ga-tmgr-combined-candidate-r2-20260910/`. Real writer terminal evidence, independently expected observed-image version, absent404 attribution and bound/refreshed attack controls; fixed-count result records retain16KiB frame guard. Pure count10 tests/vet/build PASS; independent delta review before execution. R1/M5/original20/index preserved; no goal or runtime change.

Later runtime milestone: `engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/M5-RUNTIME-R3-20260910.md` records parent-only immutable execution and separate validation exit0. Earlier source-only HOLD remains historical. No M5 rerun; proceed only to review-gated combined synthetic source under existing ga-tmgr authority.

Combined R1 source checkpoint: `engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/COMBINED-SOURCE-R1-20260910.md`; frozen `/tmp/ga-tmgr-combined-candidate-r1-20260910/`. Focused pure tests/vet/build PASS; no new runtime test. Independent source review next, decisive full combined proof HOLD with explicit remaining real-death/exec-ABA/old-decoder cases; no expanded wrapper campaign or live authority.

- R2 actual Opus SOURCE HOLD preserved. Only MF8 metadata xattr ordering and MF9 attach-denial provenance corrected; post-attach failures refuse in runtime and outer validation. Existing denial sets, FD boundaries and nonzero-child refusal unchanged.
- Evidence: `engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/M5-SOURCE-R3-20260910.md`; complete frozen source, R2 delta, plan and focused pure test/vet/build evidence: `/tmp/ga-tmgr-m5-candidate-r3-20260910/`. Independent delta review remains required before first namespace execution; no goal or runtime change.

## Initial Combined R3 Milestone and Remaining Outcomes — 2026-09-10

- Later parent-owned initial mechanism PASS: `engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/combined-runtime-r3-observation-20260910.md`, SHA256 `3ab2eedb740117ee99e818594480f3f7a2c7f4fba196e8f0dce235cafe39b4bc`. Execution and separate validator genuinely exited0; neither repeated here. Original Opus HOLD, supplement and independent Astra adjudication are preserved beside this report, not relabeled as Opus PASS.
- Remaining map: `engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/remaining-outcomes-20260910.md`, SHA256 `627d89fc6d409a6087894ce988a474064d5c6bd4dff946314a9b90e31ad7853a`. Six full-matrix groups remain; next smallest chunk extends existing synthetic acceptance, then preserves Core repair/delivery -> gct-13ku/gct-10pg -> useful Claude task -> both handovers. No source/test/runtime/goal change or acceptance closure; ga-oz9e deferred.

## Combined Acceptance R4 Source Checkpoint — 2026-09-10

- Scoped authorized ga-tmgr delta implements six remaining fixture/owning-test groups; pure tests, exact unchanged old Core decoder execution, vet/static builds pass. No R4 process/namespace execution or review inference. Original20/index and every consumed run/review are preserved.
- Review brief: `engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/combined-acceptance-r4-candidate-20260910.md`, SHA256 `a0637591b1db88ab725d61759a7d7d88344505f0773cf4b2d73dd3c0057e276d`; consolidated source/plan/compiler-input evidence at `/tmp/ga-tmgr-combined-candidate-r4-20260910/`. Next: independent exact-candidate review before fresh R4 execution; no full-matrix/live/provider PASS, no production integration, goal change or acceptance closure. Full outcome order and deferred ga-oz9e unchanged.

## R5 M1 Identity Correction — 2026-09-10

- R4 Astra SOURCE HOLD preserved, SHA256 `3e8ef99c65cdf2fe9669e2e2f3b6518c99e5eabc10da779d5385a32d46888355`. Only structured observed identity-mismatch attribution and focused regressions corrected; arbitrary/I/O/cancellation refusals cannot satisfy identity negatives. R4/R5 runroots remain absent.
- Review brief `engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/combined-identity-r5-candidate-20260910.md`, SHA256 `9879e1428f98b5ffbb37b9e2c64c7f919d6817fea6a521117d7cfe77a2d4e66f`; final R5 source/plan/helper evidence `/tmp/ga-tmgr-combined-candidate-r5-20260910/`. Pure tests/vet/static builds PASS; unchanged exact old decoder evidence reused, not rerun/copied. Next independent delta review; no runtime, production, goal or acceptance change. Original20/index, full outcome order and deferred ga-oz9e preserved.

## R6 Listener Exec Ownership — 2026-09-10

- Later R5 runtime exited1 at exec-before, consumed INCONCLUSIVE; parent proved all15 retained PIDs absent. Exact close race remains unproven. R6 retains original fd3 ownership through exec/error cleanup without Dup3; pure regressions/vet/static builds PASS, no runtime or inference.
- Review brief `/home/loucmane/gascity-core-worktrees/ga-ecwh-typed-worker-receipts/engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/combined-exec-r6-candidate-20260910.md`, SHA256 `de1b246bb9290235f8e3a0eb2555471c9db4dddbdee7a27f713c93a1d8d867c6`; packet `/tmp/ga-tmgr-combined-candidate-r6-20260910/`. Detailed R4/R5 Core reports remain authoritative; duplicate /tmp reports are preserved short CLI finals. Original20/index/decoder/prior evidence exact; R4/R6 roots absent, ga-oz9e deferred. Independent exact-source review next; full goal/outcome order unchanged.

## R6 Runtime Milestone and Production Source R1 — 2026-09-10

- R6 decisive22-case synthetic matrix PASS, actual run/validator exit0,45 owned PIDs absent; observation SHA256 ffe89ddabacd4b5798edc708ed6b6f909e6f340c39e7a8d9d82372f8adb5e3e7. Consumed, no replay. Unique supported evidence now appears in Operations session/handoff/tracker; prior applied/no-readback preserved.
- Partial production reader/lease integration implemented with focused tests/vet/build PASS. Detailed Core ACTIVE `production-vw-r1-candidate-20260910.md`, SHA256287afe66609dbe130f99e2bab6e0a5490066e0dbcb315f8dc834d22958f24d6e; artifacts `/tmp/ga-tmgr-production-vw-r1-20260910`. Full host V/confined W/closed channel/versioned irreversible writer and fresh runtime consumer integration remain unfinished/HOLD; no external authority blocker or production acceptance claimed. Original20 baseline/index/evidence preserved; full goal/outcome order incomplete, ga-oz9e deferred.

## Production V/W Continuation3 — 2026-09-10

- Callable host verifier/lease, confined writer, bounded private protocol, irreversible v2 receipt and fresh v2 dispatch/canary source are implemented. Focused owning/API/CLI tests, vet/static build PASS; independent source and production-path runtime acceptance remain HOLD. Legacy dispatch freshness widening was refused by auto-review and not bypassed.
- Evidence: `/tmp/ga-tmgr-production-vw-continuation3-candidate-20260910.md`, SHA256 `257429abac5b12f45ce5023c220ea28c455effd2c3139b38999c5f4936ba84f5`; archive `/tmp/ga-tmgr-production-vw-continuation3-20260910/source.tar`, SHA256 `faca36ed31e95ebe014306594fdc5da17ea53c124972cdc229d6956e8d4d5feb`. One owned ga-tmgr note plus supported Core/Operations session/handoff/tracker logs recorded this checkpoint. Original20/index/evidence, full goal/outcome order and deferred ga-oz9e preserved; no runtime or delivery action.

## Production V/W Continuation4 — 2026-09-10

- M1 exact supported preimage mode/GID refusal and M3 observation/mutation composition corrected; focused owning/handler/parity/race tests, vet/static build PASS. M2 listener-holder completeness remains structural SOURCE HOLD: unprivileged FD inspection cannot exclude credential-hidden inherited holders; no scanner exclusions/capabilities were added. One operator decision on trusted closed listener provenance is required before runtime acceptance.
- Evidence: `/tmp/ga-tmgr-production-vw-continuation4-candidate-20260910.md`, SHA256 `1b1d93148df67bbfa9cdf71e77429495da9f0e616d464de13e8cd5be04f5e175`; archive `/tmp/ga-tmgr-production-vw-continuation4-20260910/source.tar`, SHA256 `fb8a5ab7dc9fef39da2382884be12f207e2172f69bbf1124eb088e50a0604000`. Continuation3/index/original20 preserved, legacy dispatch unchanged and not a blocker, ga-oz9e deferred, full goal/order unchanged. Independent delta review next; no runtime, lifecycle or delivery authority exercised.

## Production V/W Continuation5 — 2026-09-10

- Single M3 correction refuses automatic v2 replay/NOOP where outer completion is unavailable, even with matching persisted FINAL. Persistent-state RED/GREEN and one owning test/vet/static-build pass; no completed-idempotence or runtime claim. M1/observation work and legacy behavior preserved.
- Candidate `/tmp/ga-tmgr-production-vw-continuation5-candidate-20260910.md`, SHA256 `8b2f0bcd5cffd25ecd1ec0a3b3c84491dc68391f07403cab4726bcbb7bbb8495`; source archive `/tmp/ga-tmgr-production-vw-continuation5-20260910/source.tar`, SHA256 `d157e8b9696b02763b6f1e2a1507a6fce7ad4e16c6583bf926bc1e5821410f7a`.
- Review `/tmp/ga-tmgr-production-vw-continuation4-astra-review-20260910.md` and adjudication `/tmp/ga-tmgr-production-vw-m2-architecture-adjudication-20260910.md` are bound in the candidate. M2 connection-bound responder PLUS accepted-socket custody remains only an unapproved operator proposal; no scanner/transport change. Original index/attempts/full goal/order and deferred ga-oz9e unchanged. Independent delta review next; no runtime or lifecycle.


## Production V/W Continuation6 — 2026-09-11

- Operator-approved M2 retained-connection plus accepted-socket-custody source candidate implemented; continuation5 replay delta independently passed. M1/M3/replay and all non-M2 production seams remain exact. Custody audit is conditional, not overall SOURCE PASS; fresh runtime acceptance remains HOLD.
- Candidate `/tmp/ga-tmgr-production-vw-continuation6-candidate-20260911.md`, SHA256 `04681c44bdbdc2062c90af3bfc5d973fa8cfd107e7931565a6addcb1d957cd7c`; frozen source manifest `0235dcfe18ab368f753f12197bca300685df92b4d9861a8fac509490012a199f`, archive `80e82b06b51864549e62cc7ba6cd4273983d787f6988023e7b04777ceaaa01c9`. Pure owning tests/race/vet/static build pass; binary unexecuted. Fixed-length framing retained after rejected chunked extension; no bypass or retry.
- Supported daily continuation preserves September10 history. One owned ga-tmgr note plus current Core/Operations log/readbacks bind this checkpoint. Original index/attempts/full goal/order and ga-oz9e deferred unchanged; next independent exact-source review, no namespace/probe/lifecycle/delivery.


## Production V/W Validation Preparation — 2026-09-11

- Frozen continuation6 now has independent Astra SOURCE PASS (`1ffe64114e52bad84d8ef9808fc3d2e5e10a7b955caaccc3dadbc0eded9cc7f6`), recorded append-forward; all61 source entries/index remain unchanged, with no execution or tests this turn.
- One package: `/tmp/ga-tmgr-production-vw-runtime-package-20260911.md`, SHA256 `c492e909b0f42c49a09654984fc140ce581583858fa8d39a16d19eb7b71c3d27`. Existing CLI can isolate GC_HOME but lacks a reviewed same-image controlled acceptance fixture for shared-listener and phase-fault cases; package not executable, no speculative permission request or new mechanism.
- Result `/tmp/ga-tmgr-production-vw-validation-preparation-result-20260911.md`, SHA256 `ba19729bcad1acd231304706c97ecd6ee2f199ec9f9e36b0483b6d7d940c8fa0`. Owed current-day findings/decisions use supported audit updates; historical September10 date HOLD remains, no staging/backdating/guard repair. Runtime/delivery/full goal remain incomplete; ga-oz9e deferred.

## Kernel Components / Ordinary Fixture Candidate — 2026-09-11

- Independent compositional ruling supersedes the same-image fault-injection requirement only: actual unchanged-image positive flow plus separate package-local kernel components; no production mode or guard change.
- `/tmp/ga-tmgr-kernel-fixture-source-report-20260911.md`; candidate `/tmp/ga-tmgr-kernel-fixture-source-20260911/candidate.tar`, SHA256 `c7bf48095090feff73a01feab09c7a6fb74a46e1527eb5386a53c657141d6162`. One new opt-in test source, exact ordinary fake-session/file-store config and bounded launch/capture plan. Offline compile/vet PASS; no runtime or suites executed.
- All61 source pins/production binary/index preserved. Independent package review is next; component runtime and actual ordinary input-bound positive flow remain unproven. Historical Operations date HOLD preserved; full goal and ga-oz9e deferred unchanged.

## Completed Bounded Runtime / Build-Contract HOLD — 2026-09-11

- Later evidence supersedes only the pending runtime observations: kernel component PASS and ordinary real CityState startup/teardown PASS, each executed once by parent; no adoption, full manifest closure or provider parity PASS. Original source-only entries remain historical.
- Evidence `/tmp/ga-tmgr-runtime-recording-20260911/record.md`, SHA256 `4a85c9ea3942aa84c96d78e78c66a49677f9750a9bd2728659b15a70d7377503`; exact input reports preserved in Core ACTIVE. Independent build-contract review `1186f587457a7d9f49bb70dc5d4d9b28500480d811e1b590d0b4207d831f2e1d` confirms dev/unknown versus mandatory hexadecimal commit and seven-setting custody conflict.
- One exact metadata-only main.commit stamp extension is a recommendation, NOT granted source/build/runtime authority. Missing time namespace requires future fresh capture, not retroactive completion. No replay/source/build/adoption here; all61/index and evidence preserved. Historical Operations date HOLD remains separate; full goal incomplete, ga-oz9e deferred.

## Corrected Build Identity / Local Parity HOLD — 2026-09-11

- Later approved correction supersedes the proposed custody-source extension: pinned Go trimpath omits ldflags from BuildInfo; retain custody source and bind one exact main.commit stamp to a real reviewed candidate commit.
- Read-only signing readiness PASS; normal formatter preflight exits0 but changes10 frozen files. No formatter output applied, staging/commit/build attempted or old base falsely stamped. Exact prospective delta SHA256 `8469e50cd48cb00ba6b699473555be5585b6573a96e3bf0a34ba21fe98b1a9bd` awaits independent disposition before exact-source local delivery.
- Report `/tmp/ga-tmgr-build-identity-result-20260911.md`, SHA256 `4d4a18becdda193a916f81d718138ef28c8728f9bf173bb08efc59e6bb5c2a4b`. Source61/kernel/index/binaries preserved; current full goal remains incomplete, ga-oz9e deferred. Separate Operations date HOLD not rerun.

## Reviewed Formatting Applied / Excluded Hook Action — 2026-09-11

- Independent Astra PASS covers exactly ten style-only outputs; applied byte-identically, remaining52 original entries/custody pins unchanged. Focused pure platforminstall/packman tests and diff check PASS; no old runtime evidence relabeled.
- Formatted62-source manifest `81ca21c2035201b7de4aee8f7580690eba5b773552117e654da5b3fbe90fa7bb`, archive `3dcb023eeaf0f1292847fb41dc9b1604e1927982acc478b6c7d012f19e883f9a`; report `/tmp/ga-tmgr-formatted-build-result-20260911.md`, SHA256 `d3073ddaba05c1fa93ed6daa73e4cfa178a38023f1c33580fd3ee8d4b2c4ba50`.
- Local signed checkpoint/build HOLD before staging: normal pre-commit:67 requires in-worktree npm ci, excluded outside disposable /tmp; :76 also requires dashboard smoke listener. Neither executed or bypassed. Resolve exact normal-hook authority boundary next; index/base/evidence and full goal preserved, ga-oz9e deferred.

## Consolidated Lint Candidate / Scoped Hook Authority — 2026-09-11

- Operator approval now permits lockfile-pinned in-worktree npm ci and bounded temporary loopback dashboard smoke for the normal hook; this resolves that earlier HOLD prospectively, not global installation or production runtime authority.
- Reviewed lint guidance implemented in17 files: joined cleanup errors, final classification after outer cleanup, once-only pipes and guaranteed child reaping, plus mechanical findings. Focused source-order RED and cleanup/publication GREEN preserved; final no-fix lint0 issues and ordinary owning regressions PASS. No new runtime/build/signing/staging.
- Report `/tmp/ga-tmgr-lint-candidate-result-20260911.md`, SHA256 `a8acb84a4d778262e4edea2df71cb7ce196f7b2826452ae07a891d4800c232f9`; full64 manifest `eca556b37e8a86a8dff6b95d729ed8867edabbd75c2c3b71339002b013cb79c4`, archive `31b970c62e01f9c1fd8f3d811fe7bc9b2d418f78216a8b5b287bc1bc8464eadb`. Independent consolidated delta review next, then authorized normal hooks/signed commit/stamped build. Original index, custody settings, history, full goal and ga-oz9e deferred preserved.

## Signed Local Source / Offline Build — 2026-09-11

- Later independent SOURCE PASS plus reviewed diagnostic/pin correction led to normal signed commit `5a74ab60da46e55ba6425839a3819e9a185c1803`, signature G; tree `d7260990f64095c871f948bd3e77353492dc7f19`. Normal lint/vet/generation/docs/dashboard hooks including authorized npm and loopback smoke passed.
- Offline image SHA256 `64bdb174ed79719195f38d00877f44ce7182a8cc5b42247bc7ac5852ef37c8b4` binds that exact revision; unchanged seven custody settings. Report `/tmp/ga-tmgr-normal-local-delivery-result-20260911.md`, SHA256 `369043f997832245d029283024314272277ccfbfff8c002ebe240453b7ce55c1`.
- Historical index/source/runtime evidence preserved. Independent final candidate/build review, final/full suites and hosted CI remain; no publication/live acceptance, goal change or ga-oz9e activation.

## Full Positive R3 Reconciliation — 2026-09-11

- R2 HOLD and interrupted prospective persistence claim preserved; R3 one-field pin-name correction independently Astra PASS with22 recorded pure tests, not runtime acceptance.
- Evidence: /tmp/ga-tmgr-r3-checkpoint-reconciliation-evidence-20260911.md. Package1c09625e7f257136e434371e9579c0daeabb73bbafaffa0baac32b1d01faae90; signed source/image unchanged, R3 runroot absent. Separate execution authority remains; full goal active/incomplete and ga-oz9e deferred.

## Full Positive R3 Runtime FAILED / Consumed — 2026-09-11

- Parent terminal run88764 exit1: finalizer0, apply1 without interruption; no transaction output. Machine result14a196d2b7d087065168d13d4cb342618cb8850b30518f092237ed4b3bea6d31 remains unknown transaction/failed fixture; no replay or retry authorized.
- /tmp/ga-tmgr-r3-runtime-recording-evidence-20260911.md links exact observation and machine evidence. Named outer PID absence/closed port does not prove protected-W subtree absence. Diagnostic suppression remains an unproven hypothesis. Next is explicit disposition or bounded independently reviewed diagnostic repair, not automatic execution. Full objective preserved; ga-oz9e deferred.

## Narrow Exit Diagnostic Source Candidate — 2026-09-11

- Approved source-only distinction now matches concrete GC commandExitError, not foreign ExitCode errors. One production line, focused tests, one exact main.go custody pin; no V/W or exit-code/JSON contract change. Observed RED then8 focused GREEN tests/22 subtests; no process fixture, build or runtime retry.
- Review packet /tmp/ga-tmgr-exit-diagnostic-repair-20260911/REVIEW.md SHA256e7e2d8930bd9041c0e95ecf670fa9f124f40ce2e91cafff6a0dd723fdda22cbd; source manifestc11ddb84e6c388d114c4bed76300d4edebd9e6e8be0e2618c1ccd94b27f894ec. R3 consumed/unknown and all prior evidence preserved; independent exact-source review next. Uncommitted; no delivery/live/goal change, ga-oz9e deferred.

## Diagnostic Signed Local Delivery — 2026-09-11

- Later independent SOURCE PASS followed by unchanged normal hooks and signed commit f1ea84d76fce1d17626349353f758eb592386ee3/tree7d42a6b1b30152201c9488dcb30b06d63d846169, signature G. Exactly3 reviewed files; no generated/source drift or unrelated staging.
- Exact-source offline imagee965252f15f12ed39955aa3ea8802ea5ccb8c6d1aee4ccbd29de866286968499 reports the new commit in bounded version JSON. /tmp/ga-tmgr-exit-diagnostic-delivery-20260911/REPORT.md SHA256f92cd50d5a95114f702e675ddf0e0d014b320908e2b51608afa36a6c2dd7fa8b. Source92 and full build inputs frozen; old image/R3/history preserved. Public delivery requires full/pre-push/hosted CI and existing review/authority gates; no runtime retry, live adoption or goal completion, ga-oz9e deferred.

## Reviewed R4 Proposal — 2026-09-11

- Independent local-build PASS61aa709c and unexecuted R4 proposal PASSf6b88aee recorded in /tmp/ga-tmgr-r4-proposal-recording-20260911/REPORT.md. Package2f27d56d06637e5a2197437580f59d73a842c30b0f299e134bfcd32ebc4f950a; parent22 pure tests/normalized AST evidence reused, no execution.
- R4 root absent; exact operator execution authority remains absent. R3 consumed FAILED/UNKNOWN and protected-W absence unproven; fresh identities do not authorize retry. No new source prerequisite or investigation; full objective and ga-oz9e deferred preserved.

## R4 Terminal Failure — 2026-09-11

- Later exact once-only authorization/run53824 ended exit1: pinned bwrap rejects --preserve-fds; separate fast-exit proc enrollment failure marks apply interrupted. R4 consumed FAILED/UNKNOWN, no W cleanup witness; no rollback/no-mutation or protected-W absence claim. R3 cause remains unproven.
- /tmp/ga-tmgr-r4-failure-recording-20260911/REPORT.md binds actual resulta133fe9317c384ddf379dc7400db91d9963b3186f4f72ec4c4b29d7e35478507 and independent disposition29fc515d. Stop after failure; next reviewed launch-contract compatibility decision, no repair/retry here. Source/full objective/history unchanged, ga-oz9e deferred.


## Bwrap FD Compatibility Source Candidate — 2026-09-11

- New approved source-only ga-tmgr repair removes only unsupported --preserve-fds 2, preserving anonymous FDs3/4 and every confinement/transaction check. Existing exact-token argv owner RED then GREEN; owning package115 tests/137 subtests and vet PASS. No runtime/build/delivery; R3/R4 remain consumed failures.
- /tmp/ga-tmgr-bwrap-fd-compatibility-20260911/executor/REPORT.md and source.sha256 bind two-file candidate, preservation and independent-review boundary. Source manifest SHA256 a4b84ae6491727c6a2ee7e360346835e8ed77f4c7a95b9d2db826990d7de41eb. Full objective unchanged; ga-oz9e deferred.

## Protected R4 Observer Source Repair — 2026-09-11

- Later delivery and consumed protected-R4 disposition are preserved in the active handoff and reports; PR38 is already merged. R4 remains FAILED / committed_evidence_incomplete, result 4f2e0110dc1d95049c1d07c225443b19177234d25d652f4029c5f5f2131a920e. No prior attempt is replayed or reclassified.
- Approved observer-only correction: same retained pidfd terminal proof and exact-Popen reap after an enrolled process exits. Initial identity, ownership, transaction, cache/protected, transcript and cleanup checks remain. Source605be0d2dab5dc8e296a4a039c083c5387174dbfcf0b2ff0031bbbe55504e207; deterministic RED preserved, focused19 and full48 PASS, independent Astra full48 SOURCE PASS with no must-fix.
- Checkpoint: engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/reports/20260911-observer-terminal-source-repair.md, SHA256 2de04d35564976a5f3fefa93e5f96ff7028183b78a3a33c973a76b0eeee217e6. Candidate at /tmp/ga-tmgr-observer-terminal-repair-20260911 is source-only, not a runnable successor package.
- Next: fresh bound/reviewed synthetic validation package and separately authorized execution, not another Core source rebuild. Live adoption, useful Claude execution and bidirectional handover acceptance still remain. No runtime, worker or service transition here; ga-tmgr remains open, ga-oz9e deferred, original full objective/history unchanged.

## Protected R5 Package Preparation — 2026-09-11

- Fresh package d5e7458afbb346716e90e50268b6b79e676a39f3d2fce109342509b095d1cac9 independently PACKAGE PASS; controller13987056a0cb10c40c546bc8cf8cce5a7ac2e59db894943fd4d908806875eb15 is byte-equivalent to the reviewed observer repair after the exact closed path/label rebinding. Unchanged R4 build/runtime pins; inherited29 pure tests PASS and observer evidence reused honestly.
- Checkpoint reports/20260911-protected-r5-package-preparation.md in the ACTIVE tracker, SHA25687f19a3acec38769b35bcd1b8e5f32a2ab00618a2813cd67a9363efc0e5fa1d4. R5 runroot absent; production readback PID1769/restarts0/start210566291 and four rigs suspended. R3/R4 consumed outcomes preserved unchanged.
- Exact execution authority remains absent. Next is one separately authorized R5 synthetic validation after immediate preflight; no live adoption, hostile-kernel acceptance, provider-parity or original-goal completion is implied by package review.

## Standing Autonomous Completion Grant — 2026-09-11

- The later explicit operator grant supersedes the prior per-attempt/source-only/separate-execution-approval restrictions within the full existing goal. Historical package and plan text above remains preserved; it no longer requires another operator confirmation for an in-scope reviewed successor or successful checkpoint.
- Verbatim grant: reports/20260911-standing-completion-authorization.md in this ACTIVE tracker, mirrored at /tmp/ga-tmgr-standing-completion-authorization-20260911.md. R5 package d5e7458afbb346716e90e50268b6b79e676a39f3d2fce109342509b095d1cac9 may execute once after immediate preflight. Independent review, exact bindings and every technical/containment gate remain.
- Continue outcome order: actual protected positive and necessary negative proofs; reviewed merge-bound metadata reconciliation; preserved gct-13ku/gct-10pg interoperability; useful Claude worker execution; bidirectional handover and original terminal acceptance. Reuse PR37/38 and valid source evidence; ga-oz9e remains deferred/nonblocking.
- Failed synthetic attempts remain consumed and may only have fresh successors after proven isolated containment, terminated owned processes and unchanged production/security state. Live retry requires pre-mutation proof or verified rollback. All excluded boundaries in the grant remain stops. No new goal, task database or execution context.

## R5 Corrective Source Acceptance — 2026-09-12

- R5 remains consumed FAILED / committed_evidence_incomplete; later separate containment passed, without rewriting the result. R3/R4/R5 roots and evidence remain preserved.
- Independent Astra SOURCE PASS binds the common-epoch mutation-budget correction and bounded same-pidfd observer completion. Exact source hashes, semantic RED/GREEN, owning-package evidence and 54 independent observer tests are recorded in reports/20260912-budget-observer-source-pass.md under this ACTIVE tracker.
- Supported daily continuation moved evidence to September12 while retaining September11 history. Normal signed delivery and required hosted gates precede a fresh independently reviewed source/image-bound synthetic package under the standing grant; no repeated micro-authorization or replay of a consumed root.
- Hostile-negative proof, reviewed live adoption, worker capabilities/useful delivery, bidirectional handover and original terminal acceptance remain owed. ga-oz9e stays deferred; protected projects and unrelated rigs/services remain untouched.

## Usability-first outcome checkpoint — 2026-09-13

- Later verified outcomes supersede the pending descriptions above without
  altering their history: Core delivery and live metadata adoption are complete;
  gct-13ku PR59 and gct-10pg PR60 are delivered and CLOSED source PASS. Do not
  repeat those deliveries, adoption or consumed worker attempts.
- PR60 merge7186568953d498e11b380543f669fa864536014f preserves exact reviewed
  treeff6521097b5108e73cae483605d23a018e082be7; corrected hosted34724445410
  passed in5m28s with the original time budget. Final independent710-test clone
  and159 mandatory cases, source review, native close/restoration and merge
  gates are bound by reports/20260912-native-metadata-adoption-pass.md.
- Next source task is gct-2tbg, freshly created/read back OPEN, unassigned and
  unrouted in the Template store after the scoped duplicate sweep. Contract
  /tmp/gct-claude-signing-followup-20260913/contract.md,
  SHA256362d26890082f661e78f0c24f7511d43b2eba575eac4daef1a4a2a97ad2e249e.
  Independent design PASS requires exact effective settings and durable
  subscription-only launch; a policy file alone is insufficient. Use a native
  Gas City implementation worker after binding/reviewing its exact source and
  launch package. Reuse existing Core typed consumers and gascity-core signer;
  no new Core/signer/root prerequisite is currently indicated.
- Source scope is the missing Claude signing-policy renderer/profile and narrow
  subscription launch integration with tests. Preserve existing candidate and
  legacy profile bytes. Do not reopen failed ga-vh6 or ga-43s as successes;
  historical Opus/Haiku proofs are reused only for their demonstrated scope.
- Still owed: merged-profile activation and current native capability/signing
  receipt; one useful Fable-orchestrated task through Gas City; both handover
  directions preserving Bead/worktree/staged/unstaged/untracked work; negative
  permission and zero-residue acceptance; terminal evidence/eligible closeouts.
- Continuity snapshot's preserved-recovery-record parser refusal is a reporting
  finding, not a new execution prerequisite. Use authoritative live Beads and
  this linked evidence; ga-oz9e remains deferred and nonblocking.
- SAME full goal remains ACTIVE, with unchanged historical identities, runtime
  and standing grant. No new operator micro-authorization is needed within
  scope; all genuine excluded boundaries and technical review gates remain.

### 2026-09-13 — native source checkpoint, append-forward

- gct-2tbg native session ci-c9xl produced its candidate; focused255tests pass,
  but independent SOURCE HOLD requires source-only Python execution, hermetic
  mandatory integration tests and a current Python pin. No source delivery or
  live Claude acceptance has occurred. Preserve this candidate and its failures.
- Native session is closed, claim released, all rigs held, exact baseline
  restored/reloaded and zero native/test residue proven. The separate source
  inventory gate remains honestly held on9generated artifacts; do not delete
  them, relabel them as initial startup, or call the combined package PASS.
- Next action is a narrow native corrective continuation under the existing
  standing grant, protecting the complete prior inventory and using fresh
  disposable test fixtures. Reuse completed proof; do not repeat Core delivery,
  metadata adoption, gct-13ku/gct-10pg delivery or prior launch operations.
- Evidence/next-scope: reports/20260912-native-metadata-adoption-pass.md and
  /tmp/gct-2tbg-native-source-20260913/independent-source-review.md. Same full
  goal ACTIVE; deferred workflow improvements remain deferred/nonblocking.

### 2026-09-13 — corrective native execution in progress

- Same gct-2tbg, fresh native ci-1l32, exact base7186568 in the registered
  gct-2tbg-claude-signing-correction-r1 worktree. Package R2 ba025d44 passed
  independent review; it verifies the existing route instead of re-slinging.
- Actual pre-edit claim/workspace, positive/negative native permission and
  preserved-candidate checks passed. One supported source release completed;
  only the three reviewed source corrections are assigned. Unchanged window
  ends2026-09-13T01:46:50.008121Z. Other rigs and service epochs are unchanged.
- Two incidental Python caches are preserved and independently attributed;
  no false zero-write claim. Source-only coordinator execution and fresh test
  fixtures prevent reuse. Full evidence is in the linked adoption report and
  /tmp/gct-2tbg-native-correction-r1-20260913.
- Next: candidate/focused tests, independent review, supported closeout and
  exact restoration, final full/mandatory tests and normal signed CI delivery.
  Actual Claude signing/useful-work and both handovers remain owed. SAME goal
  remains ACTIVE; no new approval or deferred-redesign prerequisite is added.

### 2026-09-13 — source delivery checkpoint: PR61

- Native corrective candidate997c6a4e received independent SOURCE_PASS;
  supported worker closeout and exact baseline restoration prove all rigs held,
  zero residue and unchanged service epochs. The50ms status-read refusal was
  preserved and resolved by fresh complete observation, never weaker checks.
- Final offline standalone clone:728PASS/0skip, all159 mandatory identities
  passed once, full source/evidence and host non-interference. Validation
  97e270862638b0df1507c0e561c5131bb0b5753b1ed5d8e4396424ff2e189bc8.
- Signed head a87aa87b522acd31c8477ea9869a054e3b31ade6, exact tree
  e48ca6478a96dd913333c68543b3bd7f54477624, draftPR61 on base7186568953.
  Hosted CI and all exact merge gates remain required. Keep all evidence and
  branches; no source or live-acceptance Bead is prematurely closed.
- After gated delivery, continue merge-bound actual Claude capability/signing,
  useful Fable-orchestrated worker execution and both handovers. SAME full goal
  stays ACTIVE; no new operator approval at this checkpoint.

### 2026-09-13 — PR61 delivered; source milestone closed

- Hosted run34731533545 passed in6m47 under the existing10-minute budget.
  PR61 merged5797f19cfeedc2f33e2dfd7efe7a079115a05268 after fresh exact
  signed-head/base, CLEAN/MERGEABLE and zero-unresolved-thread checks; merge
  tree e48ca6478a96dd913333c68543b3bd7f54477624 is byte-identical to the
  reviewed signed candidate. Branch, source worktrees and evidence preserved.
- Independent Astra delivery review confirmed gct-2tbg CLOSE-ELIGIBLE
  SOURCE_PASS. Supported close completed once; readback proves CLOSED,
  unassigned, acceptance/metadata/dependencies unchanged. This does not close
  the full goal or count as actual Claude signing/useful-work acceptance.
- Next: bind the merged launcher/profile to the existing reviewed provisioning
  path, then actual native Claude capability/signing and useful Fable-led task.
  The canonical Template checkout remains clean but parked at dcc29da1;
  provider commands name that canonical root. Inspect the supported source
  activation path before any adoption; do not silently change its branch,
  use arbitrary nested-worktree defaults, or replay completed source delivery.
- Both handovers, live negative-permission/zero-residue acceptance and terminal
  evidence remain OPEN. Same full goal ACTIVE, same standing grant and stop
  conditions. No new approval or deferred-redesign prerequisite is introduced.

### 2026-09-13 — canonical Template source activation PASS

- Existing canonical Template source is now detached at reviewed PR61 merge
  5797f19cfeedc2f33e2dfd7efe7a079115a05268, tree
  e48ca6478a96dd913333c68543b3bd7f54477624. All219 tracked source files
  match; the parked branch remains at dcc29da1 and4030 ignored files are
  preserved. This is not a claim that the whole checkout is clean.
- Independently reviewed source-only activation completed once with unchanged
  city configuration, provisioning receipt, supervisor/signer epochs and all
  rigs suspended. No worker or inference launched. Result
  /tmp/ga-ecwh-claude-live-20260913/source-activation-result.json,
  SHA256 e2c6f3c44d8de2196b5dcb25a4ef1b2785791f46fa061ad241ecc2530a4741ab.
  Preserve the pre-mutation GIT_PAGER refusal and focused publication-failure
  RED/GREEN proof; no completed activation or source delivery is replayed.
- Live acceptance is recorded as ga-ecwh.3, OPEN/unassigned, related to this
  primary. Next: independently reviewed existing renderer/provisioner and
  metadata reconciliation, then actual Claude signing/useful execution and
  both handovers. Reuse the reviewed Core build/adoption/interoperability
  evidence. The full goal remains ACTIVE; standing grant08f86cff applies.

### 2026-09-13 — prospective profile prepared; observation mechanism reassessed

- Exact canonical source activation is complete and must not be repeated.
  Prepared combined registry/fragment replaces only Core's entry, preserving
  Blog; independent review confirms existing Core acceptance evidence reusable
  and no additional platform metadata adoption needed for these unpinned paths.
- Non-live observer R1 failed before build completion; corrected R2 built but
  its copied city omitted convention-discovered agents, causing a BEFORE
  refusal. Both attempts and containment evidence are preserved. No provider
  launch, live configuration write, lifecycle or protected-project edit occurred.
- Reassess complete read-only actual-city observation with a private cache
  and process-local prospective fragment overlay before another execution;
  independent review is required. Do not repair Blog or weaken its checks.
- Exact report and next action: tracker reports/
  20260913-claude-live-profile-preparation.md. Same full goal ACTIVE;
  ga-ecwh.3 live acceptance remains OPEN. No new micro-authorization gate.

### 2026-09-13 — actual composition observed; complete dispatch contract next

- R3 proved actual initial/resume composition and unchanged host inputs, but
  failed overall parity because named providers synthesize aliases in every
  namespace. Preserve its evidence; no live adoption or provider launch occurred.
- Next package must preserve all old unrelated profiles, explicitly contain
  new generic aliases, and bind canonical-path config.Revision. Do not erase
  these differences or change protected work to make assertions pass.
- Correction: dispatch (unlike fragment-only integrity) requires current
  Template and signing-provider platform pins. Reuse existing native metadata
  adoption; independently review the complete supported config/adoption/receipt
  ordering before execution. Do not use stale receipt commits or repeat Core
  delivery. ga-ecwh.3 and the same full goal remain ACTIVE/open as applicable.

### 2026-09-13 — R4 prospective configuration PASS

- Independently verified actual-path observer result41ee2e86:97 old unrelated
  profiles unchanged, five explicit suspended additions, exact Claude initial/
  resume arrays and revision9f3dae18, complete owned-process terminality and
  unchanged host inputs. No provider, signing or live adoption occurred.
- Do not repeat this proof or source delivery. Next is one consolidated reviewed
  existing-surface config/metadata/provisioning transaction, retaining all prior
  integrity obligations and backups; then actual useful execution and both
  handovers. Exact sequence/hashes: /tmp/ga-ecwh-claude-live-20260913/
  next-supported-sequence.md. Same full goal ACTIVE; no new operator approval.

### 2026-09-13 — reviewed installation package and host preflight ready

- Reuse completed source delivery and R4 composition. Independent Astra passed
  the bounded live config/native-metadata package, including strict rollback and
  timer handling. Preserve initial tree-pin and private-source-mode refusals;
  corrected R2 keeps source0600 and installed0644, with no weakened check.
- Frozen bindings724d90ab and final manifest SELF4169a508 are under
  /tmp/ga-ecwh-claude-live-20260913/live-install-r1.22focused mocked tests passed;
  fresh genuine-host preflight passed with no live mutation, stable Core/signer,
  all rigs held, zero sessions and unchanged protected/cache state.
- Next: reviewed config stage, independently reviewed native observation,
  metadata-only apply/FINAL9, then truthful typed receipt and actual Claude
  signing/useful execution plus both handovers. ga-ecwh.3 remains OPEN and the
  original full goal ACTIVE. No new authorization gate or completed-work replay.

### 2026-09-13 — failed installation window contained and timer restored

- Timer stop completed; full runtime postflight returned partial, so config,
  metadata, provisioning and workers did not start. Preserve the original
  consumed pause and failed read-only reconciliation, never replay or relabel.
- The reconciliation found exactly two cached .git directory timestamps had
  changed after the supported workflow log. All other fields/content match;
  preserve attribution uncertainty and exact deltaee35edbb. Do not relax the
  check or rewrite historical timestamps.
- Independently reviewed timer-only recovery6d69ed3a completed with full host
  pre/postconditions, terminal owned process and restored active/enabled timer;
  completion17fc0482. Core/signer epochs and held rigs remain unchanged. No
  source/config/native install or worker claim occurred.
- Next window must finish durable logging before freezing its immediate
  baseline, retain full runtime/cache proofs, use fresh append-forward phase
  evidence and reuse completed source/R4 proof. Config/adoption currently HOLD;
  same original goal ACTIVE, no new operator authorization needed. Exact report:
  reports/20260913-claude-live-profile-preparation.md in the active tracker.

### 2026-09-13 — R8 preserved/restored; ga-ecwh.5 source repair

- Subsequent R8 reached Claude's workspace-trust menu but failed before inference,
  claim, source edits or signing. Preserve all 78 failed native session records
  and the complete R8 evidence; do not replay the consumed window. Exact baseline
  restoration has independent RESTORATION_PASS under
  /tmp/ga-5ot6-native-subscription-live-r8-20260913/.
- ga-ecwh.5 is attached to this existing workflow under the shared external
  coordinator ownership. The operator authorized only the narrow direct Core
  bootstrap exception for explicit Claude trust selection and opt-in durable
  task/store/template-scoped native attempt admission. Authorization digest:
  dd89fe48b6bcede2ea7500b13c2fc303ba7c928723542f432c81a6990c0cb65c.
- Independent Astra SOURCE_PASS binds the 31-file candidate corpus
  44c8ca11985abb56c1a2d6405d49241b306a973aa77a90ca30e3b74b9df5728d.
  Preserve findings and corrections, including genuine configured work-store
  lookup, resolved city identity, no-refund failed creation/start, and unchanged
  denial/cleanup assertions. Exact mapping and focused-test evidence are in
  reports/20260913-ga-ecwh.5-independent-source-review.json.
- The first final fast sweep remains FAIL: five synthetic Dolt leaks and a
  missing testenv import were caught by existing guards. Only fixture setup and
  the new package's standard environment scrubber were corrected; no guard was
  relaxed. Corrected focused cases pass. Final fast regression is still running;
  its logs are preserved at /tmp/ga-ecwh.5-final-fast-20260913/.
- Source review, lint, vet, workflow verification and dashboard byte-parity are
  delivery evidence, not live capability acceptance. Next: final regression,
  signed/required-green-CI delivery and merge-bound adoption/proof. No worker retry
  or rig/service transition occurred during this source repair. Keep the same
  full goal active; useful Claude execution and both handovers remain required.

### 2026-09-13 — final ga-ecwh.5 source validation PASS

- The final sharded fast baseline passed all ten jobs, including all six CLI
  shards; full logs are preserved at /tmp/ga-ecwh.5-final-fast-20260913/.
  Lint, final vet, dashboard build/typecheck/client/byte-parity and supported
  workflow verification passed. Review corpus44c8ca11 remains exact.
- Final record: reports/20260913-ga-ecwh.5-final-validation.md. Prior failed
  sweeps and R8 remain unchanged. Genuine-host observation confirms the five
  temporary test PIDs absent; systemd signer epoch1758/210542077/restarts0 and
  Core PID400145 remain as observed. No worker or rig transition occurred.
- Next: signed/required-green-CI source delivery, then reviewed merge-bound
  adoption and actual useful-worker/handover acceptance. Source PASS does not
  close ga-ecwh.5 or the full original goal. No fresh operator approval gate.

### 2026-09-13 — committed-source guard integration correction

- Preserved signed head791c4c7 and its failed pre-push. Remote refs stayed
  unchanged. The resource census missed an untracked new helper during the
  earlier sweep, then correctly found the committed wake-to-store-recovery path.
- Add only an explicit sessionWakeDeps resolution boundary, production wiring
  unchanged, and refusal/nonmutation tests. Independent Astra SOURCE_PASS and
  focused wake/relocation/resource-census GREEN are recorded in
  reports/20260913-ga-ecwh.5-wake-resolution-integration.md with exact digests.
  No guard/waiver/attempt policy changed. Supported active checkpoint passed;
  open repairs remain open rather than being closed for publication.
- Next: append-forward signed delivery with mandatory full pre-push validation
  and hosted CI, then reviewed merge-bound adoption. No live worker retry,
  lifecycle change, provider parity or handover completion is claimed.

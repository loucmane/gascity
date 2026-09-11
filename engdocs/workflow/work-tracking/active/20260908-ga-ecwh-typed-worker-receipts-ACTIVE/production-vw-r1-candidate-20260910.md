# ga-tmgr production V/W R1 source checkpoint — 2026-09-10

## Status
PARTIAL SOURCE INTEGRATION, HOLD for the requested complete production V/W candidate.
This is not a completed metadata-only transaction implementation or source/runtime acceptance.
No external authorization blocker is asserted. The remaining source work is listed below.
Parent full goal remains active/incomplete; ga-oz9e deferred. No goal/context/lifecycle change.

## Completed milestone
Parent R6 reviewed22-case synthetic matrix PASS: run5ff30b actual exit0,
validator4f70ad actual exit0;45 owned PIDs absent/no helper residue7552ce.
Observation SHA256 ffe89ddabacd4b5798edc708ed6b6f909e6f340c39e7a8d9d82372f8adb5e3e7;
Astra source review4c2d347f4f246ef68f77d05c67046dc54c18cca691f89ae5e8888e6566920c89.
Exact copies recorded in existing Core ACTIVE and Operations ACTIVE reports as
combined-runtime-r6-observation-20260910.md, with one owned ga-tmgr milestone note.
New unique evidence resolves the Operations projection limitation: exact new text
read back in current session, HANDOFF and TRACKER. Historical applied/no-readback
record remains unchanged. No M5/R2/R3/R4/R5/R6 execution or validator replay.

## Implemented production seams
- internal/platforminstall/committed_pair.go: receipt R1 -> manifest M -> receipt R2
  capture, equality/existence checks, decoded pair identity and expected-authority
  checks. Independent candidate parsing remains separate from installed acceptance.
- installer.go: metadata preflight captures R1/M/R2; exact no-op reload uses shared
  committed-pair validation. Existing known-partial/general rollback policy retained.
- integrity.go: installed receipt inspection uses that same reader. Existing doctor,
  managed dispatch and canary paths reach it through InspectIntegrity; no separate
  copied production decoder. New tests refuse substituted canonical metadata.
- manifest.go/installer.go: duplicate JSON field refusal before existing strict
  decoding and digest validation. V1 supported; unsupported/new schemas still refuse.
- metadata_lease.go plus platform clocks: one mutex-protected, in-memory,
  startup-random instance slot. Exact request/nonce/transaction/deadline checks;
  CLOCK_MONOTONIC, at most30 seconds, no renewal, no persistence or commands.
  Non-Linux clock support refuses.
- Existing SupervisorMux owns the slot. Two typed Huma POST routes:
  /v0/city/{cityName}/platform/metadata-lease/begin and .../check.
  City scope deliberately retains existing city write-auth; CSRF/read-only checks,
  running-city check,4096-byte body bound, no-store replies and typed errors apply.
  No daemon, general-command endpoint, new auth key or policy exemption.
- OpenAPI internal/published/compatibility copies and generated Go client updated
  from existing generators. Exact generator-output comparisons pass.

## Tests and limits
Evidence directory: /tmp/ga-tmgr-production-vw-r1-20260910
pair-red.txt, lease-red.txt and api-lease-red.txt preserve missing-symbol RED.
pair-green.txt, lease-green.txt, api-lease-green.txt preserve focused GREEN.
platforminstall-final.txt: owning suite PASS3.574s; current decoder source tests
are not a new execution of the frozen old-decoder binary or proof of a new schema.
platform-focused-final.txt PASS0.081s; doctor-final.txt PASS0.094s.
api-final-02.txt PASS0.108s: in-process recorder routing, schema and guard checks,
no socket listener or real supervisor. Pure lease tests use an injected clock;
one in-process wire test reads CLOCK_MONOTONIC. No real contention/exec race claimed.
vet-final.txt and build-final.txt exit0; built gc-source-stage was never executed.
api-final.txt preserves a generated-client patch-placement compile failure;
corrected to exact generator output before final tests/build, not waived.
Final full Core, dashboard/schema-client compatibility, hosted CI and independent
consolidated source review remain due. No new synthetic/process/namespace probe.

## Frozen source and preservation
source.sha256: 6569c848ede150c568795947777d966ec69115552d40854d48fb015d5495a56b
source.tar: a157bdd4420284259a187285cdbba152076a752385631687725cb51fdcb94cdf
source.diff: 9d5e69fbcf1695de05d7b3e46a47e0a98abf904afa4527cb3fd10a044efbcd77
gc-source-stage: 5b2c4c8b4584e5147921ee154c63e6a07484f4140fd51e2865ace8fc1506b551
source-files.txt names all18 changed/new source/generated files; diff includes new files.
Original20 manifest537bdba61fb18c4fb44c5318910d619ac8e8ddedc7669593669e5d1d157630cc
unchanged; original20-baseline-preserved.txt verifies preserved bytes. Current
installer/integrity/manifest source changes are deliberate, not a claim original20
working bytes stayed unchanged. production-before.tar, staged/unstaged/untracked
inventories and exact index-before/index-after retain the starting state.
R6 source and frozen decoder evidence rehashed unchanged; no decoder archive copy
or binary rerun. All consumed roots and earlier failures remain preserved.

## Required next source work — no implementation or execution PASS
The genuine host V image/version/boot/PID/start/listener/API/pidfd verification,
exclusive bounded V/W channel, frozen production mount/write/env/runtime-pin
profile, confined W entrypoint/descendants, retained transaction evidence and
phase-aware irreversible final receipt publication are NOT integrated yet.
The existing metadata-only entrypoint still uses the old Lifecycle path; do not
treat these reader/lease changes as permission to use it for the new contract.
New versioned receipt/manifest binding, replay/crash/partial/postcommit outcomes
and fresh V consumer verification must land together with that writer.
Do not replace the missing implementation with fake lifecycle/proof files,
permissive schemas, host-root mounts, provider helper trust or cache repair.
No production runtime reuse claim follows from synthetic R6 or this build.
Continue the same granted source repair; no new operator authority is required.

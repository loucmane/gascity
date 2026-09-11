# ga-tmgr reviewed formatting applied — local build HOLD

## Completed
Applied exactly the ten independently PASS-reviewed normal formatter outputs.
Every resulting byte hash matches /tmp/ga-tmgr-normal-format-review-20260911.md.
The remaining52 of the original61 entries are unchanged; custody production source
and its hardcoded owning-source pin map remain exact. Historical source manifests,
index snapshots, binaries, runtime results and prior preflight HOLD are preserved.
An initial apply_patch conversion rejected numeric diff hunk syntax before changing
files; exact preimages were rechecked, then the same reviewed delta applied once.

Focused pure owning validation, pinned Go1.26.7 with CGO_ENABLED=0 GOPROXY=off
GOSUMDB=off, command:
`go test -count=1 -run '^Test(Metadata|Committed|CombinedAcceptanceExactOldDecoders)' ./internal/platforminstall ./internal/packman`
PASS: platforminstall0.103s; packman0.004s. No integration tags, kernel test,
consumed-root execution or full suite. git diff --check PASS.

Frozen newly formatted62-entry manifest:
/tmp/ga-tmgr-formatted-build-20260911/source.sha256
SHA256 81ca21c2035201b7de4aee8f7580690eba5b773552117e654da5b3fbe90fa7bb.
Source archive: /tmp/ga-tmgr-formatted-build-20260911/source.tar
SHA256 3dcb023eeaf0f1292847fb41dc9b1604e1927982acc478b6c7d012f19e883f9a.
Focused-test evidence SHA256:
90a884b68cea32ef3e36a9bfba3bfa46176875582056ad6945c982f12caccc9e.
Old runtime observations are not executions of these formatted bytes.

## Exact excluded normal-hook action
The intended candidate includes internal/api/openapi.json (406 added lines versus
HEAD); npm resolves to /usr/bin/npm. Unchanged .githooks/pre-commit:67 requires:
`(cd internal/api/dashboardspa/web && npm ci --silent && npm run generate:client)`.
That installs dependencies in the existing Core worktree, outside the authorized
disposable /tmp environments. The current brief explicitly excludes that action.
The same hook:76 also invokes `make dashboard-check dashboard-smoke`;
Makefile:820 repeats in-worktree npm ci and replaces dashboard dist; :833-836
launches a loopback Vite preview listener, also outside this no-runtime checkpoint.
These are source-inspected mandatory operations, NOT executed hook failures.

Stopped before staging/commit, dependency installation or runtime. No hook bypass,
PATH masking, alternate hook, side worktree, cleanup or wrapper workaround used.
No new signed candidate commit exists, so no commit-stamped build or version read
was attempted. Do not stamp the old base96e0 as the candidate revision.
Current HEAD96e0ed0423c77ecc44237b4f9c3e667648c74a48 and tree
97a677768786601369f5a301849221acddfd0fd0 remain unchanged. Original staged index
listing remains dbf7c6ccb1ab0c1573b33e0d466df2eae10d52d41bdec386522309c583ec9e86.
Signing readiness from the preceding checkpoint is reused, not re-probed.

## Next action and owned evidence
Resolve the exact authority boundary for normal-hook in-worktree dependency
installation and its existing bounded dashboard smoke before attempting the local
signed checkpoint. No production/custody repair or guard relaxation is proposed.
Current ownership and queues verified; one consolidated owned ga-tmgr note plus
Core/Operations plan/session/tracker/handoff records bind this formatted candidate
and HOLD. Final verification/readbacks are under the fresh evidence directory.
Historical Operations September10 date HOLD remains separate and is not rerun.
Full original goal unchanged/incomplete; ga-oz9e remains deferred/nonblocking.
No publication, live adoption, worker, lifecycle or goal changes.

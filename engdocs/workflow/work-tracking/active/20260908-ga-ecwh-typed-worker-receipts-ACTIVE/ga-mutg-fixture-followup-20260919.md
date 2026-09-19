# ga-mutg validation recovery — narrow fixture correction

The earlier failed suite and cold-key checkpoint remain preserved in
`ga-mutg-validation-hold-20260919.md`; neither is relabeled PASS.

## Signing prerequisite restored

The operator ran normal `unlock-all`; read-only readiness then reported the
existing FD5585922F5335BC378AD8D42ECF4432C7E7982D signing identity ready,
agent_running=true, cached=true, proof=agent-cache. No key or configuration
change was made. The two disposable fixtures
`TestBuildRegistryPublishRequestNameFlagSatisfiesMissingManifestName` and
`TestTrackedGoFilesExcludesUntrackedScaffoldNoise` now PASS in cmd/gc (0.806s)
and internal/beadmeta (0.402s), respectively, using the unchanged signing path.

## Three store fixtures

The original full-suite logs identify three tests whose native task-attempt
admission reopens configured task authority but whose fixtures supplied only a
memory store or omitted the file provider. That defaulted to bd/Dolt; existing
leak guards detected and reaped the fixture children. The issue is not evidence
of production service failure and is not fixed by disabling those guards.

Two test files now bind their existing test intent to real disposable file
authority. The distinct city/rig test uses the existing
`newConfiguredAttemptTestStore` helper for each root. The two legacy alias tests
explicitly choose file mode without enabling scoped layout; shared backing,
distinct interface/physical handles, and all original deduplication and routing
assertions remain intact. No production implementation changes accompany this
correction; the nine independently reviewed timing-policy source hashes remain
as recorded in R1.

Exact follow-up SHA-256:

- `cmd/gc/build_desired_state_test.go`:
  `a4e5a8082467272ea76c0c82b49dd1319d27a7513387dc1dc67c08699fe4206d`
- `cmd/gc/build_desired_state_warm_crossstore_test.go`:
  `9f03e8381879ecbf2568a0bb0f1a4e00f91d95ee302b1334e4ab72d1faf8ba37`

Focused execution of the three named regressions PASS (cmd/gc 0.202s), using
Go 1.26.7, GC_FAST_UNIT=1, the normal leak guards, and scrubbed test environment.
`git diff --check` PASS. Independent read-only review of this test-only delta is
requested separately; this record does not anticipate its verdict.

## Delivery order

After independent delta review and workflow verification, prepare the signed
candidate through normal hooks. The required full fast suite runs through the
unchanged pre-push hook, with bounded concurrency and preserved logs; no second
identical preliminary full sweep is required. A failing hook prevents push.
Hosted CI and exact-head/base merge gates remain mandatory. No installed image,
metadata, rig, service, worker or provider-parity claim changes at this checkpoint.

## Independent review received

Reviewer `/root/recovery_controller_second_review` returned **SOURCE_PASS**,
no must-fix findings, bound to both exact hashes above. It independently confirmed
the separate scoped authorities, preserved legacy alias backing, unchanged warm
state/demand/route-repair/phantom-demand assertions, and absence of admission
mocking or bypass. All nine R1 hashes remain unchanged. The reviewer performed
read-only inspection only; test results are the coordinator's separate evidence.

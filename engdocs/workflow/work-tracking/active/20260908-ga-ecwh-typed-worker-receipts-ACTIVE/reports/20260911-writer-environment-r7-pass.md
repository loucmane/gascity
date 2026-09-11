# R7 synthetic runtime PASS — 2026-09-11

The current actual-host verifier/confined-writer positive blocker is cleared.
This is not live adoption, public delivery, full failure-matrix coverage or
provider parity. The full original goal and ga-tmgr remain incomplete.

Signed source c271c7a4ec0dfb8f7c326842f39a94552bfc2468, tree
e9c0e57caf235130f471503adb0a5a031eeb22ec, FD55 G signature. Exact image
cc54bdf5787a46b663e58116ec186f056a06b011d89eeda7d72c7a1d63d0aa68.
Independent source and package reviews passed before execution, as did actual
host namespace/boot/pin/unused-root preflight. No normal signing hook bypass.

One exact controller invocation; tool session33837 terminal exit0:
/tmp/ga-tmgr-full-positive-package-r7-20260911/controller.py --execute-once.
Runroot /tmp/ga-tmgr-full-positive-run-r7-20260911 is consumed and non-replayable.

- Result e7774512acaea8b18069e65f62fadd7767dfb808e5355df589d0f9034ea973f3.
- Apply count1, actual exit0 before teardown, transaction completed.
- Manifest domain digest 81392ad06087302080a34c6d1e1ee4e180ac66cf850fa86fd2063613a22d14f5.
- Receipt domain digest 6767a1217f865c5c506f12ee25173dd1ebf879b3250df12959837caee4457457.
- Receipt file SHA256 00fd4be6a56dae62f346adefc9b8a223d419e17fbd4b6ad0f78485d4bd208147.
- FINAL file SHA256 0eb104120215c2e4c882d693673d68560b2c6a9a65f2c3c9683d04bd557d404b.
- Exact committed pair, receipt/manifest/receipt readback and FINAL9/intent binding passed.
- Cache digest and byte/mode/inode/owner/timestamp inventories unchanged.
- Supervisor1587918, finalizer1587969 and adopt1587983 all exit0. Only the
  disposable supervisor received planned graceful termination; no forced kills.
- No teardown/fixture errors; elapsed30.982576s; listener absent; external owned
  process absence and protocol writer-cleanup witness passed.

Independent Astra result review verified exact files and returned synthetic-runtime
PASS. Its limits remain explicit: writer cleanup is source/protocol-witnessed,
external termination is the controller's recorded observation, and no independent
exhaustive protected-writer inventory is claimed. Cache-content preservation
relies on successful reviewed-controller digest checks. No second execution.

R3/R4/R5/R6 remain consumed FAILED/UNKNOWN with their exact evidence preserved.
R7 neither replays nor resolves them. All source workspace paths preserved through
commit/build; no installed artifact, real city/rig/service or protected HPFetcher/
Blog state changed.

Next deliverable: consolidate existing component/failure evidence with this exact
positive, run remaining final-candidate delivery validation, and prepare reviewed
signed public delivery under existing CI/merge gates. Do not repeat completed
Core PR36 or Operations recovery. Subsequent live metadata reconciliation still
needs its own exact reviewed pre/post-state package. Then gct-13ku/gct-10pg,
useful Claude execution, and both handovers remain. ga-oz9e stays deferred.

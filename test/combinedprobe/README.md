# ga-tmgr combined synthetic exec handoff correction R6

## Maintained source versus immutable attempt evidence

The 2026-09-11 delivery copy adds Linux packaging, standard test-environment
scrubbing, explicit previously-discarded results, formatting and a pure callback
test. It is a maintained diagnostic derivative, not the byte-identical source of
any historical run. Original files are preserved in the exact prepublication
archive `/tmp/ga-tmgr-delivery-evidence-20260911/pending-preserved.tar` (SHA256
`29a2502da7a6f96bc29e64b82abe0419e957da734f3bb904447299f87b1c0a91`).
No consumed run or frozen manifest may be replayed using this derivative. Default
tests use injected/pure helpers; runtime execution requires a new reviewed,
independently authorized package. The historical record below is unchanged.

R5 source passed review but its once-only actual host run exited1 at exec-before.
It is consumed INCONCLUSIVE; neither R5 nor unexecuted R4 may be replayed.
The failed listener import observed EBADF; source proves unsafe fd3 ownership,
but the exact winning close/finalizer race was not observed. All15 retained PIDs
were independently observed absent; no host checks are repeated here.

R6 retains the original inherited fd3 os.File from supervisor startup, through
net.FileListener's separate duplicate, to syscall.Exec. A single scope owns its
close on normal/import/exec-error exits; explicit runtime.KeepAlive spans exec.
There is no Dup3, replacement of an unknown slot, or late listener.File copy.
Only FD_CLOEXEC is cleared on that owned original; flags are read back before
exec. Other flags, exact listener, environment, namespace, frame and time limits
are unchanged. Exec-generation wait refuses terminal/unavailable pidfd promptly;
terminal exit is never a successful exec witness. No freshness threshold moves.

Current frozen candidate/run paths replace r5 with r6. R6 runroot remains absent;
only focused pure tests/vet/static builds and read-only plan freeze are performed.
Independent exact-source review precedes any new synthetic runtime execution.
R5 identity classifier, all22 cases, cache/descriptor checks and commit semantics
remain; old-decoder binary/input graph/evidence are reused without rerun/copy.

## Preserved R5 source description

R5 is an unexecuted M1 correction to the preserved R4 Astra SOURCE HOLD, not a
runtime retry. R4 and R5 runroots must remain absent before independent review.
Current candidate/run paths replace r4 with r5; all22 case meanings and bounds
below remain unchanged. The separately linked impostor retains the same
imageVariant=r4-impostor label intentionally; new binary paths/source produce
new pinned image hashes without changing its same-version negative semantics.

Only a structured observed executable-image or listener-ownership mismatch can
satisfy the two identity negatives. Evidence binds the inspected PID/boot/start,
actual executable/path/hash and endpoint, plus the complete observed owned-socket
inventory for listener rejection. The intended unowned listener must be the
replacement's exact inode; the original listener must remain in the target's FDs.
Proc reads/readlinks/FD enumeration, incomplete/missing evidence, cancellation,
API/transport/decode errors and composite/arbitrary refusals cannot satisfy M1.
Readlink failure is separate from a successfully observed wrong executable.
Runtime classification and retained-result validation enforce the same fields.
The FD-directory handle stays open through its readlink inventory, including
self-inspection; closing it early would manufacture a missing directory FD.
Its close failure is also an error, never an accepted observed mismatch.

Pure injected-I/O regressions and existing focused tests own this correction.
Unchanged old Core decoder binary/input graph and completed test evidence are
reused from R4; no decoder test or real synthetic process is rerun for R5.
No FD allowance, environment, namespace, cache, wait/pidfd or commit weakening.

## Preserved R4 source description

R4 is SOURCE-ONLY, not independently reviewed or executed. R3's initial runtime
PASS is recorded separately; every R2/R3/M5 runroot remains consumed and retained.
The R3 description below is historical input, not R4's current case inventory.

R4 uses the same modular verifier/writer, namespace flags, pipe framing, FD refusal,
cache-denial sets and irreversible receipt publication. Exact fresh paths are
`/tmp/ga-tmgr-combined-candidate-r4-20260910` and
`/tmp/ga-tmgr-combined-run-r4-20260910`. The latter must remain absent until review.
The plan binds an exact writer argv template and ordered closed case list rather
than repeating the same argv22 times; its existing16KiB frame bound is unchanged.

## R4 acceptance delta

- `exit-before/after`: real disposable supervisor exit, pidfd terminal witness
  at receipt-ready/published; not host PID recycling.
- `restart-before/after`: additionally starts/reaps a second real same-image
  supervisor with another PID/listener/instance; no brute-force PID reuse claim.
- `exec-before/after`: fixture-only explicit exec of the same image with retained
  listener, same PID/start and new startup instance. V observes the changed API
  generation and refuses the old bound request; not mere lease-epoch injection.
- `same-build-impostor`: second live same-image listener substituted into the
  original process binding; listener ownership must refuse before W starts.
- `wrong-image`: separately built static fixture with identical version text but
  different image bytes/path. Actual proc executable checking must refuse it.
- `expiry-before/after`: bounded deliberate wait until the request deadline at
  receipt-ready/published; V refuses and closes the channel. This proves V's
  deadline refusal, not an atomic kernel clock-and-rename primitive. W's own
  checkFrame/rename deadline checks remain unchanged and have pure coverage.
- `crash-before/after`: W emits phase-bound intent then exits23 immediately before
  receipt rename or immediately after rename, before directory sync. Actual
  writer exit and retained pair must agree; intent alone is not acceptance.
- `eof-before/after`: V closes only its owned writer input at the exact phase;
  the result must include actual W EOF and nonzero exit.
- `fsync-before/after`: actual fixture-directory O_PATH-fd fsync yields EBADF,
  before/after rename. This is a deliberate syscall adapter failure, not evidence
  of disk EIO, crash persistence or successful directory durability.
- `concurrent`: four actual readers each observe valid-old, mixed, then valid-new
  phases; a held receipt reader crosses rename and refuses its torn pair. Another
  exact transaction attempts the occupied real lease and must be refused. No
  stochastic scheduling/ABA exhaustion or competing writer commit is claimed.
- Every case initializes access/default descendant ACL fixtures. Writable controls
  must succeed; confined ACL writes must deny with1/13/30, and full cache xattr/stat
  inventory must remain identical. Unsupported ACLs stop INCONCLUSIVE, not PASS.
- `internal/platforminstall/combined_old_decoder_test.go` invokes the actual
  unchanged LoadManifest and decodeReceipt, with valid old-format positives and
  synthetic-new/schema-version refusals. No copied/lookalike decoder is used.
  This proves these exact Core decoders, not every consumer's final integration.

Whole run360s; fixture360s; existing V25s/W23s/supervisor35s/request20s/lease22s
and HTTP2s bounds remain. Expiry cases refresh process positives immediately
after negatives in V, before deliberate waiting; runner rechecks fixture identity.
All files remain evidence. Replacement processes have exact wait/pidfd teardown;
no global cleanup, installed metadata, credentials, host service or API is used.
New fixture transition routes exist only on declared disposable lifecycle cases.

Ordinary focused tests exercise serialization/refusal logic and exact old decoders
offline. Runtime methods are opt-in only through the frozen execute-reviewed plan.
Inherited source algorithms are reusable reasoning, NOT fresh R4 runtime evidence.
Review the full new source, plan, binary/dependency hashes and deliberate fault
attribution before any R4 execution. No source, full-matrix or live PASS is asserted.

## Preserved R3 source description

SOURCE CANDIDATE, unreviewed and unexecuted. R2 actual Opus SOURCE PASS followed
by once-only runtime exit1 at V's cpu.max FD refusal; R2 is consumed, never replay.
Retained setup/positive controls ran, but V version/lease and W Start did not.
All R1/R2 reviews/source/binaries/plans/runroots remain preserved. R3 changes only
the pinned child runtime initialization setting, not the isolation mechanism.
Only pure tests/vet/static build
are permitted in this source checkpoint. No default process test or production
API/cmd integration. M5 R3's parent-owned runtime PASS is preserved unchanged;
this helper is different and earns no runtime PASS from that observation.

## Owning boundary and roles

Attached ga-tmgr, existing READY ga-ecwh context, existing direct-source grant.
This prototype uses disposable fixtures only. Runner creates an absent exact
`/tmp/ga-tmgr-combined-run-r3-20260910` and retains every file. V runs in the
frozen genuine host namespaces, checks a real disposable supervisor executable,
literal bounded `/proc/PID/exe version`, boot/PID/start, pidfd liveness, loopback listener
inode/FD ownership and typed API replies before/after lease operations. V
does not publish metadata or journals. Runner records external observations.

The synthetic supervisor has one mutex-protected in-memory lease slot, startup
random instance, exact transaction/request/nonce and monotonic expiration.
No renewal or release handler exists: occupancy persists until deadline or
process exit. Only the disposable supervisor changes its epoch in fault cases.
There is no real Core lease handler, activation, service transition or adoption.

W is confined before its first instruction. The pinned M5 bwrap/runtime topology
is reused with a NEW static helper and one additional read-only request mount.
No root bind, try/fallback, privilege or host service/socket mount. Shared time
namespace preserves CLOCK_MONOTONIC. Only synthetic metadata is writable.
V/W channel is two exclusive inherited anonymous **pipes**, not a UNIX socket:
SCM_RIGHTS cannot be carried by this transport. Only stdio is inherited; child
helpers receive replacement stdio, never the live protocol endpoints. Frames
are bounded, length-prefixed, duplicate/unknown-field rejecting JSON. Transport
custody authenticates the channel, not the public nonce/digest. W cannot ask V
for paths, commands, descriptors or another observation target.

## Exact candidate cases

R1 already executed a version command. R2 tightens observed-image binding and
compares its output against the independently frozen plan/request ExpectedVersion;
the supervisor's own version constant is not the request acceptance criterion.

- `success`: genuine fixture identity/lease; all13 writable controls, all13
  confined cache mutations, unchanged full cache stat/hash/xattr inventories,
  four fixture process positive/negative controls, V proc invisibility, closed
  FDs and namespace witnesses; exact metadata stage/manifest/final receipt.
- `version-mismatch`: independently wrong frozen expected version refuses after
  actual observed-image execution and identity revalidation, before W/lease begin.
- `unsupported`: `/lease-begin` and `/lease-check` are genuinely absent from the
  HTTP mux. The default404 response is preserved with exact path/status/detail,
  identity revalidated, and W never starts. A200 application error is not accepted
  as absent support. Expected refusals are not successful adoptions.
- `before-loss`: synthetic lease invalidation at second check (receipt-ready)
  aborts with retained pending/mismatched pair; no automatic repair or rollback.
- `after-loss`: invalidation after final receipt rename preserves a committed
  pair and `committed_evidence_incomplete`; no rollback/replay or fresh PASS.

The final receipt rename is irreversible. Directory sync/FINAL failures after
rename never undo metadata. The R1→M→R2 reader refuses mismatched/unsupported
pairs and proves metadata only, not perpetual runtime liveness. Expected fault
cases require exact phase and error witnesses; arbitrary failures cannot satisfy
the matrix. Genuine outer exit0 plus separate result validation are mandatory.
Retained files are evidence, not process residue. The real writer child pidfd,
waited PID/process-state and terminal POLLIN must agree before WriterReaped is
true; unsupported/version-mismatch instead require writer_not_started with no
fabricated wait/PIDFD/reap evidence. EOF teardown and pidfd/reap witnesses cover
owned fixtures; no broad kill or cleanup. Deadlines: whole120s,
V25s, W23s, supervisor35s, request20s, lease22s, HTTP2s. Timeout kills only owned
commands after targeted SIGTERM; no live process is an attack target.

## Pure validation and freeze

From the existing Core worktree:

```sh
go test -count=1 ./test/combinedprobe
go vet ./test/combinedprobe
CGO_ENABLED=0 go build -trimpath -o /tmp/ga-tmgr-combined-candidate-r3-20260910/combinedprobe ./test/combinedprobe
/tmp/ga-tmgr-combined-candidate-r3-20260910/combinedprobe freeze
```

`freeze` starts no process or namespace: it reads/hash-checks ELF/runtime inputs
and refuses an existing runroot. Ordinary tests are pure (no real clock, process,
listener, environment or filesystem fixtures). Real execution is opt-in through
`execute-reviewed PLAN SHA256` only after independent consolidated source review.
No process test was run during preparation. Do not rerun the immutable M5 command.

R3 pins only `GODEBUG=containermaxprocs=0` in every child command constructor,
replacing rather than merging the parent environment. W receives the same literal
through `--clearenv --setenv GODEBUG containermaxprocs=0`. The frozen plan binds
the sole child environment entry and complete namespace argv; omission, extra
variables, duplicate/changed GODEBUG, GOMAXPROCS-only and argv overrides refuse.
The parent environment/configuration is never changed. `fdInventory` is
byte-identical to R2: no new allowance and no closing of unknown descriptors.
Installed Go1.26.7 runtime/cgroup_linux.go opens the cgroup limit before main but
closes it when containermaxprocs is disabled; runtime/proc.go calls that init
before reading GOMAXPROCS. Thus GOMAXPROCS alone is not an equivalent fix.
This source finding is not proof that the new runtime command has executed.

Every process attempt binds PID, boot/start/image, memory address, FD and exact
marker/memory digests. Runner checks its retained fixture pidfd and generation,
refreshes all four positive controls after each confined negative set, and checks
identity again. Missing/mismatched targets, unavailable positives or a refresh
outside the10s window after negatives make the result INCONCLUSIVE. W reports
remain self-reports; no claim of hostile-helper attestation is added.

The richer provenance exceeds one16KiB result record. `result.frames` therefore
contains exactly six existing length-prefixed records (one header, five ordered
cases), each retaining the unchanged16KiB strict JSON bound. Unknown/duplicate
fields, oversize, wrong order/count and trailing/truncated data refuse. There is
no larger individual frame allowance. Atomic retained publication and genuine
outer exit0 remain mandatory. `validate-result` decodes this bounded stream;
the old R1 result format and all original evidence remain unchanged.

## Honest limitations / review questions

This is the smallest initial combined mechanism candidate, not complete P01–P18
acceptance or production repair. Before claiming the **decisive full** combined
proof, remaining cases include real supervisor death/exec ABA and endpoint
substitution around publication, pause/expiry across rename, concurrent readers,
old frozen Core decoder execution against the new schema, and ACL coverage when
supported. Current fault injection changes the synthetic server lease generation;
it does not claim a real exec/exit test. Initial old fixture schema is synthetic,
not a test of old Core decoders. No production consumer is changed.

Boundary helpers preserve the M5 R3 algorithms (including deadline-aware ptrace,
attach-denial provenance and retained observation publication) as separate source
copies. Their new binary/mount/channel inputs need this combined review and run.
Fixture-only PR_SET_PTRACER_ANY and mmap memory are not live supervisor policy.
The trusted supervisor/V/W implementation and cooperating lifecycle model are
explicit; this is not hostile-host tamper resistance or arbitrary-helper safety.
Review must challenge listener/endpoint coherence, pipe custody and closure,
fault attribution, deadline/cancellation, publication durability and zero-process
residue. No source-review, namespace, combined or live PASS is asserted here.

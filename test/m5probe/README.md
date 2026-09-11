# M5 synthetic capability candidate (ga-tmgr)

## Maintained source versus immutable attempt evidence

The 2026-09-11 delivery copy adds Linux packaging, standard test-environment
scrubbing, explicit previously-discarded results and formatting. It is a
maintained diagnostic derivative, not byte-identical historical evidence.
Original files remain in `/tmp/ga-tmgr-delivery-evidence-20260911/pending-preserved.tar`
(SHA256 `29a2502da7a6f96bc29e64b82abe0419e957da734f3bb904447299f87b1c0a91`).
No consumed run or frozen manifest may be replayed using this derivative.
Default tests are pure/injected; no new runtime acceptance is claimed.
The historical source-stage record below is preserved.

Status: R3 source correction, NOT independently reviewed or executed. R1 received
actual Opus SOURCE HOLD; its wrapper failure and review remain preserved. No namespace,
synthetic, combined-protocol, source-review, live-adoption or provider-parity
PASS. This is a disposable capability harness, not Core adoption integration.

## Owning risk and test policy

Owner: attached ga-tmgr in the existing ga-ecwh source context. The operator
authorized this narrowly scoped direct-source repair; ordinary worker-only
policy remains intact elsewhere. The accepted design is v1 + R2 + R3; this
harness does not implement leases, receipt consumers or irreversible commit.

`plan_test.go` owns the pure plan/argv/parser/disposition matrix. It starts no
processes, opens no fixture or listener, changes no environment and uses no
real clock. Ordinary `go test ./test/m5probe` runs only these Small tests.
The Linux program is an explicit on-demand real-boundary harness, not an
untagged Go process test or a default CI job. Do not wrap its execution in an
ordinary test. Independent review and separate explicit execution are required.
No existing resource ledger, policy ceiling or default lane is modified.

## Compile and freeze (no probe)

Use the host Go shim and its normal on-disk build cache; never clear/relocate
that cache. From the existing Core worktree:

```sh
go test -count=10 ./test/m5probe
go vet ./test/m5probe
CGO_ENABLED=0 go build -trimpath -o /tmp/ga-tmgr-m5-candidate-r3-20260910/m5probe ./test/m5probe
/tmp/ga-tmgr-m5-candidate-r3-20260910/m5probe freeze
```

`freeze` reads executable/ELF dependencies and checks the runroot is absent.
It starts no subprocess and creates no namespace/fixture. Freeze stdout into
a new review artifact, never overwrite an accepted packet. The plan binds the
helper, installed bwrap, dynamic loader and complete named ELF dependency set.
The helper is CGO-disabled/static. bwrap is run through the pinned loader with
cache and hardware-capability search disabled, a closed library directory,
empty environment, no RPATH/RUNPATH and no `/etc/ld.so.preload` permitted.
No runtime/dependency is installed or copied into persistent configuration.

## Review-gated execution shape — NOT executed in this checkpoint

```text
/tmp/ga-tmgr-m5-candidate-r3-20260910/m5probe execute-reviewed \
  /tmp/ga-tmgr-m5-candidate-r3-20260910/execution-plan.json \
  <independently-reviewed exact plan SHA256>
```

The explicit verb/digest bind reviewed bytes; they do not authenticate a
reviewer. The coordinator must check the independent verdict before issuing
this command. No automatic execution follows freeze, tests or a design PASS.

The only runroot is `/tmp/ga-tmgr-m5-run-r3-20260910`. It must be absent;
`Mkdir` is exclusive. Root/setuid execution and dependency drift refuse.
No reuse, cleanup, repair or automatic retry is implemented. A changed source,
binary, dependency, argv or runroot requires a new reviewed identity.

Budget: 120 seconds for the run, 30 seconds shared by child attacks/mutations, and up to
10 seconds for each targeted command cancellation/reap. The fixture also has
its own 120-second deadline. Normal completion closes its stdin and waits;
deadline cancellation signals only the exact owned process, with Go's bounded
WaitDelay fallback. No process-group/broad kill, tmux, worker, service, or live
supervisor operation exists. Inspect a failure rather than rerunning it.

## Five capability obligations

1. bwrap must create child user/PID/mount/net/IPC views, with no `*-try` or host
   root bind. Mount isolation is intrinsic to bwrap. Actual namespace links
   must differ from the synthetic host fixture; time namespace must be equal.
   The helper is private PID 1, so its exit terminates remaining namespace
   descendants. No capability is retained by the confined helper.
2. Private read-only proc must hide the synthetic host PID. A PID collision
   refuses as inconclusive before attacks. Host positive controls prove that
   the exact freshly spawned fixture's process, memory and descriptor are
   reachable outside confinement. No live supervisor PID is accepted as input.
3. Child FD inventory permits only standard pipes and Go-created eventpoll/
   eventfd descriptors; other descriptors refuse. Parent exec uses empty env,
   no ExtraFiles and closed inherited FDs. A private fixture deliberately
   permits tracing of its own public synthetic memory; the harness tests
   signal permission (`kill(pid,0)`, not harmful signal delivery), ptrace
   attach/detach, memory read and descriptor acquisition. Denied host controls
   mean INCONCLUSIVE. Negative missing-proc results count only after those
   exact host controls and namespace checks pass. No unrelated target is used.
4. The only host mounts are static helper RO, synthetic cache RO and synthetic
   metadata RW; root/proc are remounted RO. There is no host home, socket,
   network service, credential, device or host proc mount. Staging and receipt
   share the one metadata directory; write/fsync/rename/xattr/final-chmod positives
   must work. This is not production receipt publication or atomicity proof.
5. Existing synthetic cache lock opens O_RDONLY with nonblocking shared flock.
   The old eleven cases are preserved: create, overwrite, truncate, unlink,
   rename, mkdir, symlink, chmod, utime, separate OS thread and child process.
   Same-owner chown and user-xattr cases extend metadata coverage. Every case
   must succeed against writable synthetic control and fail with EPERM,
   EACCES or EROFS against protected cache. Missing paths never count as these
   write denials. Unsupported xattrs/metadata controls are INCONCLUSIVE.

Inventory captures all synthetic cache entries, byte hashes, uid/gid/mode,
inode/link identity, atime/mtime/ctime and readable xattrs (including an ACL
xattr if present). Inventory reads use O_NOATIME. No explicit POSIX ACL
mutation case is implemented; this is a stated coverage limit, not an ACL PASS.
Every file is retained, including failed fixtures and intermediate child output.

## Readback and limitations

`result.json` retains exact plan digest, controls, namespace/FD observations,
before/after inventories and disposition. Failure can leave partial synthetic
metadata only. No postcommit rollback/replay exists. Fixture exit is checked
through the owned child wait and pidfd; successful bwrap completion reaps its
private PID-1 helper and descendants. `zero_process_residue` is process-only,
with explicit `fixture_reaped`, `fixture_pidfd_terminal` and
`confined_pid1_completed` witnesses. It does not mean that fixture files are
absent: all files are REQUIRED retained evidence, never cleanup targets.

R2 ptrace waiting uses WNOHANG and a context-cancellable 10ms polling cadence
at this black-box wait boundary, not a 10ms test deadline. After successful
attach, detach is attempted exactly once on every returned path, including
EINTR, cancellation, deadline and wait failure; detach failure is joined with
the original error. Failure to detach is not advertised as successful cleanup.
Pure injected tests own these branches; no live ptrace runs in the offline suite.

Fixture memory is anonymous mmap storage held until the fixture terminates
its wait, then explicitly unmapped with errors propagated. The fixture-only
PR_SET_PTRACER_ANY setting permits same-uid observers to trace this disposable
public-memory fixture for its lifetime; it changes no other process or host
policy. `ptracer_policy` records this fact. Child stdout/stderr are bounded to
64KiB; overflow is a command failure, not silent truncation or accepted JSON.
Nonzero child exit remains refusal even with plausible stdout. Non-errno errors
are explicitly classified and never promoted to denials. Unexpected descriptors
remain refused/INCONCLUSIVE; no speculative runtime FDs were added.

## Observation publication is not self-certifying acceptance

The result is serialized only after process teardown, written to exclusive
`result.stage`, checked for complete writes, synced and closed, then renamed
without replacement to `result.json` and its directory synced/closed. Failure
retains the stage or already-renamed observation; there is no deletion, rollback
or replay. If rename succeeded but directory sync/close fails, execution exits
nonzero and reports incomplete evidence. The file can still describe an observed
PASS; it cannot certify its own final durability or outer process success.

Final acceptance requires BOTH an independently captured successful outer
execution exit and a complete validated observation bound to the reviewed plan.
The read-only `validate-result RESULT PLAN_SHA256 OBSERVED_EXIT_CODE` mode checks
that conjunction, the full case matrix, cache equality and namespace/process/FD
witnesses. The exit argument must come from the real outer execution record,
not be manufactured as zero; the validator does not authenticate its caller.
No `report_persisted` self-claim, second receipt or endless marker chain exists.
Pure fault injection covers create/write/short-write/sync/close/rename/directory
failure and outer-exit refusal; real filesystem durability remains unexecuted.

R2 actual Opus SOURCE HOLD accepted the MF3/MF4 fixes; R3 changes only MF8/MF9.
Metadata user-xattr is set while the receipt is still writable, before its final
chmod0400; pure ordering/failure tests guard this sequence. Ptrace outcomes retain
`pre_attach`, `attach_denied` or `post_attach` provenance through error wrapping,
including joined detach errors. Runtime and outer observation validation share
the same predicate: only `attach_denied` with an already-permitted process errno
can count as a ptrace denial. A wait/detach error never becomes confinement proof.
All denial sets, descriptor boundaries and nonzero-child refusal are unchanged.

This is a reviewable capability candidate, not an adversarial
kernel proof: no arbitrary malicious helper, namespace re-entry/SCM_RIGHTS,
alternate ABI, process_vm_writev, io_uring or explicit ACL mutation matrix is
claimed. The trusted loader and fixture code run on the host; root/hostile
same-user tampering is not solved. Pin checks are preflight checks, not a new
OS-level lock against concurrent replacement. Review must challenge this
boundary, cancellation/reaping and inherited-descriptor assumptions before
execution. Any capability gap stays HOLD; do not weaken the required combined
P01/P05/P08/P11/P12 proof or infer it from these five capability checks.

No new production API/CLI, supervisor lease, platform adoption or service
activation is present. Those remain later independently reviewed delivery and
activation gates.

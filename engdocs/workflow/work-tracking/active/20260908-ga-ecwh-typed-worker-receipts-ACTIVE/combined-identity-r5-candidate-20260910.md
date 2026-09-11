# ga-tmgr R5 identity correction — independent delta review
SOURCE CANDIDATE ONLY, 2026-09-10. No R4/R5 runtime execution.
Same READY ga-ecwh parent/attached ga-tmgr; original full goal remains incomplete.
Core /home/loucmane/gascity-core-worktrees/ga-ecwh-typed-worker-receipts
HEAD 96e0ed0423c77ecc44237b4f9c3e667648c74a48
Owner external-coordinator.v1:038188a07df973a7a976387e7af363f2acab2472642d9ed3844944553e26a707

## Preserved review and narrow correction
R4 Astra SOURCE HOLD, not runtime failure:
/tmp/ga-tmgr-combined-r4-astra-review-20260910.md
SHA256 3e8ef99c65cdf2fe9669e2e2f3b6518c99e5eabc10da779d5385a32d46888355.
Only M1 changes: typed observed identityMismatch binds inspected PID/boot/start,
image path/hash, port/listener inode, expected image and owned socket inventory.
Only exact executable-image or listener-ownership mismatch can satisfy the named
negative. Missing/incomplete targets, proc/readlink/FD failures, cancellation,
API/transport/decode errors, wrapped/composite/arbitrary refusals do not qualify.
Readlink error is separate from successfully observed path mismatch.
FD-directory lifetime spans readlinks, including self-inspection; close errors
remain failures. No unknown FD is closed/allowed and no runner guard is added.
Runtime classification and outer result validation use the same binding predicate.

All22 cases, positive identity checks, wait/pidfd teardown, child environment,
16KiB frame limit, namespace/mount/cache checks and irreversible commit semantics
remain. Fresh R5 path/transaction labels and rebuilt hashes are explicit input
changes, not a reuse of R4 runtime acceptance. The impostor's r4-impostor build
label intentionally stays unchanged; it remains a separately linked same-version
image. There is no source/feasibility/full-matrix/live PASS.

## Exact changed files
Paths relative to Core:
86889ae78a93f5d67fbdcc81e8577e21fa7cd6e982f8ff77744c3ffc4f99c3ef  test/combinedprobe/README.md
17929424f5778c821ddd1e5efd2911537135c40cafbd6ea0614822f8f15178fd  test/combinedprobe/acceptance_linux.go
678dfed0f8ec6d8363efae0898e849ee41d7126889fa5fe9784200bff32b9f13  test/combinedprobe/acceptance_test.go
a2b80776f71de67d6e2bb3579d6d05b218328795a05e331b9cf8af96b4426be5  test/combinedprobe/identity_mismatch_linux.go
bbd323d3ed2a35ff48401f145bcae712492cbf14fd1d4e3cb37d9ec5dfbe6ecc  test/combinedprobe/identity_mismatch_linux_test.go
b24fdbed2803460b333a4c9ee0c958917e1e2cb3a32915eaf12ade0a00880103  test/combinedprobe/runner_linux.go
a2b2fee9537e2fd55ad14e936bb437826a59e597adef59b9fbb08abc06e1ffe7  test/combinedprobe/runtime_linux.go
0c83c06806c9aad12c6208c180765574ac39c126849ebbb658eee94b93ae7c0d  test/combinedprobe/transaction_linux.go

## Frozen source/build/plan
Directory: /tmp/ga-tmgr-combined-candidate-r5-20260910/
- source.tar: c39ff5c6824e106b11b95b2eedcd11096c6f391cfd0acf9fe89d9df04b0f6c1d
- source.sha256: 2aa20b2a541c1f0ab072f818f0b28ecab865d32a5e302ac4b91f2c60737282cd
- changed-files.sha256: 76fc4597177540d3d8a71d88d14fc9bd6ed8ef109b2c6a601ec87072352d175c
- r4-to-r5.diff: 65ab1a3b4db6fcd9d8ad164ee42fcfabc91cf589a1799b4f938bb4a4d80d8396
- execution-plan-final.json: 375e4fc5306b4dc1f42be0760bc4e88259bd88ac612d389b457fa01419005e53
- combinedprobe: 54a7f691bfc0a6a7b799109c2330ffe4ca5071a39769e4c01f995c3592001361
- impostor: 3b52c13b8363f5f57d90ac423f14b94647eb7e122c02adc3cae96e6f36c3c151
Only execution-plan-final.json binds the final binaries; earlier iteration plan
remains preserved but is superseded. Static CGO_ENABLED=0, -trimpath builds;
impostor additionally uses -ldflags '-X main.imageVariant=r4-impostor'.
Read-only freeze validates ELF/dependency hashes and absent exact runroot.

## Focused proof and reuse
red.txt reproduces arbitrary refusal acceptance. focused-final-02.txt PASS0.023s;
vet-final-02.txt and both final static builds exit0. No broad suite was run.
Pure tests cover exact two intended mismatches, unchanged positive ownership,
proc stat/boot/image/readlink failures, missing targets, FD enumeration/readlink,
truncated/absent listener evidence, empty inventory, cancellation, API errors,
composite errors and mutation of bound observations to arbitrary refusals.
These are injected deterministic tests, not fresh host/process observations.
Whole boundary_linux.go is byte-identical to R4; original20 and index unchanged.

old-decoder-reused.sha256 references, without rerun/copy, R4 old-decoders.test,
complete decoder graph/manifest/archive, owning test bytes and completed log.
R4 frozen manifest readback passed. Prior M5/R2/R3 evidence remains untouched.
R4 and R5 runroots are absent. R4 HOLD is historical and not rewritten as PASS.
ga-oz9e deferred; Core -> gct-13ku/gct-10pg -> useful Claude -> both handovers
and the full original race/decoder/provider outcome remain incomplete.

## Independent delta review and proposed execution
Review source.tar + r4-to-r5.diff against the preserved R4 review/reasoning.
Read-only source verification command from the existing Core worktree:
```sh
sha256sum -c /tmp/ga-tmgr-combined-candidate-r5-20260910/source.sha256
```
No inference or review launcher is run by this checkpoint. Challenge error
attribution, completeness, inspected-target binding and retained-result mutation.
Only after independent exact-candidate approval, actual UID1000 in the frozen
host namespaces, and fresh absent R5 runroot:
```sh
/tmp/ga-tmgr-combined-candidate-r5-20260910/combinedprobe execute-reviewed /tmp/ga-tmgr-combined-candidate-r5-20260910/execution-plan-final.json 375e4fc5306b4dc1f42be0760bc4e88259bd88ac612d389b457fa01419005e53
```
Require genuine outer exit0 before separately validating result.frames with the
same plan digest and actual exit0. No automatic retry, R4 execution or replay.

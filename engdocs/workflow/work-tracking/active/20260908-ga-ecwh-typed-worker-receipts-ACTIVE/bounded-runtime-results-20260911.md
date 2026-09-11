# ga-tmgr — approved bounded runtime validation results

## Executed once, in order

1. Exact reviewed kernel component test: **PASS**. Tool `7fb40e`, exit 0,
   stdout `PASS`, 0.172 seconds. Independent PID absence readback `4cb14e`.
   Full classification and hashes:
   `/tmp/ga-tmgr-kernel-component-r1-result-20260911.md`, SHA-256
   `16d52b801eaa5a7b286ad1cf40092eafe3a23c8b99d924aa0574b2985961e49d`.
2. Exact-image isolated ordinary fixture startup: **PASS**. Approved launch plan
   implemented by `/tmp/ga-tmgr-ordinary-capture-r1-20260911.py`, SHA-256
   `5ad20d147410a341a47bf5935c8e8b9675b623f29a633f34a9bc1cc21cb988ab`.
   Astra independently reviewed the implementation and its pre-launch corrections
   (initial readiness, complete empty listings, stable pidfd/ancestry enrollment).
   No ordinary runtime ran before that DELTA PASS. Tool `4d6afc` returned active
   handle 63736; polling that same handle produced terminal `180d9d`, exit 0.
   Duration 1.16 seconds, supervisor exit 0, no teardown errors.

## Exact ordinary observations

Root: `/tmp/ga-tmgr-ordinary-positive-run-r1-20260911`.
Image SHA-256: `b332c2e3d1788b3590ae4d34ff28e7a7ed356364f0b827ee95793c1f7e5f7456`.
Owned supervisor PID 2241753, start ticks 70833852, boot
`386902d4-a7d2-4056-a6c3-aa592cffe444`; loopback port 48371, listener inode
139290842. Actual health reached one real ready CityState, version `dev`,
build_id `unknown`. Complete agent and session lists each returned items=[] and
total=0, without partial/truncation. Proc sampling observed no descendants; this
is not a complete exec/syscall trace. The file Beads and fake session providers
were the reviewed fixture configuration, not provider-parity evidence.

Read-only independent teardown check `1918f1` confirmed the owned PID absent
and no listener on port 48371. The production supervisor and real rigs were not
controlled by either test. All fixture state and logs remain preserved.

Evidence hashes:

- result.json: `8b3bce2bf302157e272cdfac569c8803d079693ddaad762feff7f466ce08224c`
- runtime-capture.json: `f5f486b8eceb9740c19b88df6d0fbd1e7d3fd73d5da51cd48feb2d2774495df6`
- ready-inventory.json: `88b2c8326e3be5587658212b566bec245a11231d8df4cfddf5f4ad103398693d`
- supervisor.log: `52e147aa15b6ea2b125bc92e4c667d01ed99856aa55b3d3ef30dfea86403ee8c`

## Do not overstate acceptance

The fixture successfully started, exposed the expected read surfaces, captured
actual inputs, and stopped. **Complete metadata-manifest input closure is not
proven.** The capture recorded user/pid/mnt/net/ipc/uts; the metadata protocol
requires time instead of uts. Do not retroactively fill the dead supervisor's
missing time namespace from the current shell. Future fresh capture must use the
protocol's exact namespace set. All captured process epochs are historical after
teardown and cannot authorize a later apply.

The observed `unknown` build ID is also incompatible with the activation manifest's
required hexadecimal commit. The current custody profile accepts exactly seven
build settings and no linker/VCS metadata. This is a concrete next build-contract
question, sent to independent Astra for source-grounded adjudication; it is not
permission to relax either guard, fabricate a commit, rebuild/relaunch, or retry.
The already-passing kernel tests need not be repeated to investigate it.

No metadata apply, V/W transaction, receipt, product worker, signing, publication,
source repair, root operation or real lifecycle action occurred. These outcomes
do not complete ga-tmgr, provider parity, handover or the original goal.

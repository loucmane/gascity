# ga-tmgr: align hosted CI with the reviewed custody toolchain

## Preserved failure and exact cause

Signed head29672e00a17b8d6776b40d48b2b68a12f20adff1 was pushed through
all10 normal local pre-push jobs, then opened as Core PR37 against exact
mainac8f47ee37908f0e6876c67e3b5b30cff905b358. GitHub verified the signature.
Hosted CI34606069217 completed with one root failure in job103284791020:
TestMetadataCustodyOwningSourceProfile rejected net/http/server.go bytes.
The CI log proves the compiler was Go1.26.6; the reviewed production custody
profile and local R7 compiler/source audit already require Go1.26.7.
The failed log remains preserved under /tmp/ga-tmgr-public-delivery-20260911
as ci-core4-failed.log, SHA256
78108804e5bb5741d6743f054759d5d398e0e2009ce24ae8aaba39e52951df62.
CI integration/required failures are propagation of that one root failure.

## Minimal correction and independent review

Align go.mod and both Ubuntu/macOS composite-action Go defaults to1.26.7.
All other active workflow compiler selections use go-version-file:go.mod.
Independent Astra source review PASS binds three-line patch SHA256
05e0284c00c0f55dbaee44241049e49e90856978db7d04feb1322f211fc96306.
No custody source hash, runtime assertion, profile or permission is changed.

The existing CI execution-shape guard correctly refused the changed default
during focused verification. Preserve that RED as toolchain-alignment-tests.log,
SHA256b1deefc85698552f017e9d16ea07071a780fb33aa96161ac2f94b621d9999b70.
A separately reviewed exact policy synchronization updates only the setup-action
expected hash and adds two regressions. Reversing only the Go default must
recover the entire prior execution projection/hash; module and both composite
defaults must agree. All other workflow/policy hashes remain unchanged.
Independent Astra PASS binds proposed policy patch SHA256
4e23e7758925a3ff2318137f2b017cc987eb4e567857db92606bd6270337a584.

## Verification and remaining gates

Fresh Go1.26.7 integration-tagged internal/platforminstall and scripts/cipolicy
suites PASS (3.939s and0.149s). Log toolchain-alignment-green.log SHA256
ce6f76b13cb7ce43f24107c63e7084d54f2b39344641745d49341e5d85bd24fc.
git diff --check passed. No package download or installed toolchain change.
Normal signed delivery, exact-head hosted CI and merge gates remain required.
This corrects CI selection; it does not retrospectively change the inputs or
claims of the preserved R7 image/synthetic run.

## Separate actual-host hold

Read-only inspection and independent Astra review found the real canonical
metadata parent contains preserved assets/ and backups/ directories, which the
current strict v2 output-parent/mount profile cannot accommodate. No supported
unchanged-layout configuration exists. The separately recorded explicit proposal
is pinned readonly protected sibling submounts, with independent source/runtime
review before any live use. It is not implemented by this CI correction.
Do not move or clean those directories, weaken checks, or activate the supervisor
into an unsupported adoption layout. See LIVE-LAYOUT-DECISION.md SHA256
692a1a133a99190cfd8549f0dacfa6aef086614b45332f25117d73dddee353ee in the
preserved publication evidence directory. No source PASS closes ga-tmgr or the
full provider-independent execution/handover goal. Rigs and live services remain
untouched; every failed attempt and prior receipt remains preserved.

# ga-tmgr combined R2 runtime refusal and explicit reassessment

The full original goal remains active and incomplete. No mechanism replay or
production/live adoption is authorized by this observation. R2 is consumed.

## Exact completed review and execution

- Independent actual claude-opus-5, firstParty/Max, one tool-free turn: SOURCE PASS.
- Review payload: f83f2e593e1637dd3b7fb103b0d1d53d769428fc798be26783a1e61c511a9d19.
- Verdict `/tmp/ga-tmgr-combined-independent-review-r2-20260910/verdict.md`:
  a2544c53e9b9b8def2bb2638fa8bf6c637e5322da8e61665e4ecdcb2c6b1c01c.
- Verified completion: db29ade359d02cff253b1de2102abc755b635ba6a90dff0b87b61264c91e51ff.
- Source manifest: a77aa66c4a7dc063988db87b8c97589657b4d1f1073296d9acbe8a3047472e7d.
- R2 helper: 0bdd2e29575766786662bf33462dba89943c5257511f78471f40663520601e2b.
- Plan: 23e197d3d503ddf167dd9d0eef8e2b224a978369602c53ceaf23a474e220337f.
- Exact execute-reviewed command ran once via actual-host tool at UID1000,
  boot386902d4-a7d2-4056-a6c3-aa592cffe444 and all six pinned namespaces.
- Genuine tool execution chunk b4d987 returned exit1:
  `missing verifier observation: EOF: exit status 1`.
- Standalone validate-result with actual observed exit1 (chunk af7f47) refused
  `outer execution failed`, exit1. No manufactured exit0 or PASS.
- Verifier stderr: `unexpected descriptor 3: /sys/fs/cgroup/init.scope/cpu.max`.
  File SHA256 a35f8c69abdc4d41eba0a44858798c370aed61419c81b97210667ef6375144e3.

## Disposition and preservation

Runroot `/tmp/ga-tmgr-combined-run-r2-20260910` is consumed and retained intact.
Result.frames SHA256 4a00cd40111ca1a683b0125017842c7e067e0b5bb59dafbf79b036cebb62d6d5:
header INCONCLUSIVE; fixture_reaped=true; success.supervisor_reaped=true.
Remaining case records are zero-value unexecuted placeholders, NOT successful
refusals or evidence that those operations ran. The verifier emitted no outcome.
Actual refusal preceded version/lease checks and writer Start in runVerifier.

The synthetic runner DID create fixtures and exercise positive controls. This
is not an entirely pre-mutation attempt. It made no production mutation and no
adoption/metadata transaction: success metadata has exactly manifest.json and
receipt.json, each matching the request's before digest. No writer started.
Host read-only follow-up chunk 7da470 proves /proc/2112549 (fixture) and
/proc/2112569 (synthetic supervisor) absent. Exact-helper pgrep chunk 6c1b87
returned no matches (exit1). No signals, cleanup, retry or service changes.

## Cause established from exact installed runtime source

Binary build information identifies Go1.26.7, CGO_ENABLED=0. Its runtime
defaultGOMAXPROCSInit opens the cgroup CPU-limit file before main and retains it
for the process lifetime when containermaxprocs is enabled. This directly
explains the cpu.max FD rejected by V's strict fdInventory. command() has empty
Env/no inherited ExtraFiles for V. This is an omitted runtime initialization
input in the prototype, not an observed cache-confinement failure.

Authoritative local source:
- /home/loucmane/.local/share/go/1.26.7/src/runtime/cgroup_linux.go
  a2237d59862ebb1fd5f85750663cfe03965463a6d6bf21e5a411df26504f5f7f
  lines13-16,45-71: retained FD; containermaxprocs=0 closes it before main.
- /home/loucmane/.local/share/go/1.26.7/src/runtime/proc.go
  b0ea864c1dd251cd9146644863ae9126ca57761dff05862dc80ab0799925fec6
  initialization is before the explicit GOMAXPROCS environment handling.
- /home/loucmane/.local/share/go/1.26.7/src/internal/runtime/cgroup/cgroup_linux.go
  9f192b16192c9176b5c1bc100e786f5c726359d86782aa86a32221bebb07d031.

## Decision after this failed approach and understood R2 correction

Do not repeat R2, reopen namespace exploration, or allowlist this FD. Preserve
M5's real confinement PASS and every source/review/runtime failure separately.
Retain trusted-host V plus OS-confined W. The bounded next candidate, if prepared
under the existing direct-source exception, must explicitly pin and validate the
Go runtime initialization setting that prevents retained cgroup FDs. Do not close
unknown descriptors or relax fdInventory, IPC, namespace, version, deadline,
metadata, signing or permission checks. No parent/user environment mutation.

Use an exact child-only setting such as GODEBUG=containermaxprocs=0, bound into
the closed plan and used consistently where required; examine the installed
runtime source before choosing the minimal equivalent. GOMAXPROCS alone does
not prevent the earlier cgroup initialization. Reject inherited override drift.
Add focused pure configuration/argv regression tests and keep full R2 source,
binary, plan, reports, runroot and review immutable. One consolidated independent
delta review precedes any fresh append-forward synthetic run. This decision is
not permission to start another broad test-harness or wrapper repair campaign.

Full combined race/decoder matrix and all useful-worker/handover acceptance
remain open. ga-oz9e remains deferred/nonblocking. No Bead may close on this.

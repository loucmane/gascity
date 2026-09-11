# Independent Astra adjudication — ga-tmgr combined R3

Source: completed read-only review from existing independent subagent
`/root/astra_namespace_review` (Euler), requested by the operator. This file
preserves its returned verdict; the parent did not modify candidate source.

**SOURCE PASS—for this delta only.** All four supplied hashes match. M1's
missing source premise is resolved; its additional runner-level refusal is not
necessary for this frozen synthetic contract.

1. **M1(1): resolved.** Every retained CPU-limit descriptor is opened with
   O_RDONLY|O_CLOEXEC: v1 quota at internal/runtime/cgroup/cgroup_linux.go:69,
   v1 period at79, and v2 cpu.max at96. The Open wrapper in
   internal/runtime/syscall/linux/syscall_linux.go:46 passes those flags to
   SYS_OPENAT, adding only O_LARGEFILE. V's launch does not explicitly transfer
   this descriptor. Thus the runner-runtime CPU FD is not retained across its
   exec into V. V's own initialization opens its CPU descriptor independently;
   with the pinned setting, runtime/cgroup_linux.go:45 takes the disabled branch
   and calls c.Close() at70.

2. **M1(2): not a substantive must-fix here.** A parent inventory would inspect
   runner-local descriptors, including legitimate CLOEXEC resources that never
   enter V. It would not distinguish inheritance merely by finding the same
   target in the parent. Rejecting the runner's normal CLOEXEC CPU descriptor
   would prevent a valid launch, not prevent the alleged inheritance defect.

   The existing V guard still refuses an unexpected FD with its number and
   target (frozen R3 packet:1154). In archived transaction_linux.go:74-76 that
   refusal precedes host verification, version execution, lease operations and
   W creation. Runner preserves verifier stderr before rejecting the missing
   observation (packet:1394). This is already attributed, fail-closed behavior,
   not silent acceptance. Earlier disposable fixture preparation can occur;
   no stronger refusal-before-any-fixture guarantee is established or needed
   by this delta.

The earlier empty-Env/no-ExtraFiles explanation is insufficient for general
inherited non-CLOEXEC descriptors. Go syscall/exec_linux.go:615 explicitly
leaves that responsibility to callers. The supplementary CLOEXEC source, not
that shortcut, resolves this particular CPU-FD premise. This source review
does not retrospectively trace the historical FD.

No code change, new runner guard or full-suite rerun is required to resolve M1.
Preserve the original HOLD alongside this evidence-backed adjudication. No
execution authorization, R3 runtime success, confinement acceptance or provider
parity is implied.

## Exact reviewed bindings

- R3 packet: /tmp/ga-tmgr-combined-candidate-r3-20260910/independent-review-packet.md
  041e23103dfca2101e3bc04f88903210c76ef7c906a05333ef14c2d7760488b4.
- Original HOLD: /tmp/ga-tmgr-combined-independent-review-r3-20260910/verdict.md
  cc5d3a7e13dc20482ba310c9044f07d33809a72e753b17a8298d35cc6542e227.
- Installed Go1.26.7 internal/runtime/cgroup/cgroup_linux.go:
  9f192b16192c9176b5c1bc100e786f5c726359d86782aa86a32221bebb07d031.
- Installed Go1.26.7 runtime/cgroup_linux.go:
  a2237d59862ebb1fd5f85750663cfe03965463a6d6bf21e5a411df26504f5f7f.
- Source manifest: 606b0af539dd0bd39d8022d9dcf3c1ac2fb3215669b8f7271682ac9ef8f62c63.
- Plan: 80ca827ce5e91fefad2860da16e7e7ee4abb5af179ef176e289d583ae253e2f6.
- Helper: 72d4991d1c264af7829922164efe9bb6cd82eae4572398e15bb9dbba1d325b53.
- Parent evidence supplement: /tmp/ga-tmgr-combined-r3-review-supplement-20260910.md
  06a29bebe2bdfa424980348231dacd1d910080e39982128c9b808a050e7e5143.

All runtime-source paths above are beneath
/home/loucmane/.local/share/go/1.26.7/src/.

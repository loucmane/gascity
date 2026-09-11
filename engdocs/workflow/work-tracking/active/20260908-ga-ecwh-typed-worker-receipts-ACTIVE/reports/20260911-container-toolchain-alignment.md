# Complete the existing exact toolchain lockstep

The normal pre-push gate on signed head
`e40e640b061bad42a175fb7e640125c42d315d3d` failed before remote mutation.
`TestContainerBuildsUsePatchedGoToolchain` correctly requires both Docker
builders, the module directive and the setup-action defaults to move together.
The preceding correction had omitted the two builder pins and the owning test.
The failure is preserved, not retried unchanged or reclassified as infrastructure.

Exactly four source lines change: the two builder tag/digest pairs, the test's
exact version constant, and the current descriptive TESTING.md version. The
existing exact lockstep check is retained, not weakened to a minimum-version test.
Historical CVE statements and reverse-delta negative fixtures stay unchanged.

The public official registry resolves `golang:1.26.7-bookworm` to OCI index
`sha256:e8c859f5632dcfde7b32d2012b4351728f6437930887c2f6a91ea242459e5514`.
The response's digest header matches the SHA-256 of its actual body; the index
contains Linux amd64 and arm64 entries. No image layer was pulled or executed.
The anonymous public pull token was transient and not persisted.

Evidence under `/tmp/ga-tmgr-public-delivery-20260911`:

- Preserved failed gate: `toolchain-push.log` and `toolchain-push-tests/`.
- Applied patch: `container-toolchain-alignment.patch`, SHA-256
  `5ac97b3c7e7da1e48716075de976d9851b2604fadc7cde0097aa02d1e5d29134`.
- Official index metadata: `official-go-1.26.7-bookworm.json`, SHA-256
  `f3599c26b242bfac37a8b793d0462c59d9016f1f1f135159e68a779722406fb4`.
- Three focused security tests PASS: `container-toolchain-green.log`, SHA-256
  `5fd54c8060ff05458fca73f66496fb43d4b6b42039e6ada64d0c1158f9b3eb31`.
- Affected regression suite: `container-toolchain-regression.log`; its terminal
  result must pass before delivery. Independent exact-delta review is required.

No container build/deployment, package installation, installed artifact or rig
transition occurred. The full goal remains open. The operator has separately
approved the bounded protected-sibling source extension and its independent
review/fresh synthetic proof; live metadata adoption remains unproved.

# ga-tmgr build identity — local delivery preflight HOLD

## Corrected decision
The approved correction is valid: pinned Go1.26.7 load/pkg.go:2424-2435 omits
ldflags from BuildInfo with trimpath. Custody source needs no extension. An exact
review-bound `-X main.commit=<actual candidate commit>` remains the proposed build;
BuildInfo alone cannot prove absence of other linker flags. Prior recommendation
and runtime records remain unmodified. No stamped image has been built here.

## Completed and exact stopping point
Existing owner/queues and loaded hooks reverified; exact-key noninteractive signing
readiness is ready (agent-cache), without unlock, pinentry or credential changes.
All61 reviewed source entries plus the reviewed kernel addition match frozen pins.
Original15 staged files/index and complete staged/unstaged/status evidence preserved.

Read-only preflight applied the normal hook's exact `golangci-lint fmt --stdin`
operation to the candidate, writing outputs ONLY under this fresh evidence root.
It exited0 but 10 candidate files differ from their frozen source bytes. The normal
.githooks/pre-commit invokes scripts/precommit-format-staged-go, which writes
those outputs into the source and re-stages them before lint/codegen/signing.
Consequently an ordinary commit of the exact frozen bytes cannot both retain
source parity and run that hook unchanged. No hook was skipped or altered.
This is a demonstrated source-parity preflight HOLD, NOT a claimed hook execution
failure or signature failure. No git add, git commit or build was attempted.

Exact prospective hook delta (238 lines; import grouping, literal/type formatting):
/tmp/ga-tmgr-build-identity-20260911/normal-hook-format.diff
SHA256 8469e50cd48cb00ba6b699473555be5585b6573a96e3bf0a34ba21fe98b1a9bd.
Affected files are enumerated in format-preflight-drift.txt (10):
internal/packman/metadata_strict_test.go;
internal/platforminstall/combined_old_decoder_test.go;
internal/platforminstall/integrity.go;
internal/platforminstall/metadata_custody_linux_test.go;
internal/platforminstall/metadata_integration_linux_test.go;
internal/platforminstall/metadata_other.go;
internal/platforminstall/metadata_protocol.go;
internal/platforminstall/metadata_sandbox_linux.go;
internal/platforminstall/metadata_transport_linux_test.go;
internal/platforminstall/metadata_kernel_component_linux_test.go.
No formatter output was copied back. Custody production source is unchanged.
Remaining hook lint/codegen/docs/dashboard checks were not run; no claim they pass.

## Identity and preservation
No new signed candidate revision/tree exists. Current unchanged HEAD:
96e0ed0423c77ecc44237b4f9c3e667648c74a48; existing tree:
97a677768786601369f5a301849221acddfd0fd0

Do not stamp this old base as the dirty candidate. Original staged listing digest:
dbf7c6ccb1ab0c1573b33e0d466df2eae10d52d41bdec386522309c583ec9e86.
Baseline tar, raw-index archive, diffs and hash receipts are retained under
/tmp/ga-tmgr-build-identity-20260911. Existing production image remains
b332c2e3d1788b3590ae4d34ff28e7a7ed356364f0b827ee95793c1f7e5f7456;
no new image/build identity or version execution evidence exists.

Two harmless invocation mistakes are preserved, not called product failures:
/usr/local/go source path absent (correct pinned path read afterward); shell GPG
helper initially passed to Python and stopped at parse time (subsequent direct
shebang check succeeded). No signing operation or authentication retry occurred.

## One next action
Independent disposition of the exact normal-hook formatting delta, then a
review-bound formatted candidate can proceed through unchanged normal signing
hooks and exact stamped build. No custody repair, wrapper or bypass proposed.
This checkpoint stops before source changes, new tests/runtime or publication.
One consolidated owned note and current Core/Operations evidence record this HOLD.
Final verify/readback receipts live in the same evidence root. Historical
Operations date HOLD is separate and not rerun. ga-oz9e remains deferred;
full original goal stays active/incomplete, with no goal/context/ownership change.

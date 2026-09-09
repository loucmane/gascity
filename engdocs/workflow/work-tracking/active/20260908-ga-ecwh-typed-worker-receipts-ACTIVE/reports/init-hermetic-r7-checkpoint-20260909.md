# ga-ecwh.2 — deterministic initialization-test fixtures

Date: 2026-09-09. Source-only delivery repair; no live acceptance claim.

The normal push of signed head `38a45465858eb186309997f9ebeaee8ae6559477`
finished with exit 1. Eight jobs passed. CLI shards 4 and 6 each failed one
inherited initialization test because the implicit default Gas City template
downloads a remote roles pack. The failures took 614.17s (partial transfer) and
10.16s (DNS), respectively. All failed artifacts and signed commits are preserved.

Failure evidence:

- `/tmp/ga-ecwh-delivery-20260909/push-r3.log`, SHA-256 `98cefbc5d12ceda0f38dcdeb779a6243d2efdffaa4071cc81bcd46b965567ba5`.
- `/var/tmp/gp5/logs/unit-cmd-gc-4-of-6.log`, SHA-256 `5d9e19cc7f877e09e2eb77bb9b33e8badcfa9ec7fcc4a92ad19a0a76f6fcc8f0`.
- `/var/tmp/gp5/logs/unit-cmd-gc-6-of-6.log`, SHA-256 `2cb22efeec78409bc6f3bfd0af29cd27719d3b5834b7ff41f684f6e913fcf0cc`.

## Smallest owning repair and retained coverage

Only `cmd/gc/init_provider_readiness_test.go` changes: both existing test
owners explicitly choose the existing minimal template and set
`GIT_ALLOW_PROTOCOL=file`. The implicit-state test also asserts that the
fixture contains a remote import before real finalization. No production
code or original assertion is removed.

| Observable promise | Continuing owner and scope |
| --- | --- |
| Real finalization creates no implicit-import state | `TestFinalizeInitDoesNotWriteImplicitImportState`; unchanged finalization and filesystem assertion |
| `--no-start` prevents registration and prints guidance | `TestCmdInitNoStartSkipsSupervisorRegistration`; unchanged real CLI path, recording collaborator and assertions |
| Real Git import is installed before provider readiness | Unchanged `TestFinalizeInitChecksRemoteImportProvidersAfterInstall`, using its existing `file://` repository |
| Default template still selects Gas City and its roles import | All nine unchanged tests in `cmd_init_gascity_test.go` |
| Remote-import synchronization and failure reporting | Existing `TestInstallInitRemoteImports*` and `TestFinalizeInitReportsRemoteImportInstallFailure` |

The protocol restriction reproduces both faults deterministically before the
fixture change (0.07s and 0.06s), without making an external connection. The
original default-template output contract remains unchanged and has separate
owning tests; this is not a claim that all CLI tests are hermetic.

## Verification

- RED: both original fixtures fail with `transport 'https' not allowed`;
  `red.log` SHA-256 `18d02c90fe06c445a89831cbc995b41c7949d8c4ecb085f256a9bda3e170b6cc`.
- GREEN: both repaired tests, ten repetitions each, 3.310s total;
  `repeat-green.log` SHA-256 `6a73fc04920e894239c71631f000ce414622086f748b0f23c50308fe55d576fc`.
- Retained owner selection PASS, 0.370s;
  `owners-green.log` SHA-256 `76f2e3c5faf753fd72ccd3341e2b449b1a2a0ed3a40bba89c6497515ea90cd52`.
- Full resource-census package PASS, 3.635s;
  `resource-green.log` SHA-256 `f31d769ed162e0589ec6ceacacdbb30d61e4a79601064124e57d31caaac63933`.
- `git diff --check` PASS. Its empty output is only whitespace-check evidence.

Timing is focused diagnostic evidence, not a p95 or hosted-CI claim. Compile/link
scratch remains on disk in `/var/tmp/gp6`; no explicit GOCACHE or cache cleaning.
The new regression adds only test-scoped Setenv calls, not ambient resource debt.

## Independent-review follow-up

Fable R6 returned a narrow SOURCE HOLD asking whether the minimal fixture still
exercises the remote-import path. Its full verdict is preserved. Source inspection
confirmed that `doInit` always adds the bundled core import with a canonical HTTPS
source; `hasRemoteImport` therefore remains true, and real `syncImports` and
`installLockedImports` run using the already-hydrated bundled cache. The default
Gas City template's additional unbundled roles import was the download trigger.

An explicit `initHasRemoteImports` assertion now protects this fact against future
template drift. Final JSON evidence proves 20 repeated fixture executions plus 13
retained owner tests PASS, zero failures and zero skips. Package times were 3.475s
and 0.318s. These supersede the pre-assertion green logs for the final candidate;
all earlier logs remain preserved.

R7 review preparation initially refused its input-size limit before creating a
review directory or contacting the provider. The corrected preparation includes
only the relevant registry identity/URL functions, while pinning the complete
source file. It does not increase the limit or broaden disclosure.

Independent Fable R7 returned SOURCE PASS, clearing the narrow R6 HOLD after
checking the full predicate/call chain and final test events. Both reviews are
tool-free, subscription-bound source reviews using executor evidence, not
independent test reruns. Exact manifests, prompts, results and verification
records are preserved in separate R6/R7 directories.

- Final test source SHA-256: `0077230abf19a240fda896f12be7b3a5a65f0e83478c40bf39b5a98fdc26e2d3`.
- R6 HOLD result SHA-256: `de747e6e07c210da14bcbec5393d39875c48b00a3ef535569bc4338c4a656bf7`.
- R7 PASS result SHA-256: `1f52a492913c1171c0fdc16bcb65af69445601182199812fe7fb9a6e9f16332f`.
- R7 verified readback SHA-256: `4a68c230da0906583d44bcb36379150602d14a9d717af90d7d1d7cb2fad6e24c`.
- Final repeated-test JSON SHA-256: `f96e11fc29d90b2556ad8482b636e089c18615089e946facd2e16281ffff26ce`.
- Final retained-owner JSON SHA-256: `551ac96bbf844374aa9de7bf8338b51ea03ad611601ac2d3d92022aa35b8297b`.

The reviewer retained a non-blocking observation: the separate file:// import
test does not assert absence of implicit-import state specifically on that
unbundled path. No broader guarantee is claimed here. The writer is in the
existing bootstrap compatibility implementation; it is not changed by this repair.
This observation does not authorize another scope expansion or block delivery.

New signed delivery, complete pre-push and hosted CI remain required before
merge. No waiver, skip, signing defense, permission, live configuration, rig or
service is changed. No Bead closes on these local results. Raw unified-diff review
prompts are preserved byte-exact locally and bound by committed digests/results;
they are not rewritten or staged to work around whitespace checks.

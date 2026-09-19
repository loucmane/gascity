# ga-mutg timing policy R1 — independent source review

Base `728178bfc5971c8cdac5e37cf7b581b9a05d53b0`. User-authorized direct source
exception for ga-mutg only; no worker, service, installed artifact or receipt changed.

## Candidate

One shared, finite policy replaces scattered 25/28/30-second bounds with
60/63/65 seconds. The 3-second expiry gap and 2-second return reserve are unchanged.
Both pipe deadlines are 60 seconds; supervisor admission, writer lease-bound
validation and expiry-wait validation use the same 65-second horizon as the
outer context. Original monotonic epoch, no renewal, earlier-parent shortening,
all host/input/cache/protected-tree checks, cancel/wait/pidfd, irreversible
publication and failure classification remain unchanged.

This is deliberately a source-bound limit, not an adaptive timeout or runtime
override. Measured 14.054005566-second closure exceeded the consumed R5 package's
13-second limit. No wrapper threshold or consumed package was edited/replayed.
Future live proof must bind a new image and complete closure; slow or unstable
hosts can still refuse. No source result grants live acceptance/provider parity.

## Exact source SHA-256

| Path | SHA-256 |
| --- | --- |
| internal/platforminstall/metadata_limits.go | a88bf4ac5bb1eca324733b9d4c00fe4d7060c039bc19a51e82ff5864bc304c75 |
| internal/platforminstall/metadata_budget_linux.go | b75f239cb0e50ada7262910e5e7e689ca60c52354ba5291d29dd7b4fb3c226ea |
| internal/platforminstall/metadata_transaction_linux.go | 467ca3a8720351c3fe27c5de400d1ada508795158d66a174c2d864cda0501355 |
| internal/platforminstall/metadata_lease.go | 31e93f5777aefb29f188e4e3680e9a9c90d849a376452ea52c4481e5637ed9cc |
| internal/platforminstall/metadata_budget_linux_test.go | e7e932ce27bf5dec58903b5e9a763e8f93398ff9bebefc12f24e52a05a9dc727 |
| internal/platforminstall/metadata_timing_policy_linux_test.go | 9b3f5b02c7673be309635e10d16d270f1e788cf3f2ea21b6f511de75e132dcac |
| internal/platforminstall/metadata_lease_test.go | 1373db70aa530f1c096fa9eb601b1aada6ca0909a6c8a9abddb671908455f41c |
| internal/platforminstall/METADATA_TRANSACTION.md | e8c8070a53ea8050aaa3e0ca37248953e564829078aa7e8bf03785593c4faf3b |
| docs/runbooks/versioned-platform-upgrades.md | f354f5694810adbd04905611be9d25104788901ba7eff6fbd3af5fae40de349c |

## Actual test evidence

Go 1.26.7; `go test ./internal/platforminstall -run '^TestMetadataTimingPolicy'
-count=1` before production changes exited 1 with three expected failures:
slow validation returned old epoch+25/28, admission refused 63 seconds, and
natural-expiry test exhausted the old budget at 29 seconds. No live process ran.

After source changes: all `^TestMetadata` tests PASS (0.263s); full owning package
`go test ./internal/platforminstall -count=1` PASS (3.078s); package vet and
`git diff --check` PASS. New tests use arithmetic/fake clocks, not sleep or new
subprocess resources. Existing child teardown, publication/rollback, observation,
replay and custody regressions are retained. Full fast suite is in progress,
not yet claimed as PASS. Full vet/delivery/hosted CI remain required.

## Review questions

1. Are all cooperating timing consumers consistent, finite and overflow-safe?
2. Do original-epoch exhaustion, earlier-parent shortening, nonrenewal and the
   unchanged expiry/return reserves still hold?
3. Have any host, cache, sandbox, publication or child-terminal checks changed?
4. Do tests and docs make source-only limits and new-image adoption requirements
   explicit without relabeling historical evidence?

Return SOURCE_PASS/HOLD with exact findings. Review is read-only; no live probes,
signing, dispatch, receipt alteration or activation.

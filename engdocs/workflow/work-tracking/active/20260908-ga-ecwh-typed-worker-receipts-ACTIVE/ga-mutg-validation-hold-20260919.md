# ga-mutg validation checkpoint — source PASS, delivery HOLD

## Completed

- R1 source implementation and independent SOURCE_PASS; exact nine-file binding
  remains in `ga-mutg-source-review-r1-20260919.md`.
- Three focused RED failures against old limits; all metadata tests GREEN;
  full owning package PASS (3.078s), metadata race suite PASS (1.611s).
- `go vet ./...` exit 0; `git diff --check` PASS.
- Supported workflow verify PASS: live ownership, plan sync, readiness, diff
  check and portable scaffold. ga-mutg remains open/in_progress, not accepted.

## Full-suite stop (not source PASS substitution)

`make test-fast-parallel LOCAL_TEST_JOBS=2` did not pass. Disposable Git fixtures
in examples/gastown, internal/beadmeta and cmd/gc inherited the user's global
commit signing configuration and requested a locked personal key. GPG refused
without a tty. Read-only readiness independently reports agent_running=true,
cached=false, proof=none, status=cold (exit 11). No unlock or signing was attempted
by the coordinator. No credential/config changes were made.

cmd/gc shards 1–3 also reported one leaked fixture Dolt child each; their existing
leak guard reaped them and failed honestly. These are additional suite failures,
not proven consequences of locked signing and not waived by the timing review.
Reported PIDs: 859863, 886756, 931313. Fixture roots are in the retained logs.

The owned full-suite process group 775774 was verified as this turn's make
invocation and gracefully terminated after these findings (exit 143); no retry.
Subsequent **host-context** process inspection found no members of that group,
no reported PID 859863, and no Dolt processes matching the test-root pattern
`/var/tmp/gcx`. The earlier sandbox-only ps result was not used as host proof.
The independent race test finished normally. Source candidate unchanged.

Logs remain at `/var/tmp/gc-local-tests.wkGp5k/`:

| Log | SHA-256 |
| --- | --- |
| fsys-darwin-compile.log | e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855 |
| local-concurrency-selftest.log | 0a875d123abf74adf6b0e19c90b14ed6d42867ff3bc381d47dc563815c309b04 |
| push-gate-lock-selftest.log | 239bd97df6bc7b573852c3c29c8b7735561f9101f745d1e07eac303ebff24c11 |
| unit-cmd-gc-1-of-6.log | 84d1e055fceda63fc499cf8718ae9b661f2a031eab5c1a223f9bd6cb0934642a |
| unit-cmd-gc-2-of-6.log | 3fc39e62644e2ba3e324a80c0aa639d2f2eb4d4eb35b282481a833c05a5de906 |
| unit-cmd-gc-3-of-6.log | 27e5023597da265468d31d64eb5a48a6f61a10c5114a36acfa5677c6696b4b7a |
| unit-cmd-gc-4-of-6.log | 57d5a3e484b9c53bc0a11b1c331d9694f6c1ca21d478990007a466d8dedc1af6 |
| unit-core.log | 068c551eb4eff8496a5ec30bbb85b8fc15c79f6ebfb5df8b669f2243d1ec3344 |

## Next

Operator unlocks the existing signing path through normal `unlock-all`; no key
change or weakened signing is authorized. Before another broad sweep, resolve
the fixture-environment prerequisites and understand the three leak-guard
failures proportionally; retain this failed run and do not mask it with a retry.
No source change outside ga-mutg is implied. Required full checks, actual signed
candidate, hosted CI, verified-base merge and new-image adoption remain outstanding.

The original provider-independent execution and bidirectional handover goal is
unchanged/incomplete. This repair is not provider parity or a completed workflow.
No production metadata transaction, worker routing, rig lifecycle, timer change
or installed-artifact change occurred in this source-repair turn. R5 consumed
records and restoration timestamp discrepancy remain preserved, not relabeled.

# ga-tmgr combined R3 — observed initial mechanism PASS

The full original provider-independent execution/handover goal remains active
and incomplete. This is initial combined synthetic acceptance, not production
adoption, the full race/decoder matrix, useful-worker completion or provider parity.

## Genuine execution and independent validation

The exact frozen execute-reviewed command ran ONCE at UID1000, without sudo or
root, under all six reviewed host namespaces and boot
386902d4-a7d2-4056-a6c3-aa592cffe444. Actual tool chunk8615db returned exit0
(0.604982344 seconds). A standalone validate-result command received that actual
observed exit0 and returned exit0, tool chunkb4bdaf. No exit value was inferred
from a self-reported file. No R2 or M5 command was repeated.

Consumed runroot: /tmp/ga-tmgr-combined-run-r3-20260910.
Result: /tmp/ga-tmgr-combined-run-r3-20260910/result.frames
SHA256 84c2a84147e32d6bb2d18281a9b79371ee6e3e8f07b78b9dc610222f87577423.
The bounded six-frame stream decodes completely, header status PASS.

## Observed outcomes

| Case | Recorded disposition | Metadata pair | Writer |
| --- | --- | --- | --- |
| success | PASS, last_step=final | committed=true, pair=true | terminal, exit0 |
| version-mismatch | aborted_preserved at version, exact expected-version mismatch | no commit/new pair | not started, exit-1 |
| unsupported | aborted_preserved at lease-begin, exact HTTP404 absent route | no commit/new pair | not started, exit-1 |
| before-loss | aborted_preserved at receipt-ready, exact stale/lost lease | committed=false, pair=false | terminal, exit1 |
| after-loss | committed_evidence_incomplete at published, exact lost lease | committed=true, pair=true, preserved | terminal, exit1 |

For all three writer-started cases: all13 confined cache mutations refused,
all4 process attacks refused, all4 fresh host positive controls passed,
cache_unchanged=true, exact wait PID and pidfd terminal evidence passed.
Every case records supervisor_reaped=true. Early refusals correctly have no
writer, no false reap witness and no false negative-control evidence.
The before-loss mismatched pair remains retained; the after-loss committed pair
was not undone or replayed. Expected fault dispositions are not adoption PASS.

Version execution really used the observed process executable, the missing
endpoint really returned404, and the new child-only runtime pin reached the
V/W startup paths without relaxing the descriptor guard. These are observations
for this exact runtime/plan, not universal guarantees for future versions.

## Process containment

Fixture PID2276774: fixture_reaped=true.
Synthetic supervisors:2276793,2276862,2276899,2276938,2276990.
Writer parents:2276826,2276971,2277023; each wait PID matches and pidfd terminal=true.
Independent host tool chunk4b1e45 verifies all nine /proc entries absent.
Exact helper and /writer process search chunked93a0 returns no matches (exit1).
No signals, cleanup, retries, rig lifecycle or service commands were performed.
The writer pidfd binds its outer bwrap/loader process; inner writer/descendant
teardown rests on the reviewed as-PID1/die-with-parent namespace contract plus
the negative exact-process search, not a fabricated direct inner pidfd.

## Source, review and preservation bindings

- Source manifest:606b0af539dd0bd39d8022d9dcf3c1ac2fb3215669b8f7271682ac9ef8f62c63.
- Binary:72d4991d1c264af7829922164efe9bb6cd82eae4572398e15bb9dbba1d325b53.
- Plan:80ca827ce5e91fefad2860da16e7e7ee4abb5af179ef176e289d583ae253e2f6.
- Final pre-run evidence manifest:c026c3e2851360995a8b6fd371e5f65898ec629435aa3bf4cc8683e052d6cfde.
- All13 frozen source files reverify after execution (tool3ff752).
- The original Opus R3 SOURCE HOLD remains preserved with its missing-source
  question, cc5d3a7e13dc20482ba310c9044f07d33809a72e753b17a8298d35cc6542e227.
- Missing primary-source supplement:
  /tmp/ga-tmgr-combined-r3-review-supplement-20260910.md
  06a29bebe2bdfa424980348231dacd1d910080e39982128c9b808a050e7e5143.
- Independent Astra SOURCE PASS adjudication:
  /tmp/ga-tmgr-combined-r3-astra-adjudication-20260910.md
  d99bba41e4330c3d4bf414015c7922b420936797c8ea1ac9685c7e02716b666f.
  It verified the missing O_CLOEXEC primary source; no guard/source change was
  required and no original review was overwritten or silently ignored.

Preserve M5 runtime PASS, combined R1 unexecuted SOURCE HOLD, consumed R2 runtime
INCONCLUSIVE, every review, all staging/partial pairs, original Core source/index,
and every historical workflow artifact. No live installed artifact or protected
project work was touched. ga-oz9e remains deferred/nonblocking.

## Exact remaining work — do not inflate this PASS

Before decisive full combined/production acceptance: real supervisor exit/exec
ABA and endpoint substitution, pause/expiry around receipt rename, concurrent
readers, frozen old Core decoder execution against new schema, and supported
ACL coverage remain unproven. Current loss cases change a synthetic lease epoch;
they do not claim real process exec/exit. Version-negative varies expectation,
not the executable image. Synthetic-old schema is not actual old Core decoding.

Next deliverable is the smallest complete remaining acceptance/delivery map
against existing Core candidate and protocol, reusing these proven inputs.
No restart of the isolation investigation, repeated M5/R3 run, new wrapper/daemon,
production install, source-only Bead close or narrower replacement goal.

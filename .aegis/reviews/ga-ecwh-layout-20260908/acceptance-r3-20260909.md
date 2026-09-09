# ga-ecwh.1 — Core evidence relocation live acceptance

Date: 2026-09-09. This is an evidence observation, not a provider capability receipt.

## Delivered and reviewed

- Operations signed head: `7feeac49cb5deadef29f0e3ae29079f7fad5fbea`.
- Tree: `9b8f43d1c5daa2ecaf57594ee64945c9fb618a55`.
- PR384 merge: `90555c456875e8218bbd09ad80c3e7fcda70aa84`, identical tree.
  Exact parents, signature, premerge CLEAN/MERGEABLE, required green checks and zero
  review threads are retained in the Operations PR384 observations.
- Final source regression: 3013 passed, 21 unchanged historical/opt-in skips.
- Fable inventory review: PASS; result SHA-256
  `c5052fad782e4b582a30d1a13a1f363c4b3ba09458d11c47f636d1da5c2086a4`.
- Fable exact execution-bundle review: PASS; result SHA-256
  `bc07139b41edde3d6c92970d6e11ab202d18a751d308b9b142eee681f5d2f077`.
  These are supplied-artifact semantic reviews, not independent live observations.

## Exact live result

The supported target-aware daily continuation executed once and completed. Its
reviewed delta is retained in `daily-continuation-20260909.json`: four changed
bookkeeping entries, one new daily session, one exact predecessor WAL archive.
Old inventory is preserved (78 entries); refreshed inventory has 79. The review's
initial prose count of 74 was corrected before the execution-bundle review.
The older stale sync record is preserved; the supported new sync record matches
the current plan and tracker. No cause beyond the recorded facts is inferred.

Execution bundle SHA-256:
`112b5acb015a20d8ba0a1cb54d326d91e1b5534028497431685603faebe8a344`.

Plan ID:
`f1fec32ea4f45986e5b480458c5c3e692c3386f8af5217f109d2e22ec98e2706`.

The public source-bound apply returned PASS, idempotent=false.
Before any subsequent logging, its exact replay returned PASS, idempotent=true.
No manual repair, retry of a failed mutation or rollback was needed.

Actual owning documentation command:
`/home/loucmane/.local/bin/go test -json -count=1 ./test/docsync`.
Exit 0; **15 tests passed**, no failed/skipped test. Raw output is retained in
the transaction's docsync_attempt and postflight fields. The exact replay
validated the accepted state and prior proof without rerunning Go or plan-sync.

Transaction record:
`/home/loucmane/gascity/city/rigs/gascity/.git/gas-city-workflow/layout-relocations/f1fec32ea4f45986e5b480458c5c3e692c3386f8af5217f109d2e22ec98e2706/transaction.json`.
Status complete; terminal leaf WAL `91f728b2b38b4c5d9280f3e21b7d2dd4`.
Originals, backups, failed-attempt evidence and prior WALs remain preserved.
Rollback correctness is inherited from the reviewed regression tests; this live
success did not exercise rollback and is not claimed to have done so.

## Preservation and limits

Only the five reviewed evidence roots moved, with four layout-only configuration
fields. Current context resolves the relocated session and plan. Both relative
current links remain valid. The nine tracked historical plans and all 39 protected
files were checked by the transaction and its accepted-state replay.

At live acceptance and before subsequent supported workflow logging:
- Core HEAD: `a5a900f1173c8caccaa0bce23c946991ca52e50b`.
- Exact HEAD diff SHA-256: `a4e66b98cc96c1a0fb4243dde1d4fcd177a0250d79922d3665eae63bd66ea72c`.
- Index SHA-256: `9f2d00f81ed465918068494f19cbbd96b20cf84c96004fceaf4e5588708dd63f`.
- Ownership journal SHA-256: `e694063055af8057a85af6fe65a13ec0159ab6649606cb5447eb6396331d330f`.

Runtime was revalidated by preparation, apply, postflight and replay: supervisor
PID1769, start-monotonic210566291, zero restarts; all four project rigs suspended,
zero native sessions. No provider, service, key, source-content or waiver-policy
mutation occurred. Later legitimate workflow logging is not permission to replay
this frozen migration again; accepted state is intentionally checked exactly.

This discharges the evidence-layout acceptance only. ga-ecwh.2 conformance work,
Core source delivery and the full provider-independent worker/handover goal remain
open. No provider-parity, signing-canary or whole-goal completion is claimed.

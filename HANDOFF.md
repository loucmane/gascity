# harp-asg1 — ypav-a5c source handoff

Classification: internal operational evidence. 2026-10-13.

Authority: /tmp/ga-ypav-round4c-brief.md, fixing independent REVIEW-B's P1/P2.
The coordinator committed rounds 4/4b at 21c0144df44a8e402cba3ed8c713ac7a7bc1e91e.
This round started clean there; edits remain unstaged on ga-ypav-port2.
No live city operations, credentials, signing or deployment.

Managed claim/adoption now must revision-CAS a work-side incarnation marker
and new claim-attempt ID before publishing the session claim or delivering work.
Even unchanged metadata on successor adoption advances the work revision.
Drain requires that incarnation; pending rollback also requires its exact attempt.
Legacy unmarked rows stay for controller recovery. Work identity metadata and
legacy assignee repair use revision CAS. Unsupported backends fail closed.

The verified hook writes an acknowledgement containing generation and token
digest in the acknowledgement value itself. Both controller stop paths check
it against the session row and runtime token/epoch. A write onto a recycled
name is inert. Legacy agent acks with mismatched requester digests are rejected.
Provider writes themselves remain non-atomic.

Both reviewer proofs failed before the fix and pass afterwards. The combined
run passes 2,359 records: 821 cmd/gc, 1,246 session and 292 beads, retaining every
previously passing round-4 record. Changed production coverage: 178/208 (85.58%).
Vet passes; final lint and delivery evidence are in ROUND4.md. Two CGO-disabled
builds match dbd6b5cc3744645937268d1a25003b1e628edb041e23cdf78351fb9d5d3728cf.

Full evidence: /tmp/ga-ypav-window/ROUND4.md, Round 4c. Remaining limits:
- Acquisition, fencing, session publication and delivery are separate operations;
  crashes or failed fences can leave pending work for controller recovery.
- The session current-claim pointer has no attempt ID: concurrent invocations
  of the same incarnation can clear that pointer without releasing newer work.
- Continuation siblings are individually assigned, not a group transaction.
- Provider metadata can receive stale auxiliary writes; the addressed ack
  cannot authorize successor stop. This is not an atomic provider-write claim.

Ready for independent re-review, not live-upgrade approval. No new full host
cmd/gc pass is claimed. The tracked-source resource census passes but must be
rerun after staging new files. The standards wrapper still lacks this tree's
file-size baseline. Human review remains required for lifecycle/execution
and prior configuration/custody changes.

ROUND4-SOURCE.diff is cumulative from c64fd0574; ROUND4C-SOURCE.diff contains
only this round from 21c0144. Source/artifact manifests are refreshed.
The coordinator owns staging, commits, host checks and release actions.

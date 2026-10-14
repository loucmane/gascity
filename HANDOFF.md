# harp-asg1 — ypav-a5d source handoff

Classification: internal operational evidence. 2026-10-14.

Authority: updated /tmp/ga-ypav-round4d-brief.md, REVIEW-B2.md and REVIEW-A.md.
Started clean at 306c11a23a43bc3e96fd8cd161a48b45fa32ae25 on ga-ypav-port2.
Edits remain unstaged. No live city, credentials, signing, staging or commit.

Fixed the control-kind metadata regression without removing work CAS: all
managed acquisitions/adoptions still stamp incarnation and unique attempt,
but control kinds omit session ID/name back-references. Control drain/unwind
ownership uses assignee and markers; exact-attempt and revision checks remain.

Review A fixes: marked-work ownership veto checks the current incarnation;
T3 and wake readers accept only runtime-matching addressed acknowledgements;
no-op claim publication cannot clear an earlier adoption stamp on failure;
controller tests now fail when the controller guards are removed. Hook-current
help names token/epoch requirements and epoch parse errors fail before store
access. Verified provider selection intentionally ignores GC_SESSION.

Final combined run: 2,415 passing records, zero failures/skips (869 cmd/gc,
1,246 session, 292 beads, 8 T3). Every round-4/4c passing record is retained.
B2's reviewer proof failed before the fix and now passes; both adapted review-B
proofs remain green. The eight strengthened controller cases fail with the
guards removed, then pass restored. Historical compile-only red evidence is
explicitly distinguished in ROUND4.md from behavioral red proofs.

Changed executable production line coverage: 60/67 (89.55%). Two offline,
CGO-disabled builds from this source tree match SHA-256
17df59b373f7e59f0f556f179d6da5655449f65cd18921aabd74d8e168db1a5d.
Final lint reports zero issues; vet passes. Source/digest checks are retained
in round4-logs.

Deployment gates and residuals are in internal/session/REQUIREMENTS.md:
- Every session, work and relocated class leg must support real revision CAS;
  the pinned subprocess BdStore does not. Failed fences can park assignments.
- The live provider must expose its current runtime epoch and instance token.
- Stripped managed env can enter legacy claim behavior; preserve managed env.
- New incarnations must still clear current_claim_bead_id. Same-incarnation
  concurrent actual publications still lack an attempt on that session pointer.
- Acquisition/fence/publication/output and continuation siblings are separate
  operations. Provider metadata is not transactional.

Human review is required for lifecycle/execution. No live-upgrade approval or
new full host cmd/gc/networked T3 pass is claimed. The standards wrapper still
lacks the source tree's file-size baseline. The resource census passes its
tracked-file view; rerun after staging new files. Bazel was not run.

Full report: /tmp/ga-ypav-window/ROUND4.md, Round 4d.
ROUND4-SOURCE.diff is cumulative from c64fd0574; ROUND4D-SOURCE.diff is this
round's exact delta from 306c11a. The coordinator owns staging, commits,
host checks and release actions. A reusable playbook gotcha was queued.

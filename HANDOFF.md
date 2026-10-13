# harp-asg1 — ypav-a5 source handoff

Classification: internal operational evidence. 2026-10-13.

Operator ruling: adopt upstream v1.5.0 claim behavior and harden it (option C).
Authority: `/tmp/ga-ypav-round4-brief.md`. Baseline `c64fd0574`, branch
`ga-ypav-port2`. All changes remain unstaged and uncommitted for the coordinator.

Full review: `/tmp/ga-ypav-window/ROUND4.md`. It includes the accepted contract,
every changed file with line links, tests, backend findings, host commands,
remaining atomicity limits and two upstream bug-report drafts. The historical
round-3 contract question is resolved by the operator ruling; ROUND3.md remains
an immutable record of that earlier review.

Managed claim now binds own-row reads, conditional current-claim stamp/clear,
drain probes, checkout fallback and bounded drain acknowledgement to a verified
session incarnation. Foreign environment/argument targets refuse before querying
work. Controller ownership checks require both assignee and gc.session_id.
New incarnation creation clears the previous claim. Continuation assignment is
conditional. All round-3 changes, including city_config_provenance.go, are retained.

The full session suite and focused claim/CAS/regression suites pass. Final
combined test, coverage, lint, vet and reproducible-build evidence is recorded in
ROUND4.md and round4-logs. The source is ready for review before the October 14
15:00 CEST window; this is not live-upgrade approval.

Review limits: session/work/provider changes are not a shared transaction.
Provider metadata has no CAS, and the existing pending-unwind assignee guard
cannot distinguish same-assignee ABA. The report describes these remaining
races precisely. Pinned bd 1.3.1 lacks CLI revision CAS and must fail closed for
managed session stamps; the actual host-selected session backend still needs
confirmation. SQLite revision zero is valid and is handled by its backend CAS.

Host checks remain for the full cmd/gc suite (sandbox denies httptest sockets),
worker-profile VCS setup and the two documented platform custody fixtures.
The resource ledger must be rerun after including new untracked files. The
Harpoon standards wrapper lacks this repository's baseline; it was not fabricated.
Config and custody-pin changes still require human/owning review.

No live gc/bd operation, staging, commit, signing, installation, deployment,
credential access or upstream publication was performed. The coordinator owns
those next steps and any update of release/window/receipt authorization.

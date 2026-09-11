# ga-tmgr continuation2 source checkpoint — partial, not integrated acceptance

Canonical checkpoint passed READY at 2026-09-10T20:16:32Z through
/home/loucmane/gas-city-ops/plugins/gas-city-workflow/scripts/workflow.py.
The cached-entrypoint refusal remains historical. Sole-writer attribution was
recorded once through the supported owned ga-tmgr coordination surface; receipt
is /tmp/ga-tmgr-production-vw-continuation2-20260910/reconciliation-note.json.
No new context, goal, worktree, lifecycle, or source authorization was created.

## Actual source changes

- Schema-aware strict metadata traversal now uses the concrete decoded type.
  Struct keys require exact canonical JSON spelling (or Go field name for
  untagged fields); string-map keys retain case-sensitive semantics. Unknown,
  exact/escaped duplicate, case-alias, malformed and trailing input still refuse.
  Manifest, receipt and framed protocol decoding use their correct schemas.
- M1 regressions use valid self-digested manifest/receipt pairs, alias duplicates
  in both orders, nested Core fields, overridden unsupported schema/result and
  escaped duplicate names. Canonical positives, dynamic map keys including
  non-ASCII keys, and actual canonical plan-frame round trips are covered.
- Added a Linux metadata publication primitive with retained exclusive stages,
  data/mode/directory synchronization, anchored parent descriptors, exact
  preimage/postimage checking, single-link/owner/type/size checks, and single-use
  publication. It reports rename occurrence independently from later errors.
  A directory-sync failure after rename retains the committed image and returns
  renamed=true plus failure; there is no rollback or automatic retry.
- Focused filesystem regressions cover rename and post-rename sync failures,
  replay refusal, changed preimages/stages/parents, hardlinks, same-directory
  staging and preservation of existing failed stages. These are temporary-file
  tests, not namespace/process/confinement acceptance.

## Tests

m1-red.txt: expected compile failure for the new schema-aware helper signature.
m1-green.txt: focused metadata/parser suite PASS (0.713s).
publication-red.txt: expected missing-publication-method compile failure.
publication-green.txt: focused filesystem publication tests PASS (0.071s).
platforminstall-final.txt: owning suite FAIL on existing unknown-field diagnostic
compatibility; implementation corrected, failure retained.
platforminstall-final-02.txt: owning suite PASS (3.395s).
vet.txt: sandbox denied Go-cache write; no cache change or cleanup attempted.
vet-02.txt and build.txt: scoped vet/build exit0 after normal escalation.
git diff --check: exit0.

## Frozen evidence and preservation

Evidence root: /tmp/ga-tmgr-production-vw-continuation2-20260910.
source.sha256 SHA256 0dd7c8b00144df2868dee3a0dc2b67038a8b865cbd73163a18cc425efbcbb5c9.
source.tar SHA256 c120a4c2379cf131d7b0eedcba4f2731d1129b7c9c653723b0e7c0a5a46a9cab.
The six-file manifest/archive contain exact current changed files. before.tar
preserves platforminstall at this invocation's start. tracked.diff is cumulative
tracked worktree evidence, not a claim all its changes belong to this turn;
new-files.diff includes the affected untracked source files.
Index-before/after compare byte-identically; original20 manifest remains
537bdba61fb18c4fb44c5318910d619ac8e8ddedc7669593669e5d1d157630cc.
Original20 snapshot, five-file union baseline, both conflict archives, stage1,
M5 and all consumed synthetic runs/reviews were not modified or replayed.

## Explicit unfinished delivery

This is NOT the requested complete production V/W integration. The publication
primitive is not yet called by a confined production W; therefore it is not an
integrated transaction or runtime acceptance claim. No runnable production-path
acceptance command or launch plan is supplied as if one existed.

Still required: actual host image/version/boot/PID/start/listener/API/pidfd
verifier and lease client; exact production mount/input/env/runtime profile;
private confined writer entrypoint and exclusive bounded channel; full guarded
prepare/grant/publication/final state machine; current-runtime verification in
real consumers; no-op/partial/recovery and bypass regressions on those call sites.
The historical Lifecycle metadata-only path is not promoted to acceptance.
The independent stage1 HOLD is not rewritten as a PASS. These are unfinished
source work, not a new authorization or architecture blocker.

No production/runtime/provider PASS, root/probe/namespace acceptance, worker,
service/rig transition, install, signing, commit/push/merge, or goal change occurred.
Full original goal remains incomplete; ga-oz9e remains deferred/nonblocking.

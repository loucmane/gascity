# ga-ecwh.1 — Core evidence relocation design R2

Pre-implementation review only. No executor or live migration is claimed.
The separately tracked expired-waiver gate remains unchanged.

## Bound state

Core root: /home/loucmane/gascity-core-worktrees/ga-ecwh-typed-worker-receipts
Branch: codex/ga-ecwh-typed-worker-receipts
HEAD: a5a900f1173c8caccaa0bce23c946991ca52e50b

Operations PR381 merged the validated evidence-layout resolver at
89b713180e6b98653968f86c31d89094463e8699. Available signed Operations source
d76c380b28aa42d532dd2fb9d9de0cbe09666128 has tree
e401e471977f53d58240d51698861318decc8c69, identical to PR382 merge
b44b844b9f84487602232995f6ba90b4285ba5f4. PR382's attachment recovery has
already passed, including its original-pin no-op. Do not repeat it.

The read-only inventory records 78 entries (63 files / 1,280,111 bytes,
two relative symlinks, 13 directories), 39 protected files including nine
tracked historical plans, seven existing session-WAL JSONs, the index,
tracked diff and original workflow journal. It is a planning snapshot,
not an execution plan or permission to re-seal unexplained drift.
Review outputs are outside the relocation inventory under
.aegis/reviews/ga-ecwh-layout-20260908.

ga-ecwh.1 retains its Operations ownership; ga-ecwh and ga-ecwh.2 retain
their Core ownership. No new claim, attachment, Bead, route or session.

## Five moves, no broad directory cleanup

| Source | Destination |
|---|---|
| docs/ai/work-tracking | engdocs/workflow/work-tracking |
| sessions | engdocs/workflow/sessions |
| .plan_state | engdocs/workflow/plan-state |
| plans/2026-09-08-ga-ecwh-typed-worker-receipts.md | engdocs/workflow/plans/2026-09-08-ga-ecwh-typed-worker-receipts.md |
| plans/current | engdocs/workflow/plans/current |

Do not move the whole plans directory: nine tracked documents stay exact.
Leave empty old parent directories alone. No docsync exemption or ignore rule.
Both current links already use relative targets; preserve their link bytes.
The ownership journal and .aegis history remain at their existing paths.
Unexpected symlink/hardlink/special file, mode/owner drift, extra/missing entry,
destination collision or cross-device move must refuse.

## Config and active versus historical references

Create only the absent .codex/config.toml (0644), with:

```toml
[repo_structure]
sessions_root = "engdocs/workflow/sessions"
plans_root = "engdocs/workflow/plans"
plan_state_dir = "engdocs/workflow/plan-state"
work_tracking_root = "engdocs/workflow/work-tracking"
```

No permission/trust/model/provider fields. If a config appears before
execution, stop rather than overwrite or invent a merge.

First move reports, session and tracker files byte/mode-exact. Do not globally
replace old path strings in logs, reports, prior amendments or WAL history.
For the current plan only, derive and pin a deterministic postimage changing
the active front-matter evidence root, Header evidence summary, Plan Table
Evidence cells, Scope root and current reload instructions. Preserve existing
Amendments verbatim and append one relocation amendment linked to the original
backup. No status/Bead/owner/dependency/branch/scope-of-code changes.
Tracker checkboxes remain exact. Path notices are append-only; historical
session and tracker entries remain an exact prefix.

Run supported target-aware plan-sync last. Never author hashes manually.
Preserve every prior sync record, old path and ordering, plus the exact
original file image; allow only the new relocated-plan/current-tracker record
and serialization performed by the supported command.

## Transaction contract — corrected sequencing

Reuse workflow_lock, session_lock, SessionTransition and the demonstrated
transaction-aware plan-sync call. No new general transaction framework.
Implementation authority must be established separately.

The new parent inventory explicitly records engdocs as preserved 0755/1000:1000,
and engdocs/workflow, engdocs/workflow/plans and .codex as absent before /
0755/1000:1000 after. Include their creation in the outer write-ahead journal.
Rollback removes only these exact newly-created parents when proven empty.
Never remove existing engdocs or unrelated files.

Fix the backup path before deriving the current-plan postimage:
<Core-common-git>/gas-city-workflow/layout-relocations/<plan-id>/originals/.
Derive plan-id from the frozen preimages and reviewed transaction-policy digest,
not from a postimage containing plan-id. The current-plan amendment names that
fixed original plan backup and its SHA-256. No time/random path inside its
deterministic postimage. Preserve every prior attempt under its own identifier.

1. Validate source/merge, Core branch/base/index/diff, protected files, fresh
   Beads/typed graph, ownership, ready journal, no pending intent and the current
   daily session. Freeze final inventory after preparation bookkeeping; explain
   every change from the review snapshot. Unknown differences refuse.
2. Acquire workflow_lock then session_lock; recheck. Preserve verified exact
   originals, links, directory metadata, plan, config absence, index and journals
   outside moved trees, then write the outer relocation intent.
3. Under those locks perform only the five renames and declared parent creation,
   each with write-ahead intent/readback. Do NOT construct SessionTransition yet:
   its destination before-images would otherwise be captured as absent.
4. After all renames succeed, construct ONE SessionTransition(lock_held=True)
   over exactly .codex/config.toml, the relocated current plan and the relocated
   sync.log. These before-images now refer to their real post-rename locations.
   Use it to install config, plan postimage and generated sync state.
5. Use the demonstrated in-process caller shape:
   handle_plan_sync(argparse.Namespace(
       target_dir=<exact Core root>, plan=<relocated relative plan>,
       tracker=<relocated relative TRACKER.md>, folder=<exact ACTIVE name>,
       dry_run=False, _session_transaction=transaction)).
   Source evidence: the production Bead session-continuation path already calls
   handle_plan_sync with _session_transaction; the function first runs
   _configure_workflow_target(target_dir), which checks the exact Git root and
   loads that target's repo_structure into REPO_ROOT, PLAN_STATE_DIR and
   PLAN_SYNC_LOG. Config must already be installed and verified before this call.
   Invoke the merge-bound helper, not a forked/reimplemented hash writer.
6. Commit this inner leaf transaction before calling ordinary portable readiness:
   assert_no_pending_continuation correctly refuses while the leaf WAL is pending.
   Keep BOTH outer locks held. Then run normal context/readiness/ownership and
   owning docsync checks without a bypass, and verify candidate/index preservation.
   Do not call workflow.py recursively and reacquire the same repository lock;
   use its existing bounded validator functions with the same checks intact.
   The outer relocation is not a new Gas City runtime barrier. All project rigs
   stay suspended; no dispatch or handoff is permitted until outer PASS. Readiness
   after the fully installed leaf state is not itself migration acceptance.
7. Failure ordering is leaf rollback FIRST (including when a later read-only
   postcheck fails after leaf commit), THEN reverse exact outer renames/parents.
   Verify transaction-owned images before every restoration. Unknown postimages
   mean preserve and stop; never overwrite concurrent/unexplained state.
   Preserve failed postimages before restoring originals.
8. The moved/config/plan/sync data and directory state must restore exactly.
   SessionTransition's WAL is deliberately append-forward, not erased:
   - six existing archive files remain byte/mode-exact;
   - the prior current WAL is archived under its existing id, with verified
     byte/mode equality and original snapshot retained;
   - the new current WAL records this transaction complete or rolled_back.
   These are named expected journal deltas, not unexplained drift or a claim
   that the entire .aegis tree remains byte-identical. The Core ownership/
   attachment journal remains byte-identical throughout this migration.
9. Record outer PASS only after all checks, with exact accepted postimages and
   expected new WAL id/status. Prove a second apply is a read-only no-op BEFORE
   later logging advances state. Later legitimate changes refuse replay; they
   do not authorize rerunning the original migration.

Before implementing, confirm and test that SessionTransition.rollback can safely
be called by the outer controller after inner commit but before releasing locks.
The preserved source exposes that method, but composition requires regression
proof, not inference from this design.

Do not rewrite old attachment before/after hashes. The original workflow spec
remains the same root, branch and Bead. Actual consumer incompatibility is a
finding, not permission to fabricate authority.

## Required tests before actual live application

Disposable fixtures only; no live rigs/providers, installation or old cleanup.

- Old layout reproduces public-doc/session-root refusal; new layout passes
  actual modern Beads checks and correct context-root resolution.
- Nine tracked plans, implementation/index/HEAD, ownership and historical
  journals remain unchanged; both links remain target-local.
- Reports preserve bytes/modes; active plan has only declared current-reference
  edits and appended amendment; historical log prefixes/sync records survive.
- Inject failure after every move, config write, plan rewrite, sync and
  validation; exact rollback and failed postimages are proven.
- Reject unexpected paths, links, hardlinks, ownership/source/config drift,
  destination collisions, unknown partial states and unsafe replays.
- Exact second application is a no-op; later state drift refuses.
- Owning docsync passes on the actual migrated Core tree before acceptance.
- Independent review of the executor and RED/GREEN evidence still precedes
  live application. Design PASS is not implementation or activation PASS.

No runtime waiver retirement, source delivery, Claude worker/signing proof
or ga-ecwh.1 closeout follows from this design. Keep rigs/services unchanged.

## Review questions

1. Do the five moves/four configured roots cover the merged resolver without
   touching the nine historical plans or .aegis journals?
2. Are active references and immutable historical evidence separated correctly?
3. What concrete lock/WAL/generated-state/rollback gaps remain, particularly
   around supported plan-sync?
4. State any implementation/activation blocker, without inventing an executor,
   test result or permission. Return concise actionable PASS/HOLD.

## Verified plan-sync integration detail

The supplied source at scripts/codex-task:2426-2464 already accepts
args._session_transaction. With it, handle_plan_sync routes the generated
sync-log bytes through transaction.write instead of the bare write_text path.
Use this existing in-process transaction-aware call under the held locks after
its normal target configuration and validation; do not call the bare CLI writer
and then try to reconstruct its partial postimage. The executor still needs a
failure-injection proof that sync and relocation rollback compose correctly.

## R1 disposition

R1 remains preserved as HOLD. R2 resolves its ordering gap with rename-first / one
post-move leaf transaction, supplies the actual existing caller and target resolver,
records parent creation/rollback, fixes backup identity before plan rendering,
and classifies the expected append-forward WAL delta explicitly. No test or
migration was run while correcting this design.

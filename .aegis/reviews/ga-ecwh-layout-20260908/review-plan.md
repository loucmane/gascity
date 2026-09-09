# ga-ecwh.1 — Core evidence relocation design

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

## Transaction contract

The existing SessionTransition proves atomic leaf writes and byte/mode
rollback; it does not implement directory relocation. Reuse its existing
locking/image primitives where applicable, not a second general framework.
Implementation authority must be established; this design grants none.

1. Validate exact reviewed Operations source/merge, Core branch/base/index/diff,
   protected files, fresh Beads/typed graph, ownership, ready journal, no
   pending workflow/session intent and the current daily session.
2. Acquire locks in supported order (repository then session); recheck all
   preconditions. Freeze a final exact inventory after preparation bookkeeping.
   Explain and record every delta from the review snapshot, never blindly reseal.
3. Before the first move preserve and verify originals, directory metadata,
   symlink bytes, active plan, config absence, index, session/ownership journals
   in a fresh append-only backup outside all moved trees.
4. Write intent before each same-filesystem move and each config/plan change;
   verify resulting paths and images. Multiple roots are not globally atomic:
   keep supported writers locked and refuse readiness/dispatch during transition.
5. Install exact config and current-plan postimage, then supported plan-sync.
   Pin and preserve all generated writes. Inspect lock behavior first: never
   re-acquire a lock already held by this process via a subprocess.
6. Validate context roots, scaffold/plan/session/tracker parity, live ownership,
   attachments, source/index non-interference and owning Core docsync.
   Enumerate any validation-created WAL entries; preserve predecessor history.
7. On known technical failure, preserve failed postimages and restore only
   exact transaction-owned states. Reverse moves and active/config writes,
   restoring directory metadata and original inventory/index/diff. Unknown
   postimages/concurrent changes mean stop, not overwrite or best-effort repair.
   Never restore an old WAL over unexplained new records.
8. Record PASS only after all checks. Repeat apply once on the exact accepted
   poststate as a read-only no-op before later logging changes that state.
   Later legitimate changes are not authority to replay an old migration.

The primary workflow spec stays the same root/branch/Bead. Preserve old
attachment before/after hashes as historical evidence; do not rewrite them
to make old recovery replay pass after a new legitimate migration.
Any consumer incompatibility is a new finding, not a reason to skip checks.

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

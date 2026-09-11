# ga-tmgr — remaining outcomes after initial combined R3 PASS

Administrative map, 2026-09-10. No source/test/inference/runtime operation here.
Core paths below are relative to the SAME ga-ecwh Core worktree:
`/home/loucmane/gascity-core-worktrees/ga-ecwh-typed-worker-receipts`.
Current HEAD96e0ed0423c77ecc44237b4f9c3e667648c74a48; attached repair ga-tmgr.
This map preserves the full objective and existing authority/gates, not a new plan
architecture, approval gate, source context, daemon, wrapper or deferred feature.

## Completed, with exact limits

Initial combined R3 mechanism PASS: parent tool8615db exit0, separate validator
toolb4bdaf actual0/exit0. No command or validator was repeated here.
Exact runtime report: `combined-runtime-r3-observation-20260910.md`,
SHA2563ab2eedb740117ee99e818594480f3f7a2c7f4fba196e8f0dce235cafe39b4bc.
Original Opus HOLD `combined-r3-opus-source-hold-20260910.md`,
SHA256cc5d3a7e13dc20482ba310c9044f07d33809a72e753b17a8298d35cc6542e227,
is unchanged. `combined-r3-review-supplement-20260910.md`,
SHA25606a29bebe2bdfa424980348231dacd1d910080e39982128c9b808a050e7e5143,
and `combined-r3-astra-adjudication-20260910.md`,
SHA256d99bba41e4330c3d4bf414015c7922b420936797c8ea1ac9685c7e02716b666f,
resolve the omitted CLOEXEC premise without changing source/guards. Opus did
NOT return PASS. Empty Env/no ExtraFiles is not a general non-CLOEXEC guarantee.
R2 remains consumed INCONCLUSIVE; M5 retains its separate exact feasibility PASS.

Five R3 cases passed their expected dispositions, including13 cache negatives/
4 process negatives plus fresh positives for each started writer, unchanged
cache inventories, exact metadata results and terminal evidence. Expected
refusals and committed_evidence_incomplete are not adoption success.
Version-negative varied expectation, NOT executable image. Epoch-loss injection
was NOT real exec/exit. The synthetic old schema was NOT a Core decoder test.

## Remaining six acceptance groups (not yet proven)

| Remaining outcome / protocol binding | Existing synthetic code/tests | Existing production seam and smallest missing evidence |
| --- | --- | --- |
| Actual exit/restart/PID reuse/exec ABA around lease/grant/rename — v1 P12/P18, R2§2, R3§§1–4 | `runtime_linux.go:serveSupervisor`, `transaction_linux.go:verifyHost/runVerifier`, `runner_linux.go:runCase` under test/combinedprobe; `protocol_test.go:TestLeaseExclusiveAndBounded` is pure only | `cmd/gc/platform_lifecycle.go:114 inspectPlatformRuntime`, `internal/platforminstall/lifecycle.go:187 validateRuntimeProof`, existing supervisor/API code. Add real disposable process generation/death cases and phase witnesses; require precommit abort/refusing partial, postcommit committed_evidence_incomplete, no rollback/replay/stale PASS. Current in-memory epoch injection is insufficient. |
| Endpoint substitution / same-build impostor / wrong image — v1 P02/P12, R2§2 listener binding | `test/combinedprobe/runtime_linux.go:274 procIdentity`, `query`; `corrections_linux_test.go:157 TestVersionExpectationIsIndependent` | `cmd/gc/platform_lifecycle.go:114 inspectPlatformRuntime`, `cmd/gc/drift_client.go`; `internal/api/supervisor_test.go:466 TestSupervisorHealthIncludesBuildID`. Exercise another disposable listener/process and independently changed executable bytes, not just a changed expected version; fail identity binding before metadata mutation. |
| Pause/expiry at receipt rename; crash/EOF/fsync boundaries — v1 P04/P14, R3§§1–4 | `test/combinedprobe/protocol.go:165 publish`, `transaction_linux.go:340 runWriter`; `TestPublicationFailureNeverReplaysCommitted`, `TestUnsupportedReceiptAndExpiredGrant` are pure | `internal/platforminstall/installer.go:659 writeAtomic`, `adopt.go:117 adoptPrepared`, `lifecycle.go:161 finalizeActivationReceipt`; `metadata_adopt_test.go:105 TestMetadataOnlyAdoptionReceiptFailureRollsBackMetadataOnly`. Add bounded disposable pause/failure observations on both sides of irreversible rename. Preserve files; do not infer kernel-atomic clock/rename or use general rollback after commit. |
| Concurrent committed-pair readers / competing transaction — v1 P13/P14, R2§3, R3§1 | `test/combinedprobe/protocol.go:139 committedPair`, `transaction_linux.go:pairOnDisk`; `TestPairReaderAndIrreversibleDisposition` only supplies canned byte snapshots | `internal/platforminstall/installer.go:284 preflightPlatformMetadata`, `integrity.go:231 inspectReceipt`, `manifest.go:161 LoadManifest`. Exercise actual readers through the manifest→receipt window and competing lease/preimage attempts; reject mixed/torn/pending pairs, no second commit. |
| Frozen old Core decoder compatibility — v1 P11/P16, R2§3 consumer list, R3§4 | `test/combinedprobe/boundary_test.go:68 TestUnsupportedReceiptAndExpiredGrant` only tests synthetic schema | Execute EXACT preserved `internal/platforminstall/manifest.go:161 LoadManifest/179 decodeManifest` and `installer.go:837 decodeReceipt` against proposed new schema, with valid old-format positives. Existing `manifest_finalize_test.go:34` and `installer_test.go:18` are adjacent owning tests. Do not substitute a copied lookalike decoder. Bind original20 snapshot manifest537bdba61fb18c4fb44c5318910d619ac8e8ddedc7669593669e5d1d157630cc and actual compiled decoder inputs. |
| Supported ACL mutation coverage — v1 P05/P07, R2 M5 | `test/combinedprobe/boundary_linux.go:103 mutations`, snapshot and existing13-operation matrix | `internal/platforminstall/metadata_adopt.go:80 checkCandidatePackCacheReadOnly`; `metadata_adopt_test.go:125 TestMetadataOnlyAdoptionValidatesCacheOnInitialApplyAndReplay`. Add fixture-only ACL positive/negative and unchanged ACL inventory where supported, including relevant descendant path. Unavailable positive control is unsupported/INCONCLUSIVE, never a denial PASS; no live cache or permission change. |

All synthetic references above resolve inside test/combinedprobe unless qualified.
These groups are not a claim that every other v1 P01–P18 row has full coverage:
retain unproven path/IPC/replay/corruption cases and owning regression obligations.
No new probe is authorized or executed by writing this map.

## Smallest next chunks, in the preserved usability-first order

1. **Remaining synthetic acceptance delta:** extend the existing test/combinedprobe
   fixture/state boundaries for the six groups, using the existing pure tests as
   seams and exact frozen old decoders for compatibility. Do not rewrite the
   proven confinement/transport. Freeze one consolidated changed-input candidate,
   focused evidence and existing independent review before its new execution.
   This is the next implementation deliverable, not another isolation study.
2. **Preserve and finish Core repair:** after that evidence, implement only the
   reviewed V/W/lease and phase-aware metadata receipt/consumer delta in existing
   seams: metadata_adopt.go, adoptPrepared, RuntimeProof/Receipt, writeAtomic and
   the existing supervisor API (no new service). Retain no-repair cache lock,
   artifact/import/preimage checks and original20 snapshot. Update the R2§3
   consumer set together: preflight/integrity, cmd_platform.go:253, doctor
   checks_platform_install.go:25, managed_product_dispatch_gate.go:149 and
   cmd_platform_canary.go:179. Existing tests using exactFakeLifecycle are not
   genuine host/confinement proof. General installer rollback remains separate
   from the new irreversible metadata commit semantics.
3. **Existing Core delivery gates:** independent exact-candidate review, required
   final owning/full suites and hosted CI, managed signatures and reviewed merge/
   activation boundaries; reuse completed PR36 and Operations recovery, never
   redeliver them. A future supervisor lease handler is not active merely because
   source merges. Metadata reconciliation/platform integrity must meet its
   existing live authority and freshness requirements; no activation occurs now.
4. **gct-13ku then gct-10pg:** preserve the reviewed native-start package and
   original source-edit HOLD; the last recorded doctor had eight provenance
   differences, not a fresh PASS here. Reconcile those exact current inputs
   before any worker window; no guard bypass. Template handoff
   `/tmp/ga-ecwh-template-interop-handoff-20260909.md` locates
   `bin/gct-managed-worker-provision` / `bin/gct-managed-worker-canary`:
   prove real Python→Go typed receipt/control-policy/selected-profile bytes and
   Go→Python runner inputs, retaining legacy checks. gct-10pg's preserved
   selectable-model-profile worktree/candidate is reused, not recreated.
   Reconcile latest R13 preparation/HOLD and consumed R12, not old attempts or
   an assumed accepted review. Ordinary Template implementation stays in the
   authorized Gas City worker lane; ga-tmgr's direct-source exception is not
   a Template/native-delegation fallback.
5. **Useful Claude task:** Fable stays human-facing orchestrator; prove one real
   bounded Gas City worker task end-to-end (setup, edit, tests, checkpoint,
   managed signature/receipt, reviewable delivery, supported closeout), with
   subscription/workspace/control/permission/sandbox/profile-bound evidence.
   No silent Codex completion, candidate-only parity or claimed signer success.
6. **Both handovers:** prove Claude→Codex AND Codex→Claude with the same Bead,
   branch/worktree, staged/unstaged/untracked content, authorization and evidence,
   one active owner/worker, no repeated completed mutations, negative permission
   cases and zero residue. Publish existing Aegis/Bead/Obsidian recovery evidence.
   Close acceptance only when actually proven; no Bead closes in this checkpoint.

## Reuse versus fresh evidence

Reuse immutable M5/R3 observations, exact original20 source tests, review history,
completed delivery/context-recovery evidence and unchanged control algorithms.
Never rerun consumed M5/R2/R3 attempts to manufacture a new PASS. Source matching
alone does not attest a new runtime generation. Changed helper, runtime, argv,
environment, mount, lease/receipt/consumer schema or fixture inputs require their
own input-bound source/runtime evidence; fresh platform/provider/signing and
handover claims need their actual current sessions and receipts. Partial pure
tests cannot stand in for real races, old decoders or full final CI.
No new goal, architecture, authorization gate or deferred feature. Parent goal
remains active/full/incomplete; ga-oz9e stays deferred/nonblocking.

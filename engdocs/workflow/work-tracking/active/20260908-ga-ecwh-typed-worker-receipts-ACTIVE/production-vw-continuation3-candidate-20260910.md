# ga-tmgr production V/W continuation3 source candidate — 2026-09-10

## Disposition
Implemented a callable versioned production source path, not merely protocol declarations.
Focused offline tests, vet and static build PASS; independent source/runtime acceptance is still HOLD.
No candidate binary, namespace/probe, installed metadata, service, rig or worker was executed.
Parent retains the sole active full goal; ga-tmgr stays attached to READY ga-ecwh.
No commit, staging, signing, push, merge, goal or deferred ga-oz9e change.

## Exact candidate
Evidence root: /tmp/ga-tmgr-production-vw-continuation3-20260910
- source.sha256: 8f0f39aeef9f43d59cb104fc0ed7caf9e13277b94f5b60299eabccaa3621d1c5
- source.tar: faca36ed31e95ebe014306594fdc5da17ea53c124972cdc229d6956e8d4d5feb
- continuation3.diff: 10131a66151ef131ca3fb4f4d9ebda1d40fd3b7e0067ba6aa1e503f16071c943
- gc-source-candidate-reviewed-input: 0410d655a755e8e05b60cee7e631f308ce0435fc8da78de68736d61e210fe552
- source-files.txt lists complete final candidate inputs; source.sha256 hashes each.
- tracked-complete.diff plus new-files-complete-final.diff include all changed/new production source.
  The archive includes earlier API/schema/M1 changes and original staged repair inputs.
- continuation3.diff isolates this invocation from before.tar for platforminstall/main/dispatch;
  packman and CLI test deltas use their unchanged pre-turn index baseline.
- Build command: CGO_ENABLED=0 go build -trimpath -o <evidence>/gc-source-candidate-reviewed-input ./cmd/gc
  This binary was built, NEVER executed. No production launch manifest or execution authority is invented.

## Actual integration
1. metadata_host_linux.go observes actual image/path/hash, boot/PID/start/listener ownership,
   pidfd terminal state, literal observed-executable version, API BuildID and fresh exclusive
   nonrenewing lease. Refuses absent API support, endpoint aliases, redirects, auth refusal,
   incomplete proc evidence and any stale binding. V publishes no metadata.
2. metadata_sandbox_linux.go and metadata_protocol.go bind a static writer, exact bwrap/loader
   closure, explicit links/absence/import digest, namespace/environment/FD boundary and
   preexisting output-parent identities. Protected cache stays read-only with existing shared flock.
   No new cache lock, repair, broad home/root/city mount or ambient credential/provider authority.
   Writable parents reject unrelated entries and hard/symbolic aliases; unsupported layouts refuse.
   Tree/import reads preserve atimes using O_NOATIME, with no permission fallback.
3. metadata_transaction_linux.go is the actual V/W state machine: bounded anonymous fd3/4,
   fixed sequence/request/nonce/instance/deadline, retained stages/intent/backups, current-runtime
   grants, exact preimage checks, manifest publication, irreversible final receipt rename/fsync,
   final observation and successful writer wait/pidfd terminal evidence.
   Postcommit errors remain committed_evidence_incomplete, never rollback/replay/PASS.
4. Public AdoptMetadataOnly{Plan,apply} route through RunMetadataTransaction; cmd/gc's private
   exact writer entrypoint routes to MetadataWriterEntrypoint. Legacy Lifecycle is not a fallback.
   General install/adopt/rollback refuse the v2 irreversible contract and otherwise retain v1 behavior.
5. Existing artifact/source/destination/backup, import, pinned Git/provider and cache validation
   remains in W. Strict cache checking rejects absent transitive local pack inputs; helpers have
   bounded output/deadlines, empty stdin, no inherited channel FDs and remain confined.
6. Existing captured receipt/manifest/receipt reader and schema-aware M1 decoder are reused.
   V2 dispatch and its shared canary path invoke fresh VerifyMetadataRuntime.
   Doctor remains integrity only. Non-Linux entrypoints refuse unavailable confinement.
7. METADATA_TRANSACTION.md describes callable CLI, frozen read/write/mount/env contract,
   explicit unsupported cases, lease/commit semantics and remaining acceptance limits.

## Preserved permission boundary
An attempted extension of fresh verification to ALL legacy dispatch was rejected by tool auto-review:
"The patch removes the metadata-only guard and makes host-runtime verification mandatory for every
product dispatch, broadening the authorized repair and risking disruption to existing non-metadata workflows."
The rejected patch was NOT applied, retried or bypassed. Legacy v1 dispatch remains unchanged.
Therefore the audit's historical-evidence gap is closed for v2 only, not for all legacy acceptance.
Applying this extra requirement to legacy product dispatch needs explicit applicability approval.
This is not a new architecture investigation or an excuse to omit the authorized v2 integration.

## Validation and honest limits
- platform-final-03.txt: go test ./internal/platforminstall -count=1, PASS (3.500s).
- packman-final-02.txt: focused TestMetadataStrict, PASS (0.027s).
- api-lease-final.txt: TestSupervisorMetadataLease fixture handlers, PASS (0.044s).
- cli-final-tests-02.txt: owning platform install/adopt/metadata-only/rollback/manifest/register
  selection, PASS (0.290s).
- vet-final-03.txt: platforminstall/packman/api/cmd-gc vet exit0.
- build-final-03.txt: static candidate build exit0.
- closure-final.txt and earlier RED/GREEN logs retained; earlier binaries/logs were not overwritten.
- Tests use injected proc observations, pure frame/lease bindings and isolated file/handler fixtures.
  Legacy fake-Lifecycle fixtures are explicitly legacy regressions, not runtime V acceptance.
  New tests reject unknown absence, runtime symlink drift, output aliases/unrelated files,
  unbounded helper output, changed request identity and attempts at general v2 rollback.
- M1 owning tests and publication fault-injection tests ran in the focused owning package.
  No full Core suite, frozen old-decoder execution or consumed synthetic run was repeated.
- API/CLI fixtures preceded the final import-noatime/runtime-link-only delta; final owning tests,
  vet and static build include that delta. These checks do not prove live lease/confinement behavior.
- The initial diff-freeze shell used zsh's reserved path variable, causing "command not found: git";
  it stopped before later operations. Empty new-files-complete.diff is retained failed evidence;
  new-files-complete-final.diff is the successful append-forward artifact, not overwritten history.

## Preservation and next boundary
before.tar and baseline-preserved.txt retain the continuation2 union; index-before.z == index-after.z.
Index SHA256 dbf7c6ccb1ab0c1573b33e0d466df2eae10d52d41bdec386522309c583ec9e86.
Original20 manifest SHA256 537bdba61fb18c4fb44c5318910d619ac8e8ddedc7669593669e5d1d157630cc.
Its frozen source.txt remains 50ff71be831aa60aa86e1157fe4e947dae84a2feedd09382cf94f8ed5561021a.
All earlier conflict archives, HOLDs, M5/R2/R3/R4/R5/R6 source/reviews/runroots remain evidence.
core-checkpoint.json records the same READY parent/branch/HEAD via canonical workflow entrypoint.
One owned ga-tmgr note and unique supported Core/Operations log follow this report; their exact
receipts, final verify, queue and link readbacks reside in this evidence root.

Next: independent consolidated source review of this exact archive, including host observation
races, descendant/channel lifetime, complete real dependency closure, publication failure paths
and v2 consumers. Then an independently reviewed, fresh input-bound disposable production-path
fixture is required; R6 PASS is inherited algorithm evidence only, not acceptance of this helper.
Final required owning/full suites, hosted CI, review/signing/delivery and separate activation
authority remain. Core repair -> gct-13ku/gct-10pg -> useful Claude task -> both handovers remains
the original acceptance order; provider parity and full race/decoder acceptance are not claimed.

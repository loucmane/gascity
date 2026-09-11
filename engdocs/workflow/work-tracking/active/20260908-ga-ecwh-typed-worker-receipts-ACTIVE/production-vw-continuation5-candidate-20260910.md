# ga-tmgr continuation5 — outer-disposition replay correction

## Outcome
The single reviewed M3 replay correction is implemented and pure validation passes.
Independent delta review remains required; overall M2 SOURCE HOLD is unchanged.
Automatic v2 NOOP/completed idempotence is unavailable without a supported completed
outer disposition. This is deliberately conservative, not a silent success reclassification.

Read completely and rehashed:
- /tmp/ga-tmgr-production-vw-continuation5-replay-fix-20260910.md
  fb4cfb7ecc9e796250a9a6aa12ca54c580d7526ac5beed24c308e79a26b31409
- /tmp/ga-tmgr-production-vw-continuation4-astra-review-20260910.md
  95542a0d5c8aebc17b97a0312e811c6cbc08f3ae04c36350ba25fed1fb920732
- /tmp/ga-tmgr-production-vw-m2-architecture-adjudication-20260910.md
  d8da8a166dde1d21ca5b0cc0dffda3fb6ffe6c9497b9e5e0cbe0d8d20e6d149f

## Exact frozen candidate
Root: /tmp/ga-tmgr-production-vw-continuation5-20260910
- source.sha256: 2e7f559e1cf55d006c622e172e08684af57f4419d5e0dcb9dc7800bdfccb1714
- source.tar: d157e8b9696b02763b6f1e2a1507a6fce7ad4e16c6583bf926bc1e5821410f7a
- continuation4-to-5.diff: 552bfb42b36a64ff15f9bef39780f53649615b97313633e0a1deff8455c7bd06
- changed.sha256: c5fda7a52970acf8d04ad53286596527ba9e7aac382adf242d46b268573ee4b8
- gc-source-candidate: ba2226f2edddb01b7ab0485a7ad036184ad710e5448898663cd3ea5c06067896
The static binary is unexecuted; no runtime command or acceptance is authorized.

Exactly three source/doc files differ from continuation4:
- internal/platforminstall/metadata_transaction_linux.go:
  84558edc8a87f97d2ffd917c9232efa814700869bc84e007a61fd55d27b7b823
- internal/platforminstall/metadata_replay_linux_test.go:
  04498c9b4e60e86f7edbb72072854404001b0a5809f3dc6f16cf55ad1df15fbf
- internal/platforminstall/METADATA_TRANSACTION.md:
  5184e0e7959783c5183801e0308a21f16d29b2bc48e65665adb17a79558e3ca1

## Conservative supported-disposition rule
W now refuses an existing committed no-op candidate immediately after real metadata
preflight, before further helper validation or publication. A matching persisted FINAL
does not prove the later outer wait/deadline/expiry completion succeeded.
The pre-exit FINAL eligibility check and automatic W NOOP branch are removed.
V independently rejects any NOOP response; its successful NOOP reclassification branch
is removed. Fresh PLANNED/transaction processing, purpose separation, two-handler leases,
nonrenewal, writer wait/pidfd/deadline checks and expiry-wait errors remain unchanged.

The refusal also applies to dry-run of an already committed pair. Fresh observation and
fresh dry-run-to-apply composition are unchanged. No arbitrary completion proof file,
database, V metadata write, deletion of FINAL, committed rollback or renewal was added.
ReadCommittedPair remains usable for read-only diagnosis. This checkpoint introduces
no recovery authorization path and does not pretend a supported completion record exists.
Legacy v1 behavior is byte-identical; the earlier legacy extension is not a blocker here.

## Persistent-state RED/GREEN proof
metadata_replay_linux_test.go writes a genuine self-digested v2 committed pair and
matching FINAL into isolated fixture files, verifies them through ReadCommittedPair,
injects expiry-wait failure, then performs the next invocation's real preflight.
RED exercised the extracted unchanged old FINAL gate and failed specifically because
that later state was accepted as automatic NOOP (replay-red.txt).
GREEN exercises the production replay gate and requires the explicit unknown-outer-
disposition refusal. Fresh nil-receipt state remains eligible. All persisted fixture
bytes remain identical, and captured-pair diagnosis remains readable afterward.
This models persistent state and injected completion failure, not actual writer/process
execution or proof of host/namespace behavior.

Validation:
- replay-green.txt: focused replay, observation, expiry, composition and publication PASS, 0.111s.
- platform-final.txt: one final owning package run PASS, 4.647s.
- vet.txt: go vet ./internal/platforminstall ./cmd/gc, exit0.
- build.txt: CGO_ENABLED=0 go build -trimpath -o <root>/gc-source-candidate ./cmd/gc, exit0.
- diff-check.txt: git diff --check, exit0.
No generated API work, full Core suite, prior synthetic replay or new probe execution.

## M2 decision continuity only
The independent adjudication linked above recommends prospective connection-bound
responder verification PLUS source-proven accepted-socket custody over its lifetime.
Tuple/inode matching or listener provenance alone is not that contract.
It changes global listener-exclusivity guarantees, needs an explicit operator decision
and later independently reviewed implementation. No decision has been granted.
No transport, scanner, custody implementation, new exclusion or root capability changed.
The entire metadata_host_linux.go and legacy dispatch file compare exactly to continuation4.

## Preservation and owned checkpoint
Same READY ga-ecwh parent with attached ga-tmgr; existing ga-e0t1 Operations evidence context.
Canonical workflow checkpoint/log/coordinate/verify only; no new context, goal or executor.
Original index remains dbf7c6ccb1ab0c1573b33e0d466df2eae10d52d41bdec386522309c583ec9e86.
Continuation4 archive remains fb8a5ab7dc9fef39da2382884be12f207e2172f69bbf1124eb088e50a0604000.
Original20 manifest/source and earlier attempts remain exact; preservation.sha256 records them.
One owned ga-tmgr note plus current plan/session/handoff/tracker links accompany this report.
Final ownership, queues, source hashes and both workflow verifies are read back in this root.
Full parent goal/history/order and deferred ga-oz9e remain unchanged.
No stage/sign/push/merge/install/lifecycle, runtime, inference, credential or service operation.
Next: independent exact-delta review; M2 remains a separate unapproved operator decision.

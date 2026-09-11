# ga-tmgr combined synthetic R1 — source checkpoint

Prepared 2026-09-10 in the existing READY ga-ecwh parent with attached ga-tmgr.
The original full goal remains active/incomplete in the parent; no goal change.

## Accepted milestone (not repeated)

Parent-only M5 R3 execution and separate result validation both exited0.
Observation SHA256: 8b3f4fb752dd23aa0dbcc247a72c204c2b96cb751068cf1d3958b1882cd58242.
Result SHA256: d0b026583d2e499bb86b6bbc02d475845befdf147d05ad31c124f17ffc914c36.
Source PASS review SHA256: 2a3e04e7c3fff3858ca029568b970ec9e9ff31ed84465a334d7e9d72f37ad2ad.
Review verification SHA256: e767f15d16af252d98d8e3b4ffae29502e380605c37a8185e412c73db16fab69.
All references rehashed. Earlier source-only HOLD and all failed evidence remain historical.
No M5 rerun, source/binary/result modification or new inference occurred.

## New source-only deliverable

Only test/combinedprobe/ was added. Separate pure protocol/state-machine,
runtime identity/lease, confined transaction, runner and copied M5 boundary modules.
Four planned cases: success, unsupported guard, precommit lease loss, postcommit
lease loss. W writes only synthetic metadata; V publishes none. Exclusive anonymous
pipes replace a socket transport (SCM_RIGHTS is not representable), with bounded
closed frames, exact nonce/request/channel/sequence/epoch and monotonic deadlines.
Final receipt rename is irreversible; all later failures preserve committed
evidence incomplete. Known precommit partials stay preserved/refusing, not repaired.
No production Core API, metadata reader, supervisor, policy or dependency changed.

## Frozen artifacts

Directory: /tmp/ga-tmgr-combined-candidate-r1-20260910/
- source.sha256: c702cab786df33120b9060176e14428a26d87cd65cd552c812a7ad8536dde3bd
- combined-source.tar: 4e677fcc544269c9750abb53a442e1c3dc14368fed585cae44d184dee49bb656
- combinedprobe: 35f6ad3c8fdda78d7dfa1d97a8e431f1402a42ccd173fe7ea8cd2b0466777607
- execution-plan.json: 2052c6aeda9a4af859642c53560cd0e79d33dbdccaafeaef9ac23fa83532d276
- offline-red.txt: missing definitions reproduced before implementation.
- offline-final.txt: go test -count=10 ./test/combinedprobe PASS.
- vet.txt / build.txt: focused vet and CGO-disabled static build PASS.
- original20-preserved.txt, m5-source-preserved.txt, index-before.z/index-after.z:
  all original20 and M5 source digests exact; index byte-identical.

The freeze subcommand performed only runtime/ELF/hash/absent-runroot checks.
No namespace, real process test, listener, prototype transaction or review ran.

## Exact proposed execution — NOT AUTHORIZED BY THIS REPORT

Only after independent consolidated source acceptance and parent execution decision:
```sh
/tmp/ga-tmgr-combined-candidate-r1-20260910/combinedprobe execute-reviewed /tmp/ga-tmgr-combined-candidate-r1-20260910/execution-plan.json 2052c6aeda9a4af859642c53560cd0e79d33dbdccaafeaef9ac23fa83532d276
```
Exact absent runroot: /tmp/ga-tmgr-combined-run-r1-20260910.
Whole120s, V25s, W23s, supervisor35s, grant20s, lease22s, HTTP2s.
UID1000 and exact host namespace/runtime pins required. Finite disposable loopback
supervisors, ptrace/memory/signal/FD controls target only the private fixture.
All files retained; EOF/targeted own-process termination and pidfd/reap witnesses.
Acceptance requires genuine outer exit0 and separate validate-result with that exit.
No root, live service/cache/metadata, worker/rig, package, signing or delivery.

## Decisive proof boundary / focused review questions

Combined runtime remains HOLD, not P01/P05/P08/P11/P12 PASS. New helper/request
mount/channel inputs invalidate unqualified M5 runtime reuse. The initial source
models lease generation loss, not actual exec ABA, death or endpoint substitution.
Still missing before a FULL decisive proof: those actual lifecycle-race fixtures,
pause/expiry across rename, concurrent pair-reader cases, old frozen Core decoder
execution against the new schema, and supported ACL coverage. The synthetic old
schema is not evidence that existing Core decoders ran. No hidden PASS credit.

Remaining mechanism acceptance decision is whether this exact exclusive-pipe,
host-verifier/single-instance-lease/confined-writer topology satisfies the approved
trust contract after independent source review and combined runtime evidence.
No new architecture, privilege or wrapper campaign is proposed. Review must check
listener identity, FD custody, bounded teardown, fault provenance and irreversible
publication semantics. If decisive proof remains unavailable at17:01Z, retain
this exact boundary rather than expand investigation. Stop here for source review.
ga-oz9e remains deferred/nonblocking; original candidates and history preserved.

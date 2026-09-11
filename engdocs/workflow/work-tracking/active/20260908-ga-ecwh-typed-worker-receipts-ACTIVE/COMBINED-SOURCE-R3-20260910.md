# ga-tmgr combined R3 — child runtime initialization correction

SOURCE CANDIDATE. No R3 runtime probe or independent inference performed.
R2 is consumed and will not be replayed. Original full goal remains unchanged.

## Reassessment preserved

/tmp/ga-tmgr-combined-r2-runtime-reassessment-20260910.md
SHA256 3e9c21c29bcce7979edfe8d2239478c7f843f346ae3624a5be51420ab3b78f51.
R2 actual Opus SOURCE PASS followed by one actual exit1; validator also exit1.
V refused fd3 /sys/fs/cgroup/init.scope/cpu.max before version/lease/W Start.
Setup fixtures and positive controls DID run; no metadata transaction occurred.
Consumed result.frames SHA256 4a00cd40111ca1a683b0125017842c7e067e0b5bb59dafbf79b036cebb62d6d5.
R2 review/runtime archive: r2-runtime-review-preserved.tar. No cleanup or rerun.

## One narrow correction

Installed Go1.26.7 runtime/cgroup_linux.go (SHA256
a2237d59862ebb1fd5f85750663cfe03965463a6d6bf21e5a411df26504f5f7f)
retains the cgroup FD before main when containermaxprocs is enabled; disabling
it closes the FD after the initial read. proc.go calls this initialization
before handling GOMAXPROCS. Exact named runtime sources were read and rehashed.

Every child constructor now replaces its environment with exactly:
GODEBUG=containermaxprocs=0
No parent environment is read/merged or changed. W's exact namespace argv adds
--setenv GODEBUG containermaxprocs=0 after --clearenv. The plan binds the sole
child_environment entry and full argv. Missing/extra/duplicate/changed variables,
GOMAXPROCS-only substitutes and changed/extra argv overrides refuse.

fdInventory is BYTE-IDENTICAL to R2, SHA256
8c2f3cc0cf309cb8141e9a3454e800966addd820240fd76b5857cfe78e69c7a4.
No descriptor allowlisting/closing, namespace/pipe/deadline/version/commit/
negative-control change. R2 fixed observation framing and prior fixes unchanged.
protocol.go, transaction_linux.go, observation.go and corrections_linux.go were
compared byte-identical. Only fresh R3 paths/transaction label accompany the
single runtime-setting correction; no new mechanism, verb, daemon or policy.

## Frozen artifacts

Directory: /tmp/ga-tmgr-combined-candidate-r3-20260910/
- source.sha256: 606b0af539dd0bd39d8022d9dcf3c1ac2fb3215669b8f7271682ac9ef8f62c63
- combined-source.tar: f2845d32973bbcdb493f832d79759fdfdb85eafc9eff2d2f5111704718023f45
- r2-to-r3.diff: a8e0bfeb50e8ea383c8733a5d6da627eea968c09aa3330a6fcb0c56b1d6570cf
- combinedprobe: 72d4991d1c264af7829922164efe9bb6cd82eae4572398e15bb9dbba1d325b53
- execution-plan.json: 80ca827ce5e91fefad2860da16e7e7ee4abb5af179ef176e289d583ae253e2f6

Focused pure RED/GREEN: offline-red.txt / offline-green.txt.
Final go test -count=1 -v ./test/combinedprobe PASS; vet/static build PASS.
Actual commands and exit0: offline-final.txt, vet.txt, build.txt.
Tests construct commands but never start them or mutate parent environment.
Freeze performed only read-only runtime/ELF/hash/absent-runroot checks.

R2 source snapshot and complete delta preserved; r2-preservation-final.txt
verifies its sealed artifacts. Original20, original staged index and M5 source/
binary/result remain exact. See original20-preserved.txt, index-before.z/
index-after.z, m5-source-preserved.txt and preserved-inputs.sha256.

## Exact proposed command — review gate remains

```sh
/tmp/ga-tmgr-combined-candidate-r3-20260910/combinedprobe execute-reviewed /tmp/ga-tmgr-combined-candidate-r3-20260910/execution-plan.json 80ca827ce5e91fefad2860da16e7e7ee4abb5af179ef176e289d583ae253e2f6
```

Unique absent runroot: /tmp/ga-tmgr-combined-run-r3-20260910.
Parent independently reviews the frozen delta before deciding one append-forward
run. Genuine execution exit0 plus separate result.frames validation are still
mandatory; no acceptance is inferred from source or from R2's consumed result.
Whole120s and all existing deadlines/UID1000/runtime/namespace pins unchanged.

Current blocker: independent R3 delta review, then actual combined proof.
A new substantive failure must be preserved/reported, not silently broadened.
Full race/decoder/provider and useful-worker/handover acceptance remain open.
No production integration, live/service/rig/worker, root, package, signing,
push/merge, cleanup, goal change or new context. ga-oz9e remains deferred.

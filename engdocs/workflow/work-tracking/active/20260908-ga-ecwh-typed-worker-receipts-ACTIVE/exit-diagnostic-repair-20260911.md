# ga-tmgr narrow exit diagnostic candidate — 2026-09-11

## Source-only result

One-line production correction: shouldSurfaceUnhandledCommandError matches GC's concrete *commandExitError, not every foreign ExitCode interface. errExit and wrapped GC sentinels remain quiet; foreign/process errors now reach existing bounded-context diagnostic output. commandExitCode, JSON handling and V/W behavior are unchanged. Candidate is uncommitted and awaits independent review; no production build or runtime retry.

## Exact candidate

Base signed HEAD: 5a74ab60da46e55ba6425839a3819e9a185c1803.
Root: /home/loucmane/gascity-core-worktrees/ga-ecwh-typed-worker-receipts.
Evidence/package: /tmp/ga-tmgr-exit-diagnostic-repair-20260911.

- cmd/gc/main.go SHA256 47b67dd6d7ba1d01426edcc7bc50b6798ec1bb6c2618bdb0e91071df05256ae9.
- cmd/gc/main_exit_diagnostic_test.go SHA256 0ee77bf7e9b950d4568694ce84cc28a6e82504987517a50c92b862cac84186af.
- internal/platforminstall/metadata_custody_source_linux_test.go SHA256 b11937561530795b3a7c403a1e7f271b44b0d355e0e75a25b66c050de12025ce. Only the main.go digest entry changes; every other pin remains exact.
- source.sha256 SHA256 c11ddb84e6c388d114c4bed76300d4edebd9e6e8be0e2618c1ccd94b27f894ec.
- source.tar SHA256 54c15babd615b0ab2fc79b00a3e74f1db4789e4fdaae641be0ff6e08ebe9e053.
- candidate.diff SHA256 2244b632ea589ee76c6484f7db13783ea1d6f638b610f1adc2d585b0991d4ebf includes the complete new test file.

## RED/GREEN

Pinned Go1.26.7; offline readonly modules, normal GOCACHE, fresh on-disk compile/test scratch and private test socket parents. No new fixture child or namespace/supervisor/metadata operation. Ordinary cmd/gc TestMain retains its existing isolated package setup; these are not runtime acceptance tests.

RED: red.txt preserves five intended classifier failures plus an unsuitable help-output fixture (Cobra help swallows output errors). Production remained unchanged. Corrected existing completion-output seam in red-corrected-seam.txt proves five classifier failures and two real CLI missing-diagnostic failures; sentinel/exit-code checks pass. No new production seam.

GREEN: green.txt/green.exit record exit0; cmd/gc0.172s, platforminstall0.038s. Selection: TestUnhandledForeignExitDiagnostics, TestRunSurfacesForeignExitDiagnostic, TestShouldSurfaceUnhandledCommandError, TestRunSurfacesUnhandledPlatformAdoptError, TestWriteJSONErrorIncludesExitCodeOnStdoutAndStderr, TestJSONExecutionFailureIsStructured, TestJSONUnsupportedCommandFailureIsStructured, TestMetadataCustodyOwningSourceProfile. Eight top-level tests and22 classifier/CLI subtests pass. Existing go test default vet remains enabled; no standalone broad vet/build/suite.

Exact common environment: env -i HOME=/home/loucmane PATH=/home/loucmane/.local/share/go/1.26.7/bin:/usr/bin:/bin GOTOOLCHAIN=local GOENV=off GOWORK=off GOPROXY=off GOSUMDB=off GC_FAST_UNIT=1. GOTMPDIR and TMPDIR use /var/tmp/ga-tmgr-exit-diagnostic-{red,red2,green}-20260911; GC_TEST_TMUX_SOCKET_PARENT_ROOT uses package/{red,red2,green}-sockets. Invocation: /home/loucmane/.local/share/go/1.26.7/bin/go test -mod=readonly -count=1 -timeout=120s; RED selects the first two tests in ./cmd/gc; GREEN adds -v, both owning packages and the eight exact anchored test names above. All outputs and failed scratch parents retained; gofmt -d and git diff --check clean.

Coverage limits: process error type is a constructed exec.ExitError without ProcessState, preserving its existing -1 code; no real process is needed. Foreign exit7 and joined metadata-style cleanup chains are injected through io.Writer into the existing completion command and real run diagnostic path. This proves classification/output plumbing, NOT actual writer failure, protected-W absence or R3 root cause. Existing in-process missing-manifest/JSON tests cover unchanged adjacent contracts; no metadata apply reaches execution.

## Preservation and next boundary

source-before.tar and identical index-before.z/index-after.z preserve baseline.89 other source91 pins pass unchanged-source-readback.txt; the historical source91 manifest remains untouched. Signed HEAD unchanged, index empty. Old image64bdb174ed79719195f38d00877f44ce7182a8cc5b42247bc7ac5852ef37c8b4 and consumed R3 result14a196d2b7d087065168d13d4cb342618cb8850b30518f092237ed4b3bea6d31 rehashed unchanged. All prior attempts/evidence preserved.

Next: independent exact-source review by parent. No inference, signing, publication, build, runtime retry, lifecycle, installation or goal change authorized/performed here. R3 remains FAILED/consumed with unknown transaction; diagnostic suppression is a possible silent path, not proven cause. Full provider-independent execution and bidirectional handovers objective remains incomplete; ga-oz9e deferred. Supported checkpoint/log/note/readback results will be recorded in checkpoint-readback.md after execution, not presumed here.

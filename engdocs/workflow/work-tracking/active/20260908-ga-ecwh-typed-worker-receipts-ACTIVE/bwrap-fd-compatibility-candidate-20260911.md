# ga-tmgr bwrap FD compatibility — source candidate

## Scope and disposition
- Approved source-only repair in existing READY ga-ecwh parent, attached ga-tmgr; sole writer. Base HEAD f1ea84d76fce1d17626349353f758eb592386ee3.
- Candidate ready for parent-owned independent review, NOT runtime acceptance or delivery.
- R4 remains consumed FAILED/UNKNOWN: unsupported --preserve-fds plus separate process-observation failure; protected-W absence unproven. R3 cause remains unproven. No replay.
- No new image, namespace/probe, supervisor, installed metadata/cache, package, credential, lifecycle, signing, commit or publication operation.

## Exact delta
- internal/platforminstall/metadata_sandbox_linux.go: remove only "--preserve-fds", "2" from metadataSandboxArgv.
- internal/platforminstall/metadata_integration_linux_test.go: existing argv owner now rejects that unsupported option and --sync-fd/--args substitutions; NUL boundaries require exact tokens/sequences rather than partial substring matches. Existing positive namespace/env/proc/root requirements and six mount/pin negatives remain.
- ExtraFiles remains exactly childInput/childOutput (FDs3/4). Both metadataCheckFDs calls and function, anonymous-pipe identity checks, capabilities, mounts, runtime pins, environment, deadlines and transaction behavior unchanged.
- No source pin update needed. All tracked Go inputs except these two retain baseline hashes; original index checksum unchanged. source-before.tar and worktree/index baseline diffs preserve prior state.

## Source premise, not execution proof
- [Upstream bubblewrap v0.9.0](https://raw.githubusercontent.com/containers/bubblewrap/v0.9.0/bubblewrap.c): sandbox child reaches execvp with inherited descriptors; optional PID1 parent closes extras, but --as-pid-1 skips that parent. Monitor cleanup is separate from sandbox-child inheritance.
- Read local Ubuntu patch series and both patches without extraction/execution: only error text and optional bind-fd/ro-bind-fd parsing/tests change; neither changes this inheritance path. Archive SHA256 d253eccba8f6a8d636dd3df241c2d4d722785341c665c7817040aafa7ed3495e.
- Installed /usr/bin/bwrap rehash remains 52231e1caf55bcbc667b269f49c63599a6f7db4767ae6a039580d0ff853db712. This is source-level compatibility reasoning, not proof of FD inheritance through a new production runtime.

## RED/GREEN and owning validation
All commands run from the existing Core worktree. Prefix:
`/usr/bin/env GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOTMPDIR=/var/tmp TMPDIR=/var/tmp /home/loucmane/.local/bin/go`
Go1.26.7; existing /home/loucmane/.cache/go-build unchanged in configuration, no GOCACHE override.
- First invocation stopped before tests because /var/tmp/gotmp did not exist; red.log/red.exit preserve that infrastructure failure. Used existing /var/tmp subsequently, no directory/cache repair.
- RED: `test -mod=readonly -count=1 -timeout=60s ./internal/platforminstall -run '^TestMetadataSandboxExactNamespacesEnvironmentAndChannel$'`; exit1, exact unexpected "--preserve-fds"; package 0.033s.
- GREEN: same command with -v; exit0, owning test and six subtests, package 0.033s.
- Owning package: `test -mod=readonly -count=1 -timeout=120s -json ./internal/platforminstall`; exit0, 115 top-level tests plus137 subtests, zero failures/skips, 3.328s.
- `vet -mod=readonly ./internal/platforminstall` exit0; git diff --check PASS; gofmt -l both changed files empty.
- Resource inspection: process/listener kernel tests are integration-tagged and excluded; no TestMain in this package. Untagged tests use pure/scripted collaborators and private filesystem fixtures. No opt-in kernel or namespace acceptance ran.

## Frozen candidate
Evidence root: /tmp/ga-tmgr-bwrap-fd-compatibility-20260911/executor/
- metadata_sandbox_linux.go SHA256 3e69739057c93feec7a316bbc116043dc2d8fdc488820fc9a2b21e44672d6740
- metadata_integration_linux_test.go SHA256 ee7e721630b1c0a7c81b94e022f0592b3b92d1302fa7b029c52f2d0d868d3c5f
- source.sha256 SHA256 a4b84ae6491727c6a2ee7e360346835e8ed77f4c7a95b9d2db826990d7de41eb
- source.tar SHA256 d7f0b6dcab3cfc7e5ca6349890620b8f230ebee303e455d9612a96a6d9baec0a
- candidate.diff SHA256 064b8b4dd7fe16302edef101a0374b1a252cdfa8504323ebd92050cde5e4df9e
- red-argv.log SHA256 85dbd4c9cc7f707f53d164726ae9930dac00a42a6d39d79b3e20afda6a5cc033
- green-focused.log SHA256 f8ba791ce088f24cba59e148575d44ea23428c1d200951302177b07f5d500ea3
- green-package.jsonl SHA256 5119297d16030927fa7e6a7a400595090d9030ddaea8a3260c5d4f9cc820463b

## Remaining boundary
Parent-owned exact-source review before any separately authorized fresh runtime candidate/build/attempt. Reviewer should verify default inheritance premise and unchanged channel/confinement invariants; pure argv tests do not establish runtime FD inheritance or W cleanup.
Full original goal remains incomplete, ga-tmgr open, ga-oz9e deferred. Existing history preserved append-forward. Supported note/log/checkpoint and final ownership/queue readbacks are captured beside this report.

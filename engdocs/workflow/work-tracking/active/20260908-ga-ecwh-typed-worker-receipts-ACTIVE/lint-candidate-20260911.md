# ga-tmgr consolidated lint candidate — source ready for review

## Completed scope
One bounded batch implements the independent lint guidance against formatted62
manifest81ca21c2035201b7de4aee8f7580690eba5b773552117e654da5b3fbe90fa7bb.
17 changed files, including two small private cleanup source/test files; full
candidate contains64 entries. No new exported API, protocol, guard setting,
production test mode or acceptance relaxation. No staging/signing/build/runtime.

Close errors now preserve primary errors for inode/lock acquisition, file/ELF/FD
inspection and publication cleanup. Writer classification is deferred at the
outer entrypoint, after publication/output-lock/cache-lock/pipe cleanup. The
precommit manifest restoration remains inside the existing locked scope; the
final receipt rename remains irreversible even when later cleanup fails.
V classifies after its child/pipes/session cleanup; a validated COMMITTED frame
retains phase knowledge, while the existing exact committed-pair/binding readback
covers a lost commit reply. Postcommit errors remain committed_evidence_incomplete.
Parent-held child ends and channel ends have explicit once-only closers. After
successful Start, guaranteed cancel/Wait/pidfd finalization is installed before
an early close can return an error. Wait and terminal checks still run after close
failure; no double-close failure, retry, rollback or swallowed cleanup error.

Mechanical fixes include accurate exported docs, unused-parameter markers,
constant0644 publication mode, equivalent route alphabet logic, nil-safe peek
refusal with wrapped nonnil cause, redundant conversion removal and test naming.
One audit-only runtime.GOROOT deprecation annotation preserves exact source
hashes/toolchain binding. Custody production code/settings and owning pin map
are unchanged; independent review remains required on this actual delta.

## Focused evidence and limits
RED: cleanup source-wiring regression failed on the original outer-classification
placement. This is a source-order regression, not an injected runtime failure.
GREEN: pure injected close/reap/terminal tests plus real temporary-file publication
phase fixtures pass; before/after rename errors retain stage/receipt respectively.
Actual child processes and production writer execution are NOT exercised here.
One intermediate compile failure (unused fmt after t.Fatalf correction) and one
new-test gocritic finding (unchecked source index) are preserved and corrected.
Final focused cleanup tests PASS0.063s; no-fix owning lint:0 issues.
Ordinary owning regression PASS: platforminstall3.194s, packman0.890s.
No integration/kernel tests, consumed roots, full Core suite or prior acceptance
replays. Existing package fixtures do not establish live/kernel/provider parity.

## Frozen review inputs
Root: /tmp/ga-tmgr-lint-candidate-20260911
- source.sha256: eca556b37e8a86a8dff6b95d729ed8867edabbd75c2c3b71339002b013cb79c4
- source.tar: 31b970c62e01f9c1fd8f3d811fe7bc9b2d418f78216a8b5b287bc1bc8464eadb
- changed.sha256: a909ab9c387054726774404657a66a6b29b962c48547477f09010531998ddada
- formatted-to-lint.diff: b92111339b491d3502485804df93c9c30550e70feb24ac424f560b09fcb471b9
- cleanup-red.txt: df1115b5452eb4332f9d7661aaef3801f984bc6d21f2aa6185eedcd0cf62524c
- cleanup-green-final.txt: 96735e7eadfff5d90b14fcd2a4facd34957f42d73c0efde9c8441911b9f1408a
- lint-final.txt: e92606b0bf483111dff0a120c315ea165821348f31365020e2468a0059095c47
- owning-regression.txt: 1eae48009be44c8a7b28265151e85b28ee663555f5e3cc14e497891f00b40332

Original index dbf7c6ccb1ab0c1573b33e0d466df2eae10d52d41bdec386522309c583ec9e86
and HEAD96e0ed0423c77ecc44237b4f9c3e667648c74a48 remain unchanged. New source is
unstaged; prior archives, frozen binaries, all failed and passed runtime evidence
are retained. Earlier evidence is not relabeled as execution of these new bytes.

## Authority, checkpoint and next action
Operator approval now resolves the previous normal-hook boundary for lockfile-pinned
npm ci in this Core worktree and bounded temporary loopback dashboard smoke.
No such installation/smoke is performed this turn; no global install, credentials,
production service/rig/worker or adoption authority is implied.
One owned ga-tmgr note and existing Core/Operations plan/session/tracker/handoff
links record this source-ready candidate. Final verification/ownership/queue
receipts are in the evidence root; historical Operations date HOLD not rerun.
Next: independent consolidated delta review, then the already-authorized unchanged
normal hooks, signed local checkpoint and exact main.commit-stamped offline build.
No additional micro-authorization for those scoped steps is requested. Full original
goal remains active/incomplete, same context; ga-oz9e deferred/nonblocking.

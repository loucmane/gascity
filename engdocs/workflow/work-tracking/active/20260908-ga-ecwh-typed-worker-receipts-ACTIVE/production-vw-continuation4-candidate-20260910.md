# ga-tmgr continuation4 — consolidated review corrections

## Outcome
M1 and M3 source corrections are implemented with focused pure/fixture proof.
Overall SOURCE HOLD remains: M2 listener-holder completeness is not established.
This is NOT production/runtime/feasibility PASS and must not be executed.
No new scanner, optimistic exclusion, capability, daemon, probe or runtime attempt.
Legacy v1 dispatch is byte-identical to continuation3 and is not a blocker here.

Inputs read completely and rehashed:
- Fix brief: a4da697b959779ecc256d42dd0a1d6fa9748c064163f746fdd7bdd3adef75356.
- Independent continuation3 Astra review: 70bd4685f2f1aa5a7af08efd23a519d69499bce234bc5fb573e34a763c126c7e.
The original review/HOLD and frozen continuation3 remain unchanged.

## Frozen delta
Root: /tmp/ga-tmgr-production-vw-continuation4-20260910
- source.sha256: 02974929f6fcce245378fc1758844b8d5ce35f3780df1f039c3d8145da66ee96
- source.tar: fb8a5ab7dc9fef39da2382884be12f207e2172f69bbf1124eb088e50a0604000
- continuation3-to-4.diff: 74a5bf708e8ddad2da9d418c6717c9f70ba9baedefacf0b419a67421262b7081
- changed.sha256: c9b45a11e640424192b687b084df31349e7d2b8b0b50c225561f3a8425c23710
- gc-source-candidate: 16aa8a316b3435a63dbd33cd7d3f98a7cb3ea323d472f97f06a68575d68c6456
changed-files.txt lists 15 files including generated schema/client mirrors and three new tests.
The complete archive retains earlier source dependencies; delta includes every changed/new file.
Static binary was built only, never executed. No execution command is authorized by this packet.

## M1 — supported preimage form, not permission normalization
metadata_publication_linux.go now requires existing metadata images to have exact
0644 mode, effective UID/GID, regular type and one link. Special modes, 0600 and
nondefault groups refuse before staging. Absence remains explicitly digest-bound.
metadataOutputPreimages checks the whole output set before the transaction's first write;
publication checks again at its anchored parent before publication.
Rollback supports the same exact form, never widens an accepted private preimage.
Tests reproduce old 0600 acceptance (m1-red.txt), then prove refusal without a stage,
unchanged private bytes/mode, injected nondefault-GID/special-mode refusal, and exact
supported precommit byte/mode/UID/GID restoration. No real chown is used.
Existing receipt rename/fsync fault tests still prove committed=true on post-rename
sync failure with retained image and replay refusal. This is primitive/fixture proof,
not execution of a whole production transaction or an ACL preservation claim.

## M3 — observation is not mutation reservation
The SAME begin/check handlers accept a bound optional observation boolean.
Fresh observation and dry-run challenges do not occupy the mutation slot; concurrent
observations can coexist. Both handlers refuse observations while a mutation is active.
Every response remains exact request/nonce/deadline/instance/current-clock evidence,
obtained from the actual contacted process; no saved-proof input is added.
W binds observation to PlanOnly and refuses purpose changes; observation bindings
cannot be accepted as mutation receipts. V2 VerifyMetadataRuntime uses observation.
Mutation still gets one exclusive, bounded, nonrenewing slot. After writer wait/pidfd
terminal proof and its original deadline check, successful apply/no-op waits for natural
lease expiry before returning. No release endpoint, renewal or early slot clearing.
Cancellation/clock/wait errors refuse; committed receipt errors remain incomplete
evidence. A competitor after expiry can still win; sequential success is not a reservation.
Generated API/schema/client parity is updated for the optional field.
Tests cover consecutive observations, dry-run/apply ordering, occupied-slot refusal,
purpose/receipt confusion, expiry/cancellation/clock failure, genuinely concurrent
goroutines, handler composition and actual in-memory HTTP JSON field binding.
M3 RED was a missing-field compilation failure, not a runtime reproduction.

## M2 — constructibility finding and ONE decision
Required operation: establish a complete set of holders of the exact listening socket,
including a cooperating inherited holder that changed credentials or dumpability,
before excluding it from the external verifier's ownership proof.

A forked child inherits references to the same open-file descriptions; dumpability can
subsequently change without closing that listener. This supplies the relevant counterexample
without assuming hostile root. See [fork](https://man7.org/linux/man-pages/man2/fork.2.html)
and [PR_SET_DUMPABLE](https://man7.org/linux/man-pages/man2/PR_SET_DUMPABLE.2const.html).
Reading /proc/PID/fd/N uses PTRACE_MODE_READ_FSCREDS; a nondumpable target denies
unprivileged inspection without the required target-namespace capability. See
[proc_pid_fd](https://man7.org/linux/man-pages/man5/proc_pid_fd.5.html) and the
[ptrace access algorithm](https://man7.org/linux/man-pages/man2/ptrace.2.html).
Consequently, proc UID alone cannot soundly exclude that holder. The existing frozen
request and two lease handlers supply no closed FD-transfer/creation provenance.
A whole-host scan that refuses every inaccessible process is not a demonstrated
constructible production scope. No scan or injected success was manufactured.

This is a source/documentation inference about the current unprivileged contract,
NOT an observed host EACCES, a new live probe or a universal impossibility claim.
The existing M2 scanner remains byte-identical, explicitly unaccepted; the compared
listener-scan-before/current.txt files preserve that fact.

ONE operator architecture decision remains: whether to replace retrospective
all-holder enumeration with an explicitly reviewed supervisor-origin closed
listener-creation/no-export provenance invariant in the trusted contract.
That changes the cooperating-holder trust assumption; it is NOT already proven or
authorized for implementation/activation by this checkpoint. If not accepted, retain
HOLD. No privileged scanner/root capability or alternate wrapper is proposed.

## Validation and preservation
- platform-final.txt: owning platforminstall PASS, 3.715s.
- api-final.txt: affected lease handlers and OpenAPI parity PASS, 0.122s.
- client-parity.txt: generated client parity PASS, 6.850s (offline dependencies).
- concurrency-race.txt: focused platforminstall/API race tests PASS, 1.084s/1.177s.
- vet.txt / build.txt / diff-check.txt: exit0; no full Core suite or runtime fixture.
- genspec.txt / genclient.txt: existing offline generators; no service or package install.
- index-before.z == index-after.z, SHA256 dbf7c6ccb1ab0c1573b33e0d466df2eae10d52d41bdec386522309c583ec9e86.
- Original20 manifest/source and continuation3 archive hashes remain exact in preservation.sha256.
All previous runs, reviews, source snapshots and failures remain; no consumed root replay.
Same READY ga-ecwh parent/attached ga-tmgr and existing Operations ga-e0t1 context.
One owned note and supported logs/checkpoint/verify accompany this report, with
targeted plan/handoff/tracker/session, ownership and empty-queue readbacks in this root.
Full parent goal/history and outcome order remain incomplete; ga-oz9e remains deferred.
Next: independent consolidated M1/M3 delta review plus the explicit M2 decision.
No new runtime acceptance may proceed on this HOLD candidate.

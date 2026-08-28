---
title: Upgrade and Roll Back a Managed Gas City Platform
description: Build, pin, preflight, activate, verify, replay, and roll back a Gas City release without hand-copying binaries or managed assets.
---

This runbook is for operators upgrading a self-hosted Gas City whose core
binary, control rules, validators, packs, templates, providers, and runtime
identity must move as one reviewed platform release.

The managed installer is intentionally split into preparation and live
transition gates. Preparing artifacts, finalizing a manifest, and running a
dry-run do **not** authorize an install or restart. Likewise, an approved
install does not authorize a later rollback. Treat each live transition as a
separate operator decision tied to one exact manifest digest.

## Safety contract

`gc platform` enforces these properties:

- every source, destination baseline, backup, digest, mode, repository,
  provider, and runtime identity is checked before the first mutation;
- the core and managed files publish in one staged, fsynced transaction;
- a failed publication restores the exact prior bytes in reverse order;
- activation makes at most one supervisor restart attempt and verifies the
  running executable SHA-256, commit, and version;
- the receipt is canonical and self-digested;
- each successive release pins and retains the exact previous canonical
  manifest and receipt before publishing any new platform bytes;
- replaying the same manifest is a filesystem no-op and verifies the runtime
  before considering a restart;
- rollback restores the manifest-pinned prior platform, makes one restart
  attempt, and verifies that prior runtime;
- backups and failure evidence are retained. The installer never cleans
  worktrees or rewrites unrelated configuration.

## Authority matrix

Before building anything, record these values in the change review:

| Authority | Required evidence |
|---|---|
| Candidate source | Clean source checkout, signed Git commit, full 40-character lowercase commit ID, tree ID, and reproducible artifact SHA-256 |
| Installed runtime | Installed executable SHA-256 plus `/proc/<supervisor-pid>/exe`, embedded commit, exact `--version`, supervisor unit, PID, start monotonic time, and restart count |
| Configuration authority | Exact `city.toml`, generated fragment, resolved configuration, permission profile, and control-rules digests read from the supervisor's real `GC_HOME` |
| Pack/lock/cache authority | Pack source commit, lock entry, generated cache digest, template digest, and a proof that the running configuration resolves to those same bytes |
| Managed files | Candidate path/SHA/mode and either an absent destination or exact previous SHA plus backup path |
| Providers | Stable entrypoint, resolved executable, SHA-256, version arguments, and exact version output |
| Manifest and receipt | Canonical `gc.platform-install-manifest.v1` file, internal manifest digest, complete-file SHA-256, prior manifest/receipt pins, and the activation receipt self-digest |
| Runtime state | Every non-HQ rig suspended, zero sessions and worker residue, expected service epochs, and two stable reconciliation observations |

Stop if the live baseline differs from the reviewed record. Do not "update the
numbers" during the cutover; rebuild and re-review the manifest instead.

The authorities are deliberately independent. Git proves what was reviewed;
the artifact digest proves what was built; the installed path proves what was
published; `/proc/<supervisor-pid>/exe` proves what is executing; the resolved
configuration proves what the process will consume; and the receipt proves
what the installer accepted. Never substitute one surface for another.

## Authorization matrix

Preparation does not imply permission to mutate live state. Record an explicit
gate for each row that applies:

| Action | May be batched with | Requires a fresh explicit authorization |
|---|---|---|
| Read-only inventory, build, tests, manifest finalization, dry-run | Other non-mutating preparation using the same reviewed pins | No, when already inside an approved preparation scope |
| Source push, PR, hosted CI | Each other when exact head/tree/base and merge conditions are named | Merge, if the prior authorization did not explicitly include it |
| Broker apply or direct platform install | The one reviewed service transition and its postflight | Yes; name manifest file SHA-256, artifact SHA-256, prior SHA, affected files, and rollback |
| Replay of an already installed manifest | Read-only postflight | Yes; it is a separate live command and must prove `result=noop` |
| Rollback | Its one reviewed supervisor transition | Yes; rollback is not implied by install authorization |
| Rig resume or managed-worker proof | One isolated route, worker, and bounded observation window | Yes; an upgrade never authorizes product work |
| Deployment or publication | Nothing else | Always a separate product decision |

A corrected retry remains inside the same authorization only when the previous
attempt is proven pre-mutation or fully rolled back, the failure is understood,
and the corrected operation has the same bounded effects. Never retry an
ambiguous partial mutation, a credential prompt, or an unexplained invariant
failure.

## 2. Build the candidate reproducibly

Use a clean checkout at the reviewed commit. The standard build derives its
timestamp from the commit and uses `-trimpath`:

```bash
make build VERSION=<reviewed-version>
sha256sum bin/gc
go version -m bin/gc
bin/gc --version
```

For high-risk releases, repeat the build in a second clean clone at a different
absolute path and require byte equality:

```bash
cmp <clone-a>/bin/gc <clone-b>/bin/gc
sha256sum <clone-a>/bin/gc <clone-b>/bin/gc
```

The two digests must match. A dirty source checkout, `vcs.modified=true`, a
different embedded revision, or different output bytes is a hard stop.

## 3. Author the unsigned manifest

Create JSON with `schema` set to `gc.platform-install-manifest.v1` and omit
`manifest_sha256` (or set it to the empty string). All filesystem paths must be
absolute. Modes are JSON numbers: `493` is `0755`; `420` is `0644`.

```json
{
  "schema": "gc.platform-install-manifest.v1",
  "release_id": "v1.4.1-local.1-<short-commit>",
  "city_path": "/absolute/path/to/city",
  "core": {
    "name": "gc",
    "source": "/absolute/path/to/reviewed-build/gc",
    "destination": "/absolute/path/to/live/bin/gc",
    "sha256": "<candidate-sha256>",
    "mode": 493
  },
  "managed_files": [
    {
      "name": "control-rules",
      "source": "/absolute/path/to/reviewed-rules",
      "destination": "/absolute/path/to/live/rules",
      "sha256": "<candidate-rules-sha256>",
      "mode": 420,
      "previous_sha256": "<previous-rules-sha256>",
      "backup_path": "/absolute/path/to/retained/rules.backup"
    },
    {
      "name": "validator",
      "source": "/absolute/path/to/reviewed-validator",
      "destination": "/absolute/path/to/live/validator",
      "sha256": "<candidate-validator-sha256>",
      "mode": 493
    }
  ],
  "previous_metadata": {
    "manifest_sha256": "<sha256sum-of-previous-canonical-manifest-file>",
    "manifest_backup_path": "/absolute/path/to/retained/install-manifest.previous.json",
    "receipt_sha256": "<sha256sum-of-previous-install-receipt-file>",
    "receipt_backup_path": "/absolute/path/to/retained/install-receipt.previous.json"
  },
  "previous_sha256": "<previous-core-sha256>",
  "backup_path": "/absolute/path/to/retained/gc.backup",
  "receipt_path": "/absolute/path/to/city/.gc/platform/install-receipt.json",
  "activation": {
    "expected_commit": "<candidate-full-commit>",
    "expected_version": "<exact-candidate-version-output>",
    "previous_commit": "<exact-previous-runtime-build-id>",
    "previous_version": "<exact-previous-version-output>"
  },
  "integrity": {
    "files": [
      {
        "name": "permission-rules",
        "path": "/absolute/path/to/live/rules",
        "sha256": "<candidate-rules-sha256>",
        "mode": 420
      }
    ],
    "repositories": [
      {
        "name": "template",
        "path": "/absolute/path/to/template",
        "commit": "<template-full-commit>"
      }
    ],
    "providers": [
      {
        "name": "codex",
        "path": "/absolute/path/to/stable/codex-entrypoint",
        "resolved_path": "/absolute/path/to/resolved/codex-binary",
        "sha256": "<provider-sha256>",
        "version_args": ["--version"],
        "version": "<exact-provider-version-output>"
      }
    ]
  }
}
```

`expected_commit` is always the candidate's full 40-character commit. For a
legacy rollback target, `previous_commit` is the exact build ID reported by
the running supervisor; the schema accepts a 7-to-40-character lowercase
hexadecimal ID with an optional `-dirty` suffix so that verification can pin
the observed legacy runtime without pretending it was rebuilt.

Sort `managed_files` strictly by `name`. Use an empty
`previous_sha256` only when the destination is required to be absent; rollback
then removes that newly created file. Otherwise supply both the prior digest
and a distinct retained backup path.

Omit `previous_metadata` only for the first managed installation. Every later
release must include it. Its two digests are SHA-256 hashes of the complete
canonical files on disk (including their embedded self-digest fields), not the
manifest's internal `manifest_sha256` value or the receipt's internal
`receipt_sha256` value. Give each release distinct retained backup paths.

A successive manifest must retain the same city, core destination and mode,
and receipt path. It must name the installed release's core SHA as
`previous_sha256`, preserve every previously managed file by name,
destination, and mode, and bind each such file's `previous_sha256` to the
previous manifest's candidate SHA. Its activation `previous_commit` and
`previous_version` must equal the previous manifest's expected runtime. These
relationships are verified before the first write.

## 4. Finalize and review the canonical manifest

The manifest command rejects unknown fields, trailing JSON, unsafe paths or
modes, malformed commits/digests, and already-finalized input. It creates the
output without replacing an existing different file. An exact replay returns
`result=noop` without changing the output inode or modification time.

```bash
gc platform manifest \
  --input /absolute/path/platform-manifest.unsigned.json \
  --output /absolute/path/platform-manifest.json
```

Commit the finalized manifest on the reviewed change branch and sign that Git
commit. Record the command output and manifest SHA-256 in the operator review.
Do not edit the finalized JSON; change the unsigned input and produce a new
output instead.

## 5. Dry-run the install

Run from a read-only decision point before requesting live authorization:

```bash
gc platform install \
  --manifest /absolute/path/platform-manifest.json \
  --dry-run
```

Review every ordered line. `CHECK` lines are non-mutating. `MUTATE` lines show
the exact writes and lifecycle transition an approved apply would perform.
Dry-run fails before mutation if any candidate source, live baseline, backup,
pack/template/provider pin, permission rule, or config authority has drifted.

Capture immediately before apply:

- manifest digest and signed review head;
- live executable path, SHA, commit, version, and supervisor PID;
- clean/dirty state of every pinned repository;
- digests and modes of every managed destination;
- rig suspension state and the explicit list of product rigs that must remain
  quiescent during the transition.

## 6. Apply once, after explicit authorization

### Broker-activated hosts

On hosts with the fixed-operation privileged provisioning broker, use the
broker for the control-plane file replacement and the single supervisor
transition. Before sending the signed broker envelope, create the manifest's
exact rollback backup and complete the normal dry-run evidence. After the
broker reports PASS and the new runtime is stable, publish only the platform
metadata with the installed candidate binary:

```bash
gc platform adopt \
  --manifest /absolute/path/platform-manifest.json \
  --dry-run

gc platform adopt \
  --manifest /absolute/path/platform-manifest.json \
  --apply
```

`platform adopt` requires the complete candidate filesystem and rollback
backups to be present, verifies the already-running commit/version/digest, and
then atomically publishes the canonical manifest and activation receipt. It
never writes the core executable or managed files and never restarts the
supervisor. A metadata failure restores only the prior metadata; it does not
silently replace broker-owned live bytes. This is the normal post-bootstrap
upgrade lane.

### Direct platform installer

The authorization must name the exact manifest digest and permit the one
supervisor transition. Then run exactly once:

```bash
gc platform install \
  --manifest /absolute/path/platform-manifest.json \
  --apply
```

Do not wrap the command in an automatic retry. The command itself handles an
interrupted publication and verifies a replay before deciding whether a
restart is still needed.

After success, run `gc doctor`. Treat the replay as a separate live command:
present the post-install proof and obtain explicit replay authorization tied to
the same manifest digest before invoking `--apply` again.

```bash
gc doctor
```

After that replay authorization:

```bash
gc platform install \
  --manifest /absolute/path/platform-manifest.json \
  --apply
```

The doctor must report the managed-platform integrity check clean. The second
identical apply must report `result=noop` and must not replace files or restart
an already verified runtime. Preserve the manifest, receipt, all backups, the
pre/post process evidence, and command transcripts.

## Cutover checklist

Use this checklist for either the broker lane or the direct installer lane.
The implementation commands above remain authoritative; this list makes the
ordering and stop boundaries explicit.

### Preflight and consolidation

1. Consolidate the release onto one clean, signed source head. Record its
   commit, tree, artifact digest, version, and reproducible-build proof.
2. Read every authority in the authority matrix from its canonical source.
   Include both the installed file and `/proc/<supervisor-pid>/exe`.
3. Finalize the canonical manifest. Record both its internal
   `manifest_sha256` and the SHA-256 of the complete file.
4. Run the strict install or adopt dry-run. A rollback dry-run is also required
   before apply, because an unexecutable rollback is not a safe cutover.
5. Run configuration/import validation, provisioning doctor, provider
   entrypoint/version checks, validator `check_path` checks, and the managed
   rules/grants verifier from the same `GC_HOME` and PATH used by the running
   supervisor.
6. Prove all non-HQ rigs suspended, no sessions, no workers, no city-scoped
   tmux server, no unrelated service transition, and a stable supervisor
   service epoch.
7. Capture byte-exact backups before the first live write. Re-hash the backups
   and require their modes and owners to match the manifest.
8. Compare the proposed live mutation with the authorization. Stop if a path,
   digest, operation, restart, or service falls outside it.

### Cutover

1. Apply through the fixed-operation broker when that operation exists;
   otherwise use the reviewed direct installer. Do not hand-copy managed
   artifacts.
2. Permit only the manifest's bounded file replacements and one supervisor
   stop/start. A second transition is a stop condition.
3. Require the new installed digest, `/proc/<supervisor-pid>/exe` digest,
   embedded commit, version, PID/start epoch, and restart count to match the
   reviewed target.
4. Publish the canonical manifest and receipt only after the live runtime is
   proven. In the broker lane, use `gc platform adopt`; never pretend a
   metadata-only adoption performed the broker's privileged mutation.
5. Leave all product rigs suspended. Platform success is not worker-launch
   authorization.

## Postflight and two-tick reconciliation

The cutover is complete only after all of the following pass:

- installed files, modes, ownership, manifest, receipt, configuration
  authority, pack/lock/cache authority, templates, providers, permission
  rules, and backup digests exactly match the approved record;
- the supervisor and any signing or provisioning services report the expected
  stable service epoch, zero unexpected restarts, and the reviewed executable;
- `gc doctor` reports no failed checks and the managed-platform integrity check
  is clean;
- every non-HQ rig remains suspended with zero sessions, worker processes, and
  city-scoped tmux residue;
- two stable reconciliation observations, separated by a real interval, show
  the same configuration, runtime, rig, service, and process state;
- the separately authorized identical replay returns `result=noop`, with no
  inode, size, modification-time, PID, start-time, or restart-count change;
- the rollback dry-run still resolves the exact retained prior artifacts and
  lists only the expected reverse operations.

Store the postflight transcript next to the manifest, receipt, backups, and
preflight evidence. A PASS without durable evidence is not a completed managed
upgrade.

## 7. Roll back through the manifest

Rollback is a separate live transition. First inspect its exact reverse plan:

```bash
gc platform rollback \
  --manifest /absolute/path/platform-manifest.json \
  --dry-run
```

The plan restores or removes managed files in reverse publication order and
restores the prior core. For the first managed installation it removes the
candidate receipt and canonical manifest. For a successive release it instead
restores the exact previous receipt and canonical manifest from the
digest-pinned metadata backups. It then restarts once and verifies the
manifest-pinned previous runtime.

After explicit rollback authorization tied to the same manifest digest:

```bash
gc platform rollback \
  --manifest /absolute/path/platform-manifest.json \
  --apply
```

If the rollback restart fails, the prior bytes remain installed and the
command does **not** retry the restart. Preserve the error and recover the
service manager explicitly; do not republish the candidate or delete backups.

## Recovery decision tree

Start with the first question whose answer is known. Do not infer host truth
from a sandbox that cannot observe another UID or mount namespace.

1. **Did the attempt cross its first mutation boundary?**
   - **No:** preserve the refusal evidence, correct the understood preflight or
     evidence defect, regenerate exact digests, and rerun the full preflight.
   - **Unknown:** stop. Treat it as an ambiguous partial mutation until the
     installed bytes, backups, service journal, PIDs, and receipt prove
     otherwise.
   - **Yes:** continue below.
2. **Did the built-in rollback report PASS?**
   - **Yes:** verify the prior installed SHA, modes, manifest/receipt,
     configuration authority, and inactive-or-restored service state before
     preparing a corrected attempt.
   - **No:** leave the affected service inactive. Do not improvise a second
     write. Use the retained backups and manifest to prepare a separately
     reviewed recovery operation.
3. **Are the bytes correct but the runtime wrong?**
   - Compare the installed file with `/proc/<supervisor-pid>/exe`, then inspect
     the systemd service epoch, cgroup, `NRestarts`, and journal. A stale or
     unexpected epoch requires a bounded service recovery, not another file
     install.
4. **Is verification failing after a successful transition?**
   - Bind the verifier to the receipt's recorded worktree, commit, policy, and
     key home. A wrong worktree is evidence-input drift, not a signature
     failure.
   - If a file appears root-owned as an overflow UID or another process seems
     absent, repeat the observation from the authoritative host namespace.
     Namespace-limited ownership or cross-UID `/proc` output is not host truth.
5. **Is the failure one of the witnessed classes below?** Use the prescribed
   response and stop if its precondition cannot be proven.

| Failure class | Meaning | Safe response |
|---|---|---|
| Candidate or baseline digest drift | Reviewed and live bytes differ | Rebuild or re-review; never edit pins during apply |
| `permission denied` while traversing a worktree | DAC/ACL or namespace boundary, not necessarily bad file bytes | Check every path component as the service identity from host context; grant only reviewed execute/read access, or fix the observer |
| UID/owner mismatch seen only in a worker | User-namespace ID mapping | Move the ownership assertion to host context; keep worker checks to namespace-visible facts |
| Signer or verifier cannot reach the repository | Read-only namespace mounts do not grant DAC traversal | Verify execute-only traversal ACLs and the signer's private-index path; do not chmod a home directory broadly |
| Receipt-to-request mismatch | Wrong worktree, branch, bead, session, policy, commit, or tree | Use the receipt-bound values; do not rewrite the receipt or relax validation |
| Provider binary missing or moved | Stable entrypoint no longer resolves to the reviewed executable | Keep the rig suspended; restore the pinned provider or re-review the provider authority |
| Stale or alias session | Controller/session records disagree with OS or claim identity | Reconcile native claim, session record, and host process truth; never close or replace an unproven session |
| Validator or `check_path` missing | Managed validation authority is incomplete | Refuse dispatch and restore the manifest-bound validator; never skip the check |
| Reply unreadable or malformed | Worker cannot prove the controller result | Preserve the raw reply and fail closed; do not infer success from side effects |
| Approval wait or credential/pinentry prompt | Required human authority is absent | Stop without mutation and return the exact prompt boundary to the operator |
| Transport error or `no_work` | Control plane did not establish a claimable unit | Verify queue/demand/provider health and retry only after proving no durable mutation; never fabricate a claim |
| Rules or grants missing | Worker execution authority is absent | Reprovision the reviewed exact rules through its owner and rerun provisioning doctor; never widen ad hoc |
| Canary failed or receipt drifted | Managed signing/worker acceptance is not proven | Suspend, preserve branch/worktree/receipt/transcript, disposition residue separately, and require a fresh bounded canary |
| Broker schema or peer refusal | Request was rejected before a privileged operation | Preserve the refusal; correct the signed envelope or client identity before one new request |
| Broker/service dies during replacement | Partial privileged transition | Use the broker's exact rollback result; otherwise leave it inactive and escalate to a reviewed self-upgrade/recovery plan |
| Supervisor service epoch changed unexpectedly | Restart or replacement occurred outside the reviewed transition | Stop and re-pin only after cause and journal are understood |
| `result=noop` replay changes files or service epoch | Idempotency contract violated | Stop, preserve inode/time/PID evidence, and open a source defect; do not run a third apply |
| City-scoped tmux or worker residue remains | Quiescence is not proven | Drain naturally; if only a verified childless, sessionless tmux server remains, dispose of it with one reviewed graceful `tmux -L city kill-server` |
| Config or pack/lock/cache mismatch after apply | Runtime authorities diverged even if the binary is correct | Keep rigs suspended and restore/reconcile through the owning generator or manifest; never hand-edit generated fragments |

## Routine operations

Managed operation after the cutover should be boring:

- **Before routine work:** run readiness, confirm the supervisor's installed
  and `/proc` digests agree, check service epochs, and verify the intended rig
  is the only rig eligible to resume.
- **Start one rig:** route one bounded bead, resume only its rig, require exactly
  the expected worker/session and claim parity, then observe the bounded run.
- **Stop one rig:** suspend, drain naturally, prove zero sessions, workers, and
  city-scoped tmux residue, and record the terminal bead state.
- **Inspect signing/provisioning:** use service, cgroup, journal, receipt, and
  broker evidence from host context. An empty cross-UID process list from a
  worker or reviewer sandbox proves nothing.
- **Refresh canary evidence:** use one fresh bead and isolated worktree, invoke
  the managed helper exactly once, verify signature and v2 receipt parity, then
  suspend and retain the evidence. Never edit an old receipt to make it current.
- **Open the managed-product dispatch gate:** only after provisioning doctor,
  rules/grants, provider, runtime, and canary-receipt checks all pass. Route one
  bead to one rig and require claim parity; platform health alone does not
  authorize dispatch.
- **Prepare the next release:** begin from the currently installed manifest and
  receipt, preserve the exact previous artifacts, create a new release ID and
  backup paths, and rerun the full authority matrix. Never mutate an old
  canonical manifest in place.
- **Reboot recovery:** let enabled sockets and managed units establish fresh
  epochs, then run readiness and re-pin PIDs/start times. Historical PID pins
  never survive a reboot.

## Evidence checklist

Keep a durable, immutable copy of:

- authorization text, signed source head/tree, hosted-CI result, merge commit,
  candidate artifact, and reproducible-build transcript;
- unsigned and canonical manifests, complete-file and internal manifest
  digests, dry-run, rollback dry-run, and exact ordered mutation plan;
- pre/post hashes, modes, owners, ACLs when applicable, retained backups, and
  atomic rollback result;
- supervisor and related service unit properties, `/proc` executable digest,
  cgroup membership, journal window, PID/start monotonic time, and restart
  counts;
- canonical configuration and resolved configuration, managed fragment,
  pack/lock/cache, template, provider, validator, and rules digests;
- activation receipt, its self-digest, broker operation receipt if used,
  doctor report, replay `result=noop`, and rollback verification;
- rig/session/process/tmux censuses before and after, plus two stable
  reconciliation observations;
- every refusal and failed-attempt record. Do not overwrite or clean evidence
  just because a later attempt passes.

## Worked 1.4.1-local evidence

This appendix records the completed 2026-08-21 `1.4.1-loucmane.2-managed-platform`
cutover as historical evidence, **not** as a statement of the current runtime.
It demonstrates the bind expected from future local releases.

| Evidence | Recorded value |
|---|---|
| Source/live commit | `754d1fe8cc49f5e99cb7c081abe58eda5fc6ea82` |
| Candidate artifact SHA-256 | `9234c71546ca9d55458d42675dda87ed92f2dcc43dedc335bf2a455e047e1380` |
| Previous runtime SHA-256 | `54160a7737dc319557ca62b05d589eb75340bcbaf47391cecf944bfe1b936024` |
| Internal manifest digest | `13caaa393899f5a5d7eec44e165fac5836c0b5c17482190b4b2a165e1d565a89` |
| Canonical manifest file SHA-256 | `5f41cc8c60e03fc55f2c654a1920df04e3ae69236230e51d86a4acc344caa0c6` |
| Control-rules SHA-256 | `77af2666ef5fb80714c1d6500bcfee056055e9b54787ea4800cad97a13852ff6` |
| Validator SHA-256 | `7fa914ef070a96d1d5b6aa1b444bc5487b214a20d784511e76c21a3ed1c7c320` |
| Receipt self-digest | `fc3f7910253ad3c1d44f9142eebd0b6f29167e60efd7dff651b23c09859fadf9` |

The authorized cutover made one supervisor transition from PID `2304848` to
PID `3375641`. The installed binary and `/proc/3375641/exe` both matched the
candidate artifact. The retained backup at
`.gc/platform/backups/gc-54160a7737dc319557ca62b05d589eb75340bcbaf47391cecf944bfe1b936024`
matched the prior runtime. `gc doctor`
reported 104 passed, 11 warnings, and 0 failed checks; the managed-platform
integrity check was clean. Two separated observations remained stable while
HPFetcher stayed suspended. The rollback dry-run listed seven expected reverse
steps without applying them. A separately authorized identical apply returned
`result=noop` and left the supervisor PID and executable inode, size, and
modification time unchanged. The governing bead was closed PASS only after
those facts and the receipt were recorded.

## Hard stops

Stop and return to review on any of these conditions:

- the manifest, source head, artifact SHA, previous runtime, or managed-file
  baseline differs from the approved record;
- a pinned repository is dirty or at another commit;
- a provider entrypoint is dangling, resolves elsewhere, or reports another
  SHA/version;
- dry-run lists an unexpected path or mutation;
- the command requests credentials, permission widening, a second restart, or
  an implicit retry;
- doctor reports any post-install drift;
- a product rig or live process differs from the captured transition baseline.

Never hand-copy the binary or managed files to "finish" a partial cutover.
Never delete the canonical manifest, receipt, backups, evidence worktrees, or
failure transcripts as part of recovery.

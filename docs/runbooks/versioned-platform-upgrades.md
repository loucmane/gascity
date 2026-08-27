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

## 1. Establish the authorities

Before building anything, record these values in the change review:

| Authority | Required evidence |
|---|---|
| Candidate source | Clean signed Git commit; full 40-character lowercase commit ID |
| Previous runtime | Running executable path, SHA-256, embedded commit, and exact `--version` output |
| Pack and template | Exact source, commit, and content digest used by the city |
| Managed files | Candidate path/SHA/mode and either an absent destination or exact previous SHA plus backup path |
| Providers | Stable entrypoint, resolved executable, SHA-256, version arguments, and exact version output |
| Permissions/config | Exact rules digest and the reviewed config or permission-profile revision |

Stop if the live baseline differs from the reviewed record. Do not "update the
numbers" during the cutover; rebuild and re-review the manifest instead.

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

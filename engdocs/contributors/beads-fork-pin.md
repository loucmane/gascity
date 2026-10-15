# Pinned beads fork exception

The integration branch needs bd's #7203 CLI compare-and-swap flags, a public
read-only store open, and native proxied reads. Until upstream bd and Gas City
releases provide all three, the operator has approved one local module
replacement for `github.com/steveyegge/beads`.

`scripts/beads-fork-pin.json` records the reviewed module, relative checkout
path, full Git commit, and SHA-256 content digest. `go.mod` must name that exact
path, without a source or replacement version. Every other replacement,
including a released-version replacement and a second copy of the approved
entry, fails. A module graph with no replacements remains valid.

Run `make check-gomod-replace` to verify the exception. The same verifier runs
in CI, release, and release-candidate workflows and before and after the pair
build. It checks the checkout HEAD, content, and index/worktree cleanliness.
Its content digest covers sorted JSON records of each relative file path,
executable bit, and file SHA-256, excluding only the root `.git` metadata.
Ignored and untracked files are included; symlinks and special files fail.
This catches changed files hidden from Git status by `assume-unchanged`.

The existing repository-source custody, module resolver, native dependency,
and storage-provider boundaries still apply. The module-graph Go test checks
the approved replacement tuple; the release verifier checks its actual bytes.

## Prepare and qualify the pair

Provision a detached checkout of the reviewed commit at the manifest's path
relative to this repository. The offline build script can clone it from the
local sibling `bd-src` repository if absent. CI must provision the same checkout;
a missing checkout is a release failure. Updating the pin requires review of
both the new commit and the digest, followed by the qualification run.

CI and release jobs must provision `../beads-vfug-<full-commit>` **before**
`make check-gomod-replace`, Go module resolution, or a build. The existing
CI/release verifier steps do not fetch this fork. Their checkout of Gas City
alone is insufficient: those jobs fail until the reviewed fork is available
at the manifest's sibling path. Supply the reviewed fork through an accessible
Git repository or a staged local `bd-src` clone, and check out the manifest's
full commit detached. Preserve Git metadata for the HEAD and clean-index checks.
The verifier must then pass against the manifest's content hash; do not replace
it with a branch-head checkout or update the digest during CI.

## Dedicated host qualification

The operator provisions the unprivileged Linux account `vfugtest` with passwd
home `/work/vfugtest` and the Docker group, and allows `sudo -u vfugtest`.
The operator home remains mode 0750. No root runner or user/mount namespace is
used. The former `vfug-host-root.sh` and its tests are removed.

As loucmane, stage the committed inputs into a **new** directory under /work.
All sources are explicit; nothing in the test environment falls back to the
operator home. Substitute the reviewed checkout/tool paths on this host:

```bash
VFUG_GC_SOURCE="$PWD" \
VFUG_BD_SOURCE="$PWD/../bd-src" \
VFUG_GO_SOURCE="$HOME/.local/share/go/1.26.7" \
VFUG_MODCACHE_SOURCE="$HOME/go/pkg/mod" \
VFUG_TOOLS_SOURCE="$PWD/../r9/tools" \
VFUG_LINT_SOURCE="$PWD/../local-r5/bin" \
bash scripts/vfug-stage.sh /work/ga-vfug/stage-r11
sudo -u vfugtest /usr/bin/env -i PATH=/usr/bin:/bin \
  bash /work/ga-vfug/stage-r11/gc/scripts/vfug-test-run.sh \
  /work/ga-vfug/stage-r11 all
```

Staging refuses dirty gc/bd sources and a mismatched fork pin. It locally clones
the gc and bd commits and places the detached beads checkout at the exact
sibling path required by go.mod. Clones have no origin. It copies Go, the module
cache, Dolt tools, lint and codegen tools; installed tool symlinks are
dereferenced into standalone files. gc's tracked symlinks are preserved and
hashed as link text, without reading or chmodding their targets.

The world-readable stage records `manifest.json` with SHA-256 for each input
tree, including Git metadata, directory/file inventory, executable bits and
file bytes. Staging prints the manifest's SHA-256 for the coordinator's record.
This is an integrity manifest, not an authentication mechanism: the coordinator
controls the reviewed stage. Never reuse or change a stage during a run.

The test entry verifies the manifest, copies inputs into
`/work/vfugtest/ga-vfug/run.*/inputs`, verifies the copy, and makes its own
repositories and module cache writable. It executes only the copied runner.
Logs and built binaries live in that account's run tree; log directories and
files are readable by loucmane. Ordinary fixture HOME, TMPDIR, GC_HOME, GOPATH,
GOBIN, tmux paths and a writable global Git config are under that run's
`fixtures` directory. `proxied-acceptance` alone sets HOME to the account's
passwd home so the supervisor ownership check agrees. Existing tests that
restore passwd HOME still land in the dedicated account's /work home.

`vfug-env.sh` refuses a user other than vfugtest, a passwd home outside
/work, and a fixture root outside that account home. The old `--private-home`
and `--inside` entry paths no longer exist. The child receives an explicit
environment, offline Go module settings, staged tools and fixture-only GOBIN.
No operator session, endpoint, credential or runtime environment is inherited.
Docker, tmux, a C compiler and other standard host utilities must be installed;
the runner does not provision accounts, tools or services.

The runner builds bd/gc twice with Go 1.26.7 and compares binary hashes before
running dependent suites. Each check has its own log below the printed
`logs/run.*` directory, with a 30-second heartbeat. `summary.json` records
failures and skip counts; a failed pair blocks dependent checks and yields a
nonzero exit. Independent policy checks still run. The final fork and gc
integrity checks detect changes to the reviewed sources.

## Metadata custody on the fork build

Metadata-only adoption is unavailable on this fork build. Manifests with a
`metadata` section are also unavailable: runtime metadata custody deliberately
refuses **every** dependency replacement, including the approved beads pin.
The build allowlist does not relax `validateMetadataCustodyBuild`.
`TestMetadataCustodyRefusesPinnedBeadsFork` pins this decision using the current
manifest's exact replacement. Remove this limitation when the fork pin itself
is removed and the normal custody qualification passes.

Full `gc platform adopt --apply` without `--metadata-only`, with
`manifest.Metadata == nil`, uses the ordinary lifecycle. See
`cmd/gc/cmd_platform.go`'s call to `platforminstall.Adopt`,
`internal/platforminstall/adopt.go:adoptPrepared` and
`cmd/gc/platform_lifecycle.go:Verify`; this route does not call metadata
custody. `TestAdoptAlreadyActivatedPlatformPublishesMetadataWithoutRestart`
asserts a nil metadata section and completes through a fake ordinary lifecycle.
This qualification does not authorize changes to a live installation.

## Proxied initialization refusal

For an existing proxied workspace, `bd init --force` refuses with
`cannot observe` when checking whether existing issues would be overwritten.
The non-mutating observation path cannot route the proxied store, so it cannot
prove that the workspace is empty. `--force` does not override this refusal.
Keep the existing workspace intact; use the supported proxied migration path
or initialize a separate empty workspace. Do not remove metadata or point the
observation at another ledger to bypass the check.

## Remove the exception

When upstream bd and Gas City releases include #7203 CLI CAS, the public
read-only open, and native proxied reads, replace the local dependency with
those released requirements. Remove the pin manifest and exception branch,
restore a strict no-replacement gate in the verifier and module-graph tests,
and rerun the resolver/custody, reproducible-pair, read-only, CAS, and release
checks. Keep the fixture isolation and progress reporting.

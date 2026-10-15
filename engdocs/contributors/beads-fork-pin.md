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

On the integration host, run:

```bash
python3 -B scripts/vfug-host.py all
```

The runner builds bd and gc twice with Go 1.26.7, compares both binary hashes,
and runs the selected bd, gc, release, proxied acceptance, and CAS/safety
suites sequentially. Each check has its own log directory under
`~/scratch/ga-vfug/host-r9`; a heartbeat identifies the active log every
30 seconds. The final table and `summary.json` retain failures and skip counts.
A failed build comparison blocks checks that require the pair. Any failure or
blocked check makes the runner exit nonzero.
The runner selects the Gas City checkout containing its own script, so invoke
it from the reviewed worktree.

All test HOME, GC_HOME, temporary files, and fixtures live below
`/work/ga-vfug`. Logs, source, and binary artifacts stay under `~/scratch/ga-vfug`.
The host needs writable `/work/ga-vfug`, Linux unprivileged user/mount/PID
namespaces, Docker for the required Dolt container suites, tmux, and the cached
Go dependencies. Put a standalone Dolt executable in
`~/scratch/ga-vfug/r9/tools/dolt`; the runner checks it before the suites.
It does not include the live city's executable directory in PATH.

Ordinary builds, unit tests, and CAS tests run with the host UID mapping and
the scrubbed fixture environment. `TMUX_TMPDIR` is also beneath the fixture.
Only `proxied-acceptance`, which starts a supervisor, uses a private mount
namespace to map the test user's account home to its temporary HOME so
supervisor ownership checks remain enabled. It masks the personal
`.beads` directory inside that namespace, without reading or changing its
contents. A private PID namespace contains test services. The host account,
live city, and personal workspace are unchanged. Unsupported namespace setup
fails; the runner never substitutes the operator HOME.
After mounting, the runner drops inherited, ambient, and bounding-set
capabilities before executing tests. Direct `--inside` invocation outside the
runner's private user namespace refuses before attempting a mount.

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

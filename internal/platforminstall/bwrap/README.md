# Metadata setup artifact

This is a narrow patch to Ubuntu `bubblewrap 0.9.0-1ubuntu0.1`, not a replacement
of the system executable. `--ro-bind-null` has no path or mode parameters. It
binds only character device 1:3 at `/dev/null` through the existing recursive
bind implementation with `BIND_READONLY | BIND_DEVICES`. Inherited restrictions,
including NODEV, are preserved. Ordinary bind/remount operations are unchanged.
The second fixed operation, `--ro-bind-urandom`, exposes only character device1:9
at `/dev/urandom`; it accepts no path or mode. Host identity is checked before
namespace entry, then structural identity and non-caller ownership during setup.
Readonly mounts protect the device inode and synthetic parent. Because a readonly
mount alone does not forbid device-data writes, this option mandates Landlock
ABI>=3 in the single-threaded C setup after final mounts and before workload exec.
WRITE_FILE/TRUNCATE are allowed only for exact final writable metadata/evidence
binds and `/dev/null`; character/block creation remains denied. No `/` or `/dev`
write grant, alias, conditional bind or unexpected writable topology is accepted.
Every ruleset/rule/restrict error fails closed; all policy descriptors close before
exec. Core's closed argv and FD discipline remain part of the contract. No Go
thread-local Landlock call or generic device-ioctl protection is claimed.
Core still verifies canonical identity, ownership/mode, readonly device mounts and
parent, capability drop, NNP, namespace isolation, cache and host invariants.

`inputs.json` pins the upstream/Ubuntu source archives, isolated build packages,
compiler and expected executable. Download the named packages from their listed
Ubuntu URLs into a retained directory and source archives from
`https://archive.ubuntu.com/ubuntu/pool/main/b/bubblewrap/`. No package is installed.
The sources remain LGPL-2.0-or-later; the archives carry the complete source and
license. This repository carries only the patch and recipe, not an opaque binary.

Run from this directory with an absent direct-child `/tmp` output root:

```sh
python3 build.py --root /tmp/metadata-bwrap-build --downloads /tmp/pinned-downloads
```

This uses the existing Ubuntu compiler/development base and extracts the pinned
Meson/Ninja/pkgconf/headers/libraries into the output root. It runs no install
target, maintainer script, namespace or runtime probe. SELinux support, PIE,
RELRO and immediate binding remain enabled. Manuals/completions are not built.
The expected output is an ordinary 0755 non-setuid executable without file
capabilities, RPATH or RUNPATH; runtime libraries remain the existing five-path
manifest closure, not the extracted build libraries. Build provenance includes
exact inputs and commands; the independent release audit binds compiler/header
and linker inputs separately. An artifact mismatch is a refusal, not permission
to refresh Core's compiled-in digest.

The recipe also verifies the exact patched C-source SHA and runs18 offline C
parser, host-vs-namespace identity and mocked Landlock-policy regressions.
These are wiring tests, not kernel enforcement. The retained real-helper proof
must separately prove parent/descendant reads, denied write opens and privileged
ioctls, unchanged cache, actual inspector execution and publication compatibility.
Same-parent rename-back is not full transaction rollback acceptance.

Core accepts relocation only of `Runtime[0]`: its exact executable SHA is fixed
in reviewed source, its canonical file pin must appear in the readonly input
closure, and its actual loader argument uses that same path. The other five
runtime paths stay exact. There is no PATH lookup or fallback to system bwrap.
Use a reviewed retained owned artifact path for a transaction; no new installation
is required. Source/build PASS is not namespace or metadata-adoption acceptance.

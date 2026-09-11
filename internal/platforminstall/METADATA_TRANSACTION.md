# Versioned metadata-only transaction

Runtime status, 2026-09-11: the exact signed source candidate
`c271c7a4ec0dfb8f7c326842f39a94552bfc2468` passed one independently reviewed
actual-host V/confined-W synthetic positive (full-positive R7). The receipt,
FINAL9, unchanged-cache and bounded teardown evidence is recorded in
[the R7 result](../../engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/reports/20260911-writer-environment-r7-pass.md).
This is not live adoption, exhaustive failure-matrix coverage or provider parity.
Earlier helper-only R6 evidence and consumed failed full-positive attempts remain
historical, not retroactive runtime acceptance of a different image.

## Callable path

`gc platform adopt --manifest <reviewed-v2-manifest> --metadata-only --dry-run`
and the corresponding `--apply` call `RunMetadataTransaction`. They cannot use a
legacy Lifecycle implementation as substitute verification. Dry-run still verifies
the actual host, obtains a nonrenewing lease and confines all integrity helpers;
it publishes no metadata. General installation/adoption/rollback remain separate
and refuse the irreversible v2 transaction rather than applying old rollback rules.

The private `__metadata-writer-v1` entrypoint accepts no additional arguments.
Only the bounded request on inherited anonymous fd3 and responses on fd4 are
supported. It refuses root, wrong namespaces/environment, unexpected descriptors,
unbound input closure, missing cache lock, changed preimages or unsupported leases.

## Frozen launch inputs

- A v2 self-digested manifest binds transaction/attempt, actual boot/PID/start,
  executable path/hash, literal version, loopback listener inode/address, city,
  host namespaces and the server's fresh lease instance.
- The static writer digest and six ordered loader/bwrap dependencies are pinned.
  Bwrap is fixed at `52231e1caf55bcbc667b269f49c63599a6f7db4767ae6a039580d0ff853db712`.
  Runtime symlinks require explicit exact links and pinned resolved input targets.
- `Inputs`, `Trees`, `Links` and explicit `Absent` entries describe the whole
  import/Git/provider closure. `ImportsSHA256` binds the JSON encoding of the
  complete `CollectAllImports` result, compared independently by V and W.
  Undeclared paths are errors, never silently treated as absent imports.
- Trees contain only regular files/directories. Git worktree/common-dir/object
  dependencies and provider interpreters/libraries must be explicitly included.
  No whole home/city/root or host proc/sys/dev/run tree can be mounted.
- `GC_HOME/cache/repos` is the exact pinned read-only cache, with its existing
  regular lock opened read-only for shared flock. No repair, clone or lock creation.
  Host inventory/import reads use O_NOATIME and refuse unsupported permissions.
- Output/evidence parents must already exist with pinned device/inode/owner/mode.
  Group/world-writable parents, unrelated entries, undeclared directories,
  symlinks or multiply linked files inside writable parents are refused. This
  limits supported layouts rather than exposing arbitrary siblings or cache aliases.
  Each authorized output and retained staging/rollback/evidence name is fixed.

### Preserved siblings of canonical metadata

The optional `protected_trees` inventory allows at most 16 preserved directories,
each a canonical direct child of the canonical manifest parent. Each record binds
a `pin` (path, existing tree-content digest, full permission mode), device, inode,
UID and GID. The root owner/group must match the bound parent, special and
group/world-writable root modes refuse, and duplicate identities or overlaps with
outputs, stages, rollback, evidence, other writable parents, inputs or symlinks
refuse. These records add no paths to the import filesystem's read authority.

W mounts all writable parents first, then these children read-only. No writable
bind follows them. Before helpers or publication, W compares the actual statx
mount IDs with bounded kernel mountinfo: each parent must have one private RW
mount and each protected child a distinct immediate private RO mount on its pinned
device. Missing, stacked, propagated, writable or nested protected mounts refuse;
undeclared mounts beneath any output parent also refuse. The parser limits the
inventory to 4 MiB, 16,384 entries and 128 KiB per line, with kernel path escaping.

No-atime traversal and content checks preserve the original digest contract and
check root identity before and after inventory. Symbolic/special files,
regular-file hardlinks, duplicate object identities and any directory identity
also bound as writable refuse. V rechecks this host inventory and mount policy at
each existing grant checkpoint, within the original nonrenewing deadline.
This remains a cooperating-host contract; it does not defend against an unrelated
host process maliciously rewriting files between observations.

New backup/evidence writes must use separately declared parents, never a protected
tree. No directory is moved, deleted, repaired or chmodded into compliance.
The historical R7 synthetic did not exercise this layout: independent source
review and a fresh canonical-shaped W/descendant write-denial and preservation
proof are required before any live adoption using `protected_trees`.

## Confinement and bounds

V observes the actual `/proc/PID/exe`, runs only its literal trusted `version`
command, validates API BuildID and retains a live pidfd. It never publishes files.
It refuses incomplete listener and accepted-connection evidence and absent or
unauthorized lease routes. There is no credential import, proxy, redirect or
provider invocation in V.

W uses explicit user/PID/mount/network/IPC/UTS isolation, private read-only proc,
no capabilities, read-only root and inputs, and only declared metadata parents
writable. The monotonic time namespace remains the host's clock domain. Entire
helper descendant trees share confinement. Protocol fds are CLOEXEC and W is
non-dumpable before helpers execute. No host socket/pidfd/credential is transferred.

Outer loader environment is only `GODEBUG=containermaxprocs=0`; W additionally
gets the exact GC_HOME, HOME=/nonexistent, PATH=/usr/bin:/bin and bubblewrap's
PWD equal to the manifest evidence directory. Exactly those five distinct
names/values are required, and `/proc/self/cwd` must independently equal that
directory. No inherited override is admitted. A calling Go runtime retaining an unrelated cgroup fd is
refused; a reviewed launch must select its runtime setting before Go initialization.

Transaction context is 30s; channel deadlines 25s; nonrenewing mutation lease 28s with
at least a 2s commit margin. Version output is bounded to 8192 bytes, integrity
helper streams to 1MiB each and helper execution to 5s. Protocol frames and import
files use `metadataFrameLimit`. Unexpected nonzero helper exits remain failures.
Outer writer wait plus pidfd terminal evidence is required, not stdout alone.

## Publication and consumers

Preimages are supported only as single-link regular files with exact mode 0644
and current effective UID/GID (or explicitly absent). A 0600 file, different
group or special mode is refused before staging; rollback never normalizes it.

The same two lease handlers also accept `observation=true` for fresh observations
and dry-run. This is a bounded request/nonce/instance/clock challenge, not a
reservation or mutation grant. Begin and check both refuse an occupied mutation
slot; concurrent observations may coexist and leave no predecessor reservation.
Checks contact the actual process every time, never accept a saved proof file.
W binds observation to plan-only; observation evidence cannot become a receipt.
Apply retains an exclusive nonrenewing slot. After successful writer
termination before its transaction deadline, V waits for natural lease expiry
before returning success. No release endpoint or renewal is added; interruption
fails and committed images remain incomplete evidence. This bounded wait does
not claim that a competing transaction cannot acquire the slot after expiry.

PREPARED binds the receipt and frozen request. V revalidates before preparation,
before write grant, immediately before final receipt grant and after COMMITTED.
W retains intent/stages/backups, rechecks preimages and parents, publishes the
manifest, then performs the irreversible final receipt rename and directory fsync.
Precommit errors restore only the exact known manifest postimage. After rename,
errors/EOF/expiry/fsync failure are `committed_evidence_incomplete`; no undo/replay.
Final observation is evidence, not an independent execution-success certificate.

Automatic v2 replay/NOOP is unavailable: a committed pair plus matching FINAL
does not establish successful outer termination or the subsequent expiry wait.
W refuses an existing no-op candidate before further helper validation or writes;
V independently refuses a NOOP response. The same replay refusal applies to a
dry-run of an already committed pair, not to fresh observation/dry-run challenges.
No completion proof file is accepted or produced. Nothing deletes FINAL, changes
the committed pair, rolls it back, or reclassifies a failed wait as successful.
ReadCommittedPair remains available for read-only diagnosis; no automatic recovery
or completed-idempotence claim follows. Legacy behavior is unchanged.

`ReadCommittedPair` captures receipt/manifest/receipt and validates that exact
pair. V2 dispatch and its canary path additionally call fresh `VerifyMetadataRuntime`.
Doctor remains metadata integrity, not liveness or adoption authority.
Legacy v1 dispatch is unchanged: expanding fresh verification to every legacy
dispatch was rejected by tool auto-review and was not retried or circumvented.
This unresolved applicability boundary must not be represented as repaired.

## Connection-bound responder and accepted-socket custody

The approved continuation6 contract replaces global listener-holder exclusivity;
it does not claim the old unprivileged scan became complete. V retains one direct
IPv4 loopback connection, not an HTTP connection pool. All requests are sequential
HTTP/1.1 on that object. Only GET /health and POST to the exact city's two existing
metadata-lease routes are constructible. City names are bounded ASCII letters,
digits, underscores and hyphens, never escaped paths. No CONNECT, Upgrade, proxy,
redirect, retry, reconnect, alternate route, credential or multiplexing exists.

Before each request and after its bounded response while the connection remains
open, V brackets reverse-tuple/accepted-inode possession with boot/PID/start,
image/hash, UID and pidfd checks. Both network namespace links are checked before
and after the target FD read. Client inode, socket cookie, endpoint tuple and
established state remain exact. EOF, unread unsolicited data, missing/ambiguous
rows or FD links, changed cookies/inodes/namespaces or an unavailable observation
refuse. A target holding the shared listener but not this accepted socket fails;
sharing only the listener is permitted when the target accepted this connection.
There is no unrelated-process scan and no resampling of a replacement socket.

Replies require HTTP/1.1 status 200, explicit bounded Content-Length, no
Connection/Upgrade/Transfer-Encoding, no unsolicited trailing bytes, and exact
JSON. Headers are limited to 8192 bytes; each complete exchange has a maximum
five-second deadline additionally bounded by its caller. This deliberately
refuses chunked responses (including a large health response that Go elects to
chunk). A proposed chunked-response extension was refused by tool review and
was not applied or bypassed. The connection is permanently failed on transport,
inspection, response-profile or decode failure. Successful Body.Close does not
close the retained fixed-length connection; post-response inspection comes first.

Tuple/inode possession alone is NOT authorship or exclusive accepted-FD custody.
The load-bearing trusted-code property is that the exact pinned supervisor's
serving, child, error and restart paths do not hand this accepted socket to any
other responder. The independently audited closed health/lease profile stays in
Go's local HTTP handlers; separate service routes can legitimately hijack/relay,
but cannot be selected on V's connection. Ordinary exec children receive only
explicit mapped descriptors; the accepted socket is CLOEXEC, and raw child setup
does not return into Go request serving on error. CLOEXEC alone is not fork-proof.
Deliberate accepted-FD inheritance, export/import, ExtraFiles/stdio mapping,
SCM_RIGHTS, callback injection, hijack or relay of this connection is unsupported,
not claimed dynamically detected by snapshots. Hostile root or same-UID code
injection/descriptor theft is not solved by this cooperating-software contract.

The serving image must equal the verifier's own reviewed complete executable
SHA-256, as well as the frozen Core manifest pin and actual /proc/PID/exe hash.
Embedded build information must match the closed Go 1.26.7/linux-amd64-v1/gc,
static non-cgo, trimpath executable profile with no extra build settings or
module replacements and the exact Huma v2.37.3 module checksum. Thus a matching
version string alone cannot establish the supported serving profile. The source
test pins the audited owning server/middleware/child chain and critical Go accept,
serve, exec and error paths. A change requires renewed owning review, not a runtime
proof flag or sidecar. Complete dependency/compiler input hashes and the resulting
binary hash remain release-review evidence: the guard is not a cryptographic
attestation against malicious rebuilds or a substitute for source-to-binary review.

## Historical source-stage acceptance inventory

The following inventory records the continuation6 source-stage obligations,
before the compositional review and R7 positive. It is preserved to distinguish
what was originally unproved from the later evidence; its old source-only and
pending-fixture statements are not the current result. The approved composition
uses separate kernel-component evidence for the relevant negative mechanisms
and the exact unchanged-image positive path. It does not turn helper-only
failure matrices into production end-to-end proofs. Final full-suite/CI and
delivery gates, exact live-adoption validation and provider acceptance remain.

The continuation5 replay delta independently passed source review; conservative
automatic-v2-replay refusal, M1 and observation separation are unchanged.
Continuation6 is source only and requires independent exact-candidate review.
The custody audit is conditional on this final profile, not overall SOURCE PASS.

Pure tests cover schema/M1 maps and aliases, publication fault injection, host
identity errors, lease binding, closure/refusal, exact argv, output bounds and
CLI routing. Existing fake-Lifecycle tests are explicitly legacy regression only.
They do not prove production subprocess, namespace, descendant or race behavior.

Fresh input-bound acceptance still owes actual kernel observations for the retained
connection: correct target, wrong accepter on a shared listener, target accepter
on a shared listener, closure/restart/exec/instance loss, namespace/tuple/socket
replacement and unsupported routing/build profile refusal. A delegated accepted-FD
fixture must be labeled outside the audited profile, never an invented detection
PASS. Exercise small fixed-length health/lease replies and truthful refusal of
chunked responses. This candidate's changed V and unchanged W closure need a
reviewed production-path fixture; no prior consumed synthetic root is replayed.

Independent consolidated source review must precede an input-bound disposable
production-path fixture. Reuse R6 only for unchanged algorithm reasoning; this
writer, GC entrypoint and dependency closure require their own reviewed execution.
Final required owning/full suites and hosted CI, signing/review/delivery gates and
separate service activation authority remain outstanding. No live adoption follows
from successful source tests or a historical synthetic receipt.

---
title: "Proxied native reads in this fork"
description: "Transport selection, conditional CLI writes, and verification for bd-owned proxied scopes."
---

Proxied scopes use native reads by default in this fork. This avoids a bd
subprocess for every authoritative candidate read in `gc bd ready`. Writes
continue through BdStore and the bd CLI. Managed claim writes require a bd
build that supports `--if-revision`; the pinned v1.3.1 CAS backport supplies it.

The default lives in `internal/beads/proxied_flag.go`, at the store factory
boundary. It applies to the supervisor, controller, interactive CLI, and gc
invoked by workers. No service environment or worker prompt must enable it.

| Setting | Result |
| --- | --- |
| `GC_BEADS_PROXIED_NATIVE` unset or empty | Attempt native reads for persisted `proxied-server` scopes. |
| `GC_BEADS_PROXIED_NATIVE=1` or `true` | Explicitly enable the same selection. |
| `GC_BEADS_PROXIED_NATIVE=0` or `false` | Use the bd CLI for proxied reads and writes in that process. |
| Other nonempty values | Disable proxied native reads. |
| `GC_BEADS_FORCE_FALLBACK=1` | Use the existing global native-store escape hatch; it takes precedence. |

The proxy flag affects only proxied scopes. Admission still verifies ownership,
process birth, endpoint identity, both schema cursors and ignored-table
sentinels before opening the library. Each library open obtains fresh evidence
and checks its post-open observation. Refusal uses the CLI fallback and retains
the reason in diagnostics. A finite-idle proxy cannot host a long-lived native
handle; the existing admission rule continues to select the CLI there.

`ProxiedStore` owns a read-only native leaf and a BdStore write leaf. Mandatory
conditional-writer resolution, mode-based resolution, metadata CAS, atomic
close, and cache forwarding retain the wrapper. The wrapper forwards capability
probes and runtime refusal state to its write leaf. It never substitutes an
unconditional write when conditional support is missing. Managed continuation
pins read their configured work store, recheck route and group membership, and
use the same mandatory revision writer. Legacy unmanaged continuation pins
retain their existing status/assignee predicates.

After a CLI write, a changed or unobservable proxy generation retires the native
read handle. This also covers a record removed before the CLI restarts its
proxy. Subsequent reads use the CLI until a fresh admission succeeds. The
wrapper shares one local sidecar between both leaves. Graph creation still
uses the bd graph applier directly; managed claim operations do not use it.

The native leaf rejects domain mutations, but the linked beads library does
not expose a completely side-effect-free open. The schema admission and
post-open checks constrain and detect its open-time behavior; they do not
constitute a database-level read-only transaction. Keep the linked library and
deployed bd schema aligned. A post-open refusal cannot undo work performed by
the library during that open.

`gc doctor` reports native reads over the bd proxy and that claims use CLI
`--if-revision` CAS. It also reports a handle that has fallen back after opening.
The message describes the write route; actual conditional support remains
subject to BdStore's capability probe and runtime refusal latch. Doctor retains
the stopped-store gate and does not start a stopped proxied store.

For rollback, deploy the prior binary or explicitly set
`GC_BEADS_PROXIED_NATIVE=0` for every process being tested. An environment change
does not replace already-open stores: restart the affected long-lived process
through the coordinator's deployment procedure. Keep the CAS-capable bd binary
when rolling back native reads; the managed claim guards still require it.

Host verification uses disposable scopes and the CAS backport on PATH:

```sh
PATH=/var/tmp/ga-vfug-bd/bin:$PATH GC_TEST_PROXIED_CAS=1 \
  go test -tags=integration ./cmd/gc -run '^TestProxiedNativeClaimCAS$' -count=1 -v

PATH=/var/tmp/ga-vfug-bd/bin:$PATH \
  GC_ACCEPTANCE_BD_BIN=/var/tmp/ga-vfug-bd/bin/bd \
  GC_REQUIRE_ACCEPTANCE_TOOLING=1 \
  go test -tags=acceptance_a ./test/acceptance \
  -run '^TestProxiedNativeReadyForkBound$' -count=1 -v
```

The first test checks production factory selection, one winner in a conflicting
work CAS pair, native reads after CLI writes, and session claim stamp/conflict/
clear. The second requires native selection with the flag unset, checks eight
ready candidates, and fails above three bd subprocesses. A skip is not a host
verification pass.

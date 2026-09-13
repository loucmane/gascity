# Bounded native task attempts

`ga-ecwh.5` adds an opt-in one-attempt contract for a driving work bead. Pool
capacity limits concurrent sessions; it does not limit sequential failed starts.
The two controls are intentionally independent.

## Authority and state

Before routing, an authorized controller can stamp the exact canonical string
returned by `internal/taskattempt.Request(storeRef, beadID, template)` into
`gc.native_task_attempt` using the supported Beads metadata API. The store ref is
`city:<configured-name>` or `rig:<configured-name>`. This record grants neither
routing nor lifecycle authority. Example canonical value:

```json
{"schema":"gc.native-task-attempt.v1","store_ref":"rig:fixture","bead_id":"fixture-task","template":"fixture/worker","limit":1,"state":"requested"}
```

The scheduler atomically changes `requested` to `reserved` before creating a
session bead and copies the generated token into that session's
`gc.native_task_attempt_token`. Immediately before the native provider's Start,
the session manager re-reads session and work metadata from their authoritative
stores and atomically changes `reserved` to `started`, binding the session ID.
That successful transition authorizes one Start call, not one concurrent worker.
Both transitions use the existing narrow metadata compare-and-set capability.
Missing CAS capability refuses; there is no read-then-write fallback.

Work authority is separate from the session-coordination store. CLI paths open
only configured work roots and close their owned handles. API paths borrow the
controller's work stores. A same-ID unbounded bead in a relocated session store
must not bypass the work contract.

## Refusals and recovery

Failed session creation spends the reservation. Startup failure, cancellation
after consumption, stale-key fallback, controller restart, closing/reopening the
task, and ambiguous CAS responses never refund an attempt. An already-started
record refuses even for the same session. Missing/changed token, task/store or
template bindings, malformed records, and inaccessible authority refuse before
provider Start.

There is deliberately no reset or automatic refund. Preserve the consumed task,
sessions, record, and evidence. A separately authorized append-forward successor
uses a fresh task after containment and the failure cause are established; do
not replay or rewrite a consumed policy.

## Boundaries and proof

The bound applies to native starts associated with the driving task. Tasks with
no policy retain their existing behavior; sessions without a driving task remain
legacy/unbounded. This is not a claim-time fence for a manually created session
that later claims arbitrary work. It is not protection against an administrator
removing all policy and trigger metadata. Operational packages must preserve
those bindings and prevent unrelated demand during a bounded run.

The bound does not replace permission, managed-receipt, identity, orphan-cleanup,
signing, subscription-authentication, or lifecycle checks. Nor does it establish
successful inference or provider parity. Runtime adoption still requires the
reviewed delivery and live-validation gates.

Regression coverage includes concurrent CAS contenders, response loss after a
committed reservation, scheduler reconstruction, on-disk persistence, separate
session/work stores with same-ID decoys, current-metadata drift, and the real
session manager's stale-key fallback. All provider calls in these tests use a
fake runtime; no production worker is launched.

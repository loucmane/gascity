---
title: Managed Worker Contracts
description: Exact-profile provisioning, policy integrity, and scoped canary receipts.
---

Last verified against source: 2026-09-08. This describes the source protocol,
not proof that a particular host or provider has been provisioned or tested.

## Provisioning and launch

`internal/managedworker` owns the receipt protocol. The CLI and controller use
that package rather than duplicating its validation. Receipt membership uses
the actual resolved qualified agent identity; it never uses a role-name suffix
or a provider label as permission.

Provisioning v2 supports an explicit `profile_kind`:

| Kind | Signer contract | Control policy | Canary contract |
|---|---|---|---|
| Omitted (legacy) | Existing managed signer required | No new field | Existing unscoped v1 proof |
| `signing` | Existing managed signer required; `none` refuses | Optional, but fully checked when declared | Exact-profile v2 signed proof |
| `candidate` | Exactly `none`; signer probe is not invoked | Required | Exact-profile v2 unsigned proof |

The profile digest includes every declared launch-control field, including kind
and policy. Omitting the new fields preserves the existing v1/v2 digest domains.
Explicit null, unknown kinds, duplicate control fields and alternate spellings
of the new fields refuse; they are not legacy defaults. Typed profiles require
the v2 environment/toolchain contract. None of the existing argv, environment,
check-path, permission-revision, provider or signing checks is removed.

`control_policy` binds a clean absolute file path and its SHA-256. The path must
occur exactly once in the bound argv and be outside every worker-writable root.
Its `_gc` object declares the matching kind and `gc.worker-control-policy.v1`,
or the existing candidate-only `gc.candidate-control-policy.v1` marker.
This marker does not grant permissions: the adapter must prove that its actual
argv/settings, sandbox, hooks and native permission surface enforce the policy.

Launch preflight uses a dedicated safe reader, not the general read callback.
It rejects symlink path components, non-regular nodes and group/world-writable
policy files. The existing filesystem no-follow regular-file reader is reused;
device/inode identity, size, mode and modification time are checked around the
read. An unavailable safe reader or identity is a refusal, not a fallback.
Rules and the Bead's check-path remain independently hash-checked. Policy
integrity is checked before provider, toolchain, readiness or signer probes.

The protected installation and worker sandbox remain required. These path
checks neither reserve the pathname against an authorized host writer nor
prove ownership, a merge lineage, or effective provider permissions. A reviewed
producer must install a stable merge-bound policy and retain its transactional
backup and verification evidence; passing a JSON marker is insufficient.

## Scoped canaries

`gc platform canary --profile <resolved-name> --profile-kind candidate|signing`
selects exactly one receipt entry. Both flags are required together. The
no-profile compatibility form is restricted to one unambiguous legacy profile;
multiple profiles and typed profiles require explicit selection.

The runner receives the real selected profile through
`GCT_CANARY_WORKER_PROFILE_JSON`, including its actual name, argv and kind. The
provisioned runner digest and all environment/provisioning bindings must match
before scenarios execute. Production environment observation rechecks declared
policy files. The CLI additionally checks the selected policy before and after
each scenario; a failure cannot publish a PASS receipt.

Signing retains the existing nine scenarios and nine clean-launcher steps.
Candidate mode requires `candidate-launcher` with the same launcher steps minus
`sign-candidate`, and refuses `signed_candidate=true`. An unsigned candidate
result is not managed-signature, delivery, authentication, provider-parity or
handover acceptance.

Canary v2 binds one `profile` object with exact `name`, `profile_kind`, and
profile `sha256`. The full environment inventory is also bound, but inventory
membership does not mean every listed profile was exercised. Missing or extra
scenarios and kind/name/digest cross-selection refuse.

Current v2 receipts live under `.gc/runtime/canary/profiles/<name-sha256>.json`;
the existing digest-addressed history retains previous results. They do not
overwrite the legacy city-wide receipt. `VerifyProfileCanaryReceipt` requires
the exact expected name and kind. The legacy `VerifyCanaryReceipt` refuses all
v2 receipts, and unscoped v1 receipts cannot carry newly typed profiles.

CLI and HTTP sling pass the resolved rig and full qualified target to the same
product-dispatch gate before Bead/workflow mutation. The target must match exactly
one configured agent in that rig. Product routes require signing-kind coverage:
a candidate proof, or another profile's v2 proof, cannot authorize the route.
The profile-scoped path is preferred; only its absence permits legacy fallback,
and the valid legacy provisioning inventory must contain the exact target.
Unreadable, malformed or mismatched scoped evidence never falls back. Batch
expansion retains one gate evaluation for its unchanged target.

The rig-only gate used by unrouted formula construction still consumes only
legacy v1 authority; it does not infer profile coverage from a v2 inventory.
Control-plane rigs retain their existing exemption. Producer interoperability,
profile provisioning and live receipt-bound acceptance remain required; source
tests alone enable none of those host transitions.

## Regression ownership

- `internal/managedworker/profile_contract_test.go`: strict wire fields,
  legacy digest goldens, candidate/signer separation and marker integrity.
- `internal/managedworker/policy_file_test.go`: unsafe nodes and read-time
  mutation, including real-filesystem timestamp observation.
- `internal/managedworker/canary_profile_test.go`: exact-target storage,
  scenario contracts, cross-selection and publication isolation.
- `cmd/gc/managed_worker_policy_test.go`: production policy-reader composition
  with disposable files and non-executing provider callbacks.
- `cmd/gc/managed_worker_typed_launch_test.go`: resolved configured identity,
  stable pre-resume argv, environment and routed check-path reach the actual
  controller preflight before a fake runtime Start; candidate and signing
  retain different signer requirements, and mismatches stop before launch.
- `cmd/gc/cmd_platform_canary_profile_test.go`: real command selection,
  exact selected profile input and policy refusal before receipt publication.
- `cmd/gc/cmd_platform_canary_transport_integration_test.go`: one real, bounded
  subprocess round-trip of argv, CWD, complete profile JSON and response decoding.
  The synthetic shell runner invokes no provider or city operation. This does
  not certify the separately versioned Template runner's profile selection.
- `cmd/gc/managed_profile_dispatch_test.go`: exact configured target, signing-only
  product coverage, scoped-path precedence and strict legacy membership.
- `internal/sling/dispatch_gate_test.go` and the API dispatch-adapter tests:
  exact target forwarding, pre-mutation refusal and one evaluation per batch.

These tests use synthetic receipts and fixtures, not host canary evidence.

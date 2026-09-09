# ga-ecwh.2 — tracked conformance-resource audit adjustment

## Preserved parent and failure

Signed parent: `57f681a6b87c08cdfffd3c9e993c314cb9a1f09f`, tree
`2dbcb5105355a3bd891e88e66684ebed19d1120a`. It is not amended or replaced.
The normal pre-push run passed nine jobs; unit-core failed only the resource
census at 552 subprocess calls / 166 files versus the pinned 545 / 164.
The push did not publish. Both original push logs and complete job logs remain
preserved under `/tmp/ga-ecwh-delivery-20260909` and `/var/tmp/gp4/logs`.

The earlier pre-staging baseline scanned `git ls-files` and therefore omitted
the two then-untracked integration fixtures. It is not a green result on the
subsequently signed tree. The exact delta is six subprocess sites in
`cmd/gc/hybrid_conformance_fixture_test.go` and one in
`internal/runtime/ssh/conformance_fixture_test.go`; both are integration-tagged
and byte-identical to the independently reviewed signed parent.

## Explicit, independently reviewed policy change

Only the all-source subprocess audit changes: 545/164 to 552/166, in the Go
bootstrap policy, TOML ledger and generated TESTING table. This is an explicit
policy change, not a cosmetic update or a waiver. Untagged 406/113 and Small
401/110 budgets, historical 495/135 regex totals, owners, migration targets,
every expiry, all other rows and all validator/classifier logic remain unchanged.

The new regression in the existing tracked census test file proves 6+1 attribution,
integration tagging, the prior ceiling outside those fixtures, and the retained
tighter budgets and expiry. The repository census still enforces exact current
counts, including downward ratchets; this focused test does not replace it.

## Tests and review

- RED: the new regression failed only at the unchanged 545/164 audit row.
  SHA-256 `780a480fbd7c5b418eaf7ca96a3cb11b47c52910b1e9715d50c1e2c5679eaeb5`.
- Generated the table through the existing `TestRepositoryLedgerMatchesCensusAndDocumentation -update` surface.
- GREEN: complete resourcecensus package passed in 3.007s.
  SHA-256 `59c16278154cb29778f3d0678c90158e9bc1b26f9a1fb7dd817230bf1747e6c8`.
- `git diff --check` exited zero and intentionally emitted no output. Its empty
  log means whitespace validation passed, not a source-integrity attestation.
- Fable 5.1, one tool-free subscription review, SOURCE PASS. Result SHA-256
  `d511b4b0a81327f32cf62b07c25e71a969665303dbf1883bc9a0c26640c26405`.
  The review engine verified the signed parent, exact source/evidence pins and
  unchanged fixture bytes before and after review. The staged source inventory
  was exactly the four files named above (including the new regression file).
- Review caveats remain explicit: tests were executor-run, not reviewer-run;
  the pinned regression needs a reviewed update when the legitimate ledger
  changes; source review is not hosted CI or live acceptance.

The source workflow journal was already ready with ga-ecwh.2 attached.
Re-running bootstrap-style `resume` refused that still-open dependency.
The documented active-work `checkpoint` independently checked ownership,
scaffold/readiness and plan parity and returned ready. No dependency was removed,
no unfinished Bead was closed, and no rig or service was resumed.

The next gate is a new signed append-forward commit and the normal complete
pre-push/hosted-CI delivery path. No hosted CI, merge, activation, Claude signing
or provider-handover PASS is claimed by this checkpoint.

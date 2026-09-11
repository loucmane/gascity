# Final delivery audit — 2026-09-11

R7 synthetic runtime PASS remains bound to signed source
`c271c7a4ec0dfb8f7c326842f39a94552bfc2468` and its preserved image/run.
No runtime production code changed during this audit. This does not claim
public delivery, live metadata adoption, exhaustive failure injection, or
provider parity; those outcomes remain separate gates.

## Full fast sweep and corrections

The ten-job `scripts/test-local-parallel fast` sweep completed with exit123.
All six CLI shards, Darwin fsys compile, push-lock selftest and concurrency
selftest passed. The core job reported exactly three failing tests:

- `TestCreateEndpointsAreTriagedForIdempotency`: the two new metadata lease
  operations lacked explicit classification. They are fresh security handshakes,
  not replayable creates. Two exact exemptions retain nonrenewing-slot conflicts,
  instance/request/expiry checks, no-store, authentication, CSRF and read-only
  refusal. No cache wrapper or wildcard exemption was added.
- `TestRequiresDedicatedTestenvImportFile`: both preserved diagnostic probe
  directories lacked the canonical test-only environment scrub import. Added
  the exact untagged generated-template stubs; no environment guard was relaxed.
- `TestLocalMarkdownLinks`: a docs-site link incorrectly crossed into the
  GitHub-only source tree. It now links to the public repository source URL.

Independent delivery review also caught Linux-specific probe bodies without
platform constraints. A Darwin compile reproduced undefined Linux symbols.
Nine explicit Linux constraints fix packaging; all 28 original probe files are
preserved in the archive and every body is byte-identical. Untagged testenv stubs
remain buildable on Darwin, whose test-only packages now compile successfully.
No foreign binary or runtime probe was executed by these compile checks.

The runbook and transaction contract now distinguish irreversible v2 FINAL
commit and actual-host/confined-writer guarantees from legacy adoption semantics.
Historical source-stage acceptance text remains labeled as historical; the
new positive proof and its cleanup/coverage limits are recorded separately.

## Evidence and review

- Original full sweep: `/tmp/ga-tmgr-final-fast-validation-20260911`.
  Core log SHA256 `dde8db76bba3ed94274dad2301bb63077faf7b6f25c38a4ad45b54963370fbf4`.
- Original pending archive: `/tmp/ga-tmgr-delivery-evidence-20260911/pending-preserved.tar`,
  SHA256 `29a2502da7a6f96bc29e64b82abe0419e957da734f3bb904447299f87b1c0a91`.
  Inventory SHA256 `c50839144c480b149662af444bcd68fd47b993ef9b15aaaea5f0f5f0489f9b10`.
- Exact correction verification: `correction-verification.json` in that root,
  SHA256 `97b855279b17c64d60ba61582bce492c7228f1c6a8d7de63a678198f86cf3f04`.
  Linux discovery differs only by the two testenv stubs; runtime production
  source is unchanged from the R7 signed input.
- Fresh complete affected-package runs, `-count=1`: internal/api,
  internal/platforminstall, internal/testenv, test/docsync, test/combinedprobe,
  test/m5probe all PASS. Log `focused-validation.log`, SHA256
  `9edece238eeafc157c5233b8da0b69021b76da4dab7cabf793b0d7cab65db20a`.
- Independent Astra source review approved the precise classification and
  packaging correction without a security relaxation; final byte-level review,
  staged census, normal signed commit, full push gate and hosted CI remain gates.
- Bounded credential-pattern screening of pending UTF-8 and embedded base64
  evidence found zero candidates. This is not an exhaustive secret-detection claim.

No prior consumed runtime attempt was replayed. No real rig, service, installed
artifact, HPFetcher/Blog content or evidence cleanup occurred. All previous
FAILED/UNKNOWN classifications remain preserved. The next outcome is signed
source delivery with exact-head/base and independent review/CI checks, followed
only by separately reviewed and authorized live reconciliation.

## Append-forward normal lint gate — maintained diagnostic derivatives

The normal commit hook refused before signing; HEAD remained c271c7a4. Its
57 findings concerned the diagnostic helpers, not production runtime. Original
files remain in the archive above; the normal hook postimage is additionally
preserved in `/tmp/ga-tmgr-diagnostic-delivery-lint-20260911/pre-correction.tar`,
SHA256 `e74f0f8197f41c7074ef43762b2f6e86ef3669310e3505223fb6912168d27cda`.
The derivative is now explicitly distinguished from immutable attempt inputs
in both diagnostic READMEs. Earlier byte-identity statements describe the
pre-lint packaging checkpoint, not this maintained derivative.

The consolidated correction retains all prior verdict/error policy while making
49 previously discarded returns explicit, fixing style, and adding pure callback
coverage for metadata ordering. Site-level classification is preserved in
`ERROR_POLICY.md`, SHA256 `ba57554647904d8c8e4e0d6f549d0c35c8a504f492b8d9f25c8bea89f82a062c`.
Checked metadata publication, payload reads and terminal witnesses remain intact.

A pure negative test caught a substantive unsafe lint autofix: errors.As alone
accepted a joined observed mismatch plus unexpected EOF. Exact dynamic-type
validation now precedes errors.As, preserving the original direct-mismatch-only
contract without lint suppression. Existing wrapped/joined cases and additional
typed-nil/custom-As adversaries refuse. The failed test log is preserved.

Final independent Astra SOURCE PASS binds patch
`0b52121e7c33e5521356b156e07985b17dff8d9abfd4eba87a09f963f7568cb8`.
The unchanged lint gate reports zero issues; both pure probe suites PASS,
log SHA256 `dfc67caba0aad6d040b9b18f52fd4fdbb971967a596554a2f293d7a723e9e817`.
The staged resource-census gate also passed without any budget or policy change.
No failed historical runtime was replayed or reclassified. Normal signed commit,
full pre-push, hosted CI and exact source/base review remain mandatory.

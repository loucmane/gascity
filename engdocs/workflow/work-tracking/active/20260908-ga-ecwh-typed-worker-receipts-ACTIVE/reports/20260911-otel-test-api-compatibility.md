# OTel test API compatibility — ga-tmgr

PR #37 at signed head b90c7ffd81a9c56a47852084ec6dacc8baa205dd
completed its hosted main CI run 34628337239 with one underlying failure:
static job 103358849453 found six SA1019 deprecated Value.Emit calls in
internal/telemetry/telemetry_test.go. The three failed summary jobs only
propagate that failure. Container Scan 34628337269 and CodeQL 34628337244
completed successfully. The approved gRPC update brings OTel 1.44, where
Emit is deprecated. No blind rerun or merge of the failed candidate.

## Bounded correction

Exactly six test calls use Value.String instead. All asserted attributes
are STRING; the installed OTel 1.44 implementations return v.stringly in
both methods for that type. This does not claim all-type equivalence.
Assertions, production code, dependencies, lint configuration and resource
policy are unchanged. Reviewed resulting file SHA-256:
8358a07149316ae0c00e8fc67cc4765d1608b470dd2fbbb189e2877bceb3e683.

## Verification and independent review

Evidence directory: /tmp/ga-tmgr-container-dependencies-20260911.

- Hosted failed job: census-ci-static-failure.log,
  c0ebdd2ca621b97d9dafa35e72edc0ce5d247479bf76977f3c9cb3736245069e.
- Focused local lint RED six: otel-api-lint-red.log,
  18ce6dfb1a7830dd3bf8467cf0b548bc5cf0ae1df5fd9d143d793cff368d069a.
- Focused local lint GREEN zero: otel-api-lint-green.log,
  e92606b0bf483111dff0a120c315ea165821348f31365020e2468a0059095c47.
- Owning telemetry tests PASS (0.015s): otel-api-owning-tests.log,
  884fa4e739417dcaa292258c2efda1e2f9a6cfd3da2d8bc1028eb4c0ba97c351.
- Normal local make lint-full remains FAIL with only three warnings in
  Git-ignored node_modules/flatted (two govet, one revive):
  otel-api-full-lint.log,
  f156002023be27dd12fb4456a28af5eb023dc4f7a0fb56a3e5b4f67b1640f59e.
  No ignored dependency is changed or deleted and no check suppressed.
  The unchanged hosted full-lint gate must pass on the signed candidate.

Existing independent Astra reviewer confirmed SOURCE PASS, no must-fix,
and the narrow delivery disposition above. Its verdict is preserved in
otel-api-review-verdict.md. Focused evidence is not a full-suite or live
acceptance claim. Normal commit and pre-push gates remain required.

## Remaining outcomes

Signed exact-head/base delivery with required-green CI, CLEAN/MERGEABLE
and zero unresolved review threads remains next. Protected-directory
positive and hostile kernel proofs remain separately required before live
adoption. Existing R1/R2 positive packages remain unexecuted and bound to
their original source/build inputs; do not silently relabel or replay them.
No worker, rig, service, installed artifact, credential or protected
HPFetcher/Blog surface was changed. Full original handover goal stays open.

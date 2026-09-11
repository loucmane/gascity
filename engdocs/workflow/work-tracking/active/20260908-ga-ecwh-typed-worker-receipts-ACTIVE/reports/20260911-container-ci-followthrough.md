# Approved dependency correction — CI follow-through

The dependency source was signed and pushed as
2137af00f4c13f2845f087a7749a903b172525a8 after all ten normal local pre-push
jobs passed. Hosted CI 34620825074 then exposed the exact module budget:
730 selected modules versus the old 727 ceiling. Container Scan 34620825051
reported Thrift 0.23.0 only in the reached bd artifact; gh, Dolt and gc were
clean in that scan. Later images were not proved because scanning stops on
failure. Both failed runs and logs are preserved, not rerun or suppressed.

The operator-approved gRPC/Thrift correction includes necessary dependency
inputs and transitive changes. The exact graph has three additions, zero
removals, all proven reachable from gRPC 1.83.2. Attribution and full graph
fixture are recorded in engdocs/native-dependency-budget.md and
scripts/testdata/native-dependencies/grpc-1.83.2.txt. This is a count baseline,
not an allowlist; no same-count replacement-detection claim is made.

The consolidated seven-file correction:
- Changes only the total default budget 727 to the exact reviewed 730.
- Preserves family counts, exactly-one-Beads, binary size and every testhook check.
- Adds isolated positive/negative guard regressions and the exact graph fixture.
- Applies the already-approved Thrift 0.24.0 to the missed bd rebuild, with
  mandatory artifact-level assertion and its owning regression.
- Corrects explanatory comments. No Go dependency input, tool source ref,
  checksum, build flag, scanner or ignore entry changes in this delta.

## Verification and independent review

Evidence root: /tmp/ga-tmgr-container-dependencies-20260911.

- Seven-file guard-candidate-sha256.txt:
  32429e80f43d5263bb69a5828d8b5141a39b95c9fb590b93101911117337076b.
- guard-red.log:
  b833b8cea1c10f565ff57a17d1de926916be092d59c5524ab775359064b65150.
  Two defects, not 24 defects: missing bd Thrift override/assertion and the
  outdated total budget; the latter also prevented downstream assertions.
- guard-green.log:
  7b29c4b689bcc7c41e81ba435724091e7578165bfd52368cf310ed7c4a38098c.
  23 isolated guard cases plus the bd recipe regression PASS.
- guard-real.log:
  43a1ae40ece5d92d9db51cbf24d4a2ea15dbb522473085dc21fbdf7d6913997e.
  Actual default guard PASS, no override: modules730/AWS25/Azure9/DoltHub15/
  GoogleAPI1; normal binary260199728 bytes against unchanged270000000 cap.
- guard-owning-suite.log:
  b9979360ab247ed17b1e0e0017cd3ebf9340618425108c3dbeb8fd38e85eba17.
  Complete scripts suite PASS,29.631s.
- container-scan-new-failure.log:
  4a7f79ebc4458412ebe0ba952348235f73b13f1c24910c7914c2e943aea304b2.

Existing authorized independent Astra reviewer verified the inventory, graph
attribution and source delta: SOURCE PASS, no must-fix. Verdict preserved in
guard-review-verdict.md. Reviewer inspected recorded tests rather than claiming
a separate full-suite rerun. The sandbox-only GPG readiness read could not reach
the agent; the host-context read confirmed cached/ready without unlock or prompt.

## Next gates and unchanged boundaries

Normal signed delivery, full pre-push and hosted CI remain required on this
candidate. Fresh image builds and scans are not implied by source tests.
Fresh protected-sibling writer/descendant runtime proof remains owed before
any separately authorized live metadata adoption. Historical R7 is unchanged
and cannot be rebound to the new executable. No installed artifact, rig,
worker, service, private project content, credential or historical evidence
was changed. The full provider-independence and bidirectional handover goal
remains active and incomplete.

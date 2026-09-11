# ga-tmgr M5 source grant — 2026-09-10

The actual operator approved extending the existing ga-tmgr direct-source exception to the reviewed verifier/writer repair and independently reviewed unprivileged synthetic tests. This resolves the historical source-lane HOLD and supersedes v1 worker-only wording for this repair only; ordinary worker policy remains unchanged. The source design remains v1 + R2 + R3, with irreversible final receipt commit and no postcommit rollback/replay.

Current-work checkpoint passed at 2026-09-10T15:38:37Z on the existing READY ga-ecwh context with attached ga-tmgr and unchanged verified owner. The erroneous retired-child resume and parent kickoff-replay dependency refusal remain preserved in /tmp/ga-tmgr-m5-source-checkpoint-20260910/. No workflow code, dependencies or ownership were repaired or waived. Supported current-work checks remain intact.

This checkpoint produces only M5 synthetic harness source, pure/offline tests, inventory and a frozen review packet. Independent source review is mandatory before namespace/process-attack execution. No Core API/cmd integration, live installation/adoption, supervisor/rig/service transition, workers, root, credentials, packages or weakened checks. Parent goal remains sole active full goal; no goal operation.

Baseline preservation: /tmp/ga-tmgr-m5-candidate-r1-20260910/ contains exact staged/unstaged patches, index/untracked inventory, original dirty files archive and frozen core-source-r1 package archive with preservation.sha256. Historical reports and candidates remain unchanged; ga-oz9e deferred/nonblocking.

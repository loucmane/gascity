# Diagnostic fix: signed local delivery and exact-source build — 2026-09-11

## Actual outcome

Local delivery/build PASS only. Independently reviewed diagnostic bytes are committed unchanged; no new source correction. Normal unchanged pre-commit ran formatting, lint (0 issues), OpenAPI/client/schema/CLI generators and go vet ./... successfully. Exactly3 reviewed files committed; generators added no diff, dashboard conditions were not triggered. No signing bypass, manual source fix, repeated focused suite or runtime retry.

- Commit: f1ea84d76fce1d17626349353f758eb592386ee3.
- Parent: 5a74ab60da46e55ba6425839a3819e9a185c1803.
- Tree: 7d42a6b1b30152201c9488dcb30b06d63d846169.
- Signature: G; verified RSA fingerprint FD5585922F5335BC378AD8D42ECF4432C7E7982D. gpg.program=/usr/bin/gpg and hooksPath=.githooks unchanged. Read-only verification noted trustdb not writable but returned Good signature/exit0; no key/config change or pinentry.
- Image: /tmp/ga-tmgr-exit-diagnostic-delivery-20260911/gc-source-candidate.
- Image SHA256: e965252f15f12ed39955aa3ea8802ea5ccb8c6d1aee4ccbd29de866286968499; size133659245 bytes.
- Bounded10s clean-environment version --json exited0: commit matches, version dev/date unknown preserved, ok=true. This is the only candidate execution; no supervisor/namespace/metadata probe.

## Inputs and preservation

Three committed file hashes still match independent Astra reviewd3dd8d2ab55ecb8a40d62cc5eeb095f9929b9605eb5034db60d729081388ab7f. New test mode100644; all3 reviewed bytes/modes verified by committed-modes.txt and source readback. preimages.tar/index-before.z/status-before.txt preserve initial state; index empty after commit. All5065 pre-existing tracked regular-file preimages matched after hooks, including unrelated dirty evidence. Initial all-path checksum encountered the tracked .agents/skills directory symlink; retained output was replaced only by a separate regular-file inventory, with all modes/links preserved in the index snapshot. No reset/stash/cleanup or blanket staging.

Exact literal build argv: build-command.md SHA25615a9b23b41544d4cd1b7aa7d5926daaccc4106a26fed15430aa89cf11cd8d7d6. Pinned Go1.26.7, static CGO0, Linux/amd64/v1, trimpath, readonly/offline modules, buildvcs=false, exactly one -X main.commit. BuildInfo contains the unchanged seven custody settings; no overlay/instrumentation/replacement/download/GOCACHE override. Fresh on-disk compiler scratch retained.

The unchanged production dependency graph is reused: no import or production-input-set change. All1525 local production inputs matched committed Git blobs; all7003 source/dependency inputs and pinned tools rehashed before/after build. This is exact build provenance, not runtime acceptance.

- New source92 manifest SHA256 d6e38911c4f43e32e667bc8e416a8527354e72e63ffc19f91a7afa0165f4d7ab.
- Full build-input manifest SHA256 c0919475d4b9d02652cdef23f4a3d49c915fe7549d136e8c525f6358954f8ca1.
- Complete commit diff SHA256 aa0d37350eab28c9603e628a0be68116ceb9b4787edab84f070755604a321130.
- Normal hook/commit output SHA25670f949a33a4d2b2bcb099fff97fc8a974b24b97ac2e49cd2277fb71d304572eb.
- Old image64bdb174ed79719195f38d00877f44ce7182a8cc5b42247bc7ac5852ef37c8b4, old source91 manifest9f0330875f1fcea3405ecdc48d4471791db7430988f33756c65c5bd6f3f0dc6b and R3 result14a196d2b7d087065168d13d4cb342618cb8850b30518f092237ed4b3bea6d31 unchanged (preserved-predecessors.sha256).

## Remaining boundary

R3 stays FAILED/consumed, transaction unknown; this diagnostic fix does not establish its cause. No retry is authorized. Full fast/pre-push and required hosted CI remain mandatory before any public delivery, along with existing independent final candidate/build review and exact authorization gates. No push/merge/install/worker/rig/service/lifecycle operation occurred. Full original provider-independent execution and bidirectional handover objective remains active/incomplete; ga-oz9e deferred.

Supported owned note/log/checkpoint readback is saved separately in checkpoint-readback.md after actual completion. Historical Operations date HOLD and all failed evidence remain unchanged; no broad Operations verification claim.

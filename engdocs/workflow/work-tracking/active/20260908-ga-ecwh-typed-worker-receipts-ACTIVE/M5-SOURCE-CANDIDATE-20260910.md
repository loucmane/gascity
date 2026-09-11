# ga-tmgr M5 candidate — independent source review required

Status: OFFLINE VALIDATION PASS; SOURCE REVIEW / M5 EXECUTION HOLD. No namespace/probe, process attack, production integration, live adoption or model inference was executed.

Actual operator grant: direct source only for attached ga-tmgr repair, retaining ordinary worker-only rules elsewhere and independent review before synthetic execution/final delivery. Current-work checkpoint on the existing ga-ecwh READY context passed; prior retired-child/kickoff replay refusals remain preserved, not repaired.

Core HEAD 96e0ed0423c77ecc44237b4f9c3e667648c74a48. All20 original frozen source digests match manifest 537bdba61fb18c4fb44c5318910d619ac8e8ddedc7669593669e5d1d157630cc. Original index unchanged. Baseline staged/unstaged/index/untracked inventories and tar archives retained in /tmp/ga-tmgr-m5-candidate-r1-20260910/. No original candidate restaged or reset.

New files only: test/m5probe/plan.go, plan_test.go, main_linux.go and README.md. Offline tests first failed compilation before implementation (RED); final four top-level pure tests, including table cases, passed with count=10 (0.006s reported package time). Focused go vet passed; CGO_ENABLED=0 static compilation passed. Read-only freeze initially refused unlisted libselinux; static ELF inspection supplied its exact libpcre2 dependency and both were added to the closed pin inventory. No synthetic retry occurred.

Binary SHA256 e4fcc45784705f2b33cbc30d037304919eef691fe22ee38136f19ac863c805c1.
Execution plan SHA256 166cb14071d275028ab119f26d5710e97cc85a5a26cad62f3eaaf978ccc4a32a.
Source tar SHA256 19ad43c4766fc0324728d937ba84da9b7715fee0f250d49642f8496e2db5631e.
Exact source hashes: source.sha256 in the evidence directory.

Only after accepted independent source review, proposed argv is:
`/tmp/ga-tmgr-m5-candidate-r1-20260910/m5probe execute-reviewed /tmp/ga-tmgr-m5-candidate-r1-20260910/execution-plan-final.json 166cb14071d275028ab119f26d5710e97cc85a5a26cad62f3eaaf978ccc4a32a`

Runroot /tmp/ga-tmgr-m5-run-r1-20260910 is still absent. 120-second run budget, 30-second child mutation work, bounded 10-second targeted cancellation/reap intervals. All synthetic files retained; no cleanup/replay. This candidate creates only synthetic objects if later authorized to execute; it touches no live supervisor/cache/metadata, no host services, credentials or packages.

Review questions:
1. Do root/runroot/dependency/ELF/argv checks bind the intended unprivileged execution without accidental fallback or host authority exposure?
2. Does the child-user/PID/private-proc topology, closed FD handling and exact synthetic fixture identity make process positives and negatives meaningful? Assess kill(pid,0) versus delivered signals and PID collision refusal.
3. Are the original eleven cache mutations plus chown/xattr controls, noatime inventory and exact metadata positives sufficient for the bounded M5 decision? Explicit ACL mutation is not covered; no ACL PASS claimed.
4. Challenge ptrace attach/detach and cancellation/Wait behavior, private PID-1 lifetime, fixture pidfd/reap evidence and failure preservation. Does any failure risk a hung or surviving synthetic process?
5. Is the dynamically linked host launcher dependency set sufficiently frozen, and are preflight time-of-check limitations honest? No claim of protection against hostile same-user replacement.
6. Are ordinary tests truly pure and real execution opt-in only? No full suite or production API/cmd integration belongs in this checkpoint.

Return SOURCE PASS for this bounded synthetic execution candidate or precise HOLD; not capability/synthetic/live PASS. The README states remaining scope limits (no arbitrary hostile helper, SCM_RIGHTS, process_vm_writev/io_uring/alternate ABI matrix). The combined P01/P05/P08/P11/P12 protocol proof, new lease handlers, receipt consumers and live activation remain unimplemented/unproved and separately gated. Accepted v1/R2/R3 design remains unchanged; do not infer success from design review.

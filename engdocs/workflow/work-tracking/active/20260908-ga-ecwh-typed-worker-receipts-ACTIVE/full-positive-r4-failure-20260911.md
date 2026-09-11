# R4 terminal failure recording — 2026-09-11

R4 was explicitly authorized once and executed once by the parent: terminal controller53824 exit1, elapsed3.437674183049239s, one apply. Attempt consumed; stop after failure, no replay/repair/retry authorized here.

Actual retained stderr: `gc: adopt broker-activated platform: EOF` followed by `confined writer failed: exit status 1: bwrap: Unknown option --preserve-fds`. The pinned bwrap rejects the production argument. The diagnostic fix worked; this is an observed R4 launch-contract incompatibility, not proof of R3's cause, a provider/authentication error, or a demonstrated protocol defect.

Separate observation failure: /proc/488415/exe disappeared during enrollment. Apply exit1 is recorded with interrupted=true and pre-teardown exit null; finalizer0, supervisor0. Preserve machine transaction_outcome=unknown, fixture_outcome=failed, writer_cleanup_witness=false, independent_writer_inventory=false and zero-owned-residue=false. No receipt/manifest/intent/FINAL observed; empty outputs do not prove rollback eligibility or no mutation. Parent later observed direct PIDs488382/488402/488415 and listener48371 absent; no extra signal. This is not independent protected-W subtree absence.

Exact fully read/rehashed evidence:
- /tmp/ga-tmgr-full-positive-r4-runtime-result-20260911.md SHA2563da91adc864cc36b09e6c9acbc0747369e1d39254ab6b6f34c43418ae8522d17.
- /tmp/ga-tmgr-full-positive-r4-independent-disposition-20260911.md SHA25629fc515d32f11cb945ac472029c91101288e85d49805c69a80270ff501287ba9: independent Astra FAILED/consumed; no new harness prerequisite, no repair or retry authority.
- /tmp/ga-tmgr-full-positive-run-r4-20260911/evidence/result.json SHA256a133fe9317c384ddf379dc7400db91d9963b3186f4f72ec4c4b29d7e35478507.
- /tmp/ga-tmgr-full-positive-run-r4-20260911/evidence/adopt.log SHA256636aedab95779f07391f97c6f7b0ce08e74f4a4597433c97c45a9e1ef55a38c0.

Next boundary is a separately reviewed launch-contract compatibility correction preserving anonymous channel FDs3/4, confinement and descriptor checks; not blind flag deletion, package upgrade or automatic retry. The review identifies exact pinned bwrap FD-inheritance implementation/documentation as the narrow next source question; none investigated here.

Existing context/ownership reused and rechecked; HEADf1ea84d76fce1d17626349353f758eb592386ee3 unchanged, no Go source delta. ga-tmgr remains in_progress/unassigned. Full original goal stays active/incomplete; ga-oz9e deferred. R3/R4 source, packages, fixtures, raw results and historical Operations date HOLD preserved. This checkpoint performs only evidence recording, no tests/build/source/runtime/lifecycle/cleanup/delivery/goal change. Actual supported readbacks follow separately.

# Approved protected-sibling source extension — candidate checkpoint

Operator authority: exact bounded extension approved 2026-09-11, preserving
assets/ and backups/ in place as read-only writer mounts. Existing ga-tmgr Core
source-bootstrap exception and ga-ecwh external-coordinator ownership verified.
No new privilege, dependency update, live adoption or worker launch is included.

The same owned worktree remains on signed base
185e6f4acb89a50197dcd4c91d3a1de40ef8bdda, tree
9ec9459ce55551c5f9cf7a701f1e3299e2e1792c. PR37 still points at that head.
Main CI34611042372 passed; Container Scan34611042345 failed on existing grpc
and thrift dependencies. The independently assessed dependency-scope decision
is preserved in /tmp/ga-tmgr-public-delivery-20260911/container-dependency-boundary.md;
those changes have not been authorized or implemented. No merge or CI rerun.

The approved extension can progress locally without that separate mutation:
an optional bounded exact protected-tree inventory, identity/content checks,
private parent-RW/child-RO ordering, actual writer mount-ID validation, nested
mount and writable-parent object-alias refusal, and existing host grant checks.
Original metadata transaction, irreversible FINAL, rollback, import and custody
contracts remain intact. Runtime claims remain conditional on the cooperating
host and a fresh reviewed canonical-shaped synthetic proof.

## Source verification

Evidence directory: /tmp/ga-tmgr-protected-sibling-source-20260911.

- red.log SHA256 6df3b0726c8751d8b3ec44fa57e92e3720ea65a1104661ce146a6b6f2527ccd9:
  compiled regression fails valid preserved-directory and RO mount-order cases.
- alias-red.log SHA256 29cb2ec28e6ec4198db4b853213c081cd840099548915033dc34d78aa15c63a5:
  compiled identity-policy regression fails before writable-parent alias guard.
- owning-package-final.log SHA256 207fd539af2f659c1a855f1c9a024521fc5f814ed2691da04b140ec3d34c9d4c:
  full internal/platforminstall package PASS 4.509s.
- lint-final.log SHA256 e92606b0bf483111dff0a120c315ea165821348f31365020e2468a0059095c47:
  scoped golangci-lint, zero issues. Initial lint failure remains preserved.
- git diff --check PASS. Existing Go1.26.7 and dependencies used; no downloads.

Independent source review requested from existing Astra reviewer. Frozen packet:
REVIEW.md SHA256 2ed3675a4ee5daddfe6d58879c624828c86957d3c755674946dba4ea2abf05c1;
eleven-file inventory candidate-sha256.txt SHA256
1d1e4c311f2a22f226c241c5aa3a6969b229cf855107acb76eda18f2afd81d03.
All eleven files read back exact before review. New untracked files are included
in the inventory. No review verdict is claimed by this checkpoint.

One minimal lint adjustment removes an always-true private contents parameter
and makes the same content check unconditional. It does not remove a check or
introduce an identity-only bypass. Four test-local names and close-error joining
were also corrected; no lint suppressions or test expectations were weakened.

## Remaining gates and preservation

Source PASS, final delivery tests, signed/CI/merge, fresh W/descendant denial and
metadata-preservation proof, and separately authorized live adoption remain owed.
Historical R7 proves only its exact old image and layout, not this extension.
The full provider-independent execution/handover objective is unchanged: useful
Claude task execution, managed signing/delivery and both handovers remain open.
No protected HPFetcher/Blog work, service, rig, session, installed binary,
credential or previous attempt was changed. This is not a completion or closeout.

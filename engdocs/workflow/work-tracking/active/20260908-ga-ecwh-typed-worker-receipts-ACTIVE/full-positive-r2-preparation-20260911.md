# ga-tmgr full-positive R2 preparation — 2026-09-11

## Outcome

One consolidated controller-only correction is frozen for independent delta review. All four R1 review findings are addressed; this is preparation/test evidence, NOT independent SOURCE PASS or runtime acceptance. No production source change, rebuild or candidate execution occurred. R1 package/results remain unchanged and unexecuted; neither R1 nor R2 runtime root was consumed.

Package root: `/tmp/ga-tmgr-full-positive-package-r2-20260911`.
Fresh prospective root: `/tmp/ga-tmgr-full-positive-run-r2-20260911` (absent).
Prospective command, only after independent review and separate execution authority:

```sh
/usr/bin/env -i HOME=/home/loucmane PATH=/usr/bin:/bin /usr/bin/python3 -B /tmp/ga-tmgr-full-positive-package-r2-20260911/controller.py --execute-once
```

## Consolidated corrections

1. Immediately after successful Popen, the direct child is registered for unconditional bounded owned-Popen terminate/kill/Wait before fallible pidfd enrollment. No scanned PID fallback; one cleanup failure does not skip other children.
2. The short-lived finalizer uses owned Wait/exit and bounded exact finalized-manifest/digest validation, not dead-child proc access. Nonzero completion refuses. Supervisor and verifier still require live identity.
3. Sampled child enrollment verifies retained parent pidfd plus frozen identity before and after binding; ambiguous samples are closed without registration/signalling. Parent read failure is not treated as an innocently vanished child.
4. Actual apply exit before teardown and final waited exit are preserved. Bounded exact R/M/R inspection separates committed_evidence_incomplete from unknown. Successful V completion plus exact pair remains a completed transaction when later fixture/cache/controller/teardown checks fail; fixture failure remains explicit. Interrupted/forced V never proves W cleanup or complete zero residue.

Unchanged: signed production image; runtime/link closure; six manifest versus seven audit namespace fields; fixed ASCII receipt digest domain; one apply/no replay; cache/input closure and production protocol deadlines; nondumpable W boundary. External process sampling remains non-exhaustive. There is no independent external W PID inventory; unchanged successful production V terminal return is its source-bound W lifecycle witness.

## Focused evidence

- `regression-final.txt`:18 pure unittest tests PASS (0.006s), including injected enrollment/termination/kill/wait failures, fast-finalizer output/exit, parent drift/loss, receipt/pair mismatches, failed outer commit, unknown disposition and successful transaction with fixture failure.
- Tests use mocked process/pidfd/identity operations and in-memory receipt readers. Every test forbids real Popen. No process fixture, image, platform command, supervisor, namespace or kernel test ran.
- RED preserves missing-helper/wiring failures against the R1-derived controller. Initial GREEN preserved a parent-liveness mock sequencing failure; corrected mock and18 final tests pass. A shell read-only-variable output-wrapper error and a rejected multi-file patch produced no runtime or production change; final exact source is frozen independently.
- `static-validation.txt`: exact file hashes, Python AST, TOML/JSON parsing and runroot absence PASS. No controller main invocation.

## Exact frozen hashes

- controller.py: `49ac7a3d1c60b70c6533c68133914d3628dc1fa7d13d578fd1b2e51257a21f9f`
- test_controller.py: `b93c477899d288252daa4ba584fe5aa518b4907d5363f5eaca1da022896a1ea3`
- package.json: `25f0a2f18286065644aa3afa05e0da069e84b4b6f248e5295f8eeb24fcce87e6`
- files.sha256: `5aaad311fede4039184051661a86a2f3874d90fa2290bc8d40131e8d5c8feb5f`
- review-package.tar: `368959891746a5cfbf9a6ab5f01016b38325aa855e3621930aa009be04376226`
- R1-to-R2.diff: `67fa557083b1743ce77545d36ead8ef13a1a11d83e5ac9ddefb21de34391d2f7`
- REVIEW.md: `514305f470e8507fbca9c8bb2e2556d54ea91b2c77199897b9659acbd92e82e6`
- regression-final.txt: `77d67db6e6c84b46eb0b26564907a51b2cf0e502f5d84f6d4693e959362c84d2`
- city.toml: `8d8ea42126b8f70b690619084fbd10713ebe885b1a6abedc024550f3f5eb2116`
- supervisor.toml: `1aa3b63db88623dd07eb4001b4310ae48a722e41fef483f25f8d04b5abcd7622`
- cities.toml: `43b3a9e3a158f2df6b9e833337cc8f03441ca81e32da0ef3b826c86df4d7f000`
- capture_support.py: `5ad20d147410a341a47bf5935c8e8b9675b623f29a633f34a9bc1cc21cb988ab`

## Preservation and next boundary

Core HEAD5a74ab60da46e55ba6425839a3819e9a185c1803/tree d7260990f64095c871f948bd3e77353492dc7f19 unchanged; source91 pins reverified; staged index has no delta. Image133658285 bytes remains64bdb174ed79719195f38d00877f44ce7182a8cc5b42247bc7ac5852ef37c8b4. `production-source-preserved.txt`, `frozen-predecessors-preserved.sha256`, `index-readback.sha256` and `source-context.txt` preserve readbacks.

Same canonical Operations context and trusted hooks; supported existing READY checkpoint only, no begin/recover/new context. One owned ga-tmgr note and existing Core/Operations plan/log/checkpoint readbacks bind this report. Historical Operations date HOLD is untouched; full goal state unchanged, ga-oz9e deferred/nonblocking. No source/Bead closure, lifecycle, service, signing, publication or live adoption.

Next is independent review of this exact frozen R2 delta, then a separate decision on execution. No review of a moving draft was requested and no inference ran in this checkpoint.

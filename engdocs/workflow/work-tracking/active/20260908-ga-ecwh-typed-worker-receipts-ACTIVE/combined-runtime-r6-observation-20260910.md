# ga-tmgr combined R6 runtime acceptance

Observed 2026-09-10T19:22:04Z and final residue check7552ce. **PASS for the
reviewed22-case synthetic matrix**, not production adoption or provider parity.

## Exact execution and independent validation

The R6 command ran once as host UID1000 without sudo/root. Launch tool0b0358
created exec session5136; completion5ff30b reported actual outer exit0.
Separate read-only `validate-result` tool4f70ad used actual exit argument0 and
returned exit0. No failed attempt was replayed, and no R5 result was reclassified.

- helper SHA256 `4aacd07c9d1a812a66047326054a57288488404a490e3556721a8828b2fb9362`
- impostor SHA256 `980c98ee39e6eaee723755886f10057ab5f6faf8eb84e6561505ad2e298bce4c`
- plan SHA256 `5fa9e19c5f6af39b783c3b07fbe535e2f420106af0f34184bc60d3ba3cb4c3b6`
- source manifest SHA256 `639d16cf473c178499ed85d431f72e78747961827dcf1bd24b764dcc58dbf6dc`
- independent Astra source review SHA256 `4c2d347f4f246ef68f77d05c67046dc54c18cca691f89ae5e8888e6566920c89`
- retained `/tmp/ga-tmgr-combined-run-r6-20260910/result.frames` SHA256
  `16bad4661b78e8219a9bb3835c8877c5252a6b25e289509448937c14a8ca63d0`

The original20 source snapshot, staged index, failed R5, unexecuted R4, initial
R3/M5 proofs, completed old-decoder tests and every predecessor remain preserved.
R6 runroot is consumed. Do not replay it to refresh an acceptance timestamp.

## Outcomes

All22 cases passed their exact required dispositions, not22 successful adoptions:

- success and concurrent: successful committed pair and complete evidence.
- version-mismatch and unsupported API: precise pre-writer refusal.
- same-build-impostor and wrong-image: actual observed ownership/image mismatch,
  exact structured evidence; no writer starts. Arbitrary I/O errors cannot PASS.
- before-loss, exit-before, exec-before, restart-before, expiry-before,
  crash-before, eof-before, fsync-before: preserved precommit abort.
- corresponding after cases: committed_evidence_incomplete; committed bytes
  preserved with no automatic rollback, replay or false adoption success.

All22 cache inventories were unchanged. Each of18 started writers satisfied
the retained13 cache mutations, access/default-descendant ACL denials, four
process negatives and fresh positive-control requirements. Four unstarted
writers retain writer_not_started without fabricated terminal evidence.

The real exec cases retained PID/start/image/listener and changed startup
instance, with old lease/instance refused: exec-before PID2942535/listener
134045883; exec-after PID2942616/listener134035255. This proves the repaired
fixture handoff for this run, not which close race won historically in R5.
Concurrent case: four old, four mixed, four new observations, torn crossing
refused, competing exact lease refused as occupied. This is the declared bounded
matrix, not exhaustive scheduling, forced PID reuse, hostile-host resistance,
kernel-atomic clock/rename, or disk EIO/crash-durability proof.

## Independent outer teardown observation

Header fixture_reaped=true; every started writer/supervisor has required
wait/pidfd evidence. Actual-host read-only /proc check310a9c found all45 recorded
PIDs absent:
2941624,2941697,2941821,2942039,2942164,2942218,2942251,2942275,2942394,
2942414,2942464,2942483,2942516,2942535,2942585,2942616,2942649,2942696,
2942728,2942742,2942760,2942793,2942807,2942825,2942858,2942876,2942908,
2942927,2942960,2944560,2944592,2945418,2945451,2945502,2945541,2945564,
2945600,2945625,2945658,2945698,2945826,2945911,2945944,2945963,2945996.
That combined command's broad text search matched its own inspection shell;
this was not a helper residue. Separate comm-and-path scoped check7552ce returned
NO_R6_SYNTHETIC_PROCESS_MATCHES, exit0. No signal/cleanup command was used.
All synthetic files, including expected partial/committed-fault states, remain.

## Durable checkpoint / next work

R6 source checkpoint has Core and Operations verify PASS, exact owned Bead note,
zero pending queues, unchanged original20/index. Its reported Operations log
returned applied without a new exact session/handoff note; preserve the honest
limitation in candidate/checkpoint-readback.txt. Correct the reference through a
unique supported append-forward evidence record in the next consolidated
checkpoint, not by replaying/rewriting history or changing workflow source.

The decisive combined synthetic feasibility matrix is now complete. Next is
the existing production Core V/W, lease and phase-aware metadata/consumer
integration with proportional final tests, independent review and standing
signed/CI/delivery gates. No production source/installed metadata/supervisor,
rig, worker, credential or protected project was changed by this test. Real
activation remains separately gated. Then finish gct-13ku/gct-10pg, useful
Claude execution, and both handovers. Full original goal remains active and
incomplete; ga-oz9e stays deferred/nonblocking.

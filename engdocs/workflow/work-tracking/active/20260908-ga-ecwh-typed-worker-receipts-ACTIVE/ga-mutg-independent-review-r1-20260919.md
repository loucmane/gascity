# ga-mutg R1 independent review

Reviewer: `/root/recovery_controller_second_review`, independent read-only
review task. Verdict received 2026-09-19: **SOURCE_PASS**, no must-fix findings.
Binding: base `728178bfc5971c8cdac5e37cf7b581b9a05d53b0` and all nine exact
hashes in `ga-mutg-source-review-r1-20260919.md` verified independently.

Reviewer findings:

- All consumers consistently use finite 60s grant/pipes, 63s lease and 65s
  enclosing/admission horizon.
- Original epoch, earlier-parent shortening, nonrenewal, overflow/exhaustion,
  3s expiry gap and 2s return reserve remain intact.
- No host/input/cache/protected-tree/confinement/publication/replay/cancellation,
  wait or pidfd-terminal checks removed; only the reviewed timing exposure grows.
- Tests cover shortened/exhausted budgets, finite admission and natural expiry;
  docs distinguish source acceptance from adoption and preserve consumed attempts.

The reviewer ran no tests or live probes. Full regression/delivery checks are
separate. This verdict is not new-image adoption PASS or authorization to replay R5.

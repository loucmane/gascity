# ga-mutg: bounded timing repair bootstrap

The operator's `yes` authorizes Codex to implement this one Core repair directly
in the existing isolated worktree. It does not waive independent review, signing,
CI, merge, or live-adoption gates. No worker was routed and no rig/service changed.

Base: `728178bfc5971c8cdac5e37cf7b581b9a05d53b0`; fetched origin/main:
`a6cceb817644d83ab6f3376dce474549e3e52848`. HEAD is an ancestor and their
trees match. Existing branch/worktree and all prior evidence are preserved.

The new-workflow preview refused before mutation because the Core workflow is
already active. Supported daily continuation resumed ga-ecwh; coordinate depend
attached ga-mutg, whose readback is in_progress with the primary's external-owner
binding. A supported note recorded this direct-bootstrap exception.

Candidate design: one shared finite timing policy: 60-second grant, 63-second
lease, 65-second enclosing context/admission horizon, 60-second pipe limits.
Retain the original transaction epoch, nonrenewal, three-second expiry gap,
two-second return reserve, and shortening by an earlier parent context. The
measured preserved closure took 14.054005566 seconds in the last consumed R5
dry-run, exceeding its reviewed 13-second limit; that attempt is not replayed.
This is a proposed source policy change, not a waiver of that limit or live PASS.

Tests first: simulate validation beyond the former grant, finite exhaustion,
earlier-parent shortening, admission at/above the new bound, and expiry wait.
Reuse existing terminal-child, publication/rollback and boundary regressions.
No security, closure, cache, host-runtime or publication checks are removed.

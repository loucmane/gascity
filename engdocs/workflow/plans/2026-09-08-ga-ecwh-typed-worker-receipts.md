---
session_id: 2026-09-08-001
work_context: ga-ecwh-typed-worker-receipts
handler_target: .
bead_ids: [ga-ecwh]
attached_bead_ids: [ga-ecwh.2]
branch_policy: codex/ga-ecwh-typed-worker-receipts
evidence_summary:
  - engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE
  - .
  - bead:ga-ecwh
  - scripts/codex-task
plan_version: v1
emergency_bypass: false
---

# Plan - Bead ga-ecwh Bind candidate profiles across controller receipt and canary consumers

## Header
- **Session ID (S)**: 2026-09-08-001
- **Work Context (W)**: ga-ecwh-typed-worker-receipts
- **Handler Target (H)**: .
- **Bead IDs**: ga-ecwh
- **Branch Policy**: codex/ga-ecwh-typed-worker-receipts
- **Evidence Summary (E)**: engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE, ., bead:ga-ecwh, scripts/codex-task
- **Plan Version**: v1
- **Emergency Bypass**: false

## Plan Table
| Step ID | Description | Evidence | Status |
|---|---|---|---|
| plan-step-scope | Confirm scope and authority for Bind candidate profiles across controller receipt and canary consumers | engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/FINDINGS.md | completed |
| plan-step-implement | Implement Bind candidate profiles across controller receipt and canary consumers through the reviewed helper surface | .; engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/IMPLEMENTATION.md | pending |
| plan-step-verify | Capture tests, review evidence, and bead readback | engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/HANDOFF.md; engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/TRACKER.md | pending |
| plan-step-emergency | _Optional_ - only if bypass required | Waiver + post-mortem plan | n/a |

## Scope
- `engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE`
- `.`
- `scripts/codex-task`
- `scripts/codex-guard`
- `tests/`
- Primary bead `ga-ecwh`

## Branch Policy
- Working branch: `codex/ga-ecwh-typed-worker-receipts`

## Amendments & Versioning
- 2026-09-08 - Bead `ga-ecwh` kickoff created through the bead-native source workflow.
- 2026-09-08 - Corrected the authored scope row from pending to completed to match the existing checked tracker and verified Core-only authority; implementation and acceptance remain pending. The supported plan-sync validates parity, rather than changing authored statuses. Refusal and correction evidence: `docs/ai/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/reports/policy-canary-checkpoint-20260908.md`.

- Layout relocation: active references now use engdocs/workflow; original plan preserved at `/home/loucmane/gascity/city/rigs/gascity/.git/gas-city-workflow/layout-relocations/f1fec32ea4f45986e5b480458c5c3e692c3386f8af5217f109d2e22ec98e2706/originals/plans/2026-09-08-ga-ecwh-typed-worker-receipts.md` (SHA-256 b52ae5f46db6a599f410b58a6291e5a8a607867067a1b70ecb1970feb967b4a0). No task status changed.

## Continuation & Handoff
- Next owner: loucmane (default)
- Context reload steps:
  1. Read `engdocs/workflow/sessions/current` and this plan.
  2. Read primary bead `ga-ecwh` through the rig-scoped bead surface.
  3. Review `engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/TRACKER.md` before changing implementation.
  4. Run `python3 scripts/codex-task plan sync` after tracker updates.
- Outstanding risks/todos: preserve bead authority and avoid allocating shadow Taskmaster work.

## Conflict & Scope Declaration
- Related plans: none declared at kickoff.
- Guard cross-check: bead-native work must preserve plan/tracker/session compliance.

## Evidence Checklist
- Bead readback and reviewed authority
- Tracker/session entries for implementation progress
- Focused tests and guard evidence

## Emergency Bypass Protocol
- No bypass authorized.

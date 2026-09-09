# Exact source excerpts for relocation design review

Operations source head d76c380b28aa42d532dd2fb9d9de0cbe09666128; tree equals reviewed merge b44b844b9f84487602232995f6ba90b4285ba5f4. Line ranges below are excerpts, not claims that the complete program was independently re-derived.

## /home/loucmane/gas-city-ops-worktrees/ga-e0t1-orchestrator-bootstrap/plugins/gas-city-workflow/scripts/_repo_structure.py

Full-file SHA-256: 1512e8b86d2b7293b1ebf987ad8efb795affb91a4913f8c647683a2d8dd8fc65. Lines 1-210 (or EOF).

```text
"""Shared repo-structure configuration for Codex workflow scripts."""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Dict
import tomllib


DEFAULT_REPO_STRUCTURE = {
    "templates_root": "templates",
    "sessions_root": "sessions",
    "plans_root": "plans",
    "plan_state_dir": ".plan_state",
    "taskmaster_root": ".taskmaster",
    "work_tracking_root": "docs/ai/work-tracking",
    "reports_root": "reports",
}


@dataclass(frozen=True)
class RepoStructure:
    repo_root: Path
    templates_root: Path
    sessions_root: Path
    plans_root: Path
    plan_state_dir: Path
    taskmaster_root: Path
    work_tracking_root: Path
    reports_root: Path

    @property
    def current_session_link(self) -> Path:
        return self.sessions_root / "current"

    @property
    def session_state_path(self) -> Path:
        return self.sessions_root / "state.json"

    @property
    def current_plan_link(self) -> Path:
        return self.plans_root / "current"

    @property
    def plan_sync_log(self) -> Path:
        return self.plan_state_dir / "sync.log"

    @property
    def taskmaster_tasks_dir(self) -> Path:
        return self.taskmaster_root / "tasks"

    @property
    def taskmaster_tasks_json(self) -> Path:
        return self.taskmaster_tasks_dir / "tasks.json"

    @property
    def work_tracking_active_root(self) -> Path:
        return self.work_tracking_root / "active"

    @property
    def work_tracking_archive_root(self) -> Path:
        return self.work_tracking_root / "archive"

    @property
    def template_metadata_policy_path(self) -> Path:
        return self.templates_root / "metadata" / "template-metadata-policy.json"

    @property
    def template_monitoring_policy_path(self) -> Path:
        return self.templates_root / "metadata" / "template-monitoring-policy.json"

    @property
    def template_performance_policy_path(self) -> Path:
        return self.templates_root / "metadata" / "template-performance-policy.json"

    @property
    def template_cost_policy_path(self) -> Path:
        return self.templates_root / "metadata" / "template-cost-policy.json"

    @property
    def emergency_response_policy_path(self) -> Path:
        return self.templates_root / "metadata" / "emergency-response-policy.json"

    @property
    def continuation_guard_dir(self) -> Path:
        return self.reports_root / "session-continuation"

    @property
    def drift_report_dir(self) -> Path:
        return self.reports_root / "template-drift"

    @property
    def metrics_report_dir(self) -> Path:
        return self.reports_root / "template-metrics"

    @property
    def monitoring_report_dir(self) -> Path:
        return self.reports_root / "template-monitoring"

    @property
    def phase0_validation_report_dir(self) -> Path:
        return self.reports_root / "phase0-scanner-validation"

    @property
    def performance_report_dir(self) -> Path:
        return self.reports_root / "template-performance"

    @property
    def cost_report_dir(self) -> Path:
        return self.reports_root / "cost-tracking"

    @property
    def migration_health_report_dir(self) -> Path:
        return self.reports_root / "migration-health"

    @property
    def emergency_response_report_dir(self) -> Path:
        return self.reports_root / "emergency-response"


def _load_repo_structure_section(config_path: Path) -> Dict[str, str]:
    if config_path.parent.is_symlink() or config_path.is_symlink():
        raise ValueError("repository layout configuration must not contain symlinks")
    if config_path.parent.exists() and not config_path.parent.is_dir():
        raise ValueError("repository configuration parent must be a directory")
    if not config_path.exists():
        return {}
    if not config_path.is_file():
        raise ValueError("repository layout configuration must be a regular file")
    data = tomllib.loads(config_path.read_text(encoding="utf-8"))
    section = data.get("repo_structure", {})
    if not isinstance(section, dict):
        raise ValueError("repo_structure must be a table")
    if set(section) - set(DEFAULT_REPO_STRUCTURE):
        raise ValueError("repo_structure contains unknown roots")
    if any(not isinstance(value, str) for value in section.values()):
        raise ValueError("repo_structure roots must be strings")
    return dict(section)


def _resolve_path(repo_root: Path, raw_value: str) -> Path:
    if (not raw_value or Path(raw_value).is_absolute() or "\\" in raw_value
            or any(char in raw_value for char in ("\x00", "\n", "\r"))
            or any(part in {"", ".", "..", ".git"} for part in raw_value.split("/"))):
        raise ValueError("repo_structure roots must be normalized worktree-relative paths")
    path = repo_root
    for part in raw_value.split("/"):
        path = path / part
        if path.is_symlink():
            raise ValueError("repo_structure roots must not contain symlinks")
        if path.exists() and not path.is_dir():
            raise ValueError("repo_structure roots must be directories")
    return path


def load_repo_structure(repo_root: Path) -> RepoStructure:
    repo_root = repo_root.resolve()
    config_path = repo_root / ".codex" / "config.toml"
    overrides = _load_repo_structure_section(config_path)
    values = {**DEFAULT_REPO_STRUCTURE, **overrides}
    return RepoStructure(
        repo_root=repo_root,
        templates_root=_resolve_path(repo_root, values["templates_root"]),
        sessions_root=_resolve_path(repo_root, values["sessions_root"]),
        plans_root=_resolve_path(repo_root, values["plans_root"]),
        plan_state_dir=_resolve_path(repo_root, values["plan_state_dir"]),
        taskmaster_root=_resolve_path(repo_root, values["taskmaster_root"]),
        work_tracking_root=_resolve_path(repo_root, values["work_tracking_root"]),
        reports_root=_resolve_path(repo_root, values["reports_root"]),
    )
```

## /home/loucmane/gas-city-ops-worktrees/ga-e0t1-orchestrator-bootstrap/plugins/gas-city-workflow/scripts/workflow_portable.py

Full-file SHA-256: 89fca561059e59c70e7eb4926bb16a35c56bc226705a129e93ac2ec368f307b8. Lines 1-110 (or EOF).

```text
"""Strict source-work checks for consumers without an installed Aegis runtime."""

from __future__ import annotations

import os
import sys
from pathlib import Path

from workflow_common import CommandRunner, WorkflowError, plan_bead_ids, workflow_runtime_root
from workflow_ownership import check_active_ownership
from _repo_structure import load_repo_structure


def uses_portable_scaffold(root: Path) -> bool:
    """Never replace a present (even broken) installed or source adapter."""
    markers = (
        ".aegis/foundation-manifest.json",
        ".aegis/state/current-work.json",
        ".aegis/state/pending-tracking.json",
        ".claude/scripts/readiness.sh",
        "scripts/codex-task",
    )
    return not any(os.path.lexists(root / name) for name in markers)


def load_shared_runtime() -> None:
    """Use the selected Operations source, not the consumer or a stale install."""
    runtime = workflow_runtime_root().resolve()
    if str(runtime) not in sys.path:
        sys.path.insert(0, str(runtime))
    import aegis_foundation

    if not Path(aegis_foundation.__file__).resolve().is_relative_to(runtime):
        raise WorkflowError("portable checks loaded a different Aegis runtime")


def run_portable_readiness(runner: CommandRunner, root: Path) -> str:
    """Reuse the strict Beads scaffold checks, with live external ownership."""
    load_shared_runtime()
    from aegis_foundation.gate.models import BLOCKED
    from aegis_foundation.gate.session_authority import assert_no_pending_continuation
    from aegis_foundation.gate.workflow import build_bead_source_checks

    if not uses_portable_scaffold(root):
        raise WorkflowError("portable readiness cannot replace a present Aegis adapter")
    spec = check_active_ownership(runner, root)
    if spec.workflow_profile != "beads-with-aegis-evidence":
        raise WorkflowError("portable readiness requires the modern Beads evidence profile")
    if plan_bead_ids(root) != [spec.bead_id]:
        raise WorkflowError("portable plan must name exactly the journal primary Bead")
    layout = load_repo_structure(root)
    for link in (layout.current_session_link, layout.current_plan_link):
        if not link.is_symlink() or not link.resolve().is_relative_to(root.resolve()):
            raise WorkflowError(f"portable {link.relative_to(root)} must be a target-local symlink")
    try:
        assert_no_pending_continuation(root)
        _, checks = build_bead_source_checks(root, spec.branch, spec.bead_id)
    except (OSError, ValueError, RuntimeError) as exc:
        raise WorkflowError(f"portable scaffold inspection failed: {exc}") from exc
    failures = [check.message for check in checks if check.status == BLOCKED]
    if failures:
        raise WorkflowError("portable readiness blocked: " + "; ".join(failures))
    check_active_ownership(runner, root)
    return "STATE: READY\nPROFILE: portable-beads-source\n" + "\n".join(
        f"[ready] {check.message}" for check in checks
    ) + "\n"
```

## /home/loucmane/gas-city-ops-worktrees/ga-e0t1-orchestrator-bootstrap/aegis_foundation/gate/session_transition.py

Full-file SHA-256: c22fc05623bf9e5285abab74c1ac08a5512aa59afc4b9e2bcc601860d1d5fc8c. Lines 1-230 (or EOF).

```text
"""Bounded daily-session transaction; before/after images survive failure."""
from __future__ import annotations

import base64
import contextlib
import fcntl
import functools
import json
import os
import stat
import tempfile
import uuid
from pathlib import Path

from .session_authority import (
    SessionAuthorityError, assert_no_pending_continuation, contained_path,
)

JOURNAL = ".aegis/state/session-continuation.json"
SCHEMA = "aegis.session-continuation.v1"


@contextlib.contextmanager
def session_lock(root: Path, *, shared: bool = False):
    """Lock the existing directory inode: no pre-refusal filesystem writes."""
    fd = os.open(root, os.O_RDONLY | os.O_DIRECTORY)
    try:
        try:
            fcntl.flock(fd, (fcntl.LOCK_SH if shared else fcntl.LOCK_EX) | fcntl.LOCK_NB)
        except BlockingIOError as exc:
            raise SessionAuthorityError("session authority is in use; retry after the writer finishes") from exc
        yield
    finally:
        os.close(fd)


def serialized_session_writer(function):
    @functools.wraps(function)
    def wrapped(target_dir, *args, **kwargs):
        with session_lock(Path(target_dir).expanduser().resolve()):
            return function(target_dir, *args, **kwargs)
    return wrapped


def image(path: Path) -> dict:
    if path.is_symlink():
        info = path.lstat()
        return {"kind": "link", "target": os.readlink(path), "uid": info.st_uid, "gid": info.st_gid}
    if not path.exists():
        return {"kind": "absent"}
    info = path.stat()
    if not stat.S_ISREG(info.st_mode):
        raise SessionAuthorityError(f"session transaction refuses special file: {path}")
    return {"kind": "file", "data": base64.b64encode(path.read_bytes()).decode("ascii"),
            "mode": stat.S_IMODE(info.st_mode), "uid": info.st_uid, "gid": info.st_gid}


def _install(path: Path, value: dict) -> None:
    """Atomic leaf replacement, preserving exact bytes and mode."""
    path.parent.mkdir(parents=True, exist_ok=True)
    if value["kind"] == "absent":
        if path.exists() or path.is_symlink():
            path.unlink()
        return
    fd, name = tempfile.mkstemp(prefix=".session-txn-", dir=path.parent)
    temporary = Path(name)
    try:
        if value["kind"] == "file":
            with os.fdopen(fd, "wb") as stream:
                stream.write(base64.b64decode(value["data"], validate=True))
                stream.flush()
                os.fchmod(stream.fileno(), value["mode"])
                if (os.fstat(stream.fileno()).st_uid, os.fstat(stream.fileno()).st_gid) != (value["uid"], value["gid"]):
                    os.fchown(stream.fileno(), value["uid"], value["gid"])
                os.fsync(stream.fileno())
        elif value["kind"] == "link":
            os.close(fd)
            temporary.unlink()
            temporary.symlink_to(value["target"])
            if (temporary.lstat().st_uid, temporary.lstat().st_gid) != (value["uid"], value["gid"]):
                os.lchown(temporary, value["uid"], value["gid"])
        else:
            os.close(fd)
            raise SessionAuthorityError("invalid session transaction image")
        os.replace(temporary, path)
        parent_fd = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(parent_fd)
        finally:
            os.close(parent_fd)
    finally:
        if temporary.exists() or temporary.is_symlink():
            temporary.unlink()


def _json_image(payload: dict) -> dict:
    return {"kind": "file", "data": base64.b64encode(
        (json.dumps(payload, indent=2) + "\n").encode()).decode(),
        "mode": 0o600, "uid": os.getuid(), "gid": os.getgid()}


class SessionTransition:
    """Only caller-declared session surfaces may be written, each with a WAL image."""

    def __init__(self, root: Path, bead: str, paths: list[Path], *, lock_held: bool = False):
        self.root = root.resolve()
        self.lock_held = lock_held
        self.bead = bead
        self.allowed = {p.relative_to(self.root).as_posix() for p in paths}
        self.record = {"schema": SCHEMA, "id": uuid.uuid4().hex, "bead": bead,
                       "status": "pending", "steps": []}
        self.before = {}
        self.created_directories = set()
        for relative in sorted(self.allowed):
            path = contained_path(self.root, relative, "session target", leaf_link=True)
            self.before[relative] = image(path)
            parent = path.parent
            while parent != self.root and not parent.exists():
                self.created_directories.add(parent.relative_to(self.root).as_posix())
                parent = parent.parent
        self.record["created_directories"] = sorted(self.created_directories)

    def __enter__(self):
        self.lock = contextlib.nullcontext() if self.lock_held else session_lock(self.root)
        self.lock.__enter__()
        try:
            assert_no_pending_continuation(self.root)
            for relative, before in self.before.items():
                if image(self.root / relative) != before:
                    raise SessionAuthorityError("session target changed before transaction")
            journal = contained_path(self.root, JOURNAL, "session journal")
            if journal.exists():
                prior = json.loads(journal.read_text())
                self._archive(prior)
            self._save()
            return self
        except BaseException:
            self.lock.__exit__(None, None, None)
            raise

    def _save(self):
        path = contained_path(self.root, JOURNAL, "session journal")
        _install(path, _json_image(self.record))

    def _archive(self, record):
        identity = record.get("id", "")
        if not isinstance(identity, str) or len(identity) != 32 or any(c not in "0123456789abcdef" for c in identity):
            raise SessionAuthorityError("invalid previous session transaction identity")
        relative = f".aegis/state/session-continuations/{identity}.json"
        path = contained_path(self.root, relative, "session archive")
        expected = _json_image(record)
        if path.exists():
            if image(path) != expected:
                raise SessionAuthorityError("session transaction archive disagrees")
        else:
            _install(path, expected)

    def _change(self, path: Path, after: dict):
        relative = path.relative_to(self.root).as_posix()
        contained_path(self.root, relative, "session target", leaf_link=True)
        if relative not in self.allowed or any(s["path"] == relative for s in self.record["steps"]):
            raise SessionAuthorityError("session transaction target is outside its single-write contract")
        before = self.before[relative]
        if image(path) != before:
            raise SessionAuthorityError("session target changed during transaction")
        self.record["steps"].append({"path": relative, "before": before, "after": after})
        self._save()  # Write-ahead: even a crash after replace has both exact images.
        _install(path, after)
        if image(path) != after:
            raise SessionAuthorityError("session transaction readback failed")

    def write(self, path: Path, data: bytes):
        before = self.before[path.relative_to(self.root).as_posix()]
        if before["kind"] not in {"file", "absent"}:
            raise SessionAuthorityError("session file cannot replace a symlink")
        self._change(path, {"kind": "file", "data": base64.b64encode(data).decode(),
                           "mode": before.get("mode", 0o644),
                           "uid": before.get("uid", os.getuid()), "gid": before.get("gid", os.getgid())})

    def link(self, path: Path, target: Path):
        self._change(path, {"kind": "link", "target": os.path.relpath(target, path.parent),
                           "uid": os.getuid(), "gid": os.getgid()})

    def rollback(self):
        # Precheck ALL images before restoring any: ambiguous concurrent writes survive.
        for step in self.record["steps"]:
            path = contained_path(self.root, step["path"], "rollback target", leaf_link=True)
            current = image(path)
            if current not in (step["before"], step["after"]):
                raise SessionAuthorityError("session rollback refused: ambiguous target mutation")
        for step in reversed(self.record["steps"]):
            _install(self.root / step["path"], step["before"])
            if image(self.root / step["path"]) != step["before"]:
                raise SessionAuthorityError("session rollback readback failed")
        for relative in sorted(self.created_directories, key=lambda p: p.count("/"), reverse=True):
            path = contained_path(self.root, relative, "rollback directory")
            if path.exists():
                path.rmdir()  # Only an empty directory proven absent before this attempt.
        self.record["status"] = "rolled_back"
        self._save()

    def __exit__(self, exc_type, exc, traceback):
        try:
            if exc_type is not None:
                try:
                    self.rollback()
                except Exception as rollback_error:
                    raise SessionAuthorityError(
                        f"session continuation failed and rollback is unresolved: {rollback_error}"
                    ) from exc
            else:
                self.record["status"] = "complete"
                try:
                    self._save()
                except Exception as commit_error:
                    try:
                        self.rollback()
                    except Exception as rollback_error:
                        raise SessionAuthorityError(
                            f"session commit failed and rollback is unresolved: {rollback_error}"
                        ) from commit_error
                    raise
        finally:
            self.lock.__exit__(None, None, None)
        return False
```

## /home/loucmane/gas-city-ops-worktrees/ga-e0t1-orchestrator-bootstrap/scripts/codex-task

Full-file SHA-256: 77f1509490bf151d1947e56b5b24b8500a5bfafb08484d24d970e1975bc0dbec. Lines 2390-2471 (or EOF).

```text
        has_unchecked = _tracker_has_checkbox(tracker_text, step_id, False)
        if not has_checked and not has_unchecked:
            issues.append(f"Tracker missing checkbox for {step_id}")
            continue

        if status in PLAN_STATUS_COMPLETE and not has_checked:
            issues.append(f"Tracker checkbox for {step_id} should be checked (status {status})")
        elif status in PLAN_STATUS_INCOMPLETE and not has_unchecked:
            issues.append(f"Tracker checkbox for {step_id} should remain unchecked (status {status})")
        elif status in PLAN_STATUS_PASSIVE and not has_unchecked:
            issues.append(f"Tracker checkbox for {step_id} should remain unchecked (status {status})")
    if issues:
        raise TaskError("; ".join(issues))


def _compute_sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def _load_plan_sync_entries() -> List[Dict[str, str]]:
    if not PLAN_SYNC_LOG.exists():
        return []
    try:
        data = json.loads(PLAN_SYNC_LOG.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise TaskError(f"Plan sync log is not valid JSON: {exc}")
    if not isinstance(data, list):
        raise TaskError("Plan sync log root must be a list")
    return data


def _write_plan_sync_entries(entries: List[Dict[str, str]]) -> None:
    PLAN_STATE_DIR.mkdir(parents=True, exist_ok=True)
    PLAN_SYNC_LOG.write_text(json.dumps(entries, indent=2) + "\n", encoding="utf-8")


def handle_plan_sync(args: argparse.Namespace) -> None:
    _configure_workflow_target(getattr(args, "target_dir", None))
    if not args.plan and not args.tracker and _is_between_sessions_state():
        print("No active plan; repository is between sessions. Plan sync skipped.")
        return

    plan_path = _resolve_plan_path(args.plan)
    tracker_path = _resolve_tracker_path(args.tracker, getattr(args, "folder", None))

    plan_text = plan_path.read_text(encoding="utf-8")
    steps = _parse_plan_table(plan_text)
    _ensure_plan_steps_present(steps)

    tracker_text = _load_tracker_text(tracker_path)
    _validate_tracker_alignment(steps, tracker_text)

    plan_hash = _compute_sha256(plan_path)
    tracker_hash = _compute_sha256(tracker_path)

    rel_plan = plan_path.relative_to(REPO_ROOT).as_posix()
    entries = _load_plan_sync_entries()
    entry = {
        "plan": rel_plan,
        "plan_hash": plan_hash,
        "tracker_hash": tracker_hash,
        "synced_at": datetime.now().astimezone().isoformat(),
    }

    if args.dry_run:
        print(json.dumps(entry, indent=2))
        return

    entries.append(entry)
    transaction = getattr(args, "_session_transaction", None)
    if transaction is None:
        _write_plan_sync_entries(entries)
    else:
        transaction.write(PLAN_SYNC_LOG, (json.dumps(entries, indent=2) + "\n").encode())
    print(f"Plan sync recorded for {rel_plan}")
def _parse_front_matter(path: Path) -> Dict[str, str]:
    text = path.read_text(encoding="utf-8")
    if not text.startswith("---"):
        return {}
    parts = text.split("---", 2)
    if len(parts) < 3:
        return {}
```

## /home/loucmane/gas-city-ops-worktrees/ga-e0t1-orchestrator-bootstrap/plugins/gas-city-workflow/scripts/workflow.py

Full-file SHA-256: b3a88986956f3f4dc8e3f12976e69bc5a34ec2e3da4c52496cc800a25cf2baed. Lines 46-90 (or EOF).

```text
        and not (Path(context["project"]["root"]) / ".aegis" / "foundation-manifest.json").is_file()
    )


def _active_folder_name(root: Path, bead_id: str) -> str:
    active_root = load_repo_structure(root).work_tracking_active_root
    matches = sorted(
        path.name
        for path in active_root.glob("*-ACTIVE")
        if path.is_dir() and not path.is_symlink() and f"-{bead_id}-" in path.name
    )
    if len(matches) != 1:
        raise WorkflowError(f"expected exactly one ACTIVE folder for {bead_id}; found {matches}")
    return matches[0]


def _sync_plan(
    root: Path,
    context: dict[str, Any],
    runner: CommandRunner,
) -> bool:
    source_task = root / "scripts" / "codex-task"
    if source_task.is_file():
        runner.run([sys.executable, str(source_task), "plan", "sync"], cwd=root)
        return True
    if _is_lightweight_legacy(context) or uses_portable_scaffold(root):
        runtime = workflow_runtime_root()
        bead_id = active_bead_id(root)
        runner.run(
            [
                sys.executable,
                str(runtime / "scripts" / "codex-task"),
                "plan",
                "sync",
                "--target-dir",
                str(root),
                "--folder",
                _active_folder_name(root, bead_id),
            ],
            cwd=runtime,
        )
        return True
    return False


```

## /home/loucmane/gas-city-ops-worktrees/ga-e0t1-orchestrator-bootstrap/plugins/gas-city-workflow/references/workflow-contract.md

Full-file SHA-256: 1c4aba61dc9a27a756bf66833911fe37ff7c374eb10fe792e763b4b591b77ccc. Lines 190-225 (or EOF).

```text

## Workspace placement

### Repository-local evidence layout

The existing `.codex/config.toml` `[repo_structure]` table is the single layout
contract for portable Beads source workflows. Configure it in reviewed repository
source **before** kickoff when public documentation has a separate publication policy:

```toml
[repo_structure]
sessions_root = "engdocs/workflow/sessions"
plans_root = "engdocs/workflow/plans"
plan_state_dir = "engdocs/workflow/plan-state"
work_tracking_root = "engdocs/workflow/work-tracking"
```

Kickoff, project context, active-work/ownership lookup, dependency attachment,
portable readiness, evidence logging, plan sync and archive selection use the same
resolver. Default paths remain unchanged when no overrides exist. The script,
packaged Aegis asset, self-contained gate and standalone workflow-plugin mirrors are byte-parity tested;
there is no second registry layout or provider-specific override.

All configured roots must be strings naming normalized worktree-relative directories.
Absolute paths, empty/dot/parent components, `.git` components, symlink indirection,
non-directory roots, malformed tables and unknown layout keys refuse. Changing a
layout grants no write permission, Bead ownership, worker capability or lifecycle
authority, and cannot substitute a legacy pointer for the configured authority.

This is not an implicit migration. Do not change the layout under an active scaffold
and then recreate or discard evidence to regain READY. Existing scaffold relocation
requires a separately reviewed exact-inventory transaction, preserved originals and
pointer/journal history, rollback and idempotence proof. Until that migration passes,
retain the existing layout and mark the affected workflow HOLD. Do not add public-doc
exemptions, weaken tests, or generate Taskmaster records as a workaround.

```

## Workflow lock — full module plus full-file digest

```text
"""Serialize this CLI's mutations of one repository's workflow journals."""

from contextlib import contextmanager
import fcntl
import os
from pathlib import Path

from project_context import DEFAULT_REGISTRY, build_context
from workflow_common import CommandRunner, WorkflowError, git_common_dir


@contextmanager
def workflow_lock(runner: CommandRunner, root: Path, registry: Path = DEFAULT_REGISTRY):
    context = build_context(root, registry)
    common = git_common_dir(runner, Path(context["workspace"]["canonical_root"]))
    directory = common / "gas-city-workflow"
    directory.mkdir(exist_ok=True)
    fd = os.open(
        directory / "source-transition.lock", os.O_RDWR | os.O_CREAT | os.O_NOFOLLOW, 0o600
    )
    try:
        try:
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError as exc:
            raise WorkflowError("another source workflow transition is active") from exc
        yield
    finally:
        os.close(fd)
eac22f369de1fa6058390365289c5806ce7a44db8dd158d42d556fd8ec88059a  plugins/gas-city-workflow/scripts/workflow_lock.py
```

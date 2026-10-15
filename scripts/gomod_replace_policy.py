#!/usr/bin/env python3
"""The operator-approved bd fork is the only allowed module replacement."""
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import subprocess
import sys

MODULE = "github.com/steveyegge/beads"
PIN_FILE = Path(__file__).with_name("beads-fork-pin.json")


def replacements(source):
    """Parse every replace; malformed or unterminated directives fail closed."""
    found, block = [], False
    for raw in source.splitlines():
        line = raw.split("//", 1)[0].strip()
        if not line:
            continue
        if block and line == ")":
            block = False
            continue
        if not block:
            if not re.match(r"^replace(?:\s|\(|$)", line):
                continue
            line = line[len("replace"):].strip()
            if line == "(":
                block = True
                continue
        tokens = shlex.split(line)
        if tokens.count("=>") != 1 or "(" in line or ")" in line:
            raise ValueError("malformed replace directive: " + raw)
        arrow = tokens.index("=>")
        if arrow not in (1, 2) or len(tokens) - arrow - 1 not in (1, 2):
            raise ValueError("malformed replace directive: " + raw)
        found.append(tokens)
    if block:
        raise ValueError("unterminated replace block")
    return found


def load_pin():
    pin = json.loads(PIN_FILE.read_text())
    if (pin.get("module") != MODULE or
            not re.fullmatch(r"[0-9a-f]{40}", pin.get("commit", "")) or
            not re.fullmatch(r"[0-9a-f]{64}", pin.get("sha256", "")) or
            pin.get("path") != "../beads-vfug-" + pin["commit"]):
        raise ValueError("invalid reviewed beads-fork-pin.json")
    return pin


def check_replacements(directives, pin):
    expected = [MODULE, "=>", pin["path"]]
    if len(directives) > 1 or any(row != expected for row in directives):
        raise ValueError("only the exact human-approved bd fork replace is allowed; use released require entries for every other dependency")


def checkout_hash(root):
    """Hash sorted [path, executable, SHA256(bytes)] records, excluding only .git.

    Include ignored and untracked files. Refuse symlinks and special files so
    content outside the checkout cannot enter a verified dependency build.
    """
    records = []
    for parent, dirs, files in os.walk(root, followlinks=False):
        if Path(parent) == root:
            dirs[:] = [name for name in dirs if name != ".git"]
            files = [name for name in files if name != ".git"]
        for name in dirs + files:
            if (Path(parent) / name).is_symlink():
                raise ValueError("symlink in pinned checkout: " + str(Path(parent) / name))
        for name in files:
            path = Path(parent) / name
            if not path.is_file():
                raise ValueError("special file in pinned checkout: " + str(path))
            records.append([path.relative_to(root).as_posix(),
                            bool(path.stat().st_mode & 0o111),
                            hashlib.sha256(path.read_bytes()).hexdigest()])
    data = json.dumps(sorted(records), ensure_ascii=True, separators=(",", ":")).encode()
    return hashlib.sha256(data).hexdigest()


def check_identity(commit, digest, pin):
    if commit != pin["commit"]:
        raise ValueError("bd fork commit mismatch: " + commit)
    if digest != pin["sha256"]:
        raise ValueError("bd fork content hash mismatch: " + digest)


def verify_checkout(root, pin):
    if root.is_symlink() or not root.is_dir():
        raise ValueError("missing or symlinked pinned bd checkout: " + str(root))
    commit = subprocess.check_output(
        ["git", "--no-optional-locks", "-C", str(root), "rev-parse", "HEAD"],
        text=True).strip()
    check_identity(commit, checkout_hash(root), pin)
    # The content hash catches drift hidden by assume-unchanged/skip-worktree.
    # Also refuse staged/index changes even when worktree bytes still match.
    dirty = subprocess.check_output(
        ["git", "--no-optional-locks", "-C", str(root), "status", "--porcelain", "--untracked-files=all"], text=True)
    if dirty:
        raise ValueError("pinned bd checkout has index/worktree changes")


def check_manifest(gomod):
    pin = load_pin()
    directives = replacements(gomod.read_text())
    check_replacements(directives, pin)
    if directives:
        verify_checkout(gomod.parent / pin["path"], pin)


def main():
    try:
        check_manifest(Path(sys.argv[1] if len(sys.argv) > 1 else "go.mod").absolute())
    except (OSError, ValueError, subprocess.CalledProcessError) as exc:
        print("check-gomod-replace: BLOCKED: " + str(exc), file=sys.stderr)
        return 1
    print("check-gomod-replace: OK (only the verified operator-approved bd fork, or no replace)")
    return 0


if __name__ == "__main__":
    sys.exit(main())

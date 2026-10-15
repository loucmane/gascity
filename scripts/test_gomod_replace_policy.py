#!/usr/bin/env python3
"""Policy tests need no fixture files; checkout tests use /work/ga-vfug only."""
import copy
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

import gomod_replace_policy as policy


class PolicyTests(unittest.TestCase):
    def setUp(self):
        self.pin = policy.load_pin()
        self.entry = f'{policy.MODULE} => {self.pin["path"]}'

    def test_exact_pin_and_empty_manifest(self):
        for source in ("module example.org/project\n", "replace " + self.entry,
                       'replace (\n\t' + self.entry + ' // approved\n)'):
            policy.check_replacements(policy.replacements(source), self.pin)

    def test_every_other_replace_refuses(self):
        for entry in ("example.org/other => example.org/other v1.2.3",
                      f'{policy.MODULE} => {policy.MODULE} v1.3.1',
                      f'{policy.MODULE} v1.3.1 => {self.pin["path"]}',
                      f'{policy.MODULE} => ../beads-vfug-' + "a" * 40,
                      f'example.org/other => {self.pin["path"]}',
                      self.entry + " v1.3.1",
                      self.entry + "\nreplace " + self.entry):
            with self.subTest(entry=entry), self.assertRaises(ValueError):
                policy.check_replacements(policy.replacements("replace " + entry), self.pin)

    def test_malformed_never_disappears(self):
        for source in ("replace (", "replace missing-arrow", "replace (x => y)",
                       "replace x => y z extra", "replace x => y => z"):
            with self.subTest(source=source), self.assertRaises(ValueError):
                policy.replacements(source)

    def test_commit_and_content_must_both_match(self):
        policy.check_identity(self.pin["commit"], self.pin["sha256"], self.pin)
        for commit, digest in (("f" * 40, self.pin["sha256"]),
                               (self.pin["commit"], "f" * 64)):
            with self.subTest(commit=commit, digest=digest), self.assertRaises(ValueError):
                policy.check_identity(commit, digest, self.pin)


class CheckoutTests(unittest.TestCase):
    def setUp(self):
        root = Path("/work/ga-vfug")
        root.mkdir(parents=True, exist_ok=True)
        self.temp = tempfile.TemporaryDirectory(prefix="pin.", dir=root)
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.git("init", "-q")
        self.git("config", "user.name", "pin-test")
        self.git("config", "user.email", "pin@test.local")
        self.git("config", "core.hooksPath", "/dev/null")
        (self.root / "go.mod").write_text("module example.org/pin\ngo 1.26\n")
        self.git("add", "go.mod")
        self.git("-c", "commit.gpgsign=false", "commit", "-qm", "fixture")
        self.pin = copy.deepcopy(policy.load_pin())
        self.pin["commit"] = self.git("rev-parse", "HEAD").strip()
        self.pin["sha256"] = policy.checkout_hash(self.root)

    def git(self, *args):
        return subprocess.check_output(["git", "-C", str(self.root), *args], text=True,
                                       env={**os.environ, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null"})

    def test_clean_then_hidden_worktree_drift(self):
        policy.verify_checkout(self.root, self.pin)
        self.git("update-index", "--assume-unchanged", "go.mod")
        (self.root / "go.mod").write_text("module changed\n")
        with self.assertRaisesRegex(ValueError, "content hash mismatch"):
            policy.verify_checkout(self.root, self.pin)

    def test_new_commit_with_identical_files_refuses(self):
        self.git("-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "another")
        with self.assertRaisesRegex(ValueError, "commit mismatch"):
            policy.verify_checkout(self.root, self.pin)

    def test_ignored_extra_source_and_symlink_refuse(self):
        (self.root / ".git/info/exclude").write_text("injected.go\n")
        (self.root / "injected.go").write_text("package injected\n")
        with self.assertRaisesRegex(ValueError, "content hash mismatch"):
            policy.verify_checkout(self.root, self.pin)
        (self.root / "injected.go").unlink()
        (self.root / "outside").symlink_to("/etc/passwd")
        with self.assertRaisesRegex(ValueError, "symlink"):
            policy.verify_checkout(self.root, self.pin)


if __name__ == "__main__":
    unittest.main()

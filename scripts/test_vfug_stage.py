#!/usr/bin/env python3
"""Dedicated-account policy, environment and staged-input integrity tests."""
import hashlib
import importlib.util
import json
import os
import re
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

import vfug_discovery as discovery

HERE = Path(__file__).resolve().parent



class AncestorDiscoveryCases:
    """Run the same on-disk discovery cases through both entry points."""

    def test_user_state_only_beads_passes(self):
        marker = self.discovery_parent / '.beads'
        marker.mkdir()
        (marker / 'machine-id').write_text('fixture')
        (marker / 'eventsData').mkdir()
        (marker / 'registry.json').write_text('{}')
        self.assert_discovery(None)

    def test_beads_project_files_refuse_exact_path(self):
        marker = self.discovery_parent / '.beads'
        marker.mkdir()
        for name in ('metadata.json', 'config.yaml', 'dolt', 'embeddeddolt', 'issues.db'):
            path = marker / name
            with self.subTest(name=name):
                if name in ('dolt', 'embeddeddolt'):
                    path.mkdir()
                else:
                    path.write_text('fixture')
                try:
                    self.assert_discovery(path)
                finally:
                    path.rmdir() if path.is_dir() else path.unlink()

    def test_beads_redirect_refuses_exact_path(self):
        # bd follows .beads/redirect before checking project files (Review A8).
        marker = self.discovery_parent / '.beads'
        marker.mkdir()
        (marker / 'machine-id').write_text('fixture')
        redirect = marker / 'redirect'
        real_target = self.discovery_parent / 'real-target'
        real_target.mkdir()
        for kind in ('file', 'dangling-symlink', 'symlink-to-real', 'directory'):
            with self.subTest(kind=kind):
                if kind == 'file':
                    redirect.write_text('/elsewhere/.beads\n')
                elif kind == 'dangling-symlink':
                    redirect.symlink_to(self.discovery_parent / 'absent-target')
                elif kind == 'symlink-to-real':
                    redirect.symlink_to(real_target, target_is_directory=True)
                else:
                    redirect.mkdir()
                try:
                    self.assert_discovery(redirect)
                finally:
                    redirect.rmdir() if kind == 'directory' else redirect.unlink()

    def test_ancestor_symlinks_and_git_directory_refuse(self):
        for name in ('.beads', '.git'):
            path = self.discovery_parent / name
            with self.subTest(name=name):
                path.symlink_to(self.discovery_parent / 'absent')
                try:
                    self.assert_discovery(path)
                finally:
                    path.unlink()
        git = self.discovery_parent / '.git'
        git.mkdir()
        self.assert_discovery(git)

    def test_uninspectable_beads_refuses(self):
        marker = self.discovery_parent / '.beads'
        marker.mkdir(mode=0)
        try:
            self.assert_discovery(marker)
        finally:
            marker.chmod(0o700)


class DiscoveryTests(AncestorDiscoveryCases, unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='vfug-discovery.', dir=os.environ.get('TMPDIR', '/work/ga-vfug'))
        self.addCleanup(self.temp.cleanup)
        self.discovery_parent = Path(self.temp.name)

    def assert_discovery(self, refused_path):
        # Control only ancestors outside the fixture. The host may legitimately
        # carry a .git marker (including one exposed by a sandbox).
        actual = discovery.discovery_marker
        def marker(path):
            return actual(path) if path.is_relative_to(self.discovery_parent) else None
        with patch.object(discovery, 'discovery_marker', side_effect=marker):
            root = self.discovery_parent / 'nested' / 'run'
            if refused_path is None:
                discovery.refuse_ancestor_discovery(root)
            else:
                with self.assertRaisesRegex(ValueError, 'ancestor discovery marker refused: ' + re.escape(str(refused_path)) + '$'):
                    discovery.refuse_ancestor_discovery(root)

    def test_legacy_non_project_entries_pass(self):
        marker = self.discovery_parent / '.beads'
        marker.mkdir()
        for name in ('vc.db', 'issues.backup.db', 'dolt', 'embeddeddolt'):
            (marker / name).write_text('not a project')
        self.assert_discovery(None)

    def test_inspection_errors_fail_closed(self):
        marker = self.discovery_parent / '.beads'
        with patch.object(Path, 'lstat', side_effect=PermissionError('fixture')):
            self.assertEqual(discovery.discovery_marker(marker), marker)
        marker.mkdir()
        with patch.object(discovery.os, 'scandir', side_effect=PermissionError('fixture')):
            self.assertEqual(discovery.discovery_marker(marker), marker)


class EnvironmentTests(AncestorDiscoveryCases, unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='vfug-policy.', dir=os.environ.get('TMPDIR', '/work/ga-vfug'))
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.account_home = self.root / 'account'
        self.work = self.account_home / 'work'
        self.work.mkdir(parents=True)
        self.run_root = self.account_home / "run"
        self.run_root.mkdir()
        self.mock = self.root / 'mock'
        self.mock.mkdir()
        self.identity('vfugtest', self.account_home)
        self.discovery_parent = self.account_home

    def assert_discovery(self, refused_path):
        result = self.command('--', '/bin/true')
        if refused_path is None:
            self.assertEqual(result.returncode, 0, result.stderr)
        else:
            self.assertEqual(result.returncode, 2, result.stderr)
            self.assertIn('ancestor discovery marker refused: ' + str(refused_path), result.stderr)

    def identity(self, user, home):
        for name, body in {'id': 'echo ' + user, 'getent': 'echo "' + user + ':x:12345:12345::' + str(home) + ':/bin/bash"'}.items():
            path = self.mock / name
            path.write_text('#!/bin/sh\n' + body + '\n')
            path.chmod(0o755)

    def command(self, *args):
        config = {
            'PATH': str(self.mock) + ':/usr/bin:/bin',
            'VFUG_WORK_ROOT': str(self.work),
            'VFUG_ROOT': str(self.run_root),
            'VFUG_GO_ROOT': str(self.work / 'go'),
            'VFUG_MODCACHE': str(self.work / 'modcache'),
            'VFUG_TOOLS': str(self.work / 'tools'),
            'VFUG_LINT_BIN': str(self.work / 'lint-bin'),
            'VFUG_PAIR_BIN': str(self.work / 'pair'),
            'VFUG_TEST_UNTRUSTED': 'must disappear',
        }
        return subprocess.run(['bash', str(HERE / 'vfug-env.sh'), *args],
                              env=config, capture_output=True, text=True, timeout=15)

    def test_wrong_user_refuses_before_command(self):
        self.identity('operator', self.account_home)
        result = self.command('--', '/bin/true')
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertIn('requires vfugtest', result.stderr)

    def test_passwd_home_outside_work_refuses(self):
        self.identity('vfugtest', '/home/fixture')
        result = self.command('--', '/bin/true')
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertIn('passwd home must be under /work/', result.stderr)

    def test_empty_gitconfig_option_keeps_a_writable_empty_file(self):
        code = ("import os; from pathlib import Path; p = Path(os.environ['GIT_CONFIG_GLOBAL']); "
                "assert p.read_text() == '', 'must be empty'; p.write_text('[x]\\n')")
        result = self.command('--empty-gitconfig', '--', '/usr/bin/python3', '-c', code)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_removed_namespace_entry_refuses(self):
        for option in ('--private-home', '--inside'):
            result = self.command(option, '--', '/bin/true')
            self.assertEqual(result.returncode, 2, result.stderr)

    def test_environment_has_only_staged_and_fixture_paths(self):
        # Assert policy inside the child; never print the environment.
        code = """import os, sys
from pathlib import Path
assert 'VFUG_TEST_UNTRUSTED' not in os.environ
assert all('/home/loucmane' not in v for v in os.environ.values())
base = Path(sys.argv[1])
for name in ('HOME', 'GOPATH', 'GOBIN', 'GC_HOME', 'TMPDIR', 'GOTMPDIR', 'GIT_CONFIG_GLOBAL', 'XDG_RUNTIME_DIR'):
    assert Path(os.environ[name]).is_relative_to(base), name
seed = Path(os.environ['GIT_CONFIG_GLOBAL']).read_text()
assert 'role = maintainer' in seed and '[beads]' in seed, 'gitconfig must seed beads.role'
proxy = os.environ['GOPROXY']
assert proxy.startswith('file://') and proxy.endswith('/cache/download'), 'GOPROXY must stay offline'
assert Path(proxy[len('file://'):]).is_relative_to(Path(os.environ['GOMODCACHE'])), 'GOPROXY must be the staged cache'
Path(os.environ['GIT_CONFIG_GLOBAL']).write_text('[fixture]\\n')
(Path(os.environ['GOBIN']) / 'fixture').write_text('fixture')
"""
        result = self.command('--', '/usr/bin/python3', '-c', code, str(self.work))
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_supervisor_uses_passwd_home(self):
        code = "import os,sys; assert os.environ['HOME'] == sys.argv[1]"
        result = self.command('--supervisor-home', '--', '/usr/bin/python3', '-c', code, str(self.account_home))
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_ancestor_discovery_refuses_before_command(self):
        for marker in ('.beads', '.git'):
            for root in (self.work, self.run_root, self.account_home, self.root):
                with self.subTest(marker=marker, root=root):
                    path = root / marker
                    path.write_text('fixture discovery marker')
                    try:
                        result = self.command('--', '/bin/true')
                        self.assertEqual(result.returncode, 2, result.stderr)
                        self.assertIn('ancestor discovery marker', result.stderr)
                        self.assertIn(str(path), result.stderr)
                    finally:
                        path.unlink()

    def test_work_tree_escape_refuses(self):
        # Resolve path identity, not only its spelling.
        outside = self.root / 'outside'
        outside.mkdir()
        self.work.rmdir()
        self.work.symlink_to(outside, target_is_directory=True)
        result = self.command('--', '/bin/true')
        self.assertEqual(result.returncode, 2, result.stderr)


class StageTests(AncestorDiscoveryCases, unittest.TestCase):
    def setUp(self):
        spec = importlib.util.spec_from_file_location('vfug_stage', HERE / 'vfug-stage.py')
        self.stage = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.stage)
        self.temp = tempfile.TemporaryDirectory(prefix='vfug-stage.', dir=os.environ.get('TMPDIR', '/work/ga-vfug'))
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.discovery_parent = self.root

    def assert_discovery(self, refused_path):
        if refused_path is None:
            self.stage.refuse_ancestor_discovery(self.root / 'nested' / 'run')
        else:
            with self.assertRaisesRegex(ValueError, 'ancestor discovery marker refused: ' + re.escape(str(refused_path)) + '$'):
                self.stage.refuse_ancestor_discovery(self.root / 'nested' / 'run')

    def test_run_refuses_ancestor_discovery_before_stage_or_writes(self):
        home = self.root / 'account'
        home.mkdir()
        for marker in ('.beads', '.git'):
            for parent in (home, home / 'ga-vfug', self.root):
                parent.mkdir(exist_ok=True)
                path = parent / marker
                path.write_text('fixture discovery marker')
                try:
                    with self.subTest(marker=marker, parent=parent), \
                            patch.object(self.stage, 'require_test_account', return_value=home), \
                            patch.object(self.stage, 'verify_manifest') as verify, \
                            self.assertRaisesRegex(ValueError, 'ancestor discovery marker.*' + re.escape(str(path))):
                        self.stage.run_stage(self.root / 'absent-stage', 'plan')
                    verify.assert_not_called()
                finally:
                    path.unlink()

    def test_manifest_covers_every_input_and_detects_drift(self):
        inputs = ['gc', 'bd-src', 'beads-vfug-' + 'a' * 40, 'go', 'modcache', 'tools', 'lint-bin']
        for name in inputs:
            directory = self.root / name
            directory.mkdir()
            (directory / 'input').write_text(name)
        self.stage.write_manifest(self.root, {name: {'commit': 'a' * 40} if name in inputs[:3] else {} for name in inputs})
        manifest = self.stage.verify_manifest(self.root)
        self.assertEqual(set(manifest['inputs']), set(inputs))
        self.assertTrue(all(len(item['sha256']) == 64 for item in manifest['inputs'].values()))
        for name in inputs:
            path = self.root / name / 'input'
            original = path.read_text()
            path.write_text('changed')
            with self.subTest(name=name), self.assertRaisesRegex(ValueError, 'digest'):
                self.stage.verify_manifest(self.root)
            path.write_text(original)
        (self.root / 'extra').write_text('unrecorded')
        with self.assertRaisesRegex(ValueError, 'inventory'):
            self.stage.verify_manifest(self.root)

    def test_manifest_refuses_traversal_and_symlinks(self):
        (self.root / 'go').mkdir()
        (self.root / 'go' / 'input').write_text('go')
        self.stage.write_manifest(self.root, {'go': {}})
        (self.root / 'go' / 'outside').symlink_to('/etc/passwd')
        with self.assertRaisesRegex(ValueError, 'symlink'):
            self.stage.verify_manifest(self.root)
        data = json.loads((self.root / 'manifest.json').read_text())
        data['inputs']['../outside'] = data['inputs'].pop('go')
        (self.root / 'manifest.json').write_text(json.dumps(data))
        with self.assertRaisesRegex(ValueError, 'input name'):
            self.stage.verify_manifest(self.root)

    def test_stage_preserves_commit_and_relative_pin_and_dereferences_tools(self):
        source = self.root / 'source'
        source.mkdir()
        def git(repo, *args):
            return self.stage.git(repo, *args)
        def repo(name):
            path = source / name
            path.mkdir()
            git(path, 'init', '-q')
            git(path, 'config', 'user.name', 'fixture')
            git(path, 'config', 'user.email', 'fixture@local')
            return path
        bd = repo('bd-src')
        (bd / 'go.mod').write_text('module fixture\n')
        git(bd, 'add', '.')
        git(bd, '-c', 'commit.gpgsign=false', 'commit', '-qm', 'fixture')
        commit = git(bd, 'rev-parse', 'HEAD').strip()
        gc = repo('gc')
        (gc / 'scripts').mkdir()
        (gc / '.githooks').mkdir()
        (gc / '.githooks/pre-commit').write_text('# fixture hook\n')
        shutil.copyfile(HERE / 'check-githooks-owner.sh', gc / 'scripts/check-githooks-owner.sh')
        git(gc, 'config', 'core.hooksPath', '.githooks')
        (gc / 'external-bazel').symlink_to('/home/nonexistent/bazel-output')
        import gomod_replace_policy as policy
        pin = {'module': policy.MODULE, 'path': '../beads-vfug-' + commit, 'commit': commit, 'sha256': policy.checkout_hash(bd)}
        (gc / 'scripts' / 'beads-fork-pin.json').write_text(json.dumps(pin))
        (gc / 'go.mod').write_text('module fixture\nreplace ' + policy.MODULE + ' => ' + pin['path'] + '\n')
        git(gc, 'add', '.')
        git(gc, '-c', 'commit.gpgsign=false', 'commit', '-qm', 'fixture')
        self.stage.clone_commit(bd, source / ('beads-vfug-' + commit), commit)
        config = {'VFUG_GC_SOURCE': str(gc), 'VFUG_BD_SOURCE': str(bd)}
        for var, name in [('VFUG_GO_SOURCE', 'go'), ('VFUG_MODCACHE_SOURCE', 'modcache'),
                          ('VFUG_TOOLS_SOURCE', 'tools'), ('VFUG_LINT_SOURCE', 'lint-bin')]:
            directory = source / name
            directory.mkdir()
            (directory / 'input').write_text(name)
            config[var] = str(directory)
        (source / 'lint-bin' / 'linked-tool').symlink_to(source / 'tools' / 'input')
        source_config = hashlib.sha256((gc / '.git/config').read_bytes()).digest()
        destination = self.root / 'staged'
        with patch.dict(os.environ, config), patch.object(self.stage, 'require_operator'):
            self.stage.stage_inputs(destination)
        self.assertEqual(hashlib.sha256((gc / '.git/config').read_bytes()).digest(), source_config)
        manifest = self.stage.verify_manifest(destination)
        self.assertEqual(manifest['inputs']['gc']['commit'], git(gc, 'rev-parse', 'HEAD').strip())
        self.assertEqual(git(destination / 'bd-src', 'rev-parse', 'HEAD').strip(), commit)
        self.assertEqual(git(destination / 'gc' / pin['path'], 'rev-parse', 'HEAD').strip(), commit)
        self.assertFalse((destination / 'lint-bin' / 'linked-tool').is_symlink())
        self.assertEqual((destination / 'lint-bin' / 'linked-tool').read_text(), 'tools')
        self.assertEqual(os.readlink(destination / 'gc' / 'external-bazel'), '/home/nonexistent/bazel-output')
        policy.verify_checkout(destination / 'gc' / pin['path'], pin)
        account_home = self.root / 'account'
        account_home.mkdir()
        with patch.object(self.stage, 'require_test_account', return_value=account_home), \
                patch.object(self.stage.subprocess, 'call', return_value=0) as launch:
            self.assertEqual(self.stage.run_stage(destination, 'plan'), 0)
        copied_gc = Path(launch.call_args.kwargs['env']['VFUG_GC_REPO'])
        gate = subprocess.run(['bash', 'scripts/check-githooks-owner.sh'], cwd=copied_gc,
                              env={'PATH': '/usr/bin:/bin', 'HOME': str(account_home),
                                   'GIT_CONFIG_NOSYSTEM': '1', 'GIT_CONFIG_GLOBAL': '/dev/null'},
                              capture_output=True, text=True)
        self.assertEqual(gate.returncode, 0, gate.stderr)
        self.assertEqual(hashlib.sha256((gc / '.git/config').read_bytes()).digest(), source_config)
        command = launch.call_args.args[0]
        child_env = launch.call_args.kwargs['env']
        copied = Path(child_env['VFUG_GC_REPO']).parent
        self.assertNotEqual(copied, destination)
        self.assertTrue(copied.is_relative_to(account_home))
        self.assertTrue(Path(command[2]).is_relative_to(copied))
        self.assertTrue(all('/home/loucmane' not in value for value in child_env.values()))
        self.stage.verify_manifest(destination)
        policy.verify_checkout(copied / 'gc' / pin['path'], pin)
        self.assertEqual(os.readlink(copied / 'gc' / 'external-bazel'), '/home/nonexistent/bazel-output')


if __name__ == '__main__':
    unittest.main()

#!/usr/bin/env python3
"""Exercise runner failure accounting without starting builds or services."""
import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('vfug_host', Path(__file__).with_name('vfug-host.py'))
host = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = host
spec.loader.exec_module(host)


class RunnerTests(unittest.TestCase):
    def test_namespace_is_reserved_for_supervisor_acceptance(self):
        steps = host.suites('all', Path('/fixture/bin'))
        namespaced = [step.name for step in steps if getattr(step, 'private_home', False)]
        self.assertEqual(namespaced, ['proxied-acceptance'])
        for step in steps:
            command = host.step_command(step, Path('/runner'))
            self.assertEqual('--private-home' in command, step.name == 'proxied-acceptance')
        self.assertEqual(host.GC, Path(__file__).resolve().parents[1])

    @staticmethod
    def passwd_home():
        import pwd
        return pwd.getpwuid(os.getuid()).pw_dir

    def test_plain_command_refuses_operator_passwd_home(self):
        if self.passwd_home().startswith('/work/ga-vfug/'):
            self.skipTest('already isolated by host-run.sh')
        script = Path(__file__).with_name('vfug-env.sh')
        marker = Path('/work/ga-vfug') / ('plain-ran.' + str(os.getpid()))
        result = subprocess.run(['bash', str(script), '--', 'touch', str(marker)],
                                env={'PATH': '/usr/bin:/bin'}, capture_output=True, text=True, timeout=15)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('refusing: passwd home is', result.stderr)
        self.assertFalse(marker.exists(), 'plain command ran with the operator passwd home')

    def test_plain_command_keeps_host_identity_and_isolates_environment(self):
        if not self.passwd_home().startswith('/work/ga-vfug/'):
            self.skipTest('needs the host-run.sh private passwd; the refusal test covers this host')
        script = Path(__file__).with_name('vfug-env.sh')
        code = '''import json, os, pwd
from pathlib import Path
print(json.dumps({
    'home': os.environ['HOME'], 'tmp': os.environ['TMPDIR'],
    'gotmp': os.environ['GOTMPDIR'], 'tmux': os.environ['TMUX_TMPDIR'],
    'root_uid': Path('/').stat().st_uid,
    'leaked': 'VFUG_TEST_UNTRUSTED' in os.environ,
    'uid_map': Path('/proc/self/uid_map').read_text(),
    'cap_eff': next(line for line in Path('/proc/self/status').read_text().splitlines() if line.startswith('CapEff:')),
    'passwd_home': pwd.getpwuid(os.getuid()).pw_dir,
    'git_global': os.environ['GIT_CONFIG_GLOBAL'],
}))'''
        result = subprocess.run(['bash', str(script), '--', 'python3', '-c', code],
                                env={**os.environ, 'VFUG_TEST_UNTRUSTED': 'present'},
                                capture_output=True, text=True, timeout=15)
        self.assertEqual(result.returncode, 0, result.stderr)
        values = json.loads(result.stdout.splitlines()[-1])
        for key in ('home', 'tmp', 'gotmp', 'tmux'):
            self.assertTrue(values[key].startswith('/work/ga-vfug/'), values)
        self.assertFalse(values['leaked'])
        self.assertTrue(values['passwd_home'].startswith('/work/ga-vfug/'), values)
        self.assertTrue(values['git_global'].startswith('/work/ga-vfug/'), values)
        self.assertEqual(values['root_uid'], 0, 'a user namespace would show root as 65534')
        self.assertEqual(values['cap_eff'].split()[-1], '0000000000000000')
        self.assertEqual(values['root_uid'], Path('/').stat().st_uid)
        self.assertEqual(values['uid_map'], Path('/proc/self/uid_map').read_text())
        expected_caps = next(line for line in Path('/proc/self/status').read_text().splitlines() if line.startswith('CapEff:'))
        self.assertEqual(values['cap_eff'], expected_caps)

    def test_inside_refuses_host_namespace_before_mount(self):
        script = Path(__file__).with_name('vfug-env.sh')
        with tempfile.TemporaryDirectory(prefix='inside.', dir='/work/ga-vfug') as temporary:
            root = Path(temporary)
            marker = root / 'mounted'
            mount = root / 'mount'
            mount.write_text('#!/bin/sh\ntouch "' + str(marker) + '"\nexit 0\n')
            mount.chmod(0o755)
            result = subprocess.run(['bash', str(script), '--inside', str(root), '/bin/true'],
                                    env={'PATH': str(root) + ':/usr/bin:/bin'},
                                    capture_output=True, text=True, timeout=15)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('requires a private user namespace', result.stderr)
            self.assertFalse(marker.exists(), 'attempted mount before validating namespace')

    def test_private_home_drops_capabilities_before_command(self):
        source = Path(__file__).with_name('vfug-env.sh').read_text()
        inside = source.split('if [[ ${1:-} == --inside ]]; then', 1)[1].split('\nfi', 1)[0]
        self.assertIn('exec setpriv --inh-caps=-all --ambient-caps=-all --bounding-set=-all -- "$@"', inside)
        self.assertLess(inside.rindex('mount '), inside.index('exec setpriv'))

    def run_fixture(self, *, launch_error=False, drift=False):
        base = Path('/work/ga-vfug')
        base.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(prefix='runner.', dir=base) as temporary:
            root = Path(temporary)
            gc = root / 'port'
            (gc / 'scripts').mkdir(parents=True)
            for filename in ('vfug-host.py', 'vfug-env.sh', 'build-vfug.sh'):
                (gc / 'scripts' / filename).write_text('fixture runner\n')
            executed = []

            def popen(command, *, cwd, stdout, stderr, env):
                executed.append(command[-1])
                if launch_error and command[-1] == 'fail-launch':
                    raise OSError('fixture executable unavailable')
                if any(Path(arg).name == 'build-vfug.sh' for arg in command):
                    index = next(i for i, arg in enumerate(command) if Path(arg).name == 'build-vfug.sh')
                    output = Path(command[index + 1])
                    output.mkdir(parents=True)
                    for name in ('bd', 'gc'):
                        content = name + ('drift' if drift and 'build-2' in str(output) else '')
                        (output / name).write_text(content)
                stdout.write('fixture check completed\n')
                class Process:
                    def wait(self, timeout):
                        return 0
                return Process()

            def git_output(command, **kwargs):
                return '' if 'status' in command else 'a' * 40 + '\n'

            steps = [host.Step('first-check', gc, ['fail-launch']),
                     host.Step('later-check', gc, ['later']),
                     host.Step('independent', gc, ['independent'], False)]
            with patch.object(host, 'ROOT', root), patch.object(host, 'GC', gc), \
                    patch.object(host, 'BD', root / 'bd-src'), \
                    patch.object(host, 'suites', return_value=steps), \
                    patch.object(host.subprocess, 'Popen', side_effect=popen), \
                    patch.object(host.subprocess, 'check_output', side_effect=git_output), \
                    patch.object(sys, 'argv', ['vfug-host.py', 'all']), \
                    contextlib.redirect_stdout(io.StringIO()):
                code = host.main()
            report = next((root / 'host-r9').glob('run.*/summary.json'))
            return code, {row['name']: row for row in json.loads(report.read_text())}, executed

    def test_failed_launch_is_reported_and_later_checks_run(self):
        code, rows, executed = self.run_fixture(launch_error=True)
        self.assertEqual(code, 1)
        self.assertNotEqual(rows['first-check']['exit'], 0)
        self.assertEqual(rows['later-check']['exit'], 0)
        self.assertIn('later', executed)

    def test_different_builds_block_dependents_but_keep_independent_checks(self):
        code, rows, executed = self.run_fixture(drift=True)
        self.assertEqual(code, 1)
        self.assertEqual(rows['first-check']['exit'], 'BLOCKED')
        self.assertEqual(rows['later-check']['exit'], 'BLOCKED')
        self.assertEqual(rows['independent']['exit'], 0)
        self.assertNotIn('later', executed)

    def test_identical_builds_and_checks_succeed(self):
        code, rows, _ = self.run_fixture()
        self.assertEqual(code, 0)
        self.assertTrue(all(row['exit'] == 0 for row in rows.values()))


if __name__ == '__main__':
    unittest.main()

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
    def test_bd_mode_covers_public_and_concrete_readonly_contracts(self):
        steps = {step.name: step for step in host.suites('bd', Path('/fixture/bin'))}
        self.assertIn('bd-readonly', steps)
        self.assertIn('bd-readonly-cli', steps)
        boundary = steps['bd-readonly-boundaries']
        self.assertIn('./internal/storage/dolt', boundary.args)
        self.assertIn('BEADS_SCHEMA_INIT_HELPER=1', boundary.args)
        self.assertFalse(boundary.pair)
        self.assertIn('ReviewB3', boundary.args[boundary.args.index('-run') + 1])

    def test_passwd_home_is_reserved_for_supervisor_acceptance(self):
        steps = host.suites('all', Path('/fixture/bin'))
        namespaced = [step.name for step in steps if getattr(step, 'supervisor_home', False)]
        self.assertEqual(namespaced, ['proxied-acceptance'])
        for step in steps:
            command = host.step_command(step, Path('/runner'))
            self.assertEqual('--supervisor-home' in command, step.name == 'proxied-acceptance')
        self.assertEqual(host.GC, Path(__file__).resolve().parents[1])

    def run_fixture(self, *, launch_error=False, drift=False):
        base = Path(os.environ.get('TMPDIR', '/work/ga-vfug'))
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
            with patch.object(host, 'configure'), patch.object(host, 'step_environment', return_value={}), \
                    patch.object(host, 'ROOT', root), patch.object(host, 'GC', gc), \
                    patch.object(host, 'BD', root / 'bd-src'), \
                    patch.object(host, 'suites', return_value=steps), \
                    patch.object(host.subprocess, 'Popen', side_effect=popen), \
                    patch.object(host.subprocess, 'check_output', side_effect=git_output), \
                    patch.object(sys, 'argv', ['vfug-host.py', 'all']), \
                    contextlib.redirect_stdout(io.StringIO()):
                code = host.main()
            report = next((root / 'logs').glob('run.*/summary.json'))
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

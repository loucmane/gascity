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

    def test_private_home_is_cancelled_only_for_an_isolated_passwd_home(self):
        source = Path(__file__).with_name('vfug-env.sh').read_text()
        guard = source.split('vfug_passwd_home=$(getent passwd', 1)[1].split('\nesac', 1)[0]
        self.assertIn('/work/ga-vfug/*) vfug_private_home=false ;;', guard)
        self.assertIn('if ! "$vfug_private_home"; then', guard)
        self.assertIn('exit 2', guard)
        self.assertLess(source.index('vfug_passwd_home=$(getent passwd'), source.index('vfug_fixture=$(mktemp'))

    def test_plain_command_refuses_operator_passwd_home(self):
        if self.passwd_home().startswith('/work/ga-vfug/'):
            self.skipTest('already isolated by vfug-host-root.sh')
        script = Path(__file__).with_name('vfug-env.sh')
        marker = Path('/work/ga-vfug') / ('plain-ran.' + str(os.getpid()))
        result = subprocess.run(['bash', str(script), '--', 'touch', str(marker)],
                                env={'PATH': '/usr/bin:/bin'}, capture_output=True, text=True, timeout=15)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('refusing: passwd home is', result.stderr)
        self.assertFalse(marker.exists(), 'plain command ran with the operator passwd home')

    def test_plain_command_keeps_host_identity_and_isolates_environment(self):
        if not self.passwd_home().startswith('/work/ga-vfug/'):
            self.skipTest('needs the vfug-host-root.sh private passwd; the refusal test covers this host')
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

    ROOT_SCRIPT = Path(__file__).with_name('vfug-host-root.sh')

    def root_stage(self, name):
        source = self.ROOT_SCRIPT.read_text()
        if name == 'stage1':
            return source.split('# Stage 1:', 1)[1]
        return source.split('if [[ ${1:-} == --' + name + ' ]]; then', 1)[1].split('\nfi\n', 1)[0]

    def test_root_entry_refuses_without_root_or_from_operator_file(self):
        for args in (['all', 'a' * 64], ['--stage2', 'all', '/x'], ['--stage3', 'all', '/x'], ['bogus', 'a' * 64]):
            result = subprocess.run(['bash', str(self.ROOT_SCRIPT), *args], env={'PATH': '/usr/bin:/bin'},
                                    capture_output=True, text=True, timeout=15)
            self.assertEqual(result.returncode, 2, (args, result.stderr))

    def test_root_stage1_runs_only_from_verified_root_copy(self):
        stage = self.root_stage('stage1')
        self.assertIn('[[ ${BASH_SOURCE[0]} == "$state/entry.sh" ]]', stage)
        self.assertLess(stage.index('sha256sum < "$state/entry.sh"'), stage.index('exec bash "$state/entry.sh" --stage2'))
        self.assertIn('[[ $got == "$want" ]]', stage)
        self.assertNotIn('install ', stage.split('usage=', 1)[1].split('\n', 1)[1])

    def test_root_never_changes_operator_writable_paths(self):
        stage = self.root_stage('stage2')
        for line in stage.splitlines():
            if '$fixture' in line and any(word in line for word in ('mkdir', 'chmod', 'chown', 'install ')):
                self.assertTrue(line.strip().startswith('as_operator '), line)
        code = [line for line in self.ROOT_SCRIPT.read_text().splitlines() if not line.lstrip().startswith('#')]
        self.assertFalse([line for line in code if 'chown' in line])
        self.assertIn('! -L $path', stage)
        self.assertIn('realpath -e "$fixture/h"', stage)

    def test_root_passwd_rewrite_changes_exactly_the_operator_home(self):
        stage = self.root_stage('stage2')
        program = stage.split("awk -F: -v OFS=: -v uid=\"$op_uid\" -v home=\"$fixture/h\" '", 1)[1].split("'", 1)[0]
        with tempfile.TemporaryDirectory(prefix='passwd.', dir='/work/ga-vfug') as temporary:
            sample = Path(temporary) / 'passwd'
            sample.write_text('root:x:0:0:root:/root:/bin/bash\nloucmane:x:1000:1000::/home/loucmane:/usr/bin/zsh\n'
                              'other:x:10000:10000::/home/other:/bin/sh\n')
            out = subprocess.run(['awk', '-F:', '-v', 'OFS=:', '-v', 'uid=1000', '-v', 'home=/work/ga-vfug/hr/h',
                                  program, str(sample)], capture_output=True, text=True)
            self.assertEqual(out.returncode, 0)
            self.assertEqual(out.stdout.splitlines(), ['root:x:0:0:root:/root:/bin/bash',
                                                       'loucmane:x:1000:1000::/work/ga-vfug/hr/h:/usr/bin/zsh',
                                                       'other:x:10000:10000::/home/other:/bin/sh'])
            sample.write_text('root:x:0:0:root:/root:/bin/bash\n')
            missing = subprocess.run(['awk', '-F:', '-v', 'OFS=:', '-v', 'uid=1000', '-v', 'home=/h', program, str(sample)],
                                     capture_output=True, text=True)
            self.assertNotEqual(missing.returncode, 0)

    def test_root_stage3_guards_isolates_and_drops(self):
        stage = self.root_stage('stage3')
        self.assertLess(stage.index('[[ $$ -eq 1 && $EUID -eq 0 ]]'), stage.index('mount '))
        self.assertEqual(stage.index('mount '), stage.index('mount --make-rprivate /'))
        for masked in ('$op_home/.beads', '$op_home/gascity', '/run/user/$op_uid', '/tmp/tmux-$op_uid'):
            self.assertIn(masked, stage)
        self.assertIn('/proc/self/mountinfo', stage)
        self.assertNotIn('mount --bind "$op_home/go" ', stage)
        drop = stage[stage.index('exec setpriv'):]
        for flag in ('--reuid="$op_uid"', '--regid="$op_gid"', '--inh-caps=-all', '--ambient-caps=-all',
                     '--bounding-set=-all', '--no-new-privs', '/usr/bin/env -i'):
            self.assertIn(flag, drop)
        self.assertLess(stage.index('ABORT: operator home is writable'), stage.index('exec setpriv'))

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

#!/usr/bin/env python3
"""Sequential host qualification with immutable inputs and per-check logs."""
import argparse
from dataclasses import dataclass
import hashlib
import json
import os
import pwd
import re
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time

import vfug_compare
from vfug_layout import refuse_home_workspace, validate_roots
from vfug_discovery import refuse_ancestor_discovery

ROOT = None
GC = Path(__file__).resolve().parents[1]
BD = None
CONFIG_KEYS = ('VFUG_ROOT', 'VFUG_WORK_ROOT', 'VFUG_GC_REPO', 'VFUG_BD_REPO',
               'VFUG_GO_ROOT', 'VFUG_MODCACHE', 'VFUG_TOOLS', 'VFUG_LINT_BIN')


def configure():
    global ROOT, GC, BD
    for key in CONFIG_KEYS:
        if key not in os.environ or not Path(os.environ[key]).resolve().is_relative_to('/work'):
            raise ValueError(key + ' must explicitly name a staged /work path')
    account = pwd.getpwuid(os.getuid())
    if account.pw_name != 'vfugtest' or os.geteuid() == 0:
        raise ValueError('qualification requires vfugtest')
    home = Path(account.pw_dir).resolve(strict=True)
    if not home.is_relative_to('/work') or home == Path('/work'):
        raise ValueError('passwd home must be under /work/')
    ROOT = Path(os.environ['VFUG_ROOT'])
    fixtures = Path(os.environ['VFUG_WORK_ROOT'])
    validate_roots(ROOT, fixtures, home)
    refuse_home_workspace(home)
    for root in (ROOT, fixtures):
        refuse_ancestor_discovery(root)
    GC = Path(os.environ['VFUG_GC_REPO'])
    BD = Path(os.environ['VFUG_BD_REPO'])


def step_environment(pair):
    return {'PATH': '/usr/bin:/bin', 'VFUG_PAIR_BIN': str(pair),
            **{key: os.environ[key] for key in CONFIG_KEYS}}


@dataclass
class Step:
    name: str
    cwd: Path
    args: list
    pair: bool = True
    supervisor_home: bool = False
    empty_gitconfig: bool = False
    bin_dir: Path | None = None


def step_command(step, runner):
    options = ['--supervisor-home'] if step.supervisor_home else []
    if step.empty_gitconfig:
        options.append('--empty-gitconfig')
    return ['bash', str(runner / 'vfug-env.sh'), *options, '--', *step.args]


def suites(group, pair):
    bd = pair / 'bd'
    gc = pair / 'gc'
    pin = json.loads((GC / 'scripts/beads-fork-pin.json').read_text())
    source = (GC / pin['path']).resolve()
    steps = []
    if group in ('all', 'bd', 'gc', 'acceptance', 'cas', 'safety'):
        steps.append(Step('tooling', GC, ['bash', '-c',
            'set -e; for tool in dolt tmux docker golangci-lint oapi-codegen; do command -v "$tool"; done; dolt version; tmux -V; docker --version']))
    if group in ('all', 'bd'):
        for mode in ('count-init', 'guarded', 'lifecycle', 'row-lock', 'races', 'readonly', 'readonly-cli', 'vet'):
            steps.append(Step('bd-' + mode, source,
                ['env', 'BEADS_TEST_BD_BINARY=' + str(bd), 'bash', 'scripts/test-vfug.sh', mode],
                empty_gitconfig=True))
        steps.append(Step('bd-readonly-boundaries', source, [
            'env', 'BEADS_SCHEMA_INIT_HELPER=1', 'CGO_ENABLED=1', 'go', 'test',
            '-tags=gms_pure_go', './internal/storage/dolt',
            '-run', 'Test(ReadOnly|ReviewB2ReadOnly|ReviewB3)', '-count=1', '-timeout=2m', '-v'], False,
            empty_gitconfig=True))
    if group in ('all', 'gc'):
        for label, target in [('gc-fast', 'test-fast-parallel'),
                              ('gc-process', 'test-cmd-gc-process-parallel'),
                              ('gc-integration', 'test-integration-shards-parallel')]:
            steps.append(Step(label, GC, ['make', target, 'LOCAL_TEST_JOBS=2']))
    if group in ('all', 'gates', 'gc'):
        steps += [
            Step('runner-policy', GC, ['python3', '-B', '-m', 'unittest', 'discover', '-s', 'scripts', '-p', 'test_vfug*.py', '-v'], False),
            Step('fork-policy', GC, ['python3', '-B', 'scripts/test_gomod_replace_policy.py', '-v'], False),
            Step('release-replace', GC, ['make', 'check-gomod-replace']),
            Step('release-core', GC, ['make', 'check-core-boundary']),
            Step('release-native', GC, ['make', 'check-native-dependency-surface']),
            Step('release-events', GC, ['make', 'check-eventexport-isolation']),
            Step('release-ci-policy', GC, ['make', 'test-ci-policy']),
            Step('release-generated', GC, ['make', 'spec-ci']),
            Step('release-hooks-docs', GC, ['make', 'check-hooks', 'check-docs']),
            Step('gc-vet', GC, ['go', 'vet', './...']),
            Step('gc-lint', GC, ['bash', '-c', 'set -e; lint=$(command -v golangci-lint); exec make lint-changed "GOLANGCI_LINT=$lint" LINT_CHANGED_REF=fc0f8a966 LINT_CHANGED_SCOPE=tracked "LINT_FLAGS=--new-from-rev=fc0f8a966 --whole-files"']),
            Step('custody-resolver-module', GC, ['go', 'test', './internal/beads/beadstest',
                './internal/platforminstall', './cmd/gc', './scripts',
                '-run', 'Test(Pinned|RepositorySource|MetadataCustody|ModuleGraph|CheckGomodReplace|ReplaceDirective)', '-count=1', '-timeout=15m']),
            Step('bdflags', GC, ['go', 'test', '-tags=integration', './internal/bdflags', '-count=1', '-v']),
        ]
    if group in ('all', 'acceptance', 'safety'):
        pattern = ('^(TestProxiedNativeSafety|TestProxiedNativeOpenDoesNotRepairIgnorePatterns)$'
                   if group == 'safety' else
                   '^(TestProxiedNative.*|TestBeadsProxiedDefault|TestDoctorLeavesAStoppedProxiedCityStopped)$')
        steps.append(Step('proxied-acceptance', GC, ['env', 'GC_REQUIRE_ACCEPTANCE_TOOLING=1',
            'GC_ACCEPTANCE_BD_BIN=' + str(bd), 'GC_ACCEPTANCE_GC_BIN=' + str(gc),
            'go', 'test', '-tags=acceptance_a', './test/acceptance', '-run', pattern,
            '-count=1', '-timeout=45m', '-v'], supervisor_home=True))
    if group in ('all', 'cas', 'safety'):
        steps.append(Step('gc-cas', GC, ['env', 'GC_TEST_PROXIED_CAS=1', 'GC_ACCEPTANCE_BD_BIN=' + str(bd),
            'go', 'test', '-tags=integration', './cmd/gc', '-run', '^TestProxiedNativeClaimCAS$',
            '-count=1', '-timeout=15m', '-v']))
        steps.append(Step('gc-proxy-race', GC, ['go', 'test', '-race', './internal/beads',
            '-run', '^TestProxiedStore', '-count=1', '-timeout=15m']))
    return steps


def git_output(repo, *args):
    return subprocess.check_output(['git', '--no-optional-locks', '-C', str(repo), *args], text=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('mode', nargs='?', default='all',
                        choices=['all', 'build', 'bd', 'gc', 'gates', 'acceptance', 'cas', 'safety', 'plan', 'compare'])
    parser.add_argument('base_gc_commit', nargs='?', default='3bb7b3c76')
    opts = parser.parse_args()
    try:
        configure()
    except (ValueError, OSError) as exc:
        parser.error(str(exc))
    base_gc = None
    if opts.mode == 'compare':
        try:
            base_gc = Path(os.environ['VFUG_BASE_REPO']).resolve(strict=True)
            if not base_gc.is_relative_to(ROOT.resolve()):
                raise ValueError('base source must be a staged input in VFUG_ROOT')
            base_sha = git_output(base_gc, 'rev-parse', '--verify', '--end-of-options', opts.base_gc_commit + '^{commit}').strip()
            if git_output(base_gc, 'rev-parse', 'HEAD').strip() != base_sha or git_output(base_gc, 'status', '--porcelain', '--untracked-files=all'):
                raise ValueError('base source must be clean and at the requested commit')
        except (KeyError, ValueError, OSError, subprocess.CalledProcessError) as exc:
            parser.error(str(exc))
    if opts.mode == 'plan':
        print('build-1\nbuild-2\ncompare-builds')
        for step in suites('all', ROOT / 'logs/OUTPUT/build-1/bin'):
            print(step.name + ': ' + ' '.join(step_command(step, GC / 'scripts')))
        return 0
    base = ROOT / 'logs'
    base.mkdir(exist_ok=True)
    run = Path(tempfile.mkdtemp(prefix='run.', dir=base))
    run.chmod(0o755)
    print('Qualification logs: ' + str(run), flush=True)
    # Copy executable runner sources before starting any long command.
    snapshot = run / 'runner'
    snapshot.mkdir()
    for filename in ('vfug-host.py', 'vfug-env.sh', 'build-vfug.sh', 'vfug_discovery.py', 'vfug_layout.py', 'vfug_compare.py'):
        shutil.copy2(GC / 'scripts' / filename, snapshot / filename)
    results = []
    source_commit = subprocess.check_output(['git', '--no-optional-locks', '-C', str(GC), 'rev-parse', 'HEAD'], text=True).strip()
    (run / 'gc-commit.txt').write_text(source_commit + '\n')
    pair = run / 'build-1/bin'
    started = time.monotonic()

    def step_run(step):
        folder = Path(tempfile.mkdtemp(prefix=step.name + '.', dir=run))
        folder.chmod(0o755)
        log = folder / 'output.log'
        command = step_command(step, snapshot)
        (folder / 'command.json').write_text(json.dumps({'cwd': str(step.cwd), 'argv': command}, indent=2) + '\n')
        print(f'START {step.name}: {log}', flush=True)
        begin = time.monotonic()
        with log.open('w') as stream:
            try:
                proc = subprocess.Popen(command, cwd=step.cwd, stdout=stream, stderr=subprocess.STDOUT,
                                        env=step_environment(step.bin_dir or pair))
            except OSError as exc:
                stream.write('Cannot start check: ' + str(exc) + '\n')
                code = 127
            else:
                try:
                    while True:
                        try:
                            code = proc.wait(timeout=30)
                            break
                        except subprocess.TimeoutExpired:
                            print(f'RUNNING {step.name}: {int(time.monotonic()-begin)}s; log {log}', flush=True)
                except KeyboardInterrupt:
                    proc.terminate()
                    try:
                        proc.wait(timeout=15)
                    except subprocess.TimeoutExpired:
                        proc.kill()
                        proc.wait()
                    code = 130
        content = log.read_text(errors='replace')
        skips = sum('--- SKIP:' in line for line in content.splitlines())
        if step.name == 'bd-readonly-cli' and code == 0 and (skips or not re.search(
                r'^--- PASS: TestReadOnlyServerCLIMaintenanceAndPreviews \(', content, re.M)):
            code = 1
            with log.open('a') as stream:
                stream.write('FAIL: real-server readonly CLI test did not execute and pass without skips\n')
        row = {'name': step.name, 'exit': code, 'seconds': round(time.monotonic()-begin, 1),
               'skips': skips, 'log': str(log)}
        results.append(row)
        print(f'END {step.name}: exit={code} skips={skips}', flush=True)
        if code == 130:
            raise KeyboardInterrupt
        return code == 0

    try:
        # Always build twice from the exact pinned source before running a suite.
        first = step_run(Step('build-1', GC, ['bash', str(snapshot / 'build-vfug.sh'), str(pair), str(BD), str(GC)], False))
        second_dir = run / 'build-2/bin'
        second = step_run(Step('build-2', GC, ['bash', str(snapshot / 'build-vfug.sh'), str(second_dir), str(BD), str(GC)], False))
        same = False
        if first and second:
            hashes = {name: [hashlib.sha256((directory / name).read_bytes()).hexdigest()
                            for directory in (pair, second_dir)] for name in ('bd', 'gc')}
            same = all(values[0] == values[1] for values in hashes.values())
            (run / 'binary-hashes.json').write_text(json.dumps(hashes, indent=2) + '\n')
        comparison = run / 'compare-builds'
        comparison.mkdir()
        (comparison / 'output.log').write_text('PASS: identical bd and gc hashes\n' if same else 'FAIL: builds failed or hashes differ\n')
        results.append({'name': 'compare-builds', 'exit': 0 if same else 1,
                        'seconds': 0, 'skips': 0, 'log': str(comparison / 'output.log')})
        if opts.mode == 'compare':
            comparison_dir = run / 'comparison'
            comparison_dir.mkdir()
            base_pair = run / 'base-bin'
            # Build both binaries from the BASE module graph, with no fork pin
            # injection and no use of the branch pair on the base PATH.
            base_pair.mkdir()
            base_build = step_run(Step('base-build', base_gc, [
                'bash', '-c', 'set -e; CGO_ENABLED=0 go build -trimpath -o "$1/gc" ./cmd/gc; '
                '''bd_source=$(go list -m -f '{{.Dir}}' github.com/steveyegge/beads); '''
                'cd "$bd_source"; CGO_ENABLED=1 go build -tags=gms_pure_go -trimpath -o "$1/bd" ./cmd/bd',
                'vfug-base-build', str(base_pair)], False, bin_dir=base_pair))
            if same and base_build:
                def execute(name, repo, args, binaries):
                    step_run(Step(name, repo, args, bin_dir=binaries))
                    return results[-1]
                vfug_compare.run_comparison(comparison_dir,
                    {'branch': GC, 'base': base_gc}, {'branch': pair, 'base': base_pair},
                    {'branch': source_commit, 'base': base_sha}, execute)
            else:
                vfug_compare.write_report(comparison_dir, {
                    'commits': {'branch': source_commit, 'base': base_sha},
                    'branch_only': [], 'base_only': [], 'both': [], 'exclusions': [],
                    'blocked': 'comparison suites require successful branch and base builds',
                    'builds': list(results)})
            integrity = comparison_dir / 'base-integrity.log'
            current = git_output(base_gc, 'rev-parse', 'HEAD').strip()
            dirty = git_output(base_gc, 'status', '--porcelain', '--untracked-files=all')
            integrity.write_text(f'initial={base_sha}\nfinal={current}\n{dirty}')
            results.append({'name': 'final-base-integrity', 'exit': 0 if current == base_sha and not dirty else 1,
                            'seconds': 0, 'skips': 0, 'log': str(integrity)})
        for step in ([] if opts.mode == 'compare' else suites(opts.mode, pair)):
            if step.pair and not same:
                results.append({'name': step.name, 'exit': 'BLOCKED', 'seconds': 0, 'skips': 0,
                                'log': 'reproducible build pair did not pass'})
            else:
                step_run(step)
        step_run(Step('final-fork-integrity', GC, ['bash', 'scripts/check-gomod-replace.sh', 'go.mod'], False))
        current_commit = subprocess.check_output(['git', '--no-optional-locks', '-C', str(GC), 'rev-parse', 'HEAD'], text=True).strip()
        dirty = subprocess.check_output(['git', '--no-optional-locks', '-C', str(GC), 'status', '--porcelain', '--untracked-files=all'], text=True)
        integrity = run / 'final-gc-integrity'
        integrity.mkdir()
        (integrity / 'output.log').write_text(f'initial={source_commit}\nfinal={current_commit}\n{dirty}')
        results.append({'name': 'final-gc-integrity', 'exit': 0 if current_commit == source_commit and not dirty else 1,
                        'seconds': 0, 'skips': 0, 'log': str(integrity / 'output.log')})
    except KeyboardInterrupt:
        print('Interrupted; active command terminated. Remaining checks did not run.', flush=True)
    finally:
        print('\nCHECK                         EXIT       SECONDS  SKIPS  LOG')
        for row in results:
            print(f'{row["name"]:29} {str(row["exit"]):9} {row["seconds"]:7} {row["skips"]:6}  {row["log"]}')
        (run / 'summary.json').write_text(json.dumps(results, indent=2) + '\n')
        print(f'Elapsed: {int(time.monotonic()-started)}s; results: {run}', flush=True)
    return 0 if results and all(row['exit'] == 0 for row in results) else 1


if __name__ == '__main__':
    sys.exit(main())

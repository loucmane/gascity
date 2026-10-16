#!/usr/bin/env python3
"""Offline staging and unprivileged execution for the reviewed vfug pair."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import pwd
import re
import shutil
import stat
import subprocess
import tempfile

import gomod_replace_policy as policy
from vfug_discovery import refuse_ancestor_discovery
from vfug_layout import RUN_PARENT, refuse_home_workspace, validate_roots


def git(repo, *args):
    return subprocess.check_output(
        ['git', '--no-optional-locks', '-c', 'core.hooksPath=/dev/null',
         '-c', 'protocol.allow=never', '-c', 'protocol.file.allow=always',
         '-C', str(repo), *args], text=True,
        env={'PATH': '/usr/bin:/bin', 'GIT_CONFIG_NOSYSTEM': '1',
             'GIT_CONFIG_GLOBAL': '/dev/null', 'GIT_TERMINAL_PROMPT': '0'})


def require_operator():
    if pwd.getpwuid(os.getuid()).pw_name != 'loucmane' or os.geteuid() == 0:
        raise ValueError('staging requires the unprivileged loucmane account')


def require_test_account():
    account = pwd.getpwuid(os.getuid())
    if account.pw_name != 'vfugtest' or os.geteuid() == 0:
        raise ValueError('qualification requires vfugtest')
    if not Path(account.pw_dir).is_relative_to('/work'):
        raise ValueError('passwd home must be under /work/')
    home = Path(account.pw_dir).resolve(strict=True)
    if not home.is_relative_to('/work') or home == Path('/work'):
        raise ValueError('passwd home must be under /work/')
    return home


def work_path(path):
    path = path.absolute()
    resolved = path.resolve()
    if path != resolved or not resolved.is_relative_to('/work') or resolved == Path('/work'):
        raise ValueError('stage must be a real path under /work/')
    return resolved


def tree_digest(root, allow_symlinks=False):
    """Hash names, file bytes, directory inventory and executable bits."""
    records = []
    if root.is_symlink() or not root.is_dir():
        raise ValueError('input must be a directory, not a symlink: ' + str(root))
    for path in sorted(root.rglob('*')):
        mode = path.lstat().st_mode
        name = path.relative_to(root).as_posix()
        if stat.S_ISLNK(mode):
            if not allow_symlinks:
                raise ValueError('symlink in staged input: ' + str(path))
            # gc has tracked documentation/skill and Bazel links, including
            # an external Bazel output path. Hash link text; never follow it.
            records.append([name, 'symlink', os.readlink(path)])
        elif stat.S_ISDIR(mode):
            records.append([name, 'dir'])
        elif stat.S_ISREG(mode):
            with path.open('rb') as stream:
                digest = hashlib.file_digest(stream, 'sha256').hexdigest()
            records.append([name, bool(mode & 0o111), digest])
        else:
            raise ValueError('special file in staged input: ' + str(path))
    return hashlib.sha256(json.dumps(records, separators=(',', ':')).encode()).hexdigest()


def write_manifest(stage, inputs):
    manifest = {'version': 1, 'inputs': {}}
    for name, metadata in sorted(inputs.items()):
        manifest['inputs'][name] = {**metadata, 'sha256': tree_digest(stage / name, name in ('gc', 'gc-base'))}
    (stage / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')


def verify_manifest(stage):
    if stage.is_symlink() or (stage / 'manifest.json').is_symlink():
        raise ValueError('symlinked stage/manifest refused')
    manifest = json.loads((stage / 'manifest.json').read_text())
    if manifest.get('version') != 1 or not isinstance(manifest.get('inputs'), dict) or not manifest['inputs']:
        raise ValueError('invalid staging manifest')
    for name in manifest['inputs']:
        if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9._-]*', name):
            raise ValueError('invalid input name: ' + name)
    if set(p.name for p in stage.iterdir()) != set(manifest['inputs']) | {'manifest.json'}:
        raise ValueError('staging inventory differs from manifest')
    for name, metadata in manifest['inputs'].items():
        if tree_digest(stage / name, name in ('gc', 'gc-base')) != metadata.get('sha256'):
            raise ValueError('staging digest mismatch: ' + name)
    return manifest


def clone_commit(source, destination, commit):
    if not re.fullmatch('[0-9a-f]{40}', commit):
        raise ValueError('full commit required')
    git(source, 'clone', '--local', '--no-hardlinks', '--no-checkout', str(source), str(destination))
    git(destination, 'checkout', '--detach', commit)
    git(destination, 'remote', 'remove', 'origin')
    if git(destination, 'rev-parse', 'HEAD').strip() != commit:
        raise ValueError('clone commit mismatch')


def clean_commit(repo):
    if git(repo, 'status', '--porcelain', '--untracked-files=all'):
        raise ValueError('dirty source repository: ' + str(repo))
    return git(repo, 'rev-parse', 'HEAD').strip()


def check_pin(gc):
    pin = json.loads((gc / 'scripts/beads-fork-pin.json').read_text())
    if (pin.get('module') != policy.MODULE or
            not re.fullmatch('[0-9a-f]{40}', pin.get('commit', '')) or
            pin.get('path') != '../beads-vfug-' + pin['commit'] or
            not re.fullmatch('[0-9a-f]{64}', pin.get('sha256', ''))):
        raise ValueError('invalid fork pin')
    replacements = policy.replacements((gc / 'go.mod').read_text())
    policy.check_replacements(replacements, pin)
    if not replacements:
        raise ValueError('staged qualification requires the reviewed fork pin')
    checkout = (gc / pin['path']).resolve()
    policy.check_identity(clean_commit(checkout), policy.checkout_hash(checkout), pin)
    return pin


def readable_tree(root, writable=False):
    # Preserve gc's tracked symlinks without following/chmodding their targets.
    for path in [root, *root.rglob('*')]:
        mode = path.lstat().st_mode
        if stat.S_ISLNK(mode):
            continue
        path.chmod(stat.S_IMODE(mode) | (0o555 if path.is_dir() else 0o444) |
                   (0o200 if writable else 0))


def stage_inputs(destination, base_commit=None):
    require_operator()
    destination = work_path(destination)
    sources = {name: Path(os.environ[var]).resolve(strict=True) for name, var in {
        'gc': 'VFUG_GC_SOURCE', 'bd-src': 'VFUG_BD_SOURCE', 'go': 'VFUG_GO_SOURCE',
        'modcache': 'VFUG_MODCACHE_SOURCE', 'tools': 'VFUG_TOOLS_SOURCE',
        'lint-bin': 'VFUG_LINT_SOURCE'}.items()}
    commits = {name: clean_commit(sources[name]) for name in ('gc', 'bd-src')}
    if base_commit is not None:
        sources['gc-base'] = Path(os.environ['VFUG_BASE_SOURCE']).resolve(strict=True)
        commits['gc-base'] = git(sources['gc-base'], 'rev-parse', '--verify', '--end-of-options', base_commit + '^{commit}').strip()
    pin = check_pin(sources['gc'])
    if commits['bd-src'] != pin['commit']:
        raise ValueError('bd-src HEAD must equal the reviewed fork commit')
    pinned_name = Path(pin['path']).name
    sources[pinned_name] = (sources['gc'] / pin['path']).resolve()
    commits[pinned_name] = pin['commit']
    destination.mkdir(mode=0o755)  # Never overwrite a previous stage.
    inputs = {}
    for name, source in sources.items():
        print('Staging ' + name, flush=True)
        if name in commits:
            clone_commit(source, destination / name, commits[name])
            inputs[name] = {'commit': commits[name]}
        else:
            # Tool inputs may be symlinks to installed binaries. Materialize
            # their bytes so no staged input depends on the operator home.
            shutil.copytree(source, destination / name, symlinks=False)
            inputs[name] = {}
        readable_tree(destination / name)
    check_pin(destination / 'gc')
    write_manifest(destination, inputs)
    readable_tree(destination)
    verify_manifest(destination)
    print('Stage ready: ' + str(destination), flush=True)
    print('Manifest SHA256: ' + hashlib.sha256((destination / 'manifest.json').read_bytes()).hexdigest(), flush=True)


def run_stage(stage, mode, base_commit=None):
    home = require_test_account()
    refuse_home_workspace(home)
    refuse_ancestor_discovery(RUN_PARENT)
    os.umask(0o022)  # Logs and copied inputs remain readable by the coordinator.
    stage = work_path(stage)
    manifest = verify_manifest(stage)
    required = {'gc', 'bd-src', 'go', 'modcache', 'tools', 'lint-bin'}
    pinned = {name for name in manifest['inputs'] if re.fullmatch('beads-vfug-[0-9a-f]{40}', name)}
    if 'gc-base' in manifest['inputs']:
        required.add('gc-base')
    if mode == 'compare' and 'gc-base' not in required:
        raise ValueError('compare requires a staged gc-base input')
    if len(pinned) != 1 or set(manifest['inputs']) != required | pinned:
        raise ValueError('qualification input inventory incomplete')
    # Hashes catch drift in the copy as well as the source. Never execute from
    # the operator-owned stage after verification.
    root = Path(tempfile.mkdtemp(prefix='r.', dir=RUN_PARENT))
    root.chmod(0o755)
    fixture_root = root / 'f'
    fixture_root.mkdir(mode=0o755)
    validate_roots(root, fixture_root, home)
    inputs = root / 'inputs'
    print('Copying and verifying inputs: ' + str(root), flush=True)
    shutil.copytree(stage, inputs, symlinks=True)
    verify_manifest(inputs)
    readable_tree(inputs, writable=True)
    check_pin(inputs / 'gc')
    # The copied clone is the runner's own checkout: activate its .githooks
    # exactly as `make setup` does, so check-hooks tests the repository, not
    # the staging step (2026-10-16: the fresh clone had no core.hooksPath).
    for name in ('gc', 'gc-base'):
        if name in required:
            subprocess.run(['git', '-C', str(inputs / name), 'config', 'core.hooksPath', '.githooks'], check=True)
    environment = {
        'PATH': '/usr/bin:/bin', 'HOME': str(home),
        'VFUG_ROOT': str(root), 'VFUG_WORK_ROOT': str(fixture_root),
        'VFUG_GC_REPO': str(inputs / 'gc'), 'VFUG_BD_REPO': str(inputs / 'bd-src'),
        'VFUG_GO_ROOT': str(inputs / 'go'), 'VFUG_MODCACHE': str(inputs / 'modcache'),
        'VFUG_TOOLS': str(inputs / 'tools'), 'VFUG_LINT_BIN': str(inputs / 'lint-bin'),
    }
    if 'gc-base' in required:
        environment['VFUG_BASE_REPO'] = str(inputs / 'gc-base')
    command = ['python3', '-B', str(inputs / 'gc/scripts/vfug-host.py'), mode]
    if mode == 'compare':
        command.append(base_commit or '3bb7b3c76')
    print('Qualification logs: ' + str(root / 'logs'), flush=True)
    return subprocess.call(command, cwd=inputs / 'gc', env=environment)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['stage', 'run'])
    parser.add_argument('directory', type=Path)
    parser.add_argument('mode', nargs='?', default='all',
                        choices=['all', 'build', 'bd', 'gc', 'gates', 'acceptance', 'cas', 'safety', 'plan', 'compare'])
    parser.add_argument('base_commit', nargs='?')
    parser.add_argument('--base-gc-commit', help='stage a base checkout from VFUG_BASE_SOURCE')
    args = parser.parse_args()
    try:
        if args.action == 'stage':
            stage_inputs(args.directory, args.base_gc_commit)
            return 0
        return run_stage(args.directory, args.mode, args.base_commit)
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError) as exc:
        parser.exit(2, 'vfug: ' + str(exc) + '\n')


if __name__ == '__main__':
    raise SystemExit(main())

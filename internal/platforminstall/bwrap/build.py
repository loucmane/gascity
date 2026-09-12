#!/usr/bin/python3
"""Offline, unprivileged build of the exact fixed-device setup artifact.

Only a fresh /tmp output root is written. Packages are extracted, never
installed. No maintainer script, namespace launch or install target is run.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tarfile


def sha(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', required=True, type=Path)
    parser.add_argument('--downloads', required=True, type=Path)
    args = parser.parse_args()
    here = Path(__file__).resolve().parent
    root = args.root
    downloads = args.downloads
    if os.geteuid() == 0 or root.exists() or root.parent != Path('/tmp') or root.resolve() != root:
        raise SystemExit('require an absent direct-child /tmp root and an unprivileged caller')
    if downloads.resolve() != downloads or not downloads.is_dir():
        raise SystemExit('downloads must be an existing canonical directory')
    lock = json.loads((here / 'inputs.json').read_text())
    inputs = {str(here / name): sha(here / name) for name in ('build.py', 'inputs.json', 'ro-bind-null.patch', 'readonly-entropy.patch', 'test-setup.c', 'test-setup.py')}
    for row in lock['packages'] + lock['source_archives']:
        name = row['filename']
        if Path(name).name != name:
            raise SystemExit('noncanonical input filename')
        path = downloads / name
        if path.is_symlink() or not path.is_file() or sha(path) != row['sha256']:
            raise SystemExit('input digest/type mismatch: ' + name)
        inputs[str(path)] = row['sha256']
    if sha(Path('/usr/bin/gcc')) != lock['compiler']:
        raise SystemExit('compiler pin differs')
    host = {name:sha(Path(name)) for name in ('/usr/bin/gcc', '/usr/bin/as', '/usr/bin/ld', '/var/lib/dpkg/status', '/usr/bin/bwrap')}
    root.mkdir(mode=0o700)
    sysroot = root / 'sysroot'
    sysroot.mkdir()
    for row in lock['packages']:
        subprocess.run(['/usr/bin/dpkg-deb', '--extract', str(downloads / row['filename']), str(sysroot)], check=True, timeout=30)
    source_parent = root / 'source-candidate'
    source_parent.mkdir()
    with tarfile.open(downloads / lock['source_archives'][0]['filename']) as archive:
        archive.extractall(source_parent, filter='data')
    source = source_parent / 'bubblewrap-0.9.0'
    with tarfile.open(downloads / lock['source_archives'][1]['filename']) as archive:
        archive.extractall(source, filter='data')
    for name in (source / 'debian/patches/series').read_text().splitlines():
        if name and not name.startswith('#'):
            subprocess.run(['/usr/bin/patch', '--batch', '--forward', '-p1', '-i', str(source / 'debian/patches' / name)], cwd=source, check=True, timeout=10)
    subprocess.run(['/usr/bin/patch', '--batch', '--forward', '-p1', '-i', str(here / 'ro-bind-null.patch')], cwd=source, check=True, timeout=10)
    subprocess.run(['/usr/bin/patch', '--batch', '--forward', '-p1', '-i', str(here / 'readonly-entropy.patch')], cwd=source, check=True, timeout=10)
    if sha(source / 'bubblewrap.c') != lock['patched_source_sha256']:
        raise SystemExit('reviewed source mismatch')
    build = root / 'build-candidate'
    library = sysroot / 'usr/lib/x86_64-linux-gnu'
    env = {'PATH': str(sysroot / 'usr/bin') + ':/usr/bin:/bin', 'HOME': str(root),
           'LANG': 'C.UTF-8', 'LC_ALL': 'C.UTF-8', 'PYTHONDONTWRITEBYTECODE': '1',
           'PYTHONPATH': str(sysroot / 'usr/lib/python3/dist-packages'),
           'LD_LIBRARY_PATH': str(library), 'PKG_CONFIG_LIBDIR': str(library / 'pkgconfig'),
           'PKG_CONFIG_SYSROOT_DIR': str(sysroot), 'CC': '/usr/bin/gcc',
           'SOURCE_DATE_EPOCH': '1727194400', 'DEB_BUILD_MAINT_OPTIONS': 'hardening=+pie,+bindnow'}
    for key in ('CFLAGS', 'CPPFLAGS', 'LDFLAGS'):
        env[key] = subprocess.check_output(['/usr/bin/dpkg-buildflags', '--get', key], env=env, cwd=source, text=True).strip()
    env['CFLAGS'] += ' -ffile-prefix-map=' + str(root) + '=/ga-tmgr-build -fdebug-prefix-map=' + str(root) + '=/ga-tmgr-build'
    env['CFLAGS'] += ' -ffile-prefix-map=' + str(build) + '=/ga-tmgr-build/output -fdebug-prefix-map=' + str(build) + '=/ga-tmgr-build/output'
    env['LDFLAGS'] += ' -Wl,--build-id=sha1 -L' + str(library)
    commands = [
        ['/usr/bin/python3', str(sysroot / 'usr/bin/meson'), 'setup', str(build), str(source),
         '--buildtype=plain', '--wrap-mode=nodownload', '-Db_pie=true', '-Dselinux=enabled',
         '-Dman=disabled', '-Dbash_completion=disabled', '-Dzsh_completion=disabled', '-Dtests=true'],
        [str(sysroot / 'usr/bin/ninja'), '-C', str(build), 'bwrap', 'tests/test-utils'],
    ]
    before = {'inputs': inputs, 'host': host, 'commands': commands, 'environment': env}
    with (root / 'before.json').open('x') as stream:
        json.dump(before, stream, indent=2, sort_keys=True)
    with (root / 'build.log').open('x') as log:
        for command in commands:
            subprocess.run(command, cwd=source, env=env, check=True, stdout=log, stderr=subprocess.STDOUT, timeout=120)
    artifact = build / 'bwrap'
    if sha(artifact) != lock['artifact_sha256']:
        raise SystemExit('artifact mismatch: ' + sha(artifact))
    if artifact.stat().st_mode & 0o7777 != 0o755 or 'security.capability' in os.listxattr(artifact):
        raise SystemExit('artifact mode/capability mismatch')
    dynamic = subprocess.check_output(['/usr/bin/readelf', '-d', str(artifact)], text=True)
    headers = subprocess.check_output(['/usr/bin/readelf', '-l', str(artifact)], text=True)
    if '(RPATH)' in dynamic or '(RUNPATH)' in dynamic or 'BIND_NOW' not in dynamic or 'GNU_RELRO' not in headers:
        raise SystemExit('artifact hardening or runtime path mismatch')
    subprocess.run(['/usr/bin/python3', str(here / 'test-setup.py'), '--source', str(source),
                    '--build', str(build), '--sysroot', str(sysroot), '--out', str(root / 'test-fixture')],
                   env=env, check=True, timeout=120)
    if inputs != {path:sha(Path(path)) for path in inputs} or host != {path:sha(Path(path)) for path in host}:
        raise SystemExit('build input/host drift')
    result = {'status': 'build_pass', 'artifact': str(artifact), 'sha256': sha(artifact),
              'inputs_unchanged': True, 'host_unchanged': True, 'namespace_executed': False,
              'system_user_installation': False, 'dynamic': dynamic, 'headers': headers}
    with (root / 'result.json').open('x') as stream:
        json.dump(result, stream, indent=2, sort_keys=True)
    print(json.dumps({key:value for key,value in result.items() if key not in ('dynamic', 'headers')}, sort_keys=True))


if __name__ == '__main__':
    main()

"""Qualification roots live outside the supervisor account's discovery tree."""
import argparse
import os
from pathlib import Path
import stat
import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))

from vfug_discovery import discovery_marker, refuse_ancestor_discovery

RUN_PARENT = Path('/work/vft')


def refuse_home_workspace(home):
    """Acceptance uses passwd HOME, so refuse a project there without deleting it."""
    marker = discovery_marker(home / '.beads')
    if marker is not None:
        raise ValueError('passwd home contains a beads workspace; qualification refused: ' + str(marker))


def validate_roots(root, fixtures, home):
    """Require one privately owned run container, never an account's home tree."""
    home = home.resolve()
    containers = []
    for path in (root, fixtures):
        path = path.resolve()
        if (not path.is_relative_to(RUN_PARENT) or path == RUN_PARENT or
                path.is_relative_to(home) or RUN_PARENT.resolve() != RUN_PARENT):
            raise ValueError('run and fixture roots must resolve under /work/vft/<vfugtest-owned dir>, outside passwd home')
        container = RUN_PARENT / path.relative_to(RUN_PARENT).parts[0]
        containers.append(container)
        for directory in (path, *path.parents):
            if directory == RUN_PARENT:
                break
            info = directory.stat()
            if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid():
                raise ValueError('run and fixture directories must be owned by vfugtest: ' + str(directory))
            if stat.S_IMODE(info.st_mode) != 0o755:
                raise ValueError('run and fixture directories must have mode 0755: ' + str(directory))
    if containers[0] != containers[1]:
        raise ValueError('run and fixture roots must share one vfugtest-owned directory')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check-home', action='store_true')
    parser.add_argument('home', type=Path)
    parser.add_argument('root', type=Path)
    parser.add_argument('fixtures', type=Path)
    args = parser.parse_args()
    try:
        validate_roots(args.root, args.fixtures, args.home)
        if args.check_home:
            refuse_home_workspace(args.home)
        for root in (args.root, args.fixtures):
            refuse_ancestor_discovery(root)
    except (ValueError, OSError) as error:
        print(error, file=sys.stderr)
        return 2
    return 0


if __name__ == '__main__':
    sys.exit(main())

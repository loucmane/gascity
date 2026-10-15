#!/usr/bin/env python3
"""Refuse ambient project discovery while allowing bd's per-user state."""
import os
from pathlib import Path
import stat
import sys


def discovery_marker(marker):
    """Return the exact unsafe path, or None for absent/user-state markers."""
    try:
        mode = marker.lstat().st_mode
    except FileNotFoundError:
        return None
    except OSError:
        return marker  # Fail closed when discovery cannot be inspected.
    if marker.name == '.git' or not stat.S_ISDIR(mode):
        return marker  # Includes dangling symlinks; never traverse them.
    try:
        with os.scandir(marker) as entries:
            for entry in entries:
                name = entry.name
                # Match bd's hasBeadsProjectFiles, including its legacy DB
                # exclusions. Named project symlinks are refused even dangling.
                if name in ('metadata.json', 'config.yaml'):
                    return Path(entry.path)
                if name in ('dolt', 'embeddeddolt') and (entry.is_symlink() or entry.is_dir()):
                    return Path(entry.path)
                if name.endswith('.db') and '.backup' not in name and name != 'vc.db':
                    return Path(entry.path)
    except OSError:
        return marker
    return None


def refuse_ancestor_discovery(root):
    """Reject ambient Git/beads projects before creating any run fixtures."""
    root = root.resolve()
    for parent in (root, *root.parents):
        for name in ('.beads', '.git'):
            refused = discovery_marker(parent / name)
            if refused is not None:
                raise ValueError('ancestor discovery marker refused: ' + str(refused))


if __name__ == '__main__':
    try:
        for argument in sys.argv[1:]:
            refuse_ancestor_discovery(Path(argument))
    except (ValueError, OSError) as error:
        print(error, file=sys.stderr)
        sys.exit(2)

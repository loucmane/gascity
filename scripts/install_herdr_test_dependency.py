"""Fetch a digest-pinned Herdr test executable into a private /tmp directory.

Pins come from https://github.com/herdrdev/herdr/releases/tag/v0.7.1.
This does not install a system/user package, change PATH, or run the download.
Failed attempts remain non-executable for diagnosis; no automatic retry occurs.
"""
import argparse
import hashlib
import os
from pathlib import Path
import platform
import sys
import tempfile
from urllib.request import urlopen

VERSION = "0.7.1"
ASSETS = {
    ("Linux", "x86_64"): (
        "herdr-linux-x86_64", 17342944,
        "b965acaffc2c22f54b6e6c64af7cf8e98a3f4ac2622630a0599c67a4b9d8a654"),
    ("Linux", "aarch64"): (
        "herdr-linux-aarch64", 15803112,
        "3d757ac30c631e79dc45038c3ecc6423fe13a89f9cffa0f415aedd2c27f1576c"),
    ("Darwin", "arm64"): (
        "herdr-macos-aarch64", 14380704,
        "16f4653f0491ea1e7d2b46b5b02542f18e1b82e88daaf9e2900572e5bb634df8"),
    ("Darwin", "x86_64"): (
        "herdr-macos-x86_64", 15323620,
        "5780fa07dbb9a78d79e52d20b86a61013f6cba02667f20b6cf89663015090846"),
}


def asset_for_platform(system, machine):
    """Return only a reviewed upstream asset or fail before downloading."""
    try:
        return ASSETS[system, machine]
    except KeyError:
        raise ValueError(f"unsupported Herdr test platform: {system}/{machine}") from None


def install(asset, *, parent=Path("/tmp"), opener=urlopen):
    """Verify the exact size and digest before making this private file executable."""
    name, expected_size, expected_digest = asset
    root = Path(tempfile.mkdtemp(prefix=f"gascity-herdr-{VERSION}-", dir=parent))
    binary = root / "herdr"
    digest = hashlib.sha256()
    size = 0
    url = f"https://github.com/herdrdev/herdr/releases/download/v{VERSION}/{name}"
    try:
        with opener(url, timeout=30) as response:
            fd = os.open(binary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            with os.fdopen(fd, "wb") as target:
                while chunk := response.read(min(65536, expected_size + 1 - size)):
                    target.write(chunk)
                    digest.update(chunk)
                    size += len(chunk)
                    if size > expected_size:
                        raise ValueError(f"Herdr size mismatch: expected {expected_size}, received more")
        if size != expected_size:
            raise ValueError(f"Herdr size mismatch: expected {expected_size}, received {size}")
        if digest.hexdigest() != expected_digest:
            raise ValueError("Herdr digest mismatch")
        binary.chmod(0o700)
        return root
    except Exception as exc:
        exc.add_note(f"Herdr test dependency attempt preserved at {root}")
        raise


def main():
    argparse.ArgumentParser(description=__doc__).parse_args()
    try:
        print(install(asset_for_platform(platform.system(), platform.machine())))
    except (OSError, ValueError) as exc:
        print(f"Herdr test dependency refused: {exc}", file=sys.stderr)
        for note in getattr(exc, "__notes__", ()):
            print(note, file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

"""Offline tests for the pinned disposable Herdr test dependency."""
import contextlib
import hashlib
import io
from pathlib import Path
import stat
import tempfile
import unittest

try:
    import install_herdr_test_dependency as installer
except ModuleNotFoundError:
    installer = None


class HerdrDependencyTests(unittest.TestCase):
    def setUp(self):
        self.assertIsNotNone(installer, "pinned /tmp-only Herdr test installer is missing")
        self.temp = tempfile.TemporaryDirectory(prefix="gc-herdr-installer-test-", dir="/tmp")
        self.addCleanup(self.temp.cleanup)
        self.parent = Path(self.temp.name)
        self.payload = b"synthetic-test-binary"
        self.asset = ("herdr-test", len(self.payload), hashlib.sha256(self.payload).hexdigest())
        self.requests = []

    def opener(self, payload):
        def open_fixture(url, *, timeout):
            self.requests.append((url, timeout))
            return contextlib.closing(io.BytesIO(payload))
        return open_fixture

    def test_official_linux_pin(self):
        self.assertEqual(installer.VERSION, "0.7.1")
        self.assertEqual(installer.asset_for_platform("Linux", "x86_64"),
                         ("herdr-linux-x86_64", 17342944,
                          "b965acaffc2c22f54b6e6c64af7cf8e98a3f4ac2622630a0599c67a4b9d8a654"))

    def test_valid_download_is_private_and_verified_before_executable(self):
        root = installer.install(self.asset, parent=self.parent, opener=self.opener(self.payload))
        self.assertEqual(root.parent, self.parent)
        self.assertEqual(stat.S_IMODE(root.stat().st_mode), 0o700)
        self.assertEqual((root / "herdr").read_bytes(), self.payload)
        self.assertEqual(stat.S_IMODE((root / "herdr").stat().st_mode), 0o700)
        self.assertEqual(self.requests, [("https://github.com/herdrdev/herdr/releases/download/v0.7.1/herdr-test", 30)])

    def test_digest_failure_preserves_nonexecutable_attempt(self):
        with self.assertRaisesRegex(ValueError, "digest mismatch"):
            installer.install(self.asset, parent=self.parent,
                              opener=self.opener(b"x" * len(self.payload)))
        paths = list(self.parent.glob("*/herdr"))
        self.assertEqual(len(paths), 1)
        self.assertFalse(paths[0].stat().st_mode & 0o111)

    def test_short_and_oversize_downloads_refuse_without_retry(self):
        for payload in (self.payload[:-1], self.payload + b"x"):
            with self.subTest(size=len(payload)):
                before = len(self.requests)
                with self.assertRaisesRegex(ValueError, "size mismatch"):
                    installer.install(self.asset, parent=self.parent, opener=self.opener(payload))
                self.assertEqual(len(self.requests), before + 1)
        for path in self.parent.glob("*/herdr"):
            self.assertFalse(path.stat().st_mode & 0o111)

    def test_unsupported_platform_refuses(self):
        with self.assertRaisesRegex(ValueError, "unsupported"):
            installer.asset_for_platform("Unsupported", "unknown")

    def test_network_failure_is_preserved_without_retry(self):
        calls = []
        def failed(url, *, timeout):
            calls.append(url)
            raise OSError("synthetic network refusal")
        with self.assertRaisesRegex(OSError, "synthetic network refusal"):
            installer.install(self.asset, parent=self.parent, opener=failed)
        self.assertEqual(len(calls), 1)
        self.assertEqual(len(list(self.parent.iterdir())), 1)
        self.assertEqual(list(self.parent.glob("*/herdr")), [])


if __name__ == "__main__":
    unittest.main()

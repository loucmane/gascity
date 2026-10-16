"""Real directory and ownership checks without invoking qualification services."""
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


class LayoutTests(unittest.TestCase):
    def setUp(self):
        import vfug_layout
        self.layout = vfug_layout
        self.temp = tempfile.TemporaryDirectory(prefix='r14.', dir='/work/vft')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.root.chmod(0o755)
        self.fixture = self.root / 'f'
        self.fixture.mkdir()
        self.home = Path('/work/fixture-passwd-home')

    def test_run_and_fixtures_share_owned_container_outside_home(self):
        self.layout.validate_roots(self.root, self.fixture, self.home)

    def test_home_and_foreign_tree_are_refused(self):
        for root, fixture, home in [
            (self.root, self.fixture, self.root),
            (self.home / 'run', self.home / 'f', self.home),
            (self.root, Path('/work/another-account/f'), self.home),
        ]:
            with self.subTest(root=root, fixture=fixture), self.assertRaises(ValueError):
                self.layout.validate_roots(root, fixture, home)
        with patch.object(self.layout.os, 'getuid', return_value=os.getuid() + 1), \
                self.assertRaisesRegex(ValueError, 'owned'):
            self.layout.validate_roots(self.root, self.fixture, self.home)

    def test_symlink_escape_and_writable_container_are_refused(self):
        self.fixture.rmdir()
        self.fixture.symlink_to('/work/another-account')
        with self.assertRaises(ValueError):
            self.layout.validate_roots(self.root, self.fixture, self.home)
        self.fixture.unlink()
        self.fixture.mkdir()
        self.root.chmod(0o777)
        with self.assertRaisesRegex(ValueError, '0755'):
            self.layout.validate_roots(self.root, self.fixture, self.home)

    def test_home_guard_preserves_project_and_allows_user_state(self):
        home = self.root / 'account'
        marker = home / '.beads'
        marker.mkdir(parents=True)
        (marker / 'machine-id').write_text('fixture')
        self.layout.refuse_home_workspace(home)
        (marker / 'embeddeddolt').mkdir()
        with self.assertRaisesRegex(ValueError, 'passwd home contains a beads workspace.*embeddeddolt'):
            self.layout.refuse_home_workspace(home)
        self.assertTrue((marker / 'embeddeddolt').is_dir())


if __name__ == '__main__':
    unittest.main()

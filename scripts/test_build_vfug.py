"""The offline build recipe must stamp and verify the runtime activation identity."""
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


class BuildIdentityTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(dir='/work/ga-vfug', prefix='build-stamp-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.repo = self.root / 'gc'
        self.commit = 'a' * 40
        self.bd_commit = 'b' * 40
        for directory in ('gc/scripts', 'tools', 'home', 'tmp', 'gchome', 'gobin', 'gopath', 'cache', 'modcache', 'beads-vfug-' + self.bd_commit):
            (self.root / directory).mkdir(parents=True)
        (self.root / 'gitconfig').touch()
        self.env = dict(PATH=str(self.root / 'tools') + ':/usr/bin:/bin',
                        VFUG_WORK_ROOT=str(self.root), HOME=str(self.root / 'home'),
                        GC_HOME=str(self.root / 'gchome'), TMPDIR=str(self.root / 'tmp'),
                        GOTMPDIR=str(self.root / 'tmp'), GOBIN=str(self.root / 'gobin'),
                        GOPATH=str(self.root / 'gopath'), GOMODCACHE=str(self.root / 'modcache'),
                        XDG_CACHE_HOME=str(self.root / 'cache'), GIT_CONFIG_NOSYSTEM='1',
                        GIT_CONFIG_GLOBAL=str(self.root / 'gitconfig'))
        source = Path(__file__).with_name('build-vfug.sh')
        shutil.copy2(source, self.repo / 'scripts/build-vfug.sh')
        (self.repo / 'go.mod').write_text('replace github.com/steveyegge/beads => ../beads-vfug-' + self.bd_commit + '\n')
        (self.repo / 'scripts/beads-fork-pin.json').write_text('{}\n')
        (self.repo / 'scripts/check-gomod-replace.sh').write_text('exit 0\n')
        self.tool('git', '#!/usr/bin/python3\nimport sys\nif "rev-parse" in sys.argv: print(' + repr(self.bd_commit) + ' if "beads-vfug-" in " ".join(sys.argv) else ' + repr(self.commit) + ')\n')

    def tool(self, name, text):
        path = self.root / 'tools' / name
        path.write_text(text)
        path.chmod(0o755)

    def build(self, wrong_identity=False):
        self.tool('go', '''#!/usr/bin/python3
import json,pathlib,sys
a=sys.argv[1:]
if a==['version']:
 print('go version go1.26.7 linux/amd64');sys.exit(0)
assert a[0]=='build'
stamp='unknown'
if '-ldflags' in a:
 flags=a[a.index('-ldflags')+1]
 assert flags.startswith('-X main.commit=') and len(flags.split())==2, flags
 stamp=flags.split('=',1)[1]
if WRONG:stamp='unknown'
p=pathlib.Path(a[a.index('-o')+1])
payload=json.dumps({'version':'dev','commit':stamp,'date':'unknown'})
p.write_text('#!/usr/bin/python3\\nprint('+repr(payload)+')\\n')
p.chmod(0o755)
'''.replace('WRONG', repr(wrong_identity)))
        return subprocess.run(['bash', str(self.repo / 'scripts/build-vfug.sh'), str(self.root / 'out')],
                              cwd=self.root, env=self.env, text=True, capture_output=True)

    def test_build_injects_full_head_for_runtime_adoption(self):
        result = self.build()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        version = subprocess.check_output([str(self.root / 'out/gc'), 'version', '--json'],
                                          env=self.env, cwd=self.root, text=True)
        self.assertEqual(json.loads(version), {'version':'dev', 'commit':self.commit, 'date':'unknown'})

    def test_build_refuses_binary_with_missing_stamp(self):
        result = self.build(wrong_identity=True)
        self.assertNotEqual(result.returncode, 0, 'build accepted an unstamped gc binary')
        self.assertIn('runtime identity', result.stdout + result.stderr)


if __name__ == '__main__':
    unittest.main()

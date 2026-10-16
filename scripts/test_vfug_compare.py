"""Failure inventories and the three-run regression rule, without services."""
import json
import os
from pathlib import Path
import tempfile
import unittest


class CompareTests(unittest.TestCase):
    def setUp(self):
        import vfug_compare
        self.compare = vfug_compare
        self.temp = tempfile.TemporaryDirectory(dir=os.environ.get('TMPDIR', '/work/ga-vfug'))
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)

    def log(self, name, content):
        path = self.root / name
        path.write_text(content)
        return path

    def test_complete_shard_inventory_keeps_package_identity_and_evidence(self):
        log = self.log('shard.log', '''--- FAIL: TestSame (0.1s)
    compact_test.go:20: open /tmp/gc-dolt-compact/lock: permission denied
FAIL\tgithub.com/gastownhall/gascity/internal/a\t1s
--- FAIL: TestSame (0.1s)
    test.go:8: wrong result
FAIL\tgithub.com/gastownhall/gascity/internal/b\t1s
--- FAIL: TestReviewedNativeGuardCannotAdmitAdditionalSubprocess (0.1s)
    policy_test.go:9: waiver owned by ga-tmgr expired 2026-10-15
FAIL\tgithub.com/gastownhall/gascity/scripts\t1s
''')
        failures = self.compare.parse_failures(log, 'gc-fast')
        self.assertEqual(len(failures), 3)
        self.assertEqual(failures[0]['package'], 'github.com/gastownhall/gascity/internal/a')
        self.assertEqual(failures[0]['category'], 'environment')
        self.assertIn('permission denied', failures[0]['evidence'][0]['text'])
        self.assertEqual(failures[0]['evidence'][0]['line'], 2)
        self.assertEqual(failures[1]['category'], 'unclassified')
        self.assertEqual(failures[2]['category'], 'dated-policy')
        self.assertIn('2026-10-15', failures[2]['evidence'][0]['text'])

    def test_package_build_and_harness_failures_are_not_silently_dropped(self):
        log = self.log('build.log', '# example/pkg\nfile.go:3: undefined: Missing\nFAIL\texample/pkg [build failed]\nFAIL\n')
        failures = self.compare.parse_failures(log, 'gc-fast')
        self.assertEqual(len(failures), 1)
        self.assertIsNone(failures[0]['test'])
        self.assertEqual(failures[0]['package'], 'example/pkg')
        log = self.log('harness.log', 'ancestor discovery marker refused: /work/.git\n')
        failure = self.compare.parse_failures(log, 'gc-fast', exit_code=2)[0]
        self.assertEqual(failure['category'], 'environment')
        self.assertIsNone(failure['package'])

    def test_three_runs_on_each_side_before_classifying_branch_only(self):
        def failure(test):
            return {'suite': 'gc-fast', 'package': 'example/pkg', 'test': test,
                    'category': 'unclassified', 'evidence': []}
        branch = [failure('TestRegression'), failure('TestFlake'), failure('TestShared')]
        base = [failure('TestShared'), failure('TestBaseOnly')]
        calls = []
        def rerun(side, item, attempt):
            calls.append((side, item['test'], attempt))
            outcome = 'pass'
            if side == 'branch' and (item['test'] == 'TestRegression' or attempt == 1):
                outcome = 'fail'
            return {'outcome': outcome, 'log': f'{side}-{attempt}.log'}
        report = self.compare.classify(branch, base, rerun)
        self.assertEqual(len(calls), 12)
        self.assertEqual([r['test'] for r in report['both']], ['TestShared'])
        self.assertEqual([r['test'] for r in report['base_only']], ['TestBaseOnly'])
        rows = {row['test']: row for row in report['branch_only']}
        self.assertEqual(rows['TestRegression']['classification'], 'regression')
        self.assertEqual(rows['TestFlake']['classification'], 'not-reproduced-consistently')

    def test_skipped_absent_and_broken_base_reruns_cannot_certify_regression(self):
        item = {'suite': 'gc-process', 'package': 'example/pkg', 'test': 'TestA',
                'category': 'unclassified', 'evidence': []}
        for outcome in ('skip', 'absent', 'error'):
            with self.subTest(outcome=outcome):
                def rerun(side, item, attempt):
                    return {'outcome': 'fail' if side == 'branch' else outcome}
                report = self.compare.classify([item], [], rerun)
                self.assertEqual(report['branch_only'][0]['classification'], 'inconclusive')

    def test_rerun_json_requires_the_requested_test_to_finish(self):
        def outcome(events, code):
            log = self.log('rerun.log', '\n'.join(json.dumps(e) for e in events))
            return self.compare.rerun_outcome(log, 'TestA', code)
        self.assertEqual(outcome([{'Action': 'pass', 'Test': 'TestA'}], 0), 'pass')
        self.assertEqual(outcome([{'Action': 'skip', 'Test': 'TestA'}], 0), 'skip')
        self.assertEqual(outcome([{'Action': 'pass'}], 0), 'absent')
        self.assertEqual(outcome([{'Action': 'fail', 'Test': 'TestOther'}], 1), 'error')
        self.assertEqual(outcome([{'Action': 'fail', 'Test': 'TestA'}], 1), 'fail')

    def test_full_comparison_reads_untruncated_shards_and_uses_each_source(self):
        repos = {side: self.root / side for side in ('branch', 'base')}
        pairs = {side: self.root / (side + '-bin') for side in repos}
        calls = []
        def execute(name, repo, args, binaries):
            side = next(side for side in repos if repos[side] == repo)
            self.assertEqual(binaries, pairs[side])
            calls.append((name, args))
            output = self.root / (name + '.log')
            code = 0
            if name.startswith('rerun-'):
                self.assertIn('-count=1', args)
                self.assertIn('^TestBranch$', args)
                code = 1 if side == 'branch' else 0
                output.write_text(json.dumps({'Action': 'fail' if code else 'pass', 'Test': 'TestBranch'}))
            else:
                self.assertIn('scripts/test-local-parallel', args)
                logs = Path(next(arg.split('=', 1)[1] for arg in args if arg.startswith('LOCAL_TEST_LOG_DIR=')))
                content = '--- FAIL: TestPolicy (0.1s)\n    policy.go:1: waiver owned by ga-tmgr expired 2026-10-15\n'
                if side == 'branch' and name.endswith('gc-fast'):
                    content += '--- FAIL: TestBranch (0.1s)\n    branch.go:1: wrong result\n'
                content += 'FAIL\texample/pkg\t0.1s\n'
                (logs / 'unit-core.log').write_text(content)
                output.write_text('[unit-core] failed with exit 1; truncated main output\n')
                code = 1
            return {'name': name, 'exit': code, 'log': str(output)}
        dest = self.root / 'comparison'
        dest.mkdir()
        report = self.compare.run_comparison(dest, repos, pairs, {'base': 'a', 'branch': 'b'}, execute)
        self.assertEqual(len(calls), 12)  # six suites, six fresh candidate reruns
        self.assertEqual(len(report['both']), 3)
        self.assertEqual(report['branch_only'][0]['classification'], 'regression')
        self.assertEqual(report['exclusions'], [])
        self.assertTrue(all(item['base_category'] == 'dated-policy' for item in report['both']))
        self.assertIn('2026-10-15', (dest / 'compare.md').read_text())
        self.assertEqual(json.loads((dest / 'compare.json').read_text())['commits']['base'], 'a')


if __name__ == '__main__':
    unittest.main()

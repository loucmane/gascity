"""Compare complete sharded-suite failures and confirm candidates on both commits."""
import json
from pathlib import Path
import re

SUITES = {'gc-fast': 'fast', 'gc-process': 'cmd-gc-process', 'gc-integration': 'integration'}
POLICY = re.compile(r'waiver owned by \S+ expired \d{4}-\d{2}-\d{2}')
ENVIRONMENT = re.compile(
    r'ancestor discovery marker refused|passwd home contains a beads workspace|'
    r'permission denied|operation not permitted|address already in use|'
    r'cannot assign requested address|network is unreachable|'
    r'executable file not found|Docker (?:not available|unavailable)|'
    r'Cannot connect to the Docker daemon', re.I)


def failure_record(suite, package, test, lines, log):
    evidence = [{'log': str(log), 'line': number, 'text': line}
                for number, line in lines if line.strip()]
    policy = [item for item in evidence if POLICY.search(item['text'])]
    environment = [item for item in evidence if ENVIRONMENT.search(item['text'])]
    return {'suite': suite, 'package': package, 'test': test,
            'category': 'dated-policy' if policy else 'environment' if environment else 'unclassified',
            'evidence': policy or environment or evidence}


def parse_failures(log, suite, exit_code=0):
    """Read full Go text logs, retaining package/build/harness failures too."""
    lines = list(enumerate(Path(log).read_text(errors='replace').splitlines(), 1))
    records, pending, context = [], [], []
    current = None
    for number, line in lines:
        context.append((number, line))
        failed = re.match(r'\s*--- FAIL: (\S+) \(', line)
        package = re.match(r'FAIL\s+(\S+)', line)
        if failed:
            test = failed[1].split('/', 1)[0]
            if current is None or current[0] != test:
                current = (test, [])
                pending.append(current)
        if current is not None:
            current[1].append((number, line))
        if package:
            if pending:
                for test, block in pending:
                    records.append(failure_record(suite, package[1], test, block, log))
            else:
                records.append(failure_record(suite, package[1], None, context, log))
            pending, context, current = [], [], None
        elif re.match(r'(?:ok|\?)\s+\S+', line):
            context = []
    # A panic or killed test binary may never print a package summary.
    for test, block in pending:
        records.append(failure_record(suite, None, test, block, log))
    if exit_code and not records:
        records.append(failure_record(suite, None, None, lines, log))
    return merge_failures(records)


def key(item):
    return item['suite'], item['package'] or '', item['test'] or ''


def merge_failures(items):
    merged = {}
    for item in items:
        identity = key(item)
        if identity not in merged:
            merged[identity] = {**item, 'evidence': list(item['evidence'])}
        else:
            merged[identity]['evidence'].extend(item['evidence'])
            if item['category'] != 'unclassified':
                merged[identity]['category'] = item['category']
    return [merged[identity] for identity in sorted(merged)]


def rerun_outcome(log, test, exit_code):
    outcomes = []
    for line in Path(log).read_text(errors='replace').splitlines():
        try:
            event = json.loads(line)
        except ValueError:
            continue
        if isinstance(event, dict) and event.get('Test') == test and event.get('Action') in ('pass', 'fail', 'skip'):
            outcomes.append(event['Action'])
    if 'fail' in outcomes:
        return 'fail'
    if exit_code:
        return 'error'
    return outcomes[-1] if outcomes else 'absent'


def classify(branch, base, rerun):
    """A regression needs three named failures vs three named, unskipped passes."""
    sides = [{key(item): item for item in merge_failures(items)} for items in (branch, base)]
    left, right = sides
    report = {'branch_only': [], 'base_only': [], 'both': []}
    for identity in sorted(left.keys() | right.keys()):
        if identity in left and identity in right:
            report['both'].append({**left[identity], 'base_evidence': right[identity]['evidence'],
                                   'base_category': right[identity]['category'], 'classification': 'shared-failure'})
        elif identity in right:
            report['base_only'].append({**right[identity], 'classification': 'base-only-failure'})
        else:
            item = dict(left[identity])
            repeats = {'branch': [], 'base': []}
            if item['test'] and item['package']:
                for attempt in range(1, 4):
                    for side in repeats:
                        repeats[side].append(rerun(side, item, attempt))
            outcomes = {side: [row['outcome'] for row in rows] for side, rows in repeats.items()}
            if outcomes['branch'] == ['fail'] * 3 and outcomes['base'] == ['pass'] * 3:
                classification = 'regression'
            elif not all(len(rows) == 3 and all(value in ('pass', 'fail') for value in rows)
                         for rows in outcomes.values()):
                classification = 'inconclusive'
            elif outcomes['branch'] == ['fail'] * 3 and outcomes['base'] == ['fail'] * 3:
                classification = 'shared-on-rerun'
            else:
                classification = 'not-reproduced-consistently'
            item.update(classification=classification, reruns=repeats)
            report['branch_only'].append(item)
    return report


def rerun_args(item):
    args = ['env', 'GC_FAST_UNIT=' + ('1' if item['suite'] == 'gc-fast' or
            (item['suite'] == 'gc-integration' and item['package'].endswith('/cmd/gc')) else '0'),
            'go', 'test', '-json', '-count=1', '-timeout=30m']
    if item['suite'] == 'gc-integration':
        args.append('-tags=integration')
    args += [item['package'], '-run', '^' + re.escape(item['test']) + '$']
    return args


def write_report(directory, report):
    (directory / 'compare.json').write_text(json.dumps(report, indent=2) + '\n')
    table = [f"Branch: `{report['commits']['branch']}`; base: `{report['commits']['base']}`.", '']
    if report.get('blocked'):
        table += ['Comparison blocked: ' + report['blocked'], '']
        print('Comparison blocked: ' + report['blocked'], flush=True)
    table += ['| Set | Suite | Package | Test | Classification | Evidence |',
             '| --- | --- | --- | --- | --- | --- |']
    for group in ('branch_only', 'base_only', 'both'):
        for item in report[group]:
            evidence = item['evidence'] + item.get('base_evidence', [])
            citations = '<br>'.join(f"{row['log']}:{row['line']}: {row['text']}" for row in evidence)
            values = [group, item['suite'], item['package'] or '(harness)', item['test'] or '(package/setup)',
                      item['classification'] + ' / ' + item['category'], citations]
            table.append('| ' + ' | '.join(value.replace('|', '\\|').replace('\n', ' ') for value in values) + ' |')
    (directory / 'compare.md').write_text('\n'.join(table) + '\n')
    print('\nSET          SUITE            TEST / PACKAGE                                  CLASSIFICATION', flush=True)
    for group in ('branch_only', 'base_only', 'both'):
        for row in report[group]:
            print(f"{group:12} {row['suite']:16} {row['test'] or row['package'] or '(harness)':48} {row['classification']} ({row['category']})", flush=True)
            for evidence in row['evidence'] + row.get('base_evidence', []):
                if row['category'] != 'unclassified':
                    print(f"  {evidence['log']}:{evidence['line']}: {evidence['text']}", flush=True)
    print('Comparison: ' + str(directory / 'compare.json'), flush=True)


def run_comparison(directory, repos, pairs, commits, execute):
    """Run both complete suite sets sequentially, then rerun each candidate 3x."""
    inventories = {'branch': [], 'base': []}
    suite_runs = []
    for side, repo in repos.items():
        for suite, mode in SUITES.items():
            logs = directory / side / suite
            logs.mkdir(parents=True)
            # Same shared sharded runner as the Makefile targets. Invoke it
            # directly so test-fast's env -i cannot drop LOCAL_TEST_LOG_DIR.
            args = ['env', 'LOCAL_TEST_JOBS=2', 'GO_TEST_COUNT=1',
                    'LOCAL_TEST_LOG_DIR=' + str(logs), 'bash', 'scripts/test-local-parallel', mode]
            result = execute(side + '-' + suite, repo, args, pairs[side])
            suite_runs.append({**result, 'side': side, 'suite': suite, 'shards': str(logs)})
            failures = []
            for log in sorted(logs.glob('*.log')):
                failures += parse_failures(log, suite)
            # Account for failed shell jobs even if no Go test was reached.
            output = Path(result['log']).read_text(errors='replace')
            for label in re.findall(r'^\[([^\]]+)\] failed with exit \d+;', output, re.M):
                log = logs / (re.sub(r'[^A-Za-z0-9._-]', '_', label) + '.log')
                if log.is_file():
                    failures += parse_failures(log, suite, exit_code=1)
            if result['exit'] and not failures:
                failures = parse_failures(Path(result['log']), suite, exit_code=result['exit'])
            inventories[side] += merge_failures(failures)
    serial = 0

    def rerun(side, item, attempt):
        nonlocal serial
        serial += 1
        result = execute(f'rerun-{serial:04}-{side}-{attempt}', repos[side], rerun_args(item), pairs[side])
        return {**result, 'attempt': attempt,
                'outcome': rerun_outcome(result['log'], item['test'], result['exit'])}

    report = {**classify(inventories['branch'], inventories['base'], rerun),
              'commits': commits, 'suites': suite_runs,
              'regression_rule': 'branch fails named test 3/3; base passes named test without skipping 3/3',
              'exclusions': []}
    write_report(directory, report)
    return report

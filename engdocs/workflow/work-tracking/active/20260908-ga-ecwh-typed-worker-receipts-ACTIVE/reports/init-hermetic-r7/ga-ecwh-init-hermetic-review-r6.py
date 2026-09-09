"""One bounded independent source review of two offline init-test fixtures."""
import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import sys

ROOT = Path('/home/loucmane/gascity-core-worktrees/ga-ecwh-typed-worker-receipts')
REPORTS = ROOT / 'engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE/reports'
DRIVER = REPORTS / 'remaining-conformance-source-review-r1.py'
if hashlib.sha256(DRIVER.read_bytes()).hexdigest() != 'a688c2fa1d71e6ec23c0ccb65223d5412a44d1e8b2948fd1396af49ed1de95d5':
    raise RuntimeError('review engine drift')
spec = importlib.util.spec_from_file_location('init_review_engine', DRIVER)
engine = importlib.util.module_from_spec(spec)
spec.loader.exec_module(engine)
engine.BASE = '38a45465858eb186309997f9ebeaee8ae6559477'
EVIDENCE = Path('/tmp/ga-ecwh-init-hermetic-20260909')
engine.OUT = EVIDENCE / 'fable-review-r6'
CHANGED = 'cmd/gc/init_provider_readiness_test.go'
CONTEXT = ['cmd/gc/init_provider_readiness.go', 'cmd/gc/cmd_init.go',
           'cmd/gc/cmd_init_gascity_test.go', 'cmd/gc/legacy_pack_preflight.go']


def verify_parent():
    if engine.git('rev-parse', 'HEAD').strip() != engine.BASE:
        raise RuntimeError('signed parent drift')
    subprocess.run(['git', 'verify-commit', engine.BASE], cwd=ROOT, check=True, capture_output=True)
    for name in engine.git('diff', '--name-only', engine.BASE).splitlines():
        if name != CHANGED and not name.startswith('engdocs/workflow/'):
            raise RuntimeError('unexpected delta: ' + name)
    for name in CONTEXT:
        before = subprocess.run(['git', 'show', engine.BASE + ':' + name], cwd=ROOT,
                                capture_output=True, check=True).stdout
        if before != (ROOT / name).read_bytes():
            raise RuntimeError('retained owner changed: ' + name)
    return len(CONTEXT)


engine.verify_previous = verify_parent


def prepare():
    if engine.OUT.exists():
        raise RuntimeError('preserve existing review; no replay')
    verify_parent()
    pins = {name: engine.read_pin(ROOT / name)[1] for name in [CHANGED] + CONTEXT}
    evidence_pins = {}
    prompt = '''Independent SOURCE-only review of ga-ecwh.2's delivery-test isolation repair.
Tool-free/read-only review. Return concise SOURCE PASS/HOLD. Supplied test logs are
executor evidence, not an independent rerun. Review semantic parity, speed/resource
policy, and accuracy/enforceability together. No worker, signing or live acceptance.

Signed parent 38a45465858eb186309997f9ebeaee8ae6559477 passed eight normal pre-push
jobs. CLI shards 4 and 6 failed only in two initialization tests: the default gascity
template selects a real remote roles pack. One clone failed DNS after 10.16s; the
other failed partial transfer after 614.17s. Both failed logs and commits remain.
No hook bypass, test skip, network retry into green, or runtime change is proposed.

Exactly two existing test owners are changed. They assert (1) no implicit-import
state is created by real finalization, and (2) --no-start avoids supervisor
registration while producing the expected guidance. They now select the existing
minimal template explicitly and forbid non-file Git protocols. Original assertions,
real doInit/finalizeInit/run calls, and the existing supervisor collaborator remain.
The real file:// import/provider-readiness owner and default-gascity-template owners
are byte-unchanged and rerun. No broader test-hermeticity claim is made.

Check whether using minimal retains each original observable promise, whether the
guard provides an honest deterministic RED, whether real remote-import and default
template behavior retain suitable owners, and whether any coverage was silently
lost. Production code, audit ledger, dates, grants and signing are unchanged.
Normal full pre-push and hosted CI still must pass on the next signed candidate.
'''
    prompt += '\nEXACT DIFF\n' + engine.git('diff', engine.BASE, '--', CHANGED)
    text = (ROOT / CHANGED).read_text()
    for name in ['TestFinalizeInitDoesNotWriteImplicitImportState',
                 'TestCmdInitNoStartSkipsSupervisorRegistration',
                 'TestFinalizeInitChecksRemoteImportProvidersAfterInstall']:
        body = text.split('func ' + name + '(', 1)[1].split('\nfunc ', 1)[0]
        prompt += '\nOWNER\nfunc ' + name + '(' + body
    for name in ['cmd/gc/init_provider_readiness.go', 'cmd/gc/legacy_pack_preflight.go',
                 'cmd/gc/cmd_init_gascity_test.go']:
        raw = (ROOT / name).read_text()
        if name.endswith('init_provider_readiness.go'):
            raw = raw.split('func maybePrintWizardProviderGuidance', 1)[0]
        prompt += '\nUNCHANGED CONTEXT ' + name + '\n' + raw
    for name in ['red.log', 'repeat-green.log', 'owners-green.log', 'resource-green.log', 'diff-check.log']:
        path = EVIDENCE / name
        raw, evidence_pins[str(path)] = engine.read_pin(path)
        if name.endswith('green.log') and ('FAIL' in raw.decode() or 'ok ' not in raw.decode()):
            raise RuntimeError('missing green evidence: ' + name)
        prompt += '\nEXECUTOR EVIDENCE ' + name + '\n' + raw.decode()
    if len(prompt.encode()) > 50000:
        raise RuntimeError('review input exceeds bounded scope')
    engine.OUT.mkdir(mode=0o700)
    engine.write_new('source-input.txt', prompt)
    engine.write_new('review-begin.json', json.dumps({
        'base': engine.BASE, 'source_pins': pins, 'evidence_pins': evidence_pins,
        'prompt_sha256': engine.sha(prompt.encode()), 'prompt_bytes': len(prompt.encode()),
        'model': engine.MODEL, 'tools': [], 'scope': 'two local init-test owners only',
    }, indent=2) + '\n')
    print(json.dumps({'prepared': True, 'prompt_bytes': len(prompt.encode())}))


if __name__ == '__main__':
    if sys.flags.optimize or len(sys.argv) != 2:
        raise RuntimeError('invalid invocation')
    if sys.argv[1] == '--prepare':
        prepare()
    elif sys.argv[1] == '--review-once':
        engine.review()
    else:
        raise RuntimeError('unknown operation')

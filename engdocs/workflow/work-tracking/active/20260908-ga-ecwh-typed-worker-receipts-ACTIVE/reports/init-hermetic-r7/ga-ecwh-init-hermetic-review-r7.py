"""Resolve the R6 coverage question against exact source and final test events."""
import hashlib
import importlib.util
import json
from pathlib import Path
import sys

DRIVER = Path('/tmp/ga-ecwh-init-hermetic-review-r6.py')
if hashlib.sha256(DRIVER.read_bytes()).hexdigest() != '2f26026d51426155d0ed13852417867f3381fa40ddd75a2eb7e3eec95a74e5d3':
    raise RuntimeError('R6 driver drift')
spec = importlib.util.spec_from_file_location('init_r6', DRIVER)
r6 = importlib.util.module_from_spec(spec)
spec.loader.exec_module(r6)
engine = r6.engine
engine.OUT = r6.EVIDENCE / 'fable-review-r7'


def function(path, name):
    return 'func ' + name + '(' + path.read_text().split('func ' + name + '(', 1)[1].split('\nfunc ', 1)[0]


def prepare():
    if engine.OUT.exists():
        raise RuntimeError('review already exists; preserve it')
    r6.verify_parent()
    names = [r6.CHANGED] + r6.CONTEXT + ['cmd/gc/embed_builtin_packs.go',
        'cmd/gc/cmd_import.go', 'internal/builtinpacks/registry.go']
    pins = {name: engine.read_pin(r6.ROOT / name)[1] for name in names}
    evidence_pins = {}
    prior_path = r6.EVIDENCE / 'fable-review-r6/review-result.json'
    raw, evidence_pins[str(prior_path)] = engine.read_pin(prior_path)
    prior = json.loads(raw)['result']
    prompt = '''Independent SOURCE-only follow-up: resolve the single narrow R6 HOLD.
Do not rerun tests or invent observations: you have no tools. Prior SOURCE HOLD is
preserved below. All other R6 conclusions remain applicable unless this delta changes them.

Actual source answers the question: doInit always calls addBuiltinImportsToInitPack;
builtinImportsForInit always includes core, with a canonical HTTPS source plus pinned
bundled version. isRemoteImportSource recognizes HTTPS. installInitRemoteImports
therefore reaches syncImports and installLockedImports; required bundled sources
are already hydrated from embedded assets. Minimal removes the extra unbundled
roles import, not the real import-install path. Production files are unchanged.

The candidate now ALSO asserts initHasRemoteImports(cityPath) is true before real
finalization, so the absence assertion cannot silently become vacuous if a future
minimal template drops those imports. This is the only additional source delta.
All 20 repeated test executions and retained owner tests must pass with no skips.

Review the supplied chain and explicit fixture assertion. Return one concise
SOURCE PASS/HOLD with any remaining material coverage issue. This is no approval
for runtime activation, worker execution, signing, merge or complete goal acceptance.
'''
    prompt += '\nPRESERVED R6 VERDICT\n' + prior
    prompt += '\nFULL CURRENT CANDIDATE DIFF\n' + engine.git('diff', engine.BASE, '--', r6.CHANGED)
    extracts = [('cmd/gc/init_provider_readiness_test.go', 'TestFinalizeInitDoesNotWriteImplicitImportState'),
                ('cmd/gc/init_provider_readiness.go', 'initHasRemoteImports'),
                ('cmd/gc/legacy_pack_preflight.go', 'installInitRemoteImports'),
                ('cmd/gc/legacy_pack_preflight.go', 'hasRemoteImport'),
                ('cmd/gc/cmd_import.go', 'isRemoteImportSource'),
                ('cmd/gc/cmd_init.go', 'addBuiltinImportsToInitPack'),
                ('cmd/gc/embed_builtin_packs.go', 'builtinImportsForInit'),
                ('cmd/gc/embed_builtin_packs.go', 'builtinImportsForNames')]
    for name, symbol in extracts:
        prompt += '\nSOURCE ' + name + '\n' + function(r6.ROOT / name, symbol)
    registry = (r6.ROOT / 'internal/builtinpacks/registry.go').read_text()
    prompt += '\nBUNDLED SOURCE REGISTRY (identity/URL functions only)\n' + registry.split('// SourceLayout reports', 1)[0]
    source = (r6.ROOT / 'cmd/gc/cmd_init.go').read_text().splitlines()
    prompt += '\ndoInit call-site context cmd_init.go:1407-1430\n' + '\n'.join(source[1406:1430])
    for name in ['coverage-repeat-green.jsonl', 'coverage-owners-green.jsonl']:
        path = r6.EVIDENCE / name
        raw, evidence_pins[str(path)] = engine.read_pin(path)
        events = [json.loads(line) for line in raw.splitlines()]
        if any(e.get('Action') in {'skip', 'fail'} for e in events):
            raise RuntimeError('non-green test event')
        final = [e for e in events if e.get('Action') == 'pass']
        if not any('Test' not in e for e in final):
            raise RuntimeError('missing package PASS')
        prompt += '\nEXECUTOR FINAL PASS EVENTS ' + name + '\n' + json.dumps(final)
    if len(prompt.encode()) > 35000:
        raise RuntimeError('review too large')
    engine.OUT.mkdir(mode=0o700)
    engine.write_new('source-input.txt', prompt)
    engine.write_new('review-begin.json', json.dumps({
        'base': engine.BASE, 'source_pins': pins, 'evidence_pins': evidence_pins,
        'prompt_sha256': engine.sha(prompt.encode()), 'prompt_bytes': len(prompt.encode()),
        'model': engine.MODEL, 'tools': [], 'scope': 'R6 one coverage question and five-line assertion',
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

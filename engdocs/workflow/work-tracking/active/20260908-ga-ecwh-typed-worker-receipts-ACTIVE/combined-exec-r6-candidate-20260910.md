# ga-tmgr combined exec R6 source candidate — 2026-09-10

## Disposition
R5 consumed: actual host execution exited1 at exec-before; INCONCLUSIVE, never replay. Seven preceding cases completed their observations; remaining cases are not PASS. R5 source approval remains source-only. Independent diagnosis establishes unsafe inherited listener ownership, not the exact winning close race. Parent verified all15 retained PIDs absent; that evidence is reused without another probe.

R6 is source-ready only, awaiting independent exact-source review. No R6 namespace/process execution or validator ran. R4 and R6 runroots remain absent. No production integration, live changes, goal change or acceptance closure; ga-oz9e deferred.

## Narrow correction
- Original inherited listener fd3 remains explicitly owned from supervisor startup through syscall.Exec or error cleanup. HTTP uses its duplicate. No Dup3, replacement slot or unknown-FD close.
- Exact fd3 inheritance flags are checked, FD_CLOEXEC alone cleared, and read back; explicit Go KeepAlive anchors ownership.
- Existing pidfd terminal/error evidence refuses exec-generation waiting early; terminal exit is never successful exec.
- All22 cases, environment, namespace/FD/frame/deadline bounds, identity classifier, cache and irreversible commit checks remain unchanged.

## Focused evidence
Pure injected lifetime/order/inheritance/error-cleanup/no-overwrite and terminal-refusal regressions pass. RED was missing-seam compile failure, not reproduction of the host race.
Final go test -count=1 ./test/combinedprobe PASS; go vet ./test/combinedprobe PASS; static helper/impostor builds PASS.
Logs: /tmp/ga-tmgr-combined-candidate-r6-20260910/{red.txt,focused-final.txt,vet-final.txt}.
Only read-only freeze inspected pins/ELF and absent runroot; no synthetic execution, old-decoder rerun, broad suite or inference.

## Frozen packet
Directory: /tmp/ga-tmgr-combined-candidate-r6-20260910
- source.tar: 722143b3d493262ba4c4c2313c0341ed75bd669ef444fc70f184c4f176109f67
- source.sha256: 639d16cf473c178499ed85d431f72e78747961827dcf1bd24b764dcc58dbf6dc
- changed-files.sha256: bac4c63356f811e336c5c93bd9457813eb6bd65861890fa33f2894432f0c29b2
- r5-to-r6.diff (including new files): 46cbbde1b774038d2e02d3e3349aea96581af84508dc9d51ee59b5a92f9bdbb3
- execution-plan.json: 5fa9e19c5f6af39b783c3b07fbe535e2f420106af0f34184bc60d3ba3cb4c3b6
- combinedprobe: 4aacd07c9d1a812a66047326054a57288488404a490e3556721a8828b2fb9362
- impostor: 980c98ee39e6eaee723755886f10057ab5f6faf8eb84e6561505ad2e298bce4c

Proposed command, NOT executed; independent source review required first:
```
/tmp/ga-tmgr-combined-candidate-r6-20260910/combinedprobe execute-reviewed /tmp/ga-tmgr-combined-candidate-r6-20260910/execution-plan.json 5fa9e19c5f6af39b783c3b07fbe535e2f420106af0f34184bc60d3ba3cb4c3b6
```
Fresh runroot: /tmp/ga-tmgr-combined-run-r6-20260910. Existing frozen resource/timeout/write/process plan remains authoritative.

## Preservation and source-reference correction
Original20 manifest 537bdba61fb18c4fb44c5318910d619ac8e8ddedc7669593669e5d1d157630cc and staged index remain exact. R5 frozen manifest, source snapshot and existing decoder complete-input evidence rechecked unchanged; decoder archive reused, not copied/rebuilt. M5 and every prior run/review preserved.
R5 observation /tmp/ga-tmgr-combined-runtime-r5-observation-20260910.md: a2aeebf4338ecd2339359ea261979133c0b30dbb80dd5e7fd17716c12a0876d0.
Diagnosis /tmp/ga-tmgr-combined-r5-runtime-astra-diagnosis-20260910.md: dd7f0f7798285ebf7dc907ea38292e08680ba50b54b936763ddc4477ff483364.
CLI output-last-message overwrote only duplicate /tmp R4/R5 reports with short finals; both are preserved, not repaired.
Authoritative detailed Core ACTIVE reports survive:
- combined-acceptance-r4-candidate-20260910.md: a0637591b1db88ab725d61759a7d7d88344505f0773cf4b2d73dd3c0057e276d
- combined-identity-r5-candidate-20260910.md: 9879e1428f98b5ffbb37b9e2c64c7f919d6817fea6a521117d7cfe77a2d4e66f
Their directory is engdocs/workflow/work-tracking/active/20260908-ga-ecwh-typed-worker-receipts-ACTIVE in the existing Core worktree. retained-references.sha256 binds these and short finals; this is a reference correction only.

## Next
Independent R5-to-R6 exact-source review, including owner lifetime/error cleanup and terminal attribution, before any new synthetic execution. Source/offline checks do not establish runtime race resolution or full-matrix PASS. Original Core repair -> gct-13ku/gct-10pg -> useful Claude task -> both handovers remains incomplete.

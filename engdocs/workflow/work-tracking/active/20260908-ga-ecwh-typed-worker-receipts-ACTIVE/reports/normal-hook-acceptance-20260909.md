# Normal hook and final source binding — 2026-09-09

The normal pre-commit hook passed formatting, affected lint (zero issues),
OpenAPI/client/config/CLI generation, full vet and documentation sync tests.
Its preserved output SHA-256 is
adb4c58819e94fd3d09a6ba50ef67d604b19db04da7504a76d11c0ee075e2dff.

The hook formatted seven integration-tagged test files not visited by the
earlier untagged lint selection. Their original bytes were recovered exactly
from hash-verified original review inputs. Pinned golangci-lint 2.12.0 output
was verified byte-identical to each new file. The only generated artifact
delta was two CLI documentation rows for the already-reviewed profile flags.
All other reviewed source pins and generated artifacts remained unchanged.

Independent, tool-free Fable R4 reviewed only this 11,188-byte delta and
returned SOURCE PASS. Result SHA-256:
e5787d169bd7c8a28ac3c4a20c6485dcfeccfd4bbf748f2feb95d95a23724752.
The post-review verifier passed with source_unchanged=true. R1-R3 and all
original failed attempts remain preserved. The reviewer did not execute tests;
normal push-hook and hosted CI remain required on the signed candidate.

The original delivery-verification checkpoint remains byte-exact in the
repository's ignored reports archive. Its newly staged index entry was removed
after diff-check found its trailing blank line, rather than rewriting the
hash-bound original. The compact final summaries are selected for publication.

A combined verification/signing shell command was refused before execution
because sensitive Git commands cannot be composed with Python -c under the
existing hard-policy grammar. Verification and the literal Git commit command
are separate operations; no gate, native permission or signing check is waived.

No commit, push, merge, activation, worker dispatch or rig transition is claimed
by this record. Actual Claude signing and both handover directions remain open.

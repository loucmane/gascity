# Independent Astra runtime classification and build-constructibility review

Reviewer: existing read-only Astra reviewer `/root/astra_namespace_review`.
No review execution or source changes. This records its consolidated delivered
verdict after reading the actual bounded runtime evidence and relevant source.

## Runtime classification

Kernel component PASS and ordinary startup/teardown PASS are supported.
Neither proves V/W adoption. The preserved namespace snapshot contains uts
instead of the metadata protocol's required time namespace
(`metadata_protocol.go:190-198`). Do not fill it retroactively; use the exact
required set in a future fresh flow. No runtime retry is authorized here.

## Confirmed build-constructibility HOLD

- `cmd_version.go:16-28,42-61` defaults to dev/unknown. Commit fallback requires
  embedded vcs.revision; there is no environment/runtime override.
- Activation is mandatory (`metadata_protocol.go:182`).
- Expected commit must be 40 lowercase hexadecimal characters
  (`manifest.go:322`; `integrity.go:202-205`).
- The actual API commit must equal that expectation (`lifecycle.go:193-203`).
- The current exact seven-setting custody profile rejects both VCS stamping and
  added linker settings (`metadata_custody_linux.go:15-27`).

Therefore the unchanged b332c2e3... image cannot meet the genuine full positive
flow. Neither accepting "unknown" nor inserting an invented manifest hash is valid.

## One recommended correction — not yet implemented or approved

Narrowly extend the custody build contract to permit one exact reviewed,
metadata-only linker stamp for main.commit, bound to the genuine reviewed source
revision. Retain rejection of arbitrary linker flags, replacements and
instrumentation. Rebuild and freeze the same image for supervisor, V and W, with
complete source/build hashes and focused profile tests. The nonempty dev version
does not itself need changing. Source changes still require the applicable
bounded authority and independent review before any further runtime execution.

Do not weaken commit validation or fabricate provenance. Preserve all completed
evidence and both consumed runtime roots. No receipt/adoption/provider-parity claim.

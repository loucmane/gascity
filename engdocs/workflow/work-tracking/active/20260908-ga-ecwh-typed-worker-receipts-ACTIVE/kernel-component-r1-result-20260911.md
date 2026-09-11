# Kernel component run R1 — runtime PASS

The approved exact command ran once through the unprivileged host execution
surface. Tool result `7fb40e` returned exit 0 with stdout `PASS` in 0.172 seconds.
The binary SHA-256 was
`a81b39d8db4e869f3bbaca6176effa1ca5e46b8756e37c2f21e064189e3d0ae6`.

Run root: `/tmp/ga-tmgr-kernel-component-run-r1-20260911`.
Owned PIDs: target 2189761, other listener holder 2189766, isolated-network
holder 2189796. A separate host read-only check (`4cb14e`, exit 0) confirmed
all three `/proc/<pid>` paths absent after the test's own Wait/pidfd checks.

- owned-pids.jsonl: `cf450aeb7afa3cb5a49da8def745acffff7ca6c98057338e1c667ed9fba2ad80`
- component-observations.json: `21a9e0979448cd98b7122a576bbf3e6289d77533ebc5eb67a1e7c4d28934f3ab`
- Listener: `127.0.0.1:43865`, inode `139174591`.
- Accepted target socket inode: `139200799`; client inode `139174599`, cookie `12289`.
- Host network namespace: `net:[4026531840]`; isolated namespace: `net:[4026532918]`.

This is the reviewed real-kernel component proof only. It is not same-image
supervisor/V/W adoption acceptance, a metadata receipt, delivery or provider parity.
The post-exit refusal also had a closed socket; it is not an independently isolated
terminal-process-only case. No prior run was replayed and no source was changed.
The conditional approval now permits ordinary fixture startup and capture, not
metadata adoption. All evidence is preserved.

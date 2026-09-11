# Approved container dependency correction — source checkpoint

Operator approved the separately proposed gRPC 1.83.2 / Thrift 0.24.0 correction,
including necessary matching Core inputs, sums and transitives, retaining every
review, signature and CI gate. Authorization recorded on ga-tmgr through the
supported coordination surface at 2026-09-11T15:53:47Z, request SHA256
058c4fac41a4a5ad52bbed43292086aa85168aab95a265790e5969be03c07c71.
The first recording attempt timed out before process creation; one permitted
pre-mutation retry succeeded. No ambiguous write was repeated.

Source starts from signed 24a093ff5ab2c32bae2baf3cf8886a3f723c3e7f.
Only five source files change: both Dockerfiles, owning security expectations,
go.mod and go.sum. Tool versions/source refs/checksums, artifact assertions,
Go 1.26.7 builder identity, scanner and ignore policy remain unchanged.

Go tidy resolves required transitive requirements of gRPC; no broad latest
update was run. Go-shlex and x/crypto move from indirect to direct at identical
versions due existing test imports. Proxy: https://proxy.golang.org; checksum
database: sum.golang.org. Module verification passed and tidy -diff is empty.
Upstream versions: https://github.com/grpc/grpc-go/releases/tag/v1.83.2 and
https://thrift.apache.org/download.

## Evidence

Directory: /tmp/ga-tmgr-container-dependencies-20260911

- candidate-sha256.txt:
  277e1bf53631e1285df41df7589b312a5751f3295e550e65104eb6874528b905.
- red.log:
  f721dc72d62a3cb70ea1c89e85aa2b58c334409e4eb3194d7915d67b23223b0b.
  Three compiled failures identify old base/agent/Core gRPC and Dolt Thrift.
- module-resolution.log:
  8091cfe6ae001b52c275875f29300a8dd0c12655e763783ce263c4e58eded647.
- focused-green.log:
  e97738205c5141f56a0b0739a8eb09e7626285d6d6c5abc1ec23752efd2d4cf4.
  Owning scripts 35.760s and platforminstall 4.253s PASS.
- Non-installed candidate gc SHA256:
  bbc49c86aef1cddab8dee61eecbb8b8c212b9e851ef54e81c18be0ba8d03e7f5.
  Go 1.26.7, CGO0, trimpath, buildvcs=false; gRPC 1.83.2 embedded. Thrift is
  selected by the graph but not linked by this gc build; Dolt's unchanged
  artifact assertion still requires the corrected Thrift version.

Independent Astra SOURCE PASS found no must-fix. All five dependency pins and
the prior eleven protected-sibling pins were reverified. Review confirmed the
transitive chain and preservation of source/build/security contracts.
Consolidated verdict is preserved as review-verdict.md in that directory.

## Remaining gates

Normal hooks, full pre-push tests, signed exact head/base and hosted CI remain
required. PR37's failed Container Scan 34611042345 is preserved, not suppressed.
No scan PASS, merge or live acceptance is claimed by this source checkpoint.

The new graph invalidates any transfer of old binary provenance. Historical R7
remains bound to its exact consumed image. Fresh protected assets/backups writer
and descendant denial, exact metadata preservation, and separately authorized
live adoption remain owed. No installed artifact, rig, worker, service,
HPFetcher/Blog content, credential or previous evidence changed.
The full provider-independent execution and both handover outcomes remain open.

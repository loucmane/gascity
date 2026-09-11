# Native dependency budget: reviewed security-update baseline

The default module ceiling is 730 after the approved gRPC 1.83.2 / Thrift
0.24.0 update. This is an exact baseline adjustment, not growth headroom or
a CI environment override. All other limits remain unchanged: AWS 25, Azure
9, DoltHub 15, Google API 1, exactly one Beads module, and a 270,000,000-byte
normal binary. Product-metrics testhook symbols, literals and commands remain
excluded from that binary.

## Reproducible graph and attribution

`go list -mod=readonly -m all` under Go 1.26.7 reports 727 modules for signed
predecessor `24a093ff5ab2c32bae2baf3cf8886a3f723c3e7f` and 730 for signed
dependency candidate `2137af00f4c13f2845f087a7749a903b172525a8`. No module path
was removed. The full current inventory is retained as
`scripts/testdata/native-dependencies/grpc-1.83.2.txt`, SHA-256
`f82b9e2da7735ad7d914ce98c0aa9c95f93ac0004063ac735b665c9f2ffbd4c8`.

All three added paths are required through the approved gRPC update:

- gRPC 1.83.2 → `go.opentelemetry.io/otel/sdk/metric` 1.44.0 →
  `go.opentelemetry.io/otel/metric/x` 0.66.0.
- gRPC 1.83.2 → `github.com/googleapis/gax-go/v2` 2.17.0 →
  `google.golang.org/genproto` 0.0.0-20260128011058-8636f8732409 →
  `cloud.google.com/go/pubsub/v2` 2.0.0.
- The same gRPC/gax/genproto chain → `cloud.google.com/go/iam` 1.5.3 →
  `cloud.google.com/go` 0.121.6 → `github.com/zeebo/errs` 1.4.0.

Local before/after inventories and complete `go mod graph` edges are preserved
under `/tmp/ga-tmgr-container-dependencies-20260911/`:

- `baseline-module-graph-complete.txt`:
  `9c8bbd01aa5099ffb32cf49dfa0fb0903826ad7106038c24b3d6a08750a65295`.
- `current-module-graph-complete.txt`: same SHA-256 as the committed fixture.
- `current-module-edges.txt`:
  `80a43cd40f3f40ae772dfbfbae0ba9b41a55656b33ad0ab7d13f4126e9e3c354`.

## Proof boundary

The isolated shell-guard regression executes the real guard with a closed
mock Go protocol: the reviewed graph passes, a 731st unrelated module refuses
before build, family limits refuse independently of total count, and existing
binary/testhook checks still refuse. It does not build or run real `gc`.
The real default guard and normal hosted image scan are still required.

This is a count budget, not an exact module allowlist: it does not detect a
same-count replacement. Dependency-delta review supplies that distinction.
The fixture records this reviewed graph; later dependency changes still require
review and must not silently raise the ceiling or set a CI override.

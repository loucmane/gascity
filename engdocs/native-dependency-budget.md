# Native dependency budget: reviewed security-update baseline

The default module ceiling is 737 after the Beads 1.3.0 update in the v1.4.2
port. This is an exact baseline adjustment, not growth headroom or a CI
environment override. All other limits remain unchanged: AWS 25, Azure 9,
DoltHub 15, Google API 1, exactly one Beads module, and a 270,000,000-byte
normal binary. Product-metrics testhook symbols, literals and commands remain
excluded from that binary.

## Beads 1.3.0 graph and attribution

`go list -mod=readonly -m all` under Go 1.26.7 reports 730 modules for the
port base `c09c8ebf97f7b479614e1c148ea7c8d7c0b542e1` (identical to the gRPC
1.83.2 baseline below) and 737 for the port head
`d553b3fde06398972f0f56788783d8ce01d441e7`. The full current inventory is
retained as `scripts/testdata/native-dependencies/beads-1.3.0.txt`, SHA-256
`270d707d9da8e168f95dc29e9ee2230fabb2161fc5a98c6f4a5e47b7289a5eba`.

Eight module paths were added and one was removed. All eight enter the graph
only through the `github.com/steveyegge/beads` v1.3.0 module requirements, as
OpenAPI code-generation and YAML tooling:

- `github.com/dprotaso/go-yit`
- `github.com/getkin/kin-openapi`
- `github.com/oapi-codegen/oapi-codegen/v2`
- `github.com/oasdiff/yaml`
- `github.com/oasdiff/yaml3`
- `github.com/speakeasy-api/jsonpath`
- `github.com/speakeasy-api/openapi`
- `github.com/vmware-labs/yaml-jsonpath`

`github.com/wk8/go-ordered-map/v2` left the graph. None of the added modules is
linked into `gc`: `go version -m` on a `-trimpath`, `CGO_ENABLED=0` build of
`./cmd/gc` lists 107 dependencies and none of them
(`github.com/oapi-codegen/runtime` is a separate, pre-existing module). That
build is 166,298,546 bytes; the guard's own default build measured
264,708,360 bytes at the re-baseline, under the 270,000,000-byte limit. The
AWS, Azure, DoltHub and Google API family counts are unchanged (25, 9, 15 and
1).

Local before/after inventories are preserved under
`/var/tmp/ga-t4dx-inventory-20261001/`:

- `before.txt`: `f82b9e2da7735ad7d914ce98c0aa9c95f93ac0004063ac735b665c9f2ffbd4c8`
  (the gRPC 1.83.2 fixture).
- `after.txt`: same SHA-256 as the committed fixture.

## gRPC 1.83.2 baseline (730)

The previous ceiling of 730 followed the approved gRPC 1.83.2 / Thrift 0.24.0
update. `go list -mod=readonly -m all` under Go 1.26.7 reported 727 modules for
signed predecessor `24a093ff5ab2c32bae2baf3cf8886a3f723c3e7f` and 730 for signed
dependency candidate `2137af00f4c13f2845f087a7749a903b172525a8`. No module path
was removed. That reviewed inventory was the
`scripts/testdata/native-dependencies/grpc-1.83.2.txt` fixture, SHA-256
`f82b9e2da7735ad7d914ce98c0aa9c95f93ac0004063ac735b665c9f2ffbd4c8`, until the
Beads 1.3.0 re-baseline replaced it.

All three added paths were required through the approved gRPC update:

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
- `current-module-graph-complete.txt`: same SHA-256 as the former fixture.
- `current-module-edges.txt`:
  `80a43cd40f3f40ae772dfbfbae0ba9b41a55656b33ad0ab7d13f4126e9e3c354`.

## Proof boundary

The isolated shell-guard regression executes the real guard with a closed
mock Go protocol: the reviewed graph passes, a 738th unrelated module refuses
before build, family limits refuse independently of total count, and existing
binary/testhook checks still refuse. It does not build or run real `gc`.
The real default guard and normal hosted image scan are still required.

This is a count budget, not an exact module allowlist: it does not detect a
same-count replacement. Dependency-delta review supplies that distinction.
The fixture records this reviewed graph; later dependency changes still require
review and must not silently raise the ceiling or set a CI override.

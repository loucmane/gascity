package platforminstall

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// This is a source-wiring contract, not a host/writer execution or rollback
// proof. It keeps each existing validation call and its error chain identifiable
// without launching a transaction merely to exercise diagnostic text.
func TestMetadataValidationStageDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		file, function string
		calls, stages  []string
	}{
		{"metadata_transaction_linux.go", "MetadataWriterEntrypoint", []string{"metadataWriterBoundary(manifest)", "validateMetadataInputs(manifest, false)", "validateMetadataInputs(manifest, false)"}, []string{"writer boundary validation: %w", "writer prepare input validation: %w", "writer publication input validation: %w"}},
		{"metadata_transaction_linux.go", "RunMetadataTransaction", []string{"validateMetadataInputs(manifest, true)", "validateMetadataInputs(manifest, true)"}, []string{"initial host input validation: %w", "post-begin host input validation: %w"}},
		{"metadata_host_linux.go", "check", []string{"metadataProtectedHostCheckpoint(session.manifest)"}, []string{"host grant checkpoint validation: %w"}},
	} {
		t.Run(tc.function, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, tc.file, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			index := 0
			for _, declaration := range file.Decls {
				fn, ok := declaration.(*ast.FuncDecl)
				if !ok || fn.Name.Name != tc.function {
					continue
				}
				ast.Inspect(fn.Body, func(node ast.Node) bool {
					branch, ok := node.(*ast.IfStmt)
					if !ok {
						return true
					}
					assignment, ok := branch.Init.(*ast.AssignStmt)
					if !ok || len(assignment.Rhs) != 1 {
						return true
					}
					var expression bytes.Buffer
					if err := format.Node(&expression, fset, assignment.Rhs[0]); err != nil {
						t.Fatal(err)
					}
					call := expression.String()
					if call != tc.calls[0] && call != tc.calls[len(tc.calls)-1] {
						return true
					}
					if index >= len(tc.calls) || call != tc.calls[index] || len(branch.Body.List) != 1 {
						t.Fatalf("unexpected validation call/order/body: %s", call)
					}
					result, ok := branch.Body.List[0].(*ast.ReturnStmt)
					if !ok || len(result.Results) == 0 {
						t.Fatal("validation failure does not return")
					}
					expression.Reset()
					if err := format.Node(&expression, fset, result.Results[len(result.Results)-1]); err != nil {
						t.Fatal(err)
					}
					want := "fmt.Errorf(" + strconv.Quote(tc.stages[index]) + ", err)"
					if expression.String() != want {
						t.Errorf("%s diagnostic = %s, want %s", call, expression.String(), want)
					}
					index++
					return true
				})
			}
			if index != len(tc.calls) {
				t.Fatalf("validation calls=%d, want %d", index, len(tc.calls))
			}
		})
	}
}

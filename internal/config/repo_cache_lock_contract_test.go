package config

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestRepoCacheLockPlatformImplementationsUseSharedOpenPolicy(t *testing.T) {
	for _, path := range []string{"repo_cache_lock_unix.go", "repo_cache_lock_windows.go"} {
		t.Run(path, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			var matched bool
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok || len(call.Args) < 2 {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, pkgOK := sel.X.(*ast.Ident)
				if !pkgOK || pkg.Name != "os" || sel.Sel.Name != "OpenFile" {
					return true
				}
				flags, ok := call.Args[1].(*ast.CallExpr)
				if !ok {
					return true
				}
				policy, policyOK := flags.Fun.(*ast.Ident)
				if !policyOK || policy.Name != "repoCacheLockOpenFlags" || len(flags.Args) != 1 {
					return true
				}
				createRoot, ok := flags.Args[0].(*ast.Ident)
				matched = ok && createRoot.Name == "createRoot"
				return true
			})
			if !matched {
				t.Fatalf("%s must derive os.OpenFile flags from repoCacheLockOpenFlags(createRoot)", path)
			}
		})
	}
}

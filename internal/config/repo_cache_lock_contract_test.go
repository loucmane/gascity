package config

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"testing"
)

func TestRepoCacheLockOpenFlagsSeparateReadersFromWriters(t *testing.T) {
	readFlags := repoCacheLockOpenFlags(false)
	if readFlags != os.O_RDONLY {
		t.Fatalf("read flags = %#x, want O_RDONLY", readFlags)
	}
	if readFlags&(os.O_CREATE|os.O_WRONLY|os.O_RDWR) != 0 {
		t.Fatalf("read flags %#x request write or create access", readFlags)
	}

	writeFlags := repoCacheLockOpenFlags(true)
	if writeFlags&(os.O_CREATE|os.O_RDWR) != os.O_CREATE|os.O_RDWR {
		t.Fatalf("write flags = %#x, want O_CREATE|O_RDWR", writeFlags)
	}
}

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
				if !ok || len(call.Args) != 2 {
					return true
				}
				fn, ok := call.Fun.(*ast.Ident)
				if !ok || fn.Name != "openRepoCacheLockFile" {
					return true
				}
				comparison, ok := call.Args[1].(*ast.BinaryExpr)
				if !ok {
					return true
				}
				mode, modeOK := comparison.X.(*ast.Ident)
				exclusive, exclusiveOK := comparison.Y.(*ast.Ident)
				matched = comparison.Op == token.EQL && modeOK && exclusiveOK && mode.Name == "mode" && exclusive.Name == "repoCacheLockExclusive"
				return true
			})
			if !matched {
				t.Fatalf("%s must open through openRepoCacheLockFile with the platform lock mode", path)
			}
		})
	}
}

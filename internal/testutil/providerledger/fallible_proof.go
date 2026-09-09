package providerledger

import (
	"fmt"
	"go/ast"
	"go/token"
)

// proofFactoryConstructor accepts the original direct return or exactly:
//
//	provider, err := Constructor(...)
//	if err != nil { t.Fatal(err) }
//	return provider, ...
//
// Lexical objects, not identifier spelling, bind all three statements. This is
// not general control-flow inference: extra actions, reassignment, substituted
// providers and nonterminal error handling are rejected.
func proofFactoryConstructor(factory *ast.FuncLit, testParam *ast.Ident, constructor SymbolRef) (*ast.CallExpr, *ast.CallExpr, error) {
	statements := factory.Body.List
	if len(statements) == 1 {
		ret, ok := statements[0].(*ast.ReturnStmt)
		if !ok || len(ret.Results) == 0 {
			return nil, nil, fmt.Errorf("runner factory must contain exactly one direct return statement")
		}
		call, ok := unparen(ret.Results[0]).(*ast.CallExpr)
		if !ok {
			return nil, nil, fmt.Errorf("factory must return constructor %s directly", renderSymbolRef(constructor))
		}
		return call, nil, nil
	}
	if len(statements) != 3 {
		return nil, nil, fmt.Errorf("runner factory must contain exactly one direct return statement or a checked constructor/error pair")
	}
	refuse := func() (*ast.CallExpr, *ast.CallExpr, error) {
		return nil, nil, fmt.Errorf("fallible runner factory must bind the exact constructor, fail with its test parameter Fatal(error) on error != nil, and return the same provider")
	}
	assign, ok := statements[0].(*ast.AssignStmt)
	if !ok || assign.Tok != token.DEFINE || len(assign.Lhs) != 2 || len(assign.Rhs) != 1 {
		return refuse()
	}
	provider, providerOK := assign.Lhs[0].(*ast.Ident)
	failure, failureOK := assign.Lhs[1].(*ast.Ident)
	if !providerOK || !failureOK || provider.Name == "_" || failure.Name == "_" || provider.Name == failure.Name ||
		provider.Obj == nil || failure.Obj == nil || provider.Obj.Decl != assign || failure.Obj.Decl != assign ||
		provider.Obj == testParam.Obj || failure.Obj == testParam.Obj {
		return refuse()
	}
	constructorCall, ok := unparen(assign.Rhs[0]).(*ast.CallExpr)
	if !ok {
		return refuse()
	}
	guard, ok := statements[1].(*ast.IfStmt)
	if !ok || guard.Init != nil || guard.Else != nil || len(guard.Body.List) != 1 {
		return refuse()
	}
	condition, ok := unparen(guard.Cond).(*ast.BinaryExpr)
	if !ok || condition.Op != token.NEQ || !sameProofObject(condition.X, failure) {
		return refuse()
	}
	nilValue, ok := unparen(condition.Y).(*ast.Ident)
	if !ok || nilValue.Name != "nil" || nilValue.Obj != nil {
		return refuse()
	}
	failStmt, ok := guard.Body.List[0].(*ast.ExprStmt)
	if !ok {
		return refuse()
	}
	failCall, ok := unparen(failStmt.X).(*ast.CallExpr)
	if !ok || failCall.Ellipsis.IsValid() || len(failCall.Args) != 1 || !sameProofObject(failCall.Args[0], failure) {
		return refuse()
	}
	method, ok := unparen(failCall.Fun).(*ast.SelectorExpr)
	if !ok || method.Sel.Name != "Fatal" || !sameProofObject(method.X, testParam) {
		return refuse()
	}
	ret, ok := statements[2].(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 || !sameProofObject(ret.Results[0], provider) {
		return refuse()
	}
	return constructorCall, failCall, nil
}

func sameProofObject(expr ast.Expr, binding *ast.Ident) bool {
	ident, ok := unparen(expr).(*ast.Ident)
	return ok && binding.Obj != nil && ident.Obj == binding.Obj
}

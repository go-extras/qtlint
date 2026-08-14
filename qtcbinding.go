package qtlint

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/analysis"
)

// testingPkgPath is the import path of the standard library testing package.
const testingPkgPath = "testing"

// isTestingTPtr reports whether typ is exactly *testing.T.
//
// The opt-in rules that rewrite between a *testing.T and a *qt.C insist on
// this exact type rather than on testing.TB: qt.New accepts any testing.TB,
// but only a *testing.T can stand where the rewrite puts one.
func isTestingTPtr(typ types.Type) bool {
	ptr, ok := types.Unalias(typ).(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := types.Unalias(ptr.Elem()).(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj != nil && obj.Pkg() != nil &&
		obj.Pkg().Path() == testingPkgPath && obj.Name() == "T"
}

// identNamesIn collects every identifier name that appears anywhere within n.
//
// It is deliberately coarse: a name used as a struct field or a label is
// reported as taken even though it would not actually collide. Declining a
// name we could have used costs a suffix digit; using one we could not costs
// a rewrite that does not compile.
func identNamesIn(n ast.Node) map[string]bool {
	names := make(map[string]bool)
	ast.Inspect(n, func(node ast.Node) bool {
		if id, ok := node.(*ast.Ident); ok {
			names[id.Name] = true
		}
		return true
	})
	return names
}

// freeName returns base when it does not appear in taken, and otherwise the
// first of base2, base3, … that does not.
func freeName(base string, taken map[string]bool) string {
	if !taken[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := base + strconv.Itoa(i)
		if !taken[candidate] {
			return candidate
		}
	}
}

// collectQtCOrigins maps every *qt.C variable declared within root by a call
// to qt.New to the argument of that call.
//
// Only c := qt.New(t) and var c = qt.New(t) are recognized. A *qt.C obtained
// any other way (a parameter, a struct field, a helper's return value) has no
// statically known testing.TB behind it, which is why the rules below decline
// to reason about it.
func collectQtCOrigins(pass *analysis.Pass, root ast.Node) map[types.Object]ast.Expr {
	origins := make(map[types.Object]ast.Expr)

	record := func(name, value ast.Expr) {
		ident, ok := name.(*ast.Ident)
		if !ok {
			return
		}
		obj := pass.TypesInfo.Defs[ident]
		if obj == nil || !isQuicktestCType(obj.Type()) {
			return
		}
		if arg, ok := qtNewArg(pass, value); ok {
			origins[obj] = arg
		}
	}

	ast.Inspect(root, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			if stmt.Tok == token.DEFINE && len(stmt.Lhs) == 1 && len(stmt.Rhs) == 1 {
				record(stmt.Lhs[0], stmt.Rhs[0])
			}
		case *ast.DeclStmt:
			collectQtCVarSpecs(stmt, record)
		}
		return true
	})

	return origins
}

// collectQtCVarSpecs feeds every single-name, single-value var specification
// in stmt to record.
func collectQtCVarSpecs(stmt *ast.DeclStmt, record func(name, value ast.Expr)) {
	decl, ok := stmt.Decl.(*ast.GenDecl)
	if !ok || decl.Tok != token.VAR {
		return
	}
	for _, spec := range decl.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok || len(vs.Names) != 1 || len(vs.Values) != 1 {
			continue
		}
		record(vs.Names[0], vs.Values[0])
	}
}

// qtNewArg returns the sole argument of a call to quicktest's New function.
func qtNewArg(pass *analysis.Pass, expr ast.Expr) (ast.Expr, bool) {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "New" || !isPackageQualified(pass, sel) {
		return nil, false
	}
	if len(call.Args) != 1 {
		return nil, false
	}
	return call.Args[0], true
}

// funcBindsObject reports whether params declares obj.
func funcBindsObject(pass *analysis.Pass, params *ast.FieldList, obj types.Object) bool {
	if params == nil {
		return false
	}
	for _, field := range params.List {
		for _, name := range field.Names {
			if pass.TypesInfo.Defs[name] == obj {
				return true
			}
		}
	}
	return false
}

// enclosingBindingBody returns the body of the innermost function on stack
// whose parameter list declares obj, or nil when no function on the stack
// does.
func enclosingBindingBody(pass *analysis.Pass, stack []ast.Node, obj types.Object) *ast.BlockStmt {
	for i := len(stack) - 1; i >= 0; i-- {
		switch fn := stack[i].(type) {
		case *ast.FuncDecl:
			if funcBindsObject(pass, fn.Type.Params, obj) {
				return fn.Body
			}
		case *ast.FuncLit:
			if funcBindsObject(pass, fn.Type.Params, obj) {
				return fn.Body
			}
		}
	}
	return nil
}

// prependStmtEdit returns the edit that makes code the first statement of
// body. The inserted text carries no indentation: every driver that applies a
// SuggestedFix reformats the file afterwards.
func prependStmtEdit(body *ast.BlockStmt, code string) analysis.TextEdit {
	at := body.Lbrace + 1
	return analysis.TextEdit{Pos: at, End: at, NewText: []byte("\n" + code)}
}

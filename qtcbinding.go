package qtlint

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// quicktestPkgPath is the import path of the library whose usage this
// analyzer enforces practices for.
const quicktestPkgPath = "github.com/frankban/quicktest"

// testingPkgPath is the import path of the standard library testing package.
const testingPkgPath = "testing"

// importedPkgName returns the name under which file imports path, or "" when
// the file does not import it under a name a new reference could use: a blank
// or dot import qualifies nothing.
func importedPkgName(pass *analysis.Pass, file *ast.File, path string) string {
	for _, imp := range file.Imports {
		imported, err := strconv.Unquote(imp.Path.Value)
		if err != nil || imported != path {
			continue
		}
		if imp.Name != nil {
			if imp.Name.Name == "_" || imp.Name.Name == "." {
				return ""
			}
			return imp.Name.Name
		}
		if obj, ok := pass.TypesInfo.Implicits[imp].(*types.PkgName); ok {
			return obj.Name()
		}
	}
	return ""
}

// outermostFuncs returns the function declarations and literals in file that
// no other function contains. Every use of a variable declared inside a
// function lies within it, so each one bounds the search for the uses a
// rewrite would remove.
func outermostFuncs(file *ast.File) []ast.Node {
	var roots []ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			roots = append(roots, n)
			return false
		}
		return true
	})
	return roots
}

// packageQualifies reports whether name still refers to the package imported
// from path at pos.
//
// An import is a file-scope declaration, so a variable, parameter or type of
// the same name hides it from its own declaration onwards. A rewrite that
// writes a package qualifier has to ask at the position it writes it rather
// than trust the file's import list, which says only that the name meant the
// package somewhere.
func packageQualifies(pass *analysis.Pass, name, path string, pos token.Pos) bool {
	scope := pass.Pkg.Scope().Innermost(pos)
	if scope == nil {
		return false
	}
	_, obj := scope.LookupParent(name, pos)
	pkgName, ok := obj.(*types.PkgName)
	return ok && pkgName.Imported() != nil && pkgName.Imported().Path() == path
}

// inspectWithParent walks the tree rooted at root in depth-first order,
// calling fn for every node together with the node that contains it. The
// root's parent is nil.
//
// A plain ast.Inspect answers "is this node an identifier"; a rule that has
// to answer "and what is being done to it" needs the node above.
func inspectWithParent(root ast.Node, fn func(n, parent ast.Node)) {
	var stack []ast.Node
	ast.Inspect(root, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		var parent ast.Node
		if len(stack) > 0 {
			parent = stack[len(stack)-1]
		}
		fn(n, parent)
		stack = append(stack, n)
		return true
	})
}

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

// qtCOrigin records where a *qt.C variable came from.
type qtCOrigin struct {
	// arg is the expression passed to qt.New.
	arg ast.Expr
	// decl is the statement that declares the variable. A rewrite that
	// removes the variable's last use has to remove this statement with it,
	// or the result does not compile.
	decl ast.Stmt
}

// collectQtCOrigins maps every *qt.C variable declared within root by a call
// to qt.New to that call's argument and the declaring statement.
//
// Only c := qt.New(t) and var c = qt.New(t) are recognized. A *qt.C obtained
// any other way (a parameter, a struct field, a helper's return value) has no
// statically known testing.TB behind it, which is why the rules below decline
// to reason about it.
//
// A variable that is written again after its declaration is dropped, because
// the declaration is no longer a true statement about which test the *qt.C
// holds. See invalidateRebound.
func collectQtCOrigins(pass *analysis.Pass, root ast.Node) map[types.Object]qtCOrigin {
	origins := make(map[types.Object]qtCOrigin)

	record := func(name, value ast.Expr, decl ast.Stmt) {
		ident, ok := name.(*ast.Ident)
		if !ok {
			return
		}
		obj := pass.TypesInfo.Defs[ident]
		if obj == nil || !isQuicktestCType(obj.Type()) {
			return
		}
		if arg, ok := qtNewArg(pass, value); ok {
			origins[obj] = qtCOrigin{arg: arg, decl: decl}
		}
	}

	ast.Inspect(root, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			if stmt.Tok == token.DEFINE && len(stmt.Lhs) == 1 && len(stmt.Rhs) == 1 {
				record(stmt.Lhs[0], stmt.Rhs[0], stmt)
			}
		case *ast.DeclStmt:
			collectQtCVarSpecs(stmt, record)
		}
		return true
	})

	invalidateRebound(pass, root, origins)

	return origins
}

// invalidateRebound drops every origin whose variable is written again after
// its declaration, or whose address is taken.
//
// An origin is only useful as a claim about which testing.TB a *qt.C holds.
// "c := qt.New(t); c = qt.New(other)" makes that claim false from the second
// statement onwards, and a rewrite that believed it would move assertions
// onto a different test — silently, since both spellings compile. Taking the
// variable's address puts the same write out of reach of this analysis, so it
// is treated the same way.
func invalidateRebound(pass *analysis.Pass, root ast.Node, origins map[types.Object]qtCOrigin) {
	drop := func(expr ast.Expr, at ast.Stmt) {
		ident, ok := stripParens(expr).(*ast.Ident)
		if !ok {
			return
		}
		obj := pass.TypesInfo.Uses[ident]
		if obj == nil {
			obj = pass.TypesInfo.Defs[ident]
		}
		if origin, ok := origins[obj]; ok && origin.decl != at {
			delete(origins, obj)
		}
	}

	ast.Inspect(root, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range node.Lhs {
				drop(lhs, node)
			}
		case *ast.RangeStmt:
			drop(node.Key, node)
			drop(node.Value, node)
		case *ast.UnaryExpr:
			if node.Op == token.AND {
				drop(node.X, nil)
			}
		}
		return true
	})
}

// collectQtCVarSpecs feeds every single-name, single-value var specification
// in stmt to record.
func collectQtCVarSpecs(stmt *ast.DeclStmt, record func(name, value ast.Expr, decl ast.Stmt)) {
	decl, ok := stmt.Decl.(*ast.GenDecl)
	if !ok || decl.Tok != token.VAR || len(decl.Specs) != 1 {
		return
	}
	vs, ok := decl.Specs[0].(*ast.ValueSpec)
	if !ok || len(vs.Names) != 1 || len(vs.Values) != 1 {
		return
	}
	record(vs.Names[0], vs.Values[0], stmt)
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

// enclosingBinder returns the innermost function on stack whose parameter
// list declares obj, together with that function's body. Both are nil when no
// function on the stack declares it.
//
// Callers need both halves and they are not interchangeable. The body is
// where a new statement goes; the whole function is what a new name must not
// collide with. A receiver, a parameter and a named result share one scope
// with the body, so "c := qt.New(t)" prepended to the body of
// "func (c *harness) assert(t *testing.T)" is "no new variables on left side
// of :=" — a name that is taken outside the body but inside the scope the
// body writes into.
func enclosingBinder(pass *analysis.Pass, stack []ast.Node, obj types.Object) (ast.Node, *ast.BlockStmt) {
	for i := len(stack) - 1; i >= 0; i-- {
		switch fn := stack[i].(type) {
		case *ast.FuncDecl:
			if funcBindsObject(pass, fn.Type.Params, obj) {
				return fn, fn.Body
			}
		case *ast.FuncLit:
			if funcBindsObject(pass, fn.Type.Params, obj) {
				return fn, fn.Body
			}
		}
	}
	return nil, nil
}

// prependStmtEdit returns the edit that makes code the first statement of
// body.
//
// Where the body's first statement already has a line to itself, the new
// statement takes a line above it. Inserting straight after the brace also
// works, but gofmt then pulls any trailing comment from the brace's line down
// onto the inserted statement, so a comment written about a subtest ends up
// reading as a comment about the qt.New that opens it.
//
// The inserted text carries no indentation: every driver that applies a
// SuggestedFix reformats the file afterwards.
func prependStmtEdit(pass *analysis.Pass, body *ast.BlockStmt, code string) analysis.TextEdit {
	if at, ok := firstStmtLineStart(pass, body); ok {
		return analysis.TextEdit{Pos: at, End: at, NewText: []byte(code + "\n")}
	}
	// Either the body is empty, or its first statement shares the brace's line.
	// Opening a line before the inserted statement is not enough for the second
	// case: without a closing newline the statement already on that line runs
	// straight into the inserted one and the result does not parse.
	at := body.Lbrace + 1
	return analysis.TextEdit{Pos: at, End: at, NewText: []byte("\n" + code + "\n")}
}

// firstStmtLineStart returns the start of the line holding body's first
// statement, and reports false when that statement shares its line with the
// opening brace — inserting at the line start would then put the new
// statement after it.
func firstStmtLineStart(pass *analysis.Pass, body *ast.BlockStmt) (token.Pos, bool) {
	if len(body.List) == 0 {
		return token.NoPos, false
	}
	file := pass.Fset.File(body.Lbrace)
	if file == nil {
		return token.NoPos, false
	}
	first := file.Line(body.List[0].Pos())
	if first <= file.Line(body.Lbrace) {
		return token.NoPos, false
	}
	return file.LineStart(first), true
}

// wholeLineSpan returns the span covering every line node occupies, including
// the final newline, and reports whether node is the only thing on them.
//
// Callers use it to delete a declaration a rewrite left unused. Deleting only
// the node's own span would leave a blank line behind; deleting the whole
// line when the node shares it with other code would take that code too, so
// ok is false in that case and the caller must decline the rewrite.
func wholeLineSpan(pass *analysis.Pass, node ast.Node) (start, end token.Pos, ok bool) {
	file := pass.Fset.File(node.Pos())
	if file == nil {
		return token.NoPos, token.NoPos, false
	}
	content, err := pass.ReadFile(file.Name())
	if err != nil || len(content) != file.Size() {
		return token.NoPos, token.NoPos, false
	}

	lineStart := file.LineStart(file.Line(node.Pos()))
	if strings.TrimSpace(string(content[file.Offset(lineStart):file.Offset(node.Pos())])) != "" {
		return token.NoPos, token.NoPos, false
	}

	lineEnd := token.Pos(file.Base() + file.Size())
	if endLine := file.Line(node.End()); endLine < file.LineCount() {
		lineEnd = file.LineStart(endLine + 1)
	}
	if strings.TrimSpace(string(content[file.Offset(node.End()):file.Offset(lineEnd)])) != "" {
		return token.NoPos, token.NoPos, false
	}

	return lineStart, lineEnd, true
}

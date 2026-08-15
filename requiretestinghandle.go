package qtlint

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// qtCHelperMessage is the diagnostic every -require-testing-handle report
// opens with; a withheld report appends why it was withheld.
const qtCHelperMessage = "qtlint: a test helper takes the test handle, not the checker: use testing.TB and build the *qt.C inside"

// qtCHelper describes a function that takes a *qt.C parameter, whether it is
// declared at package level or bound to a variable inside a test.
//
// Both shapes are the same defect and the same repair. A test that builds its
// fixtures with a local closure taking the checker blocks -require-testing-run
// exactly as a package-level helper does, and leaving the local one out would
// mean the rule cleared a repository down to the shape it could not see.
type qtCHelper struct {
	// body is the function's body and sig its signature. decl is set only for
	// a package-level declaration; a local closure has none.
	decl  *ast.FuncDecl
	body  *ast.BlockStmt
	sig   *ast.FuncType
	param *ast.Field
	// name is the parameter's name and obj the object it declares. A blank or
	// unnamed parameter has neither.
	name string
	obj  types.Object
	// index is the parameter's position in the signature, counted in
	// parameters rather than fields, so a call's argument list lines up.
	index int
}

// checkRequireTestingHandle reports test helpers that take a *qt.C and
// suggests taking the test handle instead.
//
// A helper that takes the checker is what stops -require-testing-run from
// converting its callers. That rule cannot see what a helper does with a
// *qt.C, and what it could do includes (*qt.C).Defer, which panics unless
// Done() ran — so it reports the call site and withholds the rewrite. The
// helper is the cause; the c.Run it blocks is the symptom.
//
// The rewrite is deliberately to testing.TB rather than to *testing.T. Inside
// a subtest closure that has not been converted yet there is no *testing.T to
// pass, but there is c.TB: quicktest's C embeds testing.TB, so the checker
// yields the handle it was built from. Converting to testing.TB therefore
// leaves every caller compiling at every point, whether or not its enclosing
// closure has been converted, and -require-testing-run can then do its half.
// A *testing.T parameter would force both halves to land in one edit.
func (*analyzer) checkRequireTestingHandle(pass *analysis.Pass) {
	helpers := collectQtCHelpers(pass)
	if len(helpers) == 0 {
		return
	}

	calls, unreachable := callSitesByCallee(pass, helpers)
	for obj, helper := range helpers {
		if unreachable[obj] {
			pass.Report(analysis.Diagnostic{
				Pos:     helper.param.Pos(),
				End:     helper.param.End(),
				Message: qtCHelperMessage + "; no fix: this function is also used as a value, so its type is written down somewhere this rule does not edit",
			})
			continue
		}
		reportQtCHelper(pass, helper, calls[obj])
	}
}

// collectQtCHelpers finds every function in pass that takes a *qt.C, in both
// shapes it occurs in: a package-level declaration and a closure bound to a
// variable inside a test.
func collectQtCHelpers(pass *analysis.Pass) map[types.Object]qtCHelper {
	helpers := make(map[types.Object]qtCHelper)
	for _, file := range pass.Files {
		collectDeclaredHelpers(pass, file, helpers)
		collectBoundHelpers(pass, file, helpers)
	}
	return helpers
}

// collectDeclaredHelpers gathers the package-level declarations.
func collectDeclaredHelpers(pass *analysis.Pass, file *ast.File, into map[types.Object]qtCHelper) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		helper, ok := matchQtCHelper(pass, fn)
		if !ok {
			continue
		}
		if obj := pass.TypesInfo.Defs[fn.Name]; obj != nil {
			into[obj] = helper
		}
	}
}

// collectBoundHelpers gathers the closures bound to a variable.
//
// A closure bound to a name is the same defect wearing a local one:
// fixture := func(c *qt.C) … blocks -require-testing-run exactly as a
// package-level helper does, and leaving it out would mean the rule cleared a
// repository down to the shape it could not see.
func collectBoundHelpers(pass *analysis.Pass, file *ast.File, into map[types.Object]qtCHelper) {
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		lit, litOK := assign.Rhs[0].(*ast.FuncLit)
		ident, identOK := assign.Lhs[0].(*ast.Ident)
		if !litOK || !identOK {
			return true
		}
		obj := pass.TypesInfo.Defs[ident]
		if obj == nil {
			return true
		}
		if helper, ok := matchQtCLiteral(pass, lit); ok {
			into[obj] = helper
		}
		return true
	})
}

// matchQtCHelper parses fn into a qtCHelper.
//
// A method is skipped: its receiver ties it to a type whose other methods this
// rule has not looked at, and renaming one parameter of one method is not a
// change with a defensible boundary. A variadic *qt.C is skipped for the same
// reason a named function argument to c.Run is: the call sites do not line up
// one to one, so a rewrite would have to guess.
func matchQtCHelper(pass *analysis.Pass, fn *ast.FuncDecl) (qtCHelper, bool) {
	if fn.Recv != nil {
		return qtCHelper{}, false
	}
	helper, ok := matchQtCSignature(pass, fn.Type, fn.Body)
	if !ok {
		return qtCHelper{}, false
	}
	helper.decl = fn
	return helper, true
}

// matchQtCLiteral parses a function literal bound to a variable.
func matchQtCLiteral(pass *analysis.Pass, lit *ast.FuncLit) (qtCHelper, bool) {
	return matchQtCSignature(pass, lit.Type, lit.Body)
}

// matchQtCSignature finds the sole *qt.C parameter in a signature.
func matchQtCSignature(pass *analysis.Pass, sig *ast.FuncType, body *ast.BlockStmt) (qtCHelper, bool) {
	if sig.Params == nil || body == nil {
		return qtCHelper{}, false
	}

	index := 0
	for _, field := range sig.Params.List {
		width := len(field.Names)
		if width == 0 {
			width = 1
		}
		if _, variadic := field.Type.(*ast.Ellipsis); variadic {
			index += width
			continue
		}
		if !isQuicktestCType(pass.TypesInfo.TypeOf(field.Type)) {
			index += width
			continue
		}
		// One field may declare several parameters, and rewriting a
		// c1, c2 *qt.C field means deciding what each becomes. The shape does
		// not occur in practice and guessing is worse than declining.
		if width != 1 {
			return qtCHelper{}, false
		}
		helper := qtCHelper{body: body, sig: sig, param: field, index: index}
		if len(field.Names) == 1 && field.Names[0].Name != "_" {
			helper.name = field.Names[0].Name
			helper.obj = pass.TypesInfo.Defs[field.Names[0]]
		}
		return helper, true
	}
	return qtCHelper{}, false
}

// callSitesByCallee groups every call to one of helpers by the function it
// calls.
//
// A call reached through anything other than the function's own name — a
// value assigned to a variable, a method expression — is not collected, and
// its callee is dropped from the result entirely. Rewriting a declaration
// whose callers this rule cannot enumerate is how a package stops compiling.
func callSitesByCallee(pass *analysis.Pass, helpers map[types.Object]qtCHelper) (map[types.Object][]*ast.CallExpr, map[types.Object]bool) {
	calls := make(map[types.Object][]*ast.CallExpr)
	unreachable := make(map[types.Object]bool)

	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			ident, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			obj := pass.TypesInfo.Uses[ident]
			if obj == nil {
				return true
			}
			if _, ok := helpers[obj]; !ok {
				return true
			}
			call, direct := directCallOf(file, ident)
			if !direct {
				// The function is named somewhere that is not a call of it.
				unreachable[obj] = true
				return true
			}
			calls[obj] = append(calls[obj], call)
			return true
		})
	}

	for obj := range unreachable {
		delete(calls, obj)
	}
	return calls, unreachable
}

// directCallOf reports the call whose callee is ident, and false when ident is
// used as anything else.
func directCallOf(file *ast.File, ident *ast.Ident) (*ast.CallExpr, bool) {
	var found *ast.CallExpr
	direct := false
	inspectWithParent(file, func(n, parent ast.Node) {
		if n != ast.Node(ident) {
			return
		}
		call, ok := parent.(*ast.CallExpr)
		if ok && stripParens(call.Fun) == ast.Expr(ident) {
			found, direct = call, true
		}
	})
	return found, direct
}

// reportQtCHelper reports one helper and renders the rewrite of its
// declaration together with every call site.
func reportQtCHelper(pass *analysis.Pass, helper qtCHelper, calls []*ast.CallExpr) {
	diag := analysis.Diagnostic{
		Pos:     helper.param.Pos(),
		End:     helper.param.End(),
		Message: qtCHelperMessage,
	}

	edits, reason := qtCHelperEdits(pass, helper, calls)
	if reason == "" {
		diag.SuggestedFixes = []analysis.SuggestedFix{{
			Message:   "Take testing.TB and build the *qt.C from it",
			TextEdits: edits,
		}}
	} else {
		diag.Message += reason
	}
	pass.Report(diag)
}

// qtCHelperEdits renders the declaration change, the qt.New the body needs,
// and one edit per call site.
func qtCHelperEdits(pass *analysis.Pass, helper qtCHelper, calls []*ast.CallExpr) ([]analysis.TextEdit, string) {
	file := fileHolding(pass, helper.param.Pos())
	if file == nil {
		return nil, "; no fix: this rule could not find the file the declaration is in"
	}
	testingName := importedPkgName(pass, file, testingPkgPath)
	qtAlias := importedPkgName(pass, file, quicktestPkgPath)
	if testingName == "" {
		return nil, "; no fix: this file does not import testing under a name the new parameter type could use"
	}
	if qtAlias == "" {
		return nil, "; no fix: this file does not import quicktest under a name the inserted qt.New could use"
	}
	// Both qualifiers are written into the declaration's own file, so both
	// have to mean their packages there. A file can import a path twice.
	if !packageQualifies(pass, testingName, testingPkgPath, helper.param.Pos()) {
		return nil, "; no fix: the testing qualifier does not mean testing where the new parameter type would go"
	}
	if !packageQualifies(pass, qtAlias, quicktestPkgPath, helper.body.Lbrace) {
		return nil, "; no fix: the quicktest qualifier does not mean quicktest where the qt.New would go"
	}

	callEdits := make([]analysis.TextEdit, 0, len(calls))
	for _, call := range calls {
		if helper.index >= len(call.Args) {
			return nil, "; no fix: a call passes fewer arguments than the signature declares, so this rule cannot say which one is the checker"
		}
		arg := call.Args[helper.index]
		callEdits = append(callEdits, analysis.TextEdit{
			Pos:     arg.Pos(),
			End:     arg.End(),
			NewText: []byte(handleExprText(pass, arg)),
		})
	}

	// Go requires a signature to name every parameter or none of them, so an
	// unnamed *qt.C stays unnamed: writing a name into one field of an
	// otherwise unnamed list produces "missing parameter type".
	if len(helper.param.Names) == 0 {
		return append([]analysis.TextEdit{{
			Pos:     helper.param.Pos(),
			End:     helper.param.End(),
			NewText: []byte(testingName + ".TB"),
		}}, callEdits...), ""
	}

	// A blank parameter keeps its blank name for the same reason, and has no
	// body use to satisfy.
	if helper.name == "" {
		return append([]analysis.TextEdit{{
			Pos:     helper.param.Pos(),
			End:     helper.param.End(),
			NewText: []byte("_ " + testingName + ".TB"),
		}}, callEdits...), ""
	}

	// The handle takes a name of its own rather than reusing the checker's, so
	// the qt.New the body gains can still be spelled c := qt.New(tb) and every
	// existing use of c in the body keeps meaning the checker.
	tbName := freeName("tb", takenNamesInFunc(helper.sig, helper.body))
	edits := []analysis.TextEdit{{
		Pos:     helper.param.Pos(),
		End:     helper.param.End(),
		NewText: []byte(fmt.Sprintf("%s %s.TB", tbName, testingName)),
	}}

	// A parameter the body never reads needs no checker built for it, and
	// building one anyway declares a variable nothing uses. An unused
	// parameter is legal; an unused local is not.
	if usesObject(pass, helper.body, helper.obj) {
		edits = append(edits, prependStmtEdit(pass, helper.body,
			fmt.Sprintf("%s := %s.New(%s)", helper.name, qtAlias, tbName)))
	}

	return append(edits, callEdits...), ""
}

// usesObject reports whether obj is read anywhere inside root.
func usesObject(pass *analysis.Pass, root ast.Node, obj types.Object) bool {
	if obj == nil {
		return false
	}
	used := false
	ast.Inspect(root, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if ok && pass.TypesInfo.Uses[ident] == obj {
			used = true
		}
		return !used
	})
	return used
}

// handleExprText renders the argument a converted call site passes.
//
// A *qt.C argument becomes c.TB: C embeds testing.TB, so the checker yields
// the handle it was built from, and the call compiles whether or not the
// closure around it has been converted yet. An argument that is already a
// testing.TB is passed through.
func handleExprText(pass *analysis.Pass, arg ast.Expr) string {
	text := exprText(pass, arg)
	if isQuicktestCType(pass.TypesInfo.TypeOf(arg)) {
		return text + ".TB"
	}
	return text
}

// fileHolding returns the parsed file that contains pos.
func fileHolding(pass *analysis.Pass, pos token.Pos) *ast.File {
	for _, file := range pass.Files {
		if file.Pos() <= pos && pos < file.End() {
			return file
		}
	}
	return nil
}

// exprText renders expr back to source.
func exprText(pass *analysis.Pass, expr ast.Expr) string {
	var buf bytes.Buffer
	if err := format.Node(&buf, pass.Fset, expr); err != nil {
		return ""
	}
	return buf.String()
}

// takenNamesInFunc returns every name declared or used anywhere in fn, so a
// name chosen clear of it cannot shadow one the body relies on.
func takenNamesInFunc(nodes ...ast.Node) map[string]bool {
	taken := make(map[string]bool)
	for _, node := range nodes {
		ast.Inspect(node, func(n ast.Node) bool {
			if ident, ok := n.(*ast.Ident); ok {
				taken[ident.Name] = true
			}
			return true
		})
	}
	return taken
}

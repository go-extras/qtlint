package qtlint

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// testScopedCMethods are the *qt.C methods that act on the test the *qt.C was
// made from rather than on the assertion at hand.
//
// Under c.Run they bind to whichever test the receiver came from; after the
// rewrite the closure's *qt.C is made from the subtest, which is what a
// reader almost always meant. "Almost always" is the whole reason
// -only-stable-fixes exists, so a closure that calls one of these keeps its
// diagnostic but loses its automatic fix under that flag.
var testScopedCMethods = map[string]bool{
	"Cleanup":  true,
	"Defer":    true,
	"Mkdir":    true,
	"Parallel": true,
	"Patch":    true,
	"Setenv":   true,
	"TempDir":  true,
}

// qtCRun describes a c.Run(name, func(c *qt.C) { … }) call.
type qtCRun struct {
	// sel is the call's "c.Run" selector.
	sel *ast.SelectorExpr
	// recv is the receiver identifier and recvObj the object it refers to.
	recv    *ast.Ident
	recvObj types.Object
	// lit is the subtest closure and param its sole *qt.C parameter.
	lit   *ast.FuncLit
	param *ast.Field
	// cName is the parameter's name and cObj the object it declares. Both
	// are zero when the parameter is unnamed or blank.
	cName string
	cObj  types.Object
}

// matchQtCRun parses call into a qtCRun.
//
// The receiver is matched on its type, so an identifier other than c is
// matched just the same. The subtest argument must be a function literal:
// issue #42's case 5, a named function, has signature func(*qt.C), which
// t.Run will not accept, and rewriting it means changing a declaration that
// may have callers elsewhere. That is out of scope, and since a reported site
// without a fix is a site whose repair falls back on the author, such a call
// is not reported at all.
func matchQtCRun(pass *analysis.Pass, call *ast.CallExpr) (qtCRun, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Run" || !isQuicktestCMethod(pass, sel) {
		return qtCRun{}, false
	}
	if len(call.Args) != 2 {
		return qtCRun{}, false
	}
	recv, ok := sel.X.(*ast.Ident)
	if !ok {
		return qtCRun{}, false
	}
	recvObj := pass.TypesInfo.Uses[recv]
	if recvObj == nil {
		return qtCRun{}, false
	}
	lit, ok := call.Args[1].(*ast.FuncLit)
	if !ok {
		return qtCRun{}, false
	}
	params := lit.Type.Params
	if params == nil || len(params.List) != 1 {
		return qtCRun{}, false
	}
	param := params.List[0]
	if len(param.Names) > 1 || !isQuicktestCType(pass.TypesInfo.TypeOf(param.Type)) {
		return qtCRun{}, false
	}

	run := qtCRun{sel: sel, recv: recv, recvObj: recvObj, lit: lit, param: param}
	if len(param.Names) == 1 && param.Names[0].Name != "_" {
		run.cName = param.Names[0].Name
		run.cObj = pass.TypesInfo.Defs[param.Names[0]]
	}
	return run, true
}

// checkRequireTestingRun reports a c.Run subtest and suggests the
// t.Run(name, func(t *testing.T) { c := qt.New(t) }) form.
func (a *analyzer) checkRequireTestingRun(pass *analysis.Pass, stack []ast.Node, call *ast.CallExpr) {
	run, ok := matchQtCRun(pass, call)
	if !ok {
		return
	}

	file := enclosingFile(stack)
	if file == nil {
		return
	}
	qtAlias := importedPkgName(pass, file, quicktestPkgPath)
	testingName := importedPkgName(pass, file, testingPkgPath)
	if qtAlias == "" || testingName == "" {
		return
	}

	target, ok := a.resolveRunTarget(pass, stack, run, qtAlias, testingName)
	if !ok {
		return
	}

	// The receiver may have had no other use, in which case its declaration
	// goes with the rewrite.
	edits, ok := a.unusedReceiverDeclEdits(pass, stack, run)
	if !ok {
		return
	}

	tName := freeName("t", takenNames(run.lit, qtAlias, testingName))
	edits = append(edits,
		analysis.TextEdit{
			Pos:     run.sel.X.Pos(),
			End:     run.sel.X.End(),
			NewText: []byte(target.recv),
		},
		analysis.TextEdit{
			Pos:     run.param.Pos(),
			End:     run.param.End(),
			NewText: []byte(newRunParam(run, tName, testingName)),
		},
	)

	// The *qt.C is recreated only if the closure still needs one. A closure
	// whose only use of it was the receiver of a nested c.Run loses that use
	// to the nested rewrite, and a declaration with no uses does not compile.
	if run.cObj != nil && a.survivingUses(pass, run.lit, run.cObj) > 0 {
		edits = append(edits, prependStmtEdit(pass, run.lit.Body,
			fmt.Sprintf("%s := %s.New(%s)", run.cName, qtAlias, tName)))
	}

	diag := analysis.Diagnostic{
		Pos:     call.Fun.Pos(),
		End:     call.Fun.End(),
		Message: "qtlint: use t.Run with a per-subtest qt.New instead of c.Run",
	}
	unstable := target.unstable || closureUsesTestScopedMethod(pass, run)
	if !unstable || !a.onlyStableFixes {
		diag.SuggestedFixes = []analysis.SuggestedFix{{
			Message:   "Replace c.Run with t.Run and a per-subtest qt.New",
			TextEdits: edits,
		}}
	}
	pass.Report(diag)
}

// runTarget is what a c.Run receiver rewrites to.
type runTarget struct {
	// recv is the *testing.T expression that replaces the receiver.
	recv string
	// unstable reports that an enclosing rewrite may change behavior. It
	// travels inward: a nested rewrite that lands while its parent is
	// withheld names a t that the parent has not created yet.
	unstable bool
}

// resolveRunTarget works out what the receiver of run's Run call becomes.
//
// Two shapes reach a *testing.T. The receiver may be the *qt.C parameter of
// an enclosing c.Run closure that this rule also rewrites, in which case it
// becomes the parameter that rewrite gives that closure; or it may be a
// variable declared as c := qt.New(t), in which case it becomes that t.
// Anything else — a *qt.C parameter of a helper, a struct field, a value from
// a factory — has no statically known *testing.T, so the rule leaves it be.
func (a *analyzer) resolveRunTarget(
	pass *analysis.Pass,
	stack []ast.Node,
	run qtCRun,
	qtAlias, testingName string,
) (runTarget, bool) {
	if target, ok := a.targetFromEnclosingRun(pass, stack, run, qtAlias, testingName); ok {
		return target, true
	}
	return targetFromQtNew(pass, stack, run)
}

// targetFromEnclosingRun handles a receiver that is the *qt.C parameter of an
// enclosing c.Run closure. Resolving the enclosing rewrite first is what
// makes a nest come out right: the inner call names the parameter the outer
// rewrite introduces, so the whole nest is planned innermost-last and lands
// as one consistent set of edits.
func (a *analyzer) targetFromEnclosingRun(
	pass *analysis.Pass,
	stack []ast.Node,
	run qtCRun,
	qtAlias, testingName string,
) (runTarget, bool) {
	for i := len(stack) - 1; i > 0; i-- {
		lit, ok := stack[i].(*ast.FuncLit)
		if !ok {
			continue
		}
		outerCall, ok := stack[i-1].(*ast.CallExpr)
		if !ok {
			continue
		}
		outer, ok := matchQtCRun(pass, outerCall)
		if !ok || outer.lit != lit || outer.cObj == nil || outer.cObj != run.recvObj {
			continue
		}
		outerTarget, ok := a.resolveRunTarget(pass, stack[:i], outer, qtAlias, testingName)
		if !ok {
			return runTarget{}, false
		}
		return runTarget{
			recv:     freeName("t", takenNames(outer.lit, qtAlias, testingName)),
			unstable: outerTarget.unstable || closureUsesTestScopedMethod(pass, outer),
		}, true
	}
	return runTarget{}, false
}

// targetFromQtNew handles a receiver declared as c := qt.New(t).
func targetFromQtNew(pass *analysis.Pass, stack []ast.Node, run qtCRun) (runTarget, bool) {
	root := outermostFunc(stack)
	if root == nil {
		return runTarget{}, false
	}
	origin, ok := collectQtCOrigins(pass, root)[run.recvObj]
	if !ok {
		return runTarget{}, false
	}
	tIdent, ok := origin.arg.(*ast.Ident)
	if !ok {
		return runTarget{}, false
	}
	tObj := pass.TypesInfo.Uses[tIdent]
	if tObj == nil || !isTestingTPtr(tObj.Type()) {
		return runTarget{}, false
	}

	// The name has to still mean that object where the rewrite writes it.
	scope := pass.Pkg.Scope().Innermost(run.sel.Pos())
	if scope == nil {
		return runTarget{}, false
	}
	if _, found := scope.LookupParent(tIdent.Name, run.sel.Pos()); found != tObj {
		return runTarget{}, false
	}
	return runTarget{recv: tIdent.Name}, true
}

// unusedReceiverDeclEdits returns the edits that remove the receiver's
// declaration when the rewrite takes its last use, and no edits when it does
// not.
//
// It reports false when the declaration has to go but cannot be removed
// cleanly, because there is then no correct fix. The caller declines to
// report at all rather than offer one that does not compile.
func (a *analyzer) unusedReceiverDeclEdits(pass *analysis.Pass, stack []ast.Node, run qtCRun) ([]analysis.TextEdit, bool) {
	root := outermostFunc(stack)
	if root == nil {
		return nil, true
	}
	origin, ok := collectQtCOrigins(pass, root)[run.recvObj]
	if !ok || a.survivingUses(pass, root, run.recvObj) > 0 {
		return nil, true
	}
	start, end, ok := wholeLineSpan(pass, origin.decl)
	if !ok {
		return nil, false
	}
	return []analysis.TextEdit{{Pos: start, End: end}}, true
}

// survivingUses counts the references to obj within root that the rewrite
// leaves behind.
//
// A reference disappears when it is the receiver of a c.Run this rule
// rewrites. Under -only-stable-fixes a call whose closure uses a test-scoped
// *qt.C method keeps its fix withheld, so its receiver survives and the
// declaration must stay.
func (a *analyzer) survivingUses(pass *analysis.Pass, root ast.Node, obj types.Object) int {
	rewritten := make(map[*ast.Ident]bool)
	ast.Inspect(root, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		run, ok := matchQtCRun(pass, call)
		if !ok || run.recvObj != obj {
			return true
		}
		if a.onlyStableFixes && closureUsesTestScopedMethod(pass, run) {
			return true
		}
		rewritten[run.recv] = true
		return true
	})

	var count int
	ast.Inspect(root, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if ok && pass.TypesInfo.Uses[ident] == obj && !rewritten[ident] {
			count++
		}
		return true
	})
	return count
}

// closureUsesTestScopedMethod reports whether run's closure calls one of the
// *qt.C methods that bind to a test rather than to an assertion, on its own
// parameter.
func closureUsesTestScopedMethod(pass *analysis.Pass, run qtCRun) bool {
	if run.cObj == nil {
		return false
	}
	var found bool
	ast.Inspect(run.lit, func(n ast.Node) bool {
		if found {
			return false
		}
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || !testScopedCMethods[sel.Sel.Name] {
			return true
		}
		if ident, ok := sel.X.(*ast.Ident); ok && pass.TypesInfo.Uses[ident] == run.cObj {
			found = true
		}
		return true
	})
	return found
}

// newRunParam renders the closure's rewritten parameter, keeping the shape
// the author chose: an unnamed or blank *qt.C stays unnamed or blank, because
// nothing in the rewritten body would refer to it.
func newRunParam(run qtCRun, tName, testingName string) string {
	typeText := "*" + testingName + ".T"
	switch {
	case run.cName != "":
		return tName + " " + typeText
	case len(run.param.Names) == 1:
		return "_ " + typeText
	default:
		return typeText
	}
}

// takenNames returns every identifier appearing within lit, plus extra names
// the rewrite writes and must therefore not shadow.
func takenNames(lit *ast.FuncLit, extra ...string) map[string]bool {
	names := identNamesIn(lit)
	for _, name := range extra {
		names[name] = true
	}
	return names
}

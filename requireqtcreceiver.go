package qtlint

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// packageLevelAssertion describes a qt.Assert(t, …) or qt.Check(t, …) call
// whose first argument is an identifier of type *testing.T.
type packageLevelAssertion struct {
	// sel is the call's "qt.Assert" selector.
	sel *ast.SelectorExpr
	// method is "Assert" or "Check".
	method string
	// qtAlias is the name the file imports quicktest under.
	qtAlias string
	// tIdent is the *testing.T argument that would be dropped.
	tIdent *ast.Ident
	// tObj is the object tIdent refers to.
	tObj types.Object
}

// matchPackageLevelAssertion parses call into a packageLevelAssertion.
//
// The first argument must be an identifier of type *testing.T, because
// quicktest accepts any testing.TB there and a project may legitimately pass
// a *testing.B or a testing.TB obtained elsewhere. The rewrite this rule
// suggests has no defensible shape for those, so the rule stays out of them.
func matchPackageLevelAssertion(pass *analysis.Pass, call *ast.CallExpr) (packageLevelAssertion, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return packageLevelAssertion{}, false
	}

	method := sel.Sel.Name
	if method != "Assert" && method != "Check" {
		return packageLevelAssertion{}, false
	}
	if !isPackageQualified(pass, sel) {
		return packageLevelAssertion{}, false
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return packageLevelAssertion{}, false
	}

	// The package-level form is qt.Assert(t, got, checker, …); anything
	// shorter than "t, got" cannot be the shape we rewrite.
	if len(call.Args) < 2 {
		return packageLevelAssertion{}, false
	}
	tIdent, ok := call.Args[0].(*ast.Ident)
	if !ok {
		return packageLevelAssertion{}, false
	}
	tObj := pass.TypesInfo.Uses[tIdent]
	if tObj == nil || !isTestingTPtr(tObj.Type()) {
		return packageLevelAssertion{}, false
	}

	return packageLevelAssertion{
		sel:     sel,
		method:  method,
		qtAlias: pkgIdent.Name,
		tIdent:  tIdent,
		tObj:    tObj,
	}, true
}

// checkRequireQtCReceiver reports a package-level qt.Assert/qt.Check and
// suggests the *qt.C method form.
//
// The fix reuses a *qt.C that was created from the same *testing.T when one
// is visible, and otherwise creates one as the first statement of the
// function that binds that *testing.T — the function whose parameter list
// declares it, which is the subtest closure rather than the parent test when
// the assertion sits inside one.
func checkRequireQtCReceiver(pass *analysis.Pass, stack []ast.Node, call *ast.CallExpr) {
	m, ok := matchPackageLevelAssertion(pass, call)
	if !ok {
		return
	}

	// qt.New(t) can only be written where t is visible, so the function
	// binding t bounds both the search for an existing *qt.C and the place
	// a new one is created.
	binder, body := enclosingBinder(pass, stack, m.tObj)
	if body == nil {
		return
	}

	edits := make([]analysis.TextEdit, 0, 3)
	cName, reused := visibleQtCFrom(pass, body, m.tObj, call.Pos())
	if !reused {
		// The name is taken from the whole function, not just its body: a
		// receiver, parameter or named result shares the body's scope.
		cName = freeName("c", identNamesIn(binder))
		edits = append(edits, prependStmtEdit(pass, body,
			fmt.Sprintf("%s := %s.New(%s)", cName, m.qtAlias, m.tIdent.Name)))
	}

	// qt.Assert(t, got, …) becomes c.Assert(got, …): retarget the selector,
	// then drop the receiver argument along with the comma that follows it.
	edits = append(edits,
		analysis.TextEdit{
			Pos:     m.sel.X.Pos(),
			End:     m.sel.Sel.Pos(),
			NewText: []byte(cName + "."),
		},
		analysis.TextEdit{
			Pos: call.Args[0].Pos(),
			End: call.Args[1].Pos(),
		},
	)

	pass.Report(analysis.Diagnostic{
		Pos: call.Pos(),
		End: call.End(),
		Message: fmt.Sprintf("qtlint: use %s.%s(...) instead of %s.%s(%s, ...)",
			cName, m.method, m.qtAlias, m.method, m.tIdent.Name),
		SuggestedFixes: []analysis.SuggestedFix{{
			Message:   fmt.Sprintf("Replace with %s.%s", cName, m.method),
			TextEdits: edits,
		}},
	})
}

// visibleQtCFrom returns the name of a *qt.C that was created from tObj by a
// qt.New call inside body and is, under that name, the object that name
// resolves to at pos.
//
// The resolution check is what keeps the rewrite honest: a *qt.C named c in
// an enclosing test is not interchangeable with the t of a subtest closure,
// and reusing it would move the assertion onto the parent test.
func visibleQtCFrom(pass *analysis.Pass, body *ast.BlockStmt, tObj types.Object, pos token.Pos) (string, bool) {
	scope := pass.Pkg.Scope().Innermost(pos)
	if scope == nil {
		return "", false
	}

	var best types.Object
	for obj, origin := range collectQtCOrigins(pass, body) {
		ident, ok := origin.arg.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[ident] != tObj {
			continue
		}
		if _, found := scope.LookupParent(obj.Name(), pos); found != obj {
			continue
		}
		if best == nil || obj.Pos() < best.Pos() {
			best = obj
		}
	}
	if best == nil {
		return "", false
	}
	return best.Name(), true
}

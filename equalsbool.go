package qtlint

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// checkEqualsBoolPattern reports got, qt.Equals, want where exactly one of got
// and want is the predeclared true or false, and suggests qt.IsTrue or
// qt.IsFalse on the other operand.
//
// The rewrite never turns a passing assertion into a failing one. Equals
// passes only when the value is a bool holding the literal's value, and
// IsTrue/IsFalse pass for every value of bool kind holding it. The two differ
// where Equals always fails: on a named bool type, whose boxed value never
// equals the boxed literal, which is a bool. The message says so there.
//
// The other operand must be of bool kind, or an interface or a type parameter
// that may hold one. For any other type both checkers fail, and IsTrue only
// trades the failure for a bad check.
func checkEqualsBoolPattern(pass *analysis.Pass, call *ast.CallExpr) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}

	gotArgIndex := 0
	if isPackageQualified(pass, sel) {
		gotArgIndex = 1
	}
	checkerIndex := gotArgIndex + 1
	wantIndex := gotArgIndex + 2

	// A spread want... needs no guard of its own: the slice of wants is an
	// []any, which is neither a literal nor of bool kind.
	if len(call.Args) < wantIndex+1 {
		return
	}

	checkerArg := call.Args[checkerIndex]
	checkerSel, ok := checkerArg.(*ast.SelectorExpr)
	if !ok || checkerSel.Sel.Name != "Equals" || !isPackageQualified(pass, checkerSel) {
		return
	}
	pkgIdent, ok := checkerSel.X.(*ast.Ident)
	if !ok {
		return
	}

	gotArg, wantArg := call.Args[gotArgIndex], call.Args[wantIndex]
	gotLit, gotIsLit := boolLiteral(pass, gotArg)
	wantLit, wantIsLit := boolLiteral(pass, wantArg)
	if gotIsLit == wantIsLit {
		return
	}

	value, lit := gotArg, wantLit
	if gotIsLit {
		value, lit = wantArg, gotLit
	}

	t := pass.TypesInfo.TypeOf(value)
	if t == nil {
		return
	}
	named := false
	// A type parameter's underlying type is its constraint, so IsInterface
	// covers type parameters too.
	if !types.IsInterface(t) {
		basic, ok := t.Underlying().(*types.Basic)
		if !ok || basic.Info()&types.IsBoolean == 0 {
			return
		}
		named = !types.Identical(t, types.Typ[types.Bool])
	}

	checkerName := "IsFalse"
	if lit {
		checkerName = "IsTrue"
	}
	newChecker := pkgIdent.Name + "." + checkerName

	var pos token.Pos
	var message string
	var edits []analysis.TextEdit
	if wantIsLit {
		pos = checkerArg.Pos()
		message = fmt.Sprintf("qtlint: use qt.%s instead of qt.Equals, %t", checkerName, lit)
		edits = []analysis.TextEdit{{
			Pos:     checkerArg.Pos(),
			End:     wantArg.End(),
			NewText: []byte(newChecker),
		}}
	} else {
		// Drop the literal and the checker, then name the checker after the
		// value, which keeps the value's own source text.
		pos = gotArg.Pos()
		message = fmt.Sprintf("qtlint: use x, qt.%s instead of %t, qt.Equals, x", checkerName, lit)
		edits = []analysis.TextEdit{
			{Pos: gotArg.Pos(), End: wantArg.Pos()},
			{Pos: wantArg.End(), End: wantArg.End(), NewText: []byte(", " + newChecker)},
		}
	}
	if named {
		message += fmt.Sprintf(": %s is not bool, so qt.Equals always fails",
			types.TypeString(t, types.RelativeTo(pass.Pkg)))
	}

	pass.Report(analysis.Diagnostic{
		Pos:     pos,
		End:     wantArg.End(),
		Message: message,
		SuggestedFixes: []analysis.SuggestedFix{{
			Message:   "Replace with qt." + checkerName,
			TextEdits: edits,
		}},
	})
}

// boolLiteral reports whether expr is the predeclared true or false, and which.
// A local that shadows either name is not.
func boolLiteral(pass *analysis.Pass, expr ast.Expr) (value, ok bool) {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false, false
	}
	switch pass.TypesInfo.Uses[ident] {
	case types.Universe.Lookup("true"):
		return true, true
	case types.Universe.Lookup("false"):
		return false, true
	}
	return false, false
}

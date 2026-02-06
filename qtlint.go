// Package qtlint implements a static analysis tool that enforces
// best practices for using the frankban/quicktest testing library.
//
// The analyzer detects and reports suboptimal usage patterns, including:
//   - qt.Not(qt.IsNil) which should be replaced with qt.IsNotNil
//   - qt.Not(qt.IsTrue) which should be replaced with qt.IsFalse
//   - qt.Not(qt.IsFalse) which should be replaced with qt.IsTrue
//   - len(x), qt.Equals which should be replaced with x, qt.HasLen
//   - x == y, qt.IsTrue which should be replaced with x, qt.Equals, y
//   - x == nil, qt.IsTrue/qt.IsFalse which should be replaced with x, qt.IsNil/qt.IsNotNil
//   - x != nil, qt.IsTrue/qt.IsFalse which should be replaced with x, qt.IsNotNil/qt.IsNil
//
// This linter is designed to be used as a custom linter for golangci-lint.
package qtlint

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// NewAnalyzer creates a new instance of the qtlint analyzer.
func NewAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "qtlint",
		Doc:      "enforces best practices for quicktest usage",
		Run:      run,
		Requires: []*analysis.Analyzer{inspect.Analyzer},
	}
}

// Analyzer is the qtlint analyzer that enforces best practices
// for quicktest usage.
var Analyzer = NewAnalyzer()

// Replacement suggestions for qt.Not() patterns.
var replacements = map[string]string{
	"IsNil":   "IsNotNil",
	"IsTrue":  "IsFalse",
	"IsFalse": "IsTrue",
}

func run(pass *analysis.Pass) (any, error) {
	result := pass.ResultOf[inspect.Analyzer]
	insp, ok := result.(*inspector.Inspector)
	if !ok {
		return nil, nil
	}

	// Filter for call expressions
	nodeFilter := []ast.Node{
		(*ast.CallExpr)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return
		}
		checkQuicktestCall(pass, call)
	})

	return nil, nil
}

// checkQuicktestCall checks if a call expression is a quicktest assertion
// and validates the checker argument.
func checkQuicktestCall(pass *analysis.Pass, call *ast.CallExpr) {
	// Check if this is a call to Assert or Check
	if !isQuicktestAssertion(pass, call) {
		return
	}

	checkerArg := getCheckerArg(pass, call)
	if checkerArg == nil {
		return
	}

	// Check if the checker is qt.Not(...)
	checkNotPattern(pass, checkerArg)

	// Check for len(x), qt.Equals pattern
	checkLenEqualsPattern(pass, call)

	// Check for x == nil / x != nil with qt.IsTrue/qt.IsFalse pattern
	if checkNilComparisonPattern(pass, call) {
		return
	}

	// Check for x == y, qt.IsTrue pattern
	checkEqualityIsTruePattern(pass, call)
}

// getCheckerArg extracts the checker argument from a quicktest assertion call.
// Returns nil if the checker argument cannot be determined.
func getCheckerArg(pass *analysis.Pass, call *ast.CallExpr) ast.Expr {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}

	// qt.Assert(t, got, checker, ...) - checker is at index 2
	if isPackageQualified(pass, sel) {
		if len(call.Args) >= 3 {
			return call.Args[2]
		}
		return nil
	}

	// c.Assert(got, checker, ...) - checker is at index 1
	if len(call.Args) >= 2 {
		return call.Args[1]
	}
	return nil
}

// checkNotPattern checks if the checker is qt.Not() with a replaceable inner checker.
func checkNotPattern(pass *analysis.Pass, checkerArg ast.Expr) {
	notCall, ok := checkerArg.(*ast.CallExpr)
	if !ok {
		return
	}

	// Check if this is qt.Not
	sel, ok := notCall.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Not" {
		return
	}

	if !isPackageQualified(pass, sel) {
		return
	}

	// Check if qt.Not has an argument
	if len(notCall.Args) == 0 {
		return
	}

	innerChecker := notCall.Args[0]

	// Check if the inner checker is a selector (e.g., qt.IsNil)
	innerSel, ok := innerChecker.(*ast.SelectorExpr)
	if !ok {
		return
	}

	if !isPackageQualified(pass, innerSel) {
		return
	}

	// Check if we have a replacement for this pattern
	innerName := innerSel.Sel.Name
	replacement, ok := replacements[innerName]
	if !ok {
		return
	}

	// Get the package identifier (e.g., "qt" in qt.Not)
	pkgIdent, ok := innerSel.X.(*ast.Ident)
	if !ok {
		return
	}

	// Create the replacement text: qt.IsNotNil instead of qt.Not(qt.IsNil)
	newText := pkgIdent.Name + "." + replacement

	diagnostic := analysis.Diagnostic{
		Pos:     notCall.Pos(),
		End:     notCall.End(),
		Message: fmt.Sprintf("qtlint: use qt.%s instead of qt.Not(qt.%s)", replacement, innerName),
		SuggestedFixes: []analysis.SuggestedFix{
			{
				Message: fmt.Sprintf("Replace with qt.%s", replacement),
				TextEdits: []analysis.TextEdit{
					{
						Pos:     notCall.Pos(),
						End:     notCall.End(),
						NewText: []byte(newText),
					},
				},
			},
		},
	}
	pass.Report(diagnostic)
}

// isQuicktestAssertion checks if a call is to qt.Assert, qt.Check, c.Assert, or c.Check.
func isQuicktestAssertion(pass *analysis.Pass, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	name := sel.Sel.Name
	if name != "Assert" && name != "Check" {
		return false
	}

	// Check if it's qt.Assert/qt.Check or a method on *qt.C
	return isPackageQualified(pass, sel) || isQuicktestCMethod(pass, sel)
}

// isPackageQualified checks if a selector expression refers to a symbol in the quicktest package.
func isPackageQualified(pass *analysis.Pass, sel *ast.SelectorExpr) bool {
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}

	obj := pass.TypesInfo.Uses[ident]
	if obj == nil {
		return false
	}

	pkg, ok := obj.(*types.PkgName)
	if !ok {
		return false
	}

	return pkg.Imported().Path() == "github.com/frankban/quicktest"
}

// isQuicktestCMethod checks if a selector is a method on *qt.C.
func isQuicktestCMethod(pass *analysis.Pass, sel *ast.SelectorExpr) bool {
	t := pass.TypesInfo.TypeOf(sel.X)
	if t == nil {
		return false
	}

	// Check if the type is *qt.C or qt.C
	named, ok := t.(*types.Pointer)
	if ok {
		t = named.Elem()
	}

	namedType, ok := t.(*types.Named)
	if !ok {
		return false
	}

	obj := namedType.Obj()
	if obj == nil || obj.Pkg() == nil {
		return false
	}

	return obj.Pkg().Path() == "github.com/frankban/quicktest" && obj.Name() == "C"
}

// checkLenEqualsPattern checks if the pattern is len(x), qt.Equals and suggests x, qt.HasLen.
func checkLenEqualsPattern(pass *analysis.Pass, call *ast.CallExpr) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}

	// Determine the position of the "got" argument based on whether it's
	// qt.Assert(t, got, checker, ...) or c.Assert(got, checker, ...)
	var gotArgIndex int
	if isPackageQualified(pass, sel) {
		// qt.Assert(t, got, checker, ...) - got is at index 1
		gotArgIndex = 1
	} else {
		// c.Assert(got, checker, ...) - got is at index 0
		gotArgIndex = 0
	}

	// Make sure we have enough arguments
	if len(call.Args) < gotArgIndex+2 {
		return
	}

	gotArg := call.Args[gotArgIndex]
	checkerArg := call.Args[gotArgIndex+1]

	// Check if gotArg is a call to len()
	lenCall, ok := gotArg.(*ast.CallExpr)
	if !ok {
		return
	}

	lenIdent, ok := lenCall.Fun.(*ast.Ident)
	if !ok || lenIdent.Name != "len" {
		return
	}

	// Ensure we're dealing with the builtin len function, not a user-defined one.
	obj := pass.TypesInfo.Uses[lenIdent]
	if obj == nil {
		return
	}
	builtin, ok := obj.(*types.Builtin)
	if !ok || builtin.Name() != "len" {
		return
	}

	// Check if len() has exactly one argument
	if len(lenCall.Args) != 1 {
		return
	}

	// Check if the checker is qt.Equals
	checkerSel, ok := checkerArg.(*ast.SelectorExpr)
	if !ok {
		return
	}

	if checkerSel.Sel.Name != "Equals" {
		return
	}

	if !isPackageQualified(pass, checkerSel) {
		return
	}

	// Get the package identifier (e.g., "qt" in qt.Equals)
	pkgIdent, ok := checkerSel.X.(*ast.Ident)
	if !ok {
		return
	}

	// Get the argument to len()
	lenArg := lenCall.Args[0]

	// Create the replacement text by formatting the AST node
	var buf bytes.Buffer
	if err := format.Node(&buf, pass.Fset, lenArg); err != nil {
		return
	}
	newGotText := buf.String()
	newCheckerText := pkgIdent.Name + ".HasLen"

	diagnostic := analysis.Diagnostic{
		Pos:     gotArg.Pos(),
		End:     checkerArg.End(),
		Message: "qtlint: use qt.HasLen instead of len(x), qt.Equals",
		SuggestedFixes: []analysis.SuggestedFix{
			{
				Message: "Replace with qt.HasLen",
				TextEdits: []analysis.TextEdit{
					{
						Pos:     gotArg.Pos(),
						End:     gotArg.End(),
						NewText: []byte(newGotText),
					},
					{
						Pos:     checkerArg.Pos(),
						End:     checkerArg.End(),
						NewText: []byte(newCheckerText),
					},
				},
			},
		},
	}
	pass.Report(diagnostic)
}

// isNilIdent checks if an expression is the nil identifier.
func isNilIdent(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "nil"
}

// checkNilComparisonPattern checks if the pattern is x == nil / x != nil with
// qt.IsTrue or qt.IsFalse and suggests using qt.IsNil or qt.IsNotNil.
// Returns true if the pattern was matched (to skip further checks).
func checkNilComparisonPattern(pass *analysis.Pass, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	var gotArgIndex int
	if isPackageQualified(pass, sel) {
		gotArgIndex = 1
	} else {
		gotArgIndex = 0
	}

	if len(call.Args) < gotArgIndex+2 {
		return false
	}

	gotArg := call.Args[gotArgIndex]
	checkerArg := call.Args[gotArgIndex+1]

	// Check if gotArg is a binary == or != expression
	binExpr, ok := gotArg.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	if binExpr.Op != token.EQL && binExpr.Op != token.NEQ {
		return false
	}

	// Check if one side is nil
	var nonNilExpr ast.Expr
	if isNilIdent(binExpr.Y) {
		nonNilExpr = binExpr.X
	} else if isNilIdent(binExpr.X) {
		nonNilExpr = binExpr.Y
	} else {
		return false
	}

	// Check if the checker is qt.IsTrue or qt.IsFalse
	checkerSel, ok := checkerArg.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	checkerName := checkerSel.Sel.Name
	if checkerName != "IsTrue" && checkerName != "IsFalse" {
		return false
	}

	if !isPackageQualified(pass, checkerSel) {
		return false
	}

	pkgIdent, ok := checkerSel.X.(*ast.Ident)
	if !ok {
		return false
	}

	// Determine the replacement checker:
	// x == nil, qt.IsTrue  -> qt.IsNil
	// x == nil, qt.IsFalse -> qt.IsNotNil
	// x != nil, qt.IsTrue  -> qt.IsNotNil
	// x != nil, qt.IsFalse -> qt.IsNil
	isEq := binExpr.Op == token.EQL
	isTrue := checkerName == "IsTrue"
	var replacement string
	if isEq == isTrue {
		replacement = "IsNil"
	} else {
		replacement = "IsNotNil"
	}

	// Format the non-nil operand
	var buf bytes.Buffer
	if err := format.Node(&buf, pass.Fset, nonNilExpr); err != nil {
		return false
	}

	opStr := "=="
	if binExpr.Op == token.NEQ {
		opStr = "!="
	}

	newGotText := buf.String()
	newCheckerText := pkgIdent.Name + "." + replacement

	diagnostic := analysis.Diagnostic{
		Pos:     gotArg.Pos(),
		End:     checkerArg.End(),
		Message: fmt.Sprintf("qtlint: use qt.%s instead of x %s nil, qt.%s", replacement, opStr, checkerName),
		SuggestedFixes: []analysis.SuggestedFix{
			{
				Message: fmt.Sprintf("Replace with qt.%s", replacement),
				TextEdits: []analysis.TextEdit{
					{
						Pos:     gotArg.Pos(),
						End:     gotArg.End(),
						NewText: []byte(newGotText),
					},
					{
						Pos:     checkerArg.Pos(),
						End:     checkerArg.End(),
						NewText: []byte(newCheckerText),
					},
				},
			},
		},
	}
	pass.Report(diagnostic)
	return true
}

// checkEqualityIsTruePattern checks if the pattern is x == y, qt.IsTrue and suggests x, qt.Equals, y.
func checkEqualityIsTruePattern(pass *analysis.Pass, call *ast.CallExpr) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}

	// Determine the position of the "got" argument based on whether it's
	// qt.Assert(t, got, checker, ...) or c.Assert(got, checker, ...)
	var gotArgIndex int
	if isPackageQualified(pass, sel) {
		gotArgIndex = 1
	} else {
		gotArgIndex = 0
	}

	// Make sure we have enough arguments
	if len(call.Args) < gotArgIndex+2 {
		return
	}

	gotArg := call.Args[gotArgIndex]
	checkerArg := call.Args[gotArgIndex+1]

	// Check if gotArg is a binary == expression
	binExpr, ok := gotArg.(*ast.BinaryExpr)
	if !ok || binExpr.Op != token.EQL {
		return
	}

	// Check if the checker is qt.IsTrue
	checkerSel, ok := checkerArg.(*ast.SelectorExpr)
	if !ok {
		return
	}

	if checkerSel.Sel.Name != "IsTrue" {
		return
	}

	if !isPackageQualified(pass, checkerSel) {
		return
	}

	// Get the package identifier (e.g., "qt" in qt.IsTrue)
	pkgIdent, ok := checkerSel.X.(*ast.Ident)
	if !ok {
		return
	}

	// Format the left and right operands
	var lhsBuf bytes.Buffer
	if err := format.Node(&lhsBuf, pass.Fset, binExpr.X); err != nil {
		return
	}

	var rhsBuf bytes.Buffer
	if err := format.Node(&rhsBuf, pass.Fset, binExpr.Y); err != nil {
		return
	}

	newGotText := lhsBuf.String()
	newCheckerText := pkgIdent.Name + ".Equals, " + rhsBuf.String()

	diagnostic := analysis.Diagnostic{
		Pos:     gotArg.Pos(),
		End:     checkerArg.End(),
		Message: "qtlint: use qt.Equals instead of x == y, qt.IsTrue",
		SuggestedFixes: []analysis.SuggestedFix{
			{
				Message: "Replace with qt.Equals",
				TextEdits: []analysis.TextEdit{
					{
						Pos:     gotArg.Pos(),
						End:     gotArg.End(),
						NewText: []byte(newGotText),
					},
					{
						Pos:     checkerArg.Pos(),
						End:     checkerArg.End(),
						NewText: []byte(newCheckerText),
					},
				},
			},
		},
	}
	pass.Report(diagnostic)
}

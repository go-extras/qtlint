// Package qtlint implements a static analysis tool that enforces
// best practices for using the frankban/quicktest testing library.
//
// The analyzer detects and reports suboptimal usage patterns, including:
//   - qt.Not(qt.IsNil) which should be replaced with qt.IsNotNil
//   - qt.Not(qt.IsTrue) which should be replaced with qt.IsFalse
//   - qt.Not(qt.IsFalse) which should be replaced with qt.IsTrue
//
// This linter is designed to be used as a custom linter for golangci-lint.
package qtlint

import (
	"fmt"
	"go/ast"
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

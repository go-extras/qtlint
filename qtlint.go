// Package qtlint implements a static analysis tool that enforces
// best practices for using the frankban/quicktest testing library.
//
// The analyzer detects and reports suboptimal usage patterns, including:
//   - qt.Not(qt.IsNil) which should be replaced with qt.IsNotNil
//   - qt.Not(qt.IsTrue) which should be replaced with qt.IsFalse
//   - qt.Not(qt.IsFalse) which should be replaced with qt.IsTrue
//   - len(x), qt.Equals which should be replaced with x, qt.HasLen
//   - x == y, qt.IsTrue which should be replaced with x, qt.Equals, y
//   - x == y, qt.IsFalse which should be replaced with x, qt.Not(qt.Equals), y
//   - x != y, qt.IsTrue which should be replaced with x, qt.Not(qt.Equals), y
//   - x != y, qt.IsFalse which should be replaced with x, qt.Equals, y
//   - x == nil, qt.IsTrue/qt.IsFalse which should be replaced with x, qt.IsNil/qt.IsNotNil
//   - x != nil, qt.IsTrue/qt.IsFalse which should be replaced with x, qt.IsNotNil/qt.IsNil
//   - strings.Contains(x, y), qt.IsTrue which should be replaced with x, qt.Contains, y
//   - strings.Contains(x, y), qt.IsFalse which should be replaced with x, qt.Not(qt.Contains), y
//   - slices.Contains(x, y), qt.IsTrue which should be replaced with x, qt.Contains, y
//   - slices.Contains(x, y), qt.IsFalse which should be replaced with x, qt.Not(qt.Contains), y
//   - if err != nil { t.Fatal[f](...) } which should be replaced with c.Assert(err, qt.IsNil, qt.Commentf(...))
//   - if err != nil { t.Error[f](...) } which should be replaced with c.Check(err, qt.IsNil, qt.Commentf(...))
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

// analyzer is the receiver for analysis pass methods.
type analyzer struct{}

// NewAnalyzer creates a new instance of the qtlint analyzer.
func NewAnalyzer() *analysis.Analyzer {
	a := &analyzer{}
	return &analysis.Analyzer{
		Name:     "qtlint",
		Doc:      "enforces best practices for quicktest usage",
		Run:      a.run,
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

func (a *analyzer) run(pass *analysis.Pass) (any, error) {
	result := pass.ResultOf[inspect.Analyzer]
	insp, ok := result.(*inspector.Inspector)
	if !ok {
		return nil, nil
	}

	// Filter for nodes we want to inspect.
	nodeFilter := []ast.Node{
		(*ast.CallExpr)(nil),
		(*ast.IfStmt)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		switch n := n.(type) {
		case *ast.CallExpr:
			checkQuicktestCall(pass, n)
		case *ast.IfStmt:
			a.checkErrNilFatalPattern(pass, n)
		}
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

	// Check for x == y / x != y with qt.IsTrue/qt.IsFalse patterns
	checkEqualityComparisonPattern(pass, call)

	// Check for strings.Contains(x, y) or slices.Contains(x, y) with qt.IsTrue/qt.IsFalse pattern
	checkContainsPattern(pass, call)
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

// hasLenCheckerInfo holds the resolved information for a HasLen checker replacement.
type hasLenCheckerInfo struct {
	diagMessage    string
	fixMessage     string
	newCheckerText string
	editPos        token.Pos
	editEnd        token.Pos
}

// extractBuiltinLenArg returns the single argument of a builtin len() call, or
// (nil, false) if expr is not such a call.
func extractBuiltinLenArg(pass *analysis.Pass, expr ast.Expr) (ast.Expr, bool) {
	lenCall, ok := expr.(*ast.CallExpr)
	if !ok {
		return nil, false
	}
	lenIdent, ok := lenCall.Fun.(*ast.Ident)
	if !ok || lenIdent.Name != "len" {
		return nil, false
	}
	obj := pass.TypesInfo.Uses[lenIdent]
	if obj == nil {
		return nil, false
	}
	if _, ok := obj.(*types.Builtin); !ok {
		return nil, false
	}
	if len(lenCall.Args) != 1 {
		return nil, false
	}
	return lenCall.Args[0], true
}

// resolveHasLenChecker inspects checkerArg and returns a hasLenCheckerInfo when
// the checker is qt.Equals or qt.Not(qt.Equals).
func resolveHasLenChecker(pass *analysis.Pass, checkerArg ast.Expr) (hasLenCheckerInfo, bool) {
	switch checker := checkerArg.(type) {
	case *ast.SelectorExpr:
		if checker.Sel.Name != "Equals" || !isPackageQualified(pass, checker) {
			return hasLenCheckerInfo{}, false
		}
		pkgIdent, ok := checker.X.(*ast.Ident)
		if !ok {
			return hasLenCheckerInfo{}, false
		}
		return hasLenCheckerInfo{
			diagMessage:    "qtlint: use qt.HasLen instead of len(x), qt.Equals",
			fixMessage:     "Replace with qt.HasLen",
			newCheckerText: pkgIdent.Name + ".HasLen",
			editPos:        checkerArg.Pos(),
			editEnd:        checkerArg.End(),
		}, true

	case *ast.CallExpr:
		notSel, ok := checker.Fun.(*ast.SelectorExpr)
		if !ok || notSel.Sel.Name != "Not" || !isPackageQualified(pass, notSel) {
			return hasLenCheckerInfo{}, false
		}
		if len(checker.Args) != 1 {
			return hasLenCheckerInfo{}, false
		}
		innerSel, ok := checker.Args[0].(*ast.SelectorExpr)
		if !ok || innerSel.Sel.Name != "Equals" || !isPackageQualified(pass, innerSel) {
			return hasLenCheckerInfo{}, false
		}
		pkgIdent, ok := innerSel.X.(*ast.Ident)
		if !ok {
			return hasLenCheckerInfo{}, false
		}
		return hasLenCheckerInfo{
			diagMessage:    "qtlint: use qt.Not(qt.HasLen) instead of len(x), qt.Not(qt.Equals)",
			fixMessage:     "Replace with qt.Not(qt.HasLen)",
			newCheckerText: pkgIdent.Name + ".HasLen",
			editPos:        innerSel.Pos(),
			editEnd:        innerSel.End(),
		}, true

	default:
		return hasLenCheckerInfo{}, false
	}
}

// checkLenEqualsPattern checks if the pattern is len(x), qt.Equals or len(x), qt.Not(qt.Equals)
// and suggests using x, qt.HasLen (or its negation) instead.
func checkLenEqualsPattern(pass *analysis.Pass, call *ast.CallExpr) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}

	gotArgIndex := 0
	if isPackageQualified(pass, sel) {
		gotArgIndex = 1
	}
	if len(call.Args) < gotArgIndex+2 {
		return
	}

	gotArg := call.Args[gotArgIndex]
	checkerArg := call.Args[gotArgIndex+1]

	lenArg, ok := extractBuiltinLenArg(pass, gotArg)
	if !ok {
		return
	}

	newGotText, ok := formatExpr(pass, lenArg)
	if !ok {
		return
	}

	info, ok := resolveHasLenChecker(pass, checkerArg)
	if !ok {
		return
	}

	pass.Report(analysis.Diagnostic{
		Pos:     gotArg.Pos(),
		End:     checkerArg.End(),
		Message: info.diagMessage,
		SuggestedFixes: []analysis.SuggestedFix{
			{
				Message: info.fixMessage,
				TextEdits: []analysis.TextEdit{
					{
						Pos:     gotArg.Pos(),
						End:     gotArg.End(),
						NewText: []byte(newGotText),
					},
					{
						Pos:     info.editPos,
						End:     info.editEnd,
						NewText: []byte(info.newCheckerText),
					},
				},
			},
		},
	})
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
	switch {
	case isNilIdent(binExpr.Y):
		nonNilExpr = binExpr.X
	case isNilIdent(binExpr.X):
		nonNilExpr = binExpr.Y
	default:
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

// checkEqualityComparisonPattern checks if the pattern is x ==/!= y, qt.IsTrue/qt.IsFalse
// and suggests the appropriate qt.Equals or qt.Not(qt.Equals) replacement.
func checkEqualityComparisonPattern(pass *analysis.Pass, call *ast.CallExpr) {
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

	// Check if gotArg is a binary == or != expression
	binExpr, ok := gotArg.(*ast.BinaryExpr)
	if !ok {
		return
	}
	if binExpr.Op != token.EQL && binExpr.Op != token.NEQ {
		return
	}

	// Check if the checker is qt.IsTrue or qt.IsFalse
	checkerSel, ok := checkerArg.(*ast.SelectorExpr)
	if !ok {
		return
	}

	checkerName := checkerSel.Sel.Name
	if checkerName != "IsTrue" && checkerName != "IsFalse" {
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

	// Determine whether the result is Equals or Not(Equals):
	// x == y, qt.IsTrue  -> qt.Equals
	// x != y, qt.IsFalse -> qt.Equals
	// x == y, qt.IsFalse -> qt.Not(qt.Equals)
	// x != y, qt.IsTrue  -> qt.Not(qt.Equals)
	isEq := binExpr.Op == token.EQL
	isTrue := checkerName == "IsTrue"
	useEquals := isEq == isTrue

	opStr := "=="
	if binExpr.Op == token.NEQ {
		opStr = "!="
	}

	newGotText := lhsBuf.String()
	var newCheckerText, message, fixMessage string
	if useEquals {
		newCheckerText = pkgIdent.Name + ".Equals, " + rhsBuf.String()
		message = fmt.Sprintf("qtlint: use qt.Equals instead of x %s y, qt.%s", opStr, checkerName)
		fixMessage = "Replace with qt.Equals"
	} else {
		newCheckerText = pkgIdent.Name + ".Not(" + pkgIdent.Name + ".Equals), " + rhsBuf.String()
		message = fmt.Sprintf("qtlint: use qt.Not(qt.Equals) instead of x %s y, qt.%s", opStr, checkerName)
		fixMessage = "Replace with qt.Not(qt.Equals)"
	}

	diagnostic := analysis.Diagnostic{
		Pos:     gotArg.Pos(),
		End:     checkerArg.End(),
		Message: message,
		SuggestedFixes: []analysis.SuggestedFix{
			{
				Message: fixMessage,
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

// checkContainsPattern checks if the pattern is strings.Contains(x, y) or slices.Contains(x, y) with
// qt.IsTrue or qt.IsFalse and suggests using qt.Contains or qt.Not(qt.Contains).
func checkContainsPattern(pass *analysis.Pass, call *ast.CallExpr) {
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

	// Check if gotArg is a call to Contains
	containsCall, ok := gotArg.(*ast.CallExpr)
	if !ok {
		return
	}

	containsSel, ok := containsCall.Fun.(*ast.SelectorExpr)
	if !ok || containsSel.Sel.Name != "Contains" {
		return
	}

	// Check if it's from the strings or slices package
	pkgIdent, ok := containsSel.X.(*ast.Ident)
	if !ok {
		return
	}

	obj := pass.TypesInfo.Uses[pkgIdent]
	if obj == nil {
		return
	}

	pkgName, ok := obj.(*types.PkgName)
	if !ok {
		return
	}

	pkgPath := pkgName.Imported().Path()
	if pkgPath != "strings" && pkgPath != "slices" {
		return
	}

	// Check if Contains has exactly 2 arguments
	if len(containsCall.Args) != 2 {
		return
	}

	// Check if the checker is qt.IsTrue or qt.IsFalse
	checkerSel, ok := checkerArg.(*ast.SelectorExpr)
	if !ok {
		return
	}

	checkerName := checkerSel.Sel.Name
	if checkerName != "IsTrue" && checkerName != "IsFalse" {
		return
	}

	if !isPackageQualified(pass, checkerSel) {
		return
	}

	// Get the package identifier (e.g., "qt" in qt.IsTrue)
	qtPkgIdent, ok := checkerSel.X.(*ast.Ident)
	if !ok {
		return
	}

	// Format the arguments to Contains
	firstArg := containsCall.Args[0]
	secondArg := containsCall.Args[1]

	var firstBuf bytes.Buffer
	if err := format.Node(&firstBuf, pass.Fset, firstArg); err != nil {
		return
	}

	var secondBuf bytes.Buffer
	if err := format.Node(&secondBuf, pass.Fset, secondArg); err != nil {
		return
	}

	// Determine whether to use qt.Contains or qt.Not(qt.Contains)
	// strings.Contains(x, y), qt.IsTrue  -> x, qt.Contains, y
	// strings.Contains(x, y), qt.IsFalse -> x, qt.Not(qt.Contains), y
	// slices.Contains(x, y), qt.IsTrue   -> x, qt.Contains, y
	// slices.Contains(x, y), qt.IsFalse  -> x, qt.Not(qt.Contains), y
	useContains := checkerName == "IsTrue"

	newGotText := firstBuf.String()
	var newCheckerText, message, fixMessage string
	pkgNameStr := pkgIdent.Name // e.g., "strings" or "slices"

	if useContains {
		newCheckerText = qtPkgIdent.Name + ".Contains, " + secondBuf.String()
		message = fmt.Sprintf("qtlint: use qt.Contains instead of %s.Contains(x, y), qt.IsTrue", pkgNameStr)
		fixMessage = "Replace with qt.Contains"
	} else {
		newCheckerText = qtPkgIdent.Name + ".Not(" + qtPkgIdent.Name + ".Contains), " + secondBuf.String()
		message = fmt.Sprintf("qtlint: use qt.Not(qt.Contains) instead of %s.Contains(x, y), qt.IsFalse", pkgNameStr)
		fixMessage = "Replace with qt.Not(qt.Contains)"
	}

	diagnostic := analysis.Diagnostic{
		Pos:     gotArg.Pos(),
		End:     checkerArg.End(),
		Message: message,
		SuggestedFixes: []analysis.SuggestedFix{
			{
				Message: fixMessage,
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

func stripParens(expr ast.Expr) ast.Expr {
	for {
		p, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = p.X
	}
}

func formatNode(pass *analysis.Pass, node ast.Node) (string, bool) {
	var buf bytes.Buffer
	if err := format.Node(&buf, pass.Fset, node); err != nil {
		return "", false
	}
	return buf.String(), true
}

// formatExpr formats an AST expression node.
//
// It exists as a semantic wrapper around formatNode to clearly indicate
// that only ast.Expr values are expected at the call sites, even though
// it currently delegates directly to formatNode.
func formatExpr(pass *analysis.Pass, expr ast.Expr) (string, bool) {
	return formatNode(pass, expr)
}

func isErrorType(pass *analysis.Pass, expr ast.Expr) bool {
	t := pass.TypesInfo.TypeOf(expr)
	if t == nil {
		return false
	}

	errTypeObj := types.Universe.Lookup("error")
	if errTypeObj == nil {
		return false
	}
	errType := errTypeObj.Type()

	return types.AssignableTo(t, errType)
}

func findQuicktestPkgAlias(pass *analysis.Pass, start ast.Node) string {
	scope := pass.TypesInfo.Scopes[start]
	if scope == nil {
		return ""
	}
	for ; scope != nil; scope = scope.Parent() {
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			pkgName, ok := obj.(*types.PkgName)
			if !ok {
				continue
			}
			if pkgName.Imported().Path() == "github.com/frankban/quicktest" {
				return name
			}
		}
	}
	return ""
}

func isQuicktestCType(t types.Type) bool {
	if t == nil {
		return false
	}
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}

	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil {
		return false
	}
	return obj.Pkg().Path() == "github.com/frankban/quicktest" && obj.Name() == "C"
}

func findQuicktestCVarName(pass *analysis.Pass, start ast.Node) string {
	scope := pass.TypesInfo.Scopes[start]
	if scope == nil {
		return ""
	}
	for ; scope != nil; scope = scope.Parent() {
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			v, ok := obj.(*types.Var)
			if !ok {
				continue
			}
			if isQuicktestCType(v.Type()) {
				return name
			}
		}
	}
	return ""
}

// errNilFatalMatch holds the pattern parsed by matchErrNilFatal.
type errNilFatalMatch struct {
	errExpr    ast.Expr
	call       *ast.CallExpr
	sel        *ast.SelectorExpr
	methodName string // "Fatal", "Fatalf", "Error", or "Errorf"
	qtMethod   string // "Assert" or "Check"
}

// matchErrNilFatal validates and parses ifStmt into an errNilFatalMatch.
func matchErrNilFatal(pass *analysis.Pass, ifStmt *ast.IfStmt) (errNilFatalMatch, bool) {
	if ifStmt == nil || ifStmt.Else != nil {
		return errNilFatalMatch{}, false
	}

	cond := stripParens(ifStmt.Cond)
	binExpr, ok := cond.(*ast.BinaryExpr)
	if !ok || binExpr.Op != token.NEQ {
		return errNilFatalMatch{}, false
	}

	// Match: <expr> != nil (or nil != <expr>) where <expr> is assignable to error.
	var errExpr ast.Expr
	switch {
	case isNilIdent(binExpr.Y):
		errExpr = binExpr.X
	case isNilIdent(binExpr.X):
		errExpr = binExpr.Y
	default:
		return errNilFatalMatch{}, false
	}
	if !isErrorType(pass, errExpr) {
		return errNilFatalMatch{}, false
	}

	if len(ifStmt.Body.List) != 1 {
		return errNilFatalMatch{}, false
	}

	exprStmt, ok := ifStmt.Body.List[0].(*ast.ExprStmt)
	if !ok {
		return errNilFatalMatch{}, false
	}
	call, ok := exprStmt.X.(*ast.CallExpr)
	if !ok {
		return errNilFatalMatch{}, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return errNilFatalMatch{}, false
	}

	switch sel.Sel.Name {
	case "Fatal", "Fatalf":
		return errNilFatalMatch{errExpr, call, sel, sel.Sel.Name, "Assert"}, true
	case "Error", "Errorf":
		return errNilFatalMatch{errExpr, call, sel, sel.Sel.Name, "Check"}, true
	default:
		return errNilFatalMatch{}, false
	}
}

// isFromTestingPkg reports whether the method in s is defined in the testing package.
func isFromTestingPkg(s *types.Selection) bool {
	obj := s.Obj()
	return obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == "testing"
}

// errMismatch reports whether the call clearly uses a different error variable
// than errExpr in its direct (non-spread) arguments. When the call uses spread
// arguments we cannot trace into the slice, so we conservatively return false
// (i.e. no detected mismatch, allow the rule to fire).
func errMismatch(pass *analysis.Pass, errExpr ast.Expr, call *ast.CallExpr) bool {
	// Spread args: can't trace into the slice — assume no mismatch.
	if call.Ellipsis != token.NoPos {
		return false
	}

	// Only check when errExpr is a simple identifier with a resolved object.
	errIdent, ok := errExpr.(*ast.Ident)
	if !ok {
		return false
	}
	errObj := pass.TypesInfo.Uses[errIdent]
	if errObj == nil {
		return false
	}

	// Look for a direct arg that is an error-typed identifier distinct from errExpr.
	for _, arg := range call.Args {
		argIdent, ok := arg.(*ast.Ident)
		if !ok {
			continue
		}
		argObj := pass.TypesInfo.Uses[argIdent]
		if argObj == nil {
			continue
		}
		if isErrorType(pass, argIdent) && argObj != errObj {
			return true
		}
	}
	return false
}

func (*analyzer) checkErrNilFatalPattern(pass *analysis.Pass, ifStmt *ast.IfStmt) {
	m, ok := matchErrNilFatal(pass, ifStmt)
	if !ok {
		return
	}

	// Verify the method belongs to the testing package so we don't accidentally
	// flag calls to custom types that happen to have a Fatal/Error method.
	selection, selOk := pass.TypesInfo.Selections[m.sel]
	if !selOk || !isFromTestingPkg(selection) {
		return
	}

	// Don't fire when the call clearly uses a different error variable.
	if errMismatch(pass, m.errExpr, m.call) {
		return
	}

	// The scope attached to the body is a good starting point for finding visible names.
	startScopeNode := ast.Node(ifStmt.Body)
	qtAlias := findQuicktestPkgAlias(pass, startScopeNode)
	cVar := findQuicktestCVarName(pass, startScopeNode)
	if qtAlias == "" || cVar == "" {
		return
	}

	errText, ok := formatExpr(pass, m.errExpr)
	if !ok {
		return
	}

	receiverText, ok := formatExpr(pass, m.sel.X)
	if !ok {
		return
	}

	shortAssertText := fmt.Sprintf("%s.%s(%s, %s.IsNil)", cVar, m.qtMethod, errText, qtAlias)
	if len(m.call.Args) > 0 {
		shortAssertText = fmt.Sprintf("%s.%s(%s, %s.IsNil, %s.Commentf(...))", cVar, m.qtMethod, errText, qtAlias, qtAlias)
	}

	pass.Report(analysis.Diagnostic{
		Pos:     ifStmt.Pos(),
		End:     ifStmt.End(),
		Message: fmt.Sprintf("qtlint: use %s instead of %s.%s(...)", shortAssertText, receiverText, m.methodName),
	})
}

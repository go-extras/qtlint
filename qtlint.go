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
//   - errors.Is(err, target), qt.IsTrue which should be replaced with err, qt.ErrorIs, target
//   - errors.Is(err, target), qt.IsFalse which should be replaced with err, qt.Not(qt.ErrorIs), target
//   - errors.As(err, &target), qt.IsTrue which should be replaced with err, qt.ErrorAs, &target
//   - errors.As(err, &target), qt.IsFalse which should be replaced with err, qt.Not(qt.ErrorAs), &target
//   - if err != nil { t.Fatal[f](...) } which should be replaced with c.Assert(err, qt.IsNil, qt.Commentf(...))
//   - if err != nil { t.Error[f](...) } which should be replaced with c.Check(err, qt.IsNil, qt.Commentf(...))
//   - x, qt.Equals, nil which should be replaced with x, qt.IsNil
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
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// analyzer is the receiver for analysis pass methods.
type analyzer struct {
	// onlyStableFixes, when true, strips SuggestedFix from diagnostics whose
	// rewrite is best-effort and may change runtime semantics or output
	// formatting (e.g. synthesizing qt.Commentf for a multi-arg t.Fatal whose
	// args were joined by t.Fatal's Sprintln). The diagnostic is still
	// reported; only the auto-applicable fix is withheld.
	onlyStableFixes bool
}

// NewAnalyzer creates a new instance of the qtlint analyzer.
func NewAnalyzer() *analysis.Analyzer {
	a := &analyzer{}
	aa := &analysis.Analyzer{
		Name:     "qtlint",
		Doc:      "enforces best practices for quicktest usage",
		Run:      a.run,
		Requires: []*analysis.Analyzer{inspect.Analyzer},
	}
	aa.Flags.BoolVar(&a.onlyStableFixes, "only-stable-fixes", false,
		"emit SuggestedFix only for diagnostics whose rewrite is reliable; "+
			"best-effort fixes (e.g. errnil-fatal with non-literal format or "+
			"multi-arg non-formatted call) are reported without an auto-fix")
	return aa
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

	// Check for errors.Is(err, target) or errors.As(err, &target) with qt.IsTrue/qt.IsFalse pattern.
	checkErrorIsAsPattern(pass, call)

	// Check for x, qt.Equals, nil pattern.
	checkEqualsNilPattern(pass, call)
}

// checkEqualsNilPattern checks if the pattern is x, qt.Equals, nil and suggests
// using x, qt.IsNil instead. The quicktest Equals checker compares got and want
// with ==, so a typed nil (e.g. (*T)(nil)) never equals the untyped nil literal;
// only an untyped nil interface happens to pass. The qt.IsNil checker is the
// correct way to check for nil, as documented by quicktest itself.
func checkEqualsNilPattern(pass *analysis.Pass, call *ast.CallExpr) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}

	// Determine the position of the "got" argument based on whether it's
	// qt.Assert(t, got, checker, want, ...) or c.Assert(got, checker, want, ...).
	gotArgIndex := 0
	if isPackageQualified(pass, sel) {
		gotArgIndex = 1
	}

	checkerIndex := gotArgIndex + 1
	wantIndex := gotArgIndex + 2

	// Need at least got, checker, and want arguments.
	if len(call.Args) < wantIndex+1 {
		return
	}

	checkerArg := call.Args[checkerIndex]
	wantArg := call.Args[wantIndex]

	// The checker must be qt.Equals.
	checkerSel, ok := checkerArg.(*ast.SelectorExpr)
	if !ok || checkerSel.Sel.Name != "Equals" {
		return
	}
	if !isPackageQualified(pass, checkerSel) {
		return
	}

	// The want argument must be the nil identifier.
	if !isNilIdent(wantArg) {
		return
	}

	pkgIdent, ok := checkerSel.X.(*ast.Ident)
	if !ok {
		return
	}

	// Replace the "qt.Equals, nil" span with "qt.IsNil", dropping the want
	// argument. Any trailing arguments (e.g. qt.Commentf(...)) are preserved.
	newText := pkgIdent.Name + ".IsNil"

	pass.Report(analysis.Diagnostic{
		Pos:     checkerArg.Pos(),
		End:     wantArg.End(),
		Message: "qtlint: use qt.IsNil instead of qt.Equals, nil",
		SuggestedFixes: []analysis.SuggestedFix{
			{
				Message: "Replace with qt.IsNil",
				TextEdits: []analysis.TextEdit{
					{
						Pos:     checkerArg.Pos(),
						End:     wantArg.End(),
						NewText: []byte(newText),
					},
				},
			},
		},
	})
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

// checkErrorIsAsPattern checks if the pattern is errors.Is(err, target) or
// errors.As(err, &target) with qt.IsTrue or qt.IsFalse and suggests using
// qt.ErrorIs / qt.ErrorAs (or their negations via qt.Not).
func checkErrorIsAsPattern(pass *analysis.Pass, call *ast.CallExpr) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}

	// Determine the position of the "got" argument based on whether it's
	// qt.Assert(t, got, checker, ...) or c.Assert(got, checker, ...).
	gotArgIndex := 0
	if isPackageQualified(pass, sel) {
		gotArgIndex = 1
	}

	if len(call.Args) < gotArgIndex+2 {
		return
	}

	gotArg := call.Args[gotArgIndex]
	checkerArg := call.Args[gotArgIndex+1]

	// gotArg must be a call to errors.Is or errors.As.
	fnCall, ok := gotArg.(*ast.CallExpr)
	if !ok {
		return
	}

	fnSel, ok := fnCall.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}

	var newCheckerName string
	switch fnSel.Sel.Name {
	case "Is":
		newCheckerName = "ErrorIs"
	case "As":
		newCheckerName = "ErrorAs"
	default:
		return
	}

	// Verify the receiver refers to the standard "errors" package.
	pkgIdent, ok := fnSel.X.(*ast.Ident)
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
	if pkgName.Imported().Path() != "errors" {
		return
	}

	// errors.Is/As take exactly two arguments.
	if len(fnCall.Args) != 2 {
		return
	}

	// The checker must be qt.IsTrue or qt.IsFalse.
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
	qtPkgIdent, ok := checkerSel.X.(*ast.Ident)
	if !ok {
		return
	}

	firstText, ok := formatExpr(pass, fnCall.Args[0])
	if !ok {
		return
	}
	secondText, ok := formatExpr(pass, fnCall.Args[1])
	if !ok {
		return
	}

	// errors.Is/As(...), qt.IsTrue  -> err, qt.ErrorIs/ErrorAs, target
	// errors.Is/As(...), qt.IsFalse -> err, qt.Not(qt.ErrorIs/ErrorAs), target
	usePositive := checkerName == "IsTrue"
	pkgNameStr := pkgIdent.Name // e.g., "errors" or an alias

	var newCheckerText, message, fixMessage string
	if usePositive {
		newCheckerText = qtPkgIdent.Name + "." + newCheckerName + ", " + secondText
		message = fmt.Sprintf("qtlint: use qt.%s instead of %s.%s(err, target), qt.IsTrue",
			newCheckerName, pkgNameStr, fnSel.Sel.Name)
		fixMessage = fmt.Sprintf("Replace with qt.%s", newCheckerName)
	} else {
		newCheckerText = qtPkgIdent.Name + ".Not(" + qtPkgIdent.Name + "." + newCheckerName + "), " + secondText
		message = fmt.Sprintf("qtlint: use qt.Not(qt.%s) instead of %s.%s(err, target), qt.IsFalse",
			newCheckerName, pkgNameStr, fnSel.Sel.Name)
		fixMessage = fmt.Sprintf("Replace with qt.Not(qt.%s)", newCheckerName)
	}

	pass.Report(analysis.Diagnostic{
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
						NewText: []byte(firstText),
					},
					{
						Pos:     checkerArg.Pos(),
						End:     checkerArg.End(),
						NewText: []byte(newCheckerText),
					},
				},
			},
		},
	})
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

func (a *analyzer) checkErrNilFatalPattern(pass *analysis.Pass, ifStmt *ast.IfStmt) {
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

	diag := analysis.Diagnostic{
		Pos:     ifStmt.Pos(),
		End:     ifStmt.End(),
		Message: fmt.Sprintf("qtlint: use %s instead of %s.%s(...)", shortAssertText, receiverText, m.methodName),
	}

	if fix, ok := buildErrNilFatalFix(pass, ifStmt, m, cVar, qtAlias, errText); ok {
		if fix.stable || !a.onlyStableFixes {
			diag.SuggestedFixes = []analysis.SuggestedFix{{
				Message: fix.message,
				TextEdits: []analysis.TextEdit{{
					Pos:     ifStmt.Pos(),
					End:     ifStmt.End(),
					NewText: []byte(fix.text),
				}},
			}}
		}
	}

	pass.Report(diag)
}

// errNilFatalFix describes a single-statement rewrite for the
// `if err != nil { t.Fatal[f]/t.Error[f](...) }` pattern.
type errNilFatalFix struct {
	// text is the replacement source for the entire ifStmt span. It is a
	// single line of code (no indentation prefix, no trailing newline);
	// the surrounding whitespace in the file is preserved by the TextEdit.
	text string
	// message is shown to users in IDE/golangci-lint when offering the fix.
	message string
	// stable is true when the rewrite preserves the original call's
	// semantics exactly (modulo the trailing newline that t.Fatal's Sprintln
	// added). Unstable fixes synthesize a format string or wrap a
	// non-error argument and are skipped under -only-stable-fixes.
	stable bool
}

// buildErrNilFatalFix returns a rewrite for the if-err-fatal pattern, or
// ok=false when we cannot safely produce one (init-stmt scoping or spread args).
func buildErrNilFatalFix(
	pass *analysis.Pass,
	ifStmt *ast.IfStmt,
	m errNilFatalMatch,
	cVar, qtAlias, errText string,
) (errNilFatalFix, bool) {
	// `if err := f(); err != nil { ... }` would require pulling the init
	// statement out, which changes scoping (err leaks into the enclosing
	// block). Refuse the fix and let the diagnostic stand on its own.
	if ifStmt.Init != nil {
		return errNilFatalFix{}, false
	}
	// `t.Fatal(args...)` (spread): we cannot enumerate the underlying
	// arguments at lint time, so any rewrite would silently drop them.
	if m.call.Ellipsis != token.NoPos {
		return errNilFatalFix{}, false
	}

	bare := fmt.Sprintf("%s.%s(%s, %s.IsNil)", cVar, m.qtMethod, errText, qtAlias)
	withComment := func(commentArgs string) string {
		return fmt.Sprintf("%s.%s(%s, %s.IsNil, %s.Commentf(%s))",
			cVar, m.qtMethod, errText, qtAlias, qtAlias, commentArgs)
	}

	isFmtVariant := m.methodName == "Fatalf" || m.methodName == "Errorf"

	if isFmtVariant {
		// Fatalf/Errorf require a format argument; defensively refuse if absent.
		if len(m.call.Args) == 0 {
			return errNilFatalFix{}, false
		}
		argTexts, ok := formatArgs(pass, m.call.Args)
		if !ok {
			return errNilFatalFix{}, false
		}
		_, formatIsLiteral := m.call.Args[0].(*ast.BasicLit)
		return errNilFatalFix{
			text:    withComment(strings.Join(argTexts, ", ")),
			message: fmt.Sprintf("Replace with %s.%s(..., qt.Commentf(...))", cVar, m.qtMethod),
			// A literal format string round-trips through fmt.Sprintf inside
			// Commentf identically. A non-literal could be anything (a const
			// alias, a function call), so we mark the rewrite unstable.
			stable: formatIsLiteral,
		}, true
	}

	// Non-formatted Fatal/Error: zero args is a clean rewrite to bare assert.
	if len(m.call.Args) == 0 {
		return errNilFatalFix{
			text:    bare,
			message: fmt.Sprintf("Replace with %s.%s(%s, %s.IsNil)", cVar, m.qtMethod, errText, qtAlias),
			stable:  true,
		}, true
	}

	// Single arg: when it's the same err identifier, drop it entirely (clean).
	// Otherwise wrap it via qt.Commentf("%v", arg) — semantically close to
	// t.Fatal's Sprintln output but loses the trailing newline, so unstable.
	if len(m.call.Args) == 1 {
		argText, ok := formatExpr(pass, m.call.Args[0])
		if !ok {
			return errNilFatalFix{}, false
		}
		if argText == errText {
			return errNilFatalFix{
				text:    bare,
				message: fmt.Sprintf("Replace with %s.%s(%s, %s.IsNil)", cVar, m.qtMethod, errText, qtAlias),
				stable:  true,
			}, true
		}
		// Marked unstable to stay consistent with the multi-arg synthesis below:
		// both wrap arguments in a synthesized Commentf format and both drop the
		// trailing newline that Sprintln would have produced. The format is
		// fixed ("%v") so user-supplied "%" characters in arg pass through
		// verbatim — there is no format-string injection risk here.
		return errNilFatalFix{
			text:    withComment(`"%v", ` + argText),
			message: fmt.Sprintf("Replace with %s.%s(..., qt.Commentf(\"%%v\", ...))", cVar, m.qtMethod),
			stable:  false,
		}, true
	}

	// Multi-arg non-formatted: approximate Sprintln join with "%v %v ..." in
	// Commentf.
	//
	// Behavioral gap to be aware of: t.Fatal(a, b, c) uses fmt.Sprintln which
	// joins args with spaces *and appends \n*; qt.Commentf("%v %v %v", a, b, c)
	// uses fmt.Sprintf which honors the format string and produces no trailing
	// newline. We do not reproduce the newline because qt.Commentf is lazy —
	// it's only formatted on assertion failure and most consumers will not
	// notice the trailing newline either way. This is the canonical reason
	// this branch's fix is classified unstable: the failure-message text is
	// nearly but not exactly identical to the original t.Fatal output.
	argTexts, ok := formatArgs(pass, m.call.Args)
	if !ok {
		return errNilFatalFix{}, false
	}
	placeholders := strings.TrimRight(strings.Repeat("%v ", len(m.call.Args)), " ")
	commentArgs := strconv.Quote(placeholders) + ", " + strings.Join(argTexts, ", ")
	return errNilFatalFix{
		text:    withComment(commentArgs),
		message: fmt.Sprintf("Replace with %s.%s(..., qt.Commentf(\"%%v ...\", ...))", cVar, m.qtMethod),
		stable:  false,
	}, true
}

// formatArgs formats a slice of AST expressions, returning ok=false if any
// expression cannot be rendered (e.g. token positions out of range).
func formatArgs(pass *analysis.Pass, exprs []ast.Expr) ([]string, bool) {
	out := make([]string, 0, len(exprs))
	for _, e := range exprs {
		s, ok := formatExpr(pass, e)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

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
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// analyzer holds per-instance configuration flags.
type analyzer struct {
	fixErrNil bool
}

// NewAnalyzer creates a new instance of the qtlint analyzer.
func NewAnalyzer() *analysis.Analyzer {
	a := &analyzer{}
	an := &analysis.Analyzer{
		Name:     "qtlint",
		Doc:      "enforces best practices for quicktest usage",
		Run:      a.run,
		Requires: []*analysis.Analyzer{inspect.Analyzer},
	}
	an.Flags.BoolVar(&a.fixErrNil, "fix-err-nil", false,
		"enable auto-fix suggestions for if err != nil { t.Fatal/Error } patterns")
	return an
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

func fileForPos(pass *analysis.Pass, pos token.Pos) *ast.File {
	for _, f := range pass.Files {
		if f.Pos() <= pos && pos <= f.End() {
			return f
		}
	}
	return nil
}

func hasImport(file *ast.File, importPath string) bool {
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if path == importPath {
			return true
		}
	}
	return false
}

func addImportTextEdit(file *ast.File, importPath string) (analysis.TextEdit, bool) {
	if file == nil {
		return analysis.TextEdit{}, false
	}

	// Prefer adding to an existing parenthesized import block so gofmt can sort it
	// and we don't create a second import declaration.
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.IMPORT {
			continue
		}
		if genDecl.Lparen != token.NoPos && genDecl.Rparen != token.NoPos {
			// Insert at the start of the import block so it ends up in the first group
			// (usually stdlib), and gofmt can sort it.
			if len(genDecl.Specs) == 0 {
				break
			}
			insertPos := genDecl.Specs[0].Pos()
			newText := []byte("\t" + strconv.Quote(importPath) + "\n")
			return analysis.TextEdit{Pos: insertPos, End: insertPos, NewText: newText}, true
		}
	}

	// Fallback: insert a new import decl after the package clause.
	if file.Name == nil {
		return analysis.TextEdit{}, false
	}
	insertPos := file.Name.End()
	newText := []byte("\n\nimport " + strconv.Quote(importPath) + "\n")
	return analysis.TextEdit{Pos: insertPos, End: insertPos, NewText: newText}, true
}

// errNilFatalMatch holds the parts of an "if err != nil { t.Fatal/Error(...) }"
// pattern parsed by matchErrNilFatal.
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

func (a *analyzer) checkErrNilFatalPattern(pass *analysis.Pass, ifStmt *ast.IfStmt) {
	m, ok := matchErrNilFatal(pass, ifStmt)
	if !ok {
		return
	}

	// Verify the method belongs to the testing package so we don't accidentally
	// rewrite calls to custom types that happen to have a Fatal/Error method.
	selection, selOk := pass.TypesInfo.Selections[m.sel]
	if !selOk || !isFromTestingPkg(selection) {
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

	args, ok := formatCallArgs(pass, m.call)
	if !ok {
		return
	}

	var commentText string
	var needsFmtImport bool
	if m.call.Ellipsis != token.NoPos {
		var fmtAlias string
		if f := fileForPos(pass, ifStmt.Pos()); f != nil {
			fmtAlias = fmtPkgAlias(f)
		}
		commentText, needsFmtImport = buildSpreadCommentf(qtAlias, fmtAlias, m.methodName, args)
	} else {
		commentText = buildDirectCommentf(qtAlias, m.methodName, args)
	}

	assertStmtText := fmt.Sprintf("%s.%s(%s, %s.IsNil", cVar, m.qtMethod, errText, qtAlias)
	shortAssertText := fmt.Sprintf("%s.%s(%s, %s.IsNil)", cVar, m.qtMethod, errText, qtAlias)
	if commentText != "" {
		assertStmtText += ", " + commentText
		shortAssertText = fmt.Sprintf("%s.%s(%s, %s.IsNil, %s.Commentf(...))", cVar, m.qtMethod, errText, qtAlias, qtAlias)
	}
	assertStmtText += ")"

	newStmtText := assertStmtText
	if ifStmt.Init != nil {
		initText, ok := formatNode(pass, ifStmt.Init)
		if !ok {
			return
		}
		// Keep the init variables scoped like they are in the if statement
		// by using an explicit block.
		newStmtText = "{\n" + initText + "\n" + assertStmtText + "\n}"
	}

	textEdits := []analysis.TextEdit{{
		Pos:     ifStmt.Pos(),
		End:     ifStmt.End(),
		NewText: []byte(newStmtText),
	}}
	if needsFmtImport {
		textEdits = appendFmtImportEdit(pass, ifStmt.Pos(), textEdits)
	}

	var fixes []analysis.SuggestedFix
	if a.fixErrNil {
		fixes = []analysis.SuggestedFix{{
			Message:   fmt.Sprintf("Replace with %s", shortAssertText),
			TextEdits: textEdits,
		}}
	}
	pass.Report(analysis.Diagnostic{
		Pos:            ifStmt.Pos(),
		End:            ifStmt.End(),
		Message:        fmt.Sprintf("qtlint: use %s instead of %s.%s(...)", shortAssertText, receiverText, m.methodName),
		SuggestedFixes: fixes,
	})
}

// formatCallArgs formats each argument of call as source text, appending "..."
// to the last element when the call uses an ellipsis. Returns false if any
// argument cannot be formatted.
func formatCallArgs(pass *analysis.Pass, call *ast.CallExpr) ([]string, bool) {
	args := make([]string, 0, len(call.Args))
	for _, arg := range call.Args {
		text, ok := formatExpr(pass, arg)
		if !ok {
			return nil, false
		}
		args = append(args, text)
	}
	if call.Ellipsis != token.NoPos && len(args) > 0 {
		args[len(args)-1] += "..."
	}
	return args, true
}

// fmtPkgAlias returns the local identifier used to access the "fmt" package
// in file, or an empty string if "fmt" is not imported or uses a blank/dot import.
func fmtPkgAlias(file *ast.File) string {
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != "fmt" {
			continue
		}
		if imp.Name == nil {
			return "fmt"
		}
		if imp.Name.Name != "_" && imp.Name.Name != "." {
			return imp.Name.Name
		}
		return ""
	}
	return ""
}

// buildSpreadCommentf returns the Commentf option text for a spread call
// and reports that a "fmt" import needs to be added.
// Pass an empty fmtAlias when fmt is not yet imported; "fmt" will be used as the identifier.
func buildSpreadCommentf(qtAlias, fmtAlias, methodName string, args []string) (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	needsFmt := fmtAlias == ""
	if needsFmt {
		fmtAlias = "fmt"
	}
	joined := strings.Join(args, ", ")
	switch methodName {
	case "Fatalf", "Errorf":
		// Spread: pre-render the message so we don't repeat the format string
		// with unknown arity directly in Commentf.
		return fmt.Sprintf("%s.Commentf(%s.Sprintf(%s))", qtAlias, fmtAlias, joined), needsFmt
	default: // Fatal, Error
		// Collect all args into a single string via fmt.Sprint.
		return fmt.Sprintf("%s.Commentf(%s.Sprint(%s))", qtAlias, fmtAlias, joined), needsFmt
	}
}

// buildDirectCommentf returns the Commentf option text for a non-spread call.
func buildDirectCommentf(qtAlias, methodName string, args []string) string {
	if len(args) == 0 {
		return ""
	}
	joined := strings.Join(args, ", ")
	switch methodName {
	case "Fatalf", "Errorf":
		// Pass through format+args unchanged.
		return fmt.Sprintf("%s.Commentf(%s)", qtAlias, joined)
	default: // Fatal, Error
		// Generate a format string matching the number of args.
		formatString := strings.TrimSpace(strings.Repeat("%v ", len(args)))
		return fmt.Sprintf("%s.Commentf(%s, %s)", qtAlias, strconv.Quote(formatString), joined)
	}
}

// appendFmtImportEdit appends a text edit that adds a "fmt" import to the file
// containing pos, if that import is not already present.
func appendFmtImportEdit(pass *analysis.Pass, pos token.Pos, edits []analysis.TextEdit) []analysis.TextEdit {
	file := fileForPos(pass, pos)
	if file == nil || hasImport(file, "fmt") {
		return edits
	}
	if edit, ok := addImportTextEdit(file, "fmt"); ok {
		edits = append(edits, edit)
	}
	return edits
}

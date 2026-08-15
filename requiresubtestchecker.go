package qtlint

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"

	"golang.org/x/tools/go/analysis"
)

// subtestClosure describes a t.Run, b.Run or f.Fuzz closure together with the
// test handle it is given.
type subtestClosure struct {
	// lit is the closure and param its *testing.T parameter.
	lit   *ast.FuncLit
	param *ast.Field
	// handle is the parameter's name. It is empty when the parameter is blank
	// or unnamed, and the closure then has no handle to build a checker from.
	handle string
}

// matchSubtestClosure parses call into a subtestClosure.
//
// The receiver is not required to resolve to a testing.TB. A custom runner
// that takes a func(*testing.T) hands the closure a test handle whatever the
// receiver's type, and requiring the receiver to type-check silences real
// violations wherever it is a struct field, an embedded wrapper or a call
// result — shapes a single-file pass cannot see through. What the rule needs
// is the closure's own handle, and the signature is what carries that.
func matchSubtestClosure(pass *analysis.Pass, call *ast.CallExpr) (subtestClosure, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return subtestClosure{}, false
	}
	switch sel.Sel.Name {
	case "Run", "Fuzz":
	default:
		return subtestClosure{}, false
	}
	if len(call.Args) == 0 {
		return subtestClosure{}, false
	}

	lit, ok := call.Args[len(call.Args)-1].(*ast.FuncLit)
	if !ok {
		return subtestClosure{}, false
	}
	params := lit.Type.Params
	if params == nil || len(params.List) == 0 {
		return subtestClosure{}, false
	}
	param := params.List[0]
	if len(param.Names) > 1 || !isTestingTPtr(pass.TypesInfo.TypeOf(param.Type)) {
		return subtestClosure{}, false
	}

	closure := subtestClosure{lit: lit, param: param}
	if len(param.Names) == 1 && param.Names[0].Name != "_" {
		closure.handle = param.Names[0].Name
	}
	return closure, true
}

// checkRequireSubtestChecker reports a subtest closure that asserts through a
// *qt.C belonging to the test around it.
//
// The shape compiles and passes, which is what makes it worth a rule. A
// checker reports against whichever test it was built from, so a subtest
// asserting through its parent's checker names the parent in the failure and,
// on Assert, stops the parent rather than the subtest. Neither of the other
// two rules sees it: there is no c.Run to rewrite and the assertion already
// goes through a receiver.
func (*analyzer) checkRequireSubtestChecker(pass *analysis.Pass) {
	for _, file := range pass.Files {
		qtAlias := importedPkgName(pass, file, quicktestPkgPath)
		for _, root := range outermostFuncs(file) {
			planBorrowedCheckers(pass, root, qtAlias)
		}
	}
}

// borrowSite is one subtest closure that reads a checker from outside itself.
type borrowSite struct {
	closure subtestClosure
	name    string
	obj     types.Object
	reason  string
	fixable bool
}

// planBorrowedCheckers decides every borrowing closure in one outermost
// function together, then reports them.
//
// The decisions are not independent. Giving a closure its own checker shadows
// the outer name, so the outer declaration loses that use — and when the
// closures were its only readers, the declaration has to go with them or the
// function stops compiling. Answering that per closure cannot see the others.
func planBorrowedCheckers(pass *analysis.Pass, root ast.Node, qtAlias string) {
	var closures []subtestClosure
	ast.Inspect(root, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if closure, ok := matchSubtestClosure(pass, call); ok {
			closures = append(closures, closure)
		}
		return true
	})
	if len(closures) == 0 {
		return
	}

	sites := borrowSites(pass, root, closures, qtAlias)
	if len(sites) == 0 {
		return
	}

	// A declaration whose every reader is a closure being fixed goes with
	// them. wholeLineSpan declines a declaration sharing its line with
	// anything else, and the sites that depended on it lose their fix.
	declEdits := make(map[types.Object][]analysis.TextEdit)
	for obj := range borrowedObjects(sites) {
		if survivingBorrowUses(pass, root, obj, sites) > 0 {
			continue
		}
		edits, reason := declRemovalEdits(pass, root, obj)
		if reason != "" {
			for _, s := range sites {
				if s.obj == obj {
					s.fixable = false
					s.reason = reason
				}
			}
			continue
		}
		declEdits[obj] = edits
	}

	for _, s := range sites {
		diag := analysis.Diagnostic{
			Pos:     s.closure.lit.Type.Pos(),
			End:     s.closure.lit.Type.End(),
			Message: "qtlint: this subtest asserts through a *qt.C built from the test around it, so a failure names that test instead" + s.reason,
		}
		if s.fixable {
			edits := []analysis.TextEdit{prependStmtEdit(pass, s.closure.lit.Body,
				fmt.Sprintf("%s := %s.New(%s)", s.name, qtAlias, s.closure.handle))}
			diag.SuggestedFixes = []analysis.SuggestedFix{{
				Message:   "Give the subtest its own qt.New",
				TextEdits: append(edits, declEdits[s.obj]...),
			}}
		}
		pass.Report(diag)
	}
}

// borrowSites attributes every borrowed read to the innermost subtest closure
// that contains it.
//
// Attribution has to be innermost-wins, and the shape that shows why is a
// subtest whose only assertion sits inside a nested subtest. Reading the outer
// closure's body finds that assertion too, so charging the borrow to both
// closures inserts a checker into the outer one that nothing then reads —
// which does not compile. The innermost subtest is also the one whose failure
// attribution is actually wrong, so it is the one that needs its own checker.
func borrowSites(pass *analysis.Pass, root ast.Node, closures []subtestClosure, qtAlias string) []*borrowSite {
	type key struct {
		lit *ast.FuncLit
		obj types.Object
	}
	seen := make(map[key]string)

	ast.Inspect(root, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		// A variable, not a type name: the C in *qt.C is an identifier whose
		// object is the type itself, and reading it is a type reference rather
		// than an assertion through a checker.
		v, ok := pass.TypesInfo.Uses[ident].(*types.Var)
		if !ok || !isQuicktestCType(v.Type()) {
			return true
		}
		obj := types.Object(v)
		// A package-level checker is not a parent's checker, and shadowing one
		// would change what the test asserts through rather than which test it
		// reports against.
		if obj.Parent() == pass.Pkg.Scope() {
			return true
		}
		lit := innermostClosureAt(closures, ident.Pos())
		if lit == nil {
			return true
		}
		// A checker the closure declares itself is the conforming shape,
		// however it spells the name.
		if declaredWithin(lit, obj) {
			return true
		}
		seen[key{lit: lit, obj: obj}] = ident.Name
		return true
	})

	byLit := make(map[*ast.FuncLit]subtestClosure)
	for _, closure := range closures {
		byLit[closure.lit] = closure
	}

	counts := make(map[*ast.FuncLit]int)
	for k := range seen {
		counts[k.lit]++
	}

	sites := make([]*borrowSite, 0, len(seen))
	for k, name := range seen {
		closure := byLit[k.lit]
		reason := ""
		switch {
		case closure.handle == "":
			reason = "; no fix: the closure's *testing.T is blank, so there is no handle to build a checker from"
		case qtAlias == "":
			reason = "; no fix: this file does not import quicktest under a name the inserted qt.New could use"
		case !packageQualifies(pass, qtAlias, quicktestPkgPath, closure.lit.Body.Lbrace):
			reason = "; no fix: the quicktest qualifier does not mean quicktest where the qt.New would go"
		case counts[k.lit] > 1:
			reason = "; no fix: the closure reads more than one checker from outside itself"
		}
		sites = append(sites, &borrowSite{
			closure: closure,
			name:    name,
			obj:     k.obj,
			reason:  reason,
			fixable: reason == "",
		})
	}
	sort.Slice(sites, func(i, j int) bool {
		if sites[i].closure.lit.Pos() != sites[j].closure.lit.Pos() {
			return sites[i].closure.lit.Pos() < sites[j].closure.lit.Pos()
		}
		return sites[i].name < sites[j].name
	})
	return sites
}

// innermostClosureAt returns the closure with the smallest span containing pos.
func innermostClosureAt(closures []subtestClosure, pos token.Pos) *ast.FuncLit {
	var best *ast.FuncLit
	for _, closure := range closures {
		lit := closure.lit
		if lit.Pos() > pos || pos >= lit.End() {
			continue
		}
		if best == nil || lit.Pos() > best.Pos() {
			best = lit
		}
	}
	return best
}

// declaredWithin reports whether obj is declared inside lit.
func declaredWithin(lit *ast.FuncLit, obj types.Object) bool {
	return lit.Pos() <= obj.Pos() && obj.Pos() < lit.End()
}

// borrowedObjects returns the distinct checkers the fixable sites borrow.
func borrowedObjects(sites []*borrowSite) map[types.Object]bool {
	objs := make(map[types.Object]bool)
	for _, s := range sites {
		if s.fixable {
			objs[s.obj] = true
		}
	}
	return objs
}

// survivingBorrowUses counts the reads of obj that outlive the rewrite: every
// read outside a closure this plan gives its own checker.
func survivingBorrowUses(pass *analysis.Pass, root ast.Node, obj types.Object, sites []*borrowSite) int {
	var shadowed []*ast.FuncLit
	for _, s := range sites {
		if s.fixable && s.obj == obj {
			shadowed = append(shadowed, s.closure.lit)
		}
	}

	var count int
	ast.Inspect(root, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[ident] != obj {
			return true
		}
		for _, lit := range shadowed {
			if lit.Pos() <= ident.Pos() && ident.Pos() < lit.End() {
				return true
			}
		}
		count++
		return true
	})
	return count
}

// declRemovalEdits renders the removal of obj's declaration, and says why it
// could not when it could not.
//
// Both spellings a checker is declared with are handled: c := qt.New(t) is an
// assignment and var c = qt.New(t) a declaration statement. Answering only the
// first would leave a var-declared checker reported with a cause that is not
// its own.
func declRemovalEdits(pass *analysis.Pass, root ast.Node, obj types.Object) ([]analysis.TextEdit, string) {
	var decl ast.Node
	ast.Inspect(root, func(n ast.Node) bool {
		switch d := n.(type) {
		case *ast.AssignStmt:
			if len(d.Lhs) != 1 {
				return true
			}
			if ident, ok := d.Lhs[0].(*ast.Ident); ok && pass.TypesInfo.Defs[ident] == obj {
				decl = d
			}
		case *ast.DeclStmt:
			gen, ok := d.Decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR || len(gen.Specs) != 1 {
				return true
			}
			spec, ok := gen.Specs[0].(*ast.ValueSpec)
			if !ok || len(spec.Names) != 1 {
				return true
			}
			if pass.TypesInfo.Defs[spec.Names[0]] == obj {
				decl = d
			}
		}
		return true
	})
	if decl == nil {
		return nil, "; no fix: the checker's declaration is not a shape this rule can remove, and it would be left with no reader"
	}
	start, end, ok := wholeLineSpan(pass, decl)
	if !ok {
		return nil, "; no fix: the checker's declaration shares its line, so removing it would take that with it"
	}
	return []analysis.TextEdit{{Pos: start, End: end}}, ""
}

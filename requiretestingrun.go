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

// checkRequireTestingRun reports the c.Run subtests in pass and suggests the
// t.Run(name, func(t *testing.T) { c := qt.New(t) }) form.
//
// It plans one outermost function at a time. Nothing smaller is a safe unit:
// the sites within a function decide each other's fate, and every use of a
// variable declared inside a function lies within it.
func (a *analyzer) checkRequireTestingRun(pass *analysis.Pass) {
	for _, file := range pass.Files {
		for _, root := range outermostFuncs(file) {
			a.planTestingRun(pass, file, root).report(pass)
		}
	}
}

// runSite is one c.Run call together with the plan's decisions about it.
type runSite struct {
	// run is the parsed call and call the call node itself.
	run  qtCRun
	call *ast.CallExpr

	// lexParent is the innermost site whose closure lexically contains this
	// one; outer is the innermost site whose closure parameter is this site's
	// receiver. They are not the same question. A c.Run on a *qt.C that was
	// declared inside another subtest closure has a lexical parent and no
	// outer, and the chain has to stay walkable through it.
	lexParent *runSite
	outer     *runSite

	// tName is the name the rewrite gives this site's closure parameter, and
	// recvText what it writes in front of .Run.
	tName    string
	recvText string

	// reported says the rule reports this site at all; fixed says the report
	// carries edits. A site that is not reported is never fixed.
	reported bool
	fixed    bool
}

// runPlan holds the -require-testing-run decisions for every c.Run site
// within one outermost function.
//
// The rule plans the whole function before it reports any of it, because the
// decisions are not independent. Whether a receiver's declaration survives
// depends on which of its sites are rewritten, and a nested site names a
// parameter that exists only if its enclosing site is rewritten. Answering
// those questions separately is how a declaration comes to be deleted while a
// sibling site that was declined still refers to it.
type runPlan struct {
	// root is the function the plan covers and origins the c := qt.New(t)
	// declarations within it.
	root    ast.Node
	origins map[types.Object]qtCOrigin

	// qtAlias and testingName are the names the file imports the two packages
	// under. Both are empty when the file does not import one of them under a
	// name a new reference could use, and the plan is then empty.
	qtAlias, testingName string

	// sites is every c.Run site within root, outermost first, so that a
	// site's enclosing decisions are already made when its own is taken.
	sites []*runSite
}

// planTestingRun works out which c.Run sites within root the rule reports and
// which of those receive edits.
func (a *analyzer) planTestingRun(pass *analysis.Pass, file *ast.File, root ast.Node) *runPlan {
	plan := &runPlan{
		root:        root,
		qtAlias:     importedPkgName(pass, file, quicktestPkgPath),
		testingName: importedPkgName(pass, file, testingPkgPath),
	}
	if plan.qtAlias == "" || plan.testingName == "" {
		// The rewrite has to name both packages, so there is nothing to plan.
		return plan
	}
	plan.origins = collectQtCOrigins(pass, root)
	plan.collect(pass, root, nil)
	plan.resolve(pass, a.onlyStableFixes)
	plan.dropInfeasible(pass)
	return plan
}

// collect records every c.Run site within n, outermost first.
func (p *runPlan) collect(pass *analysis.Pass, n ast.Node, lexParent *runSite) {
	ast.Inspect(n, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		run, ok := matchQtCRun(pass, call)
		if !ok {
			return true
		}

		site := &runSite{
			run:       run,
			call:      call,
			lexParent: lexParent,
			outer:     bindingSite(lexParent, run.recvObj),
			tName:     freeName("t", takenNames(run.lit, p.qtAlias, p.testingName)),
		}
		p.sites = append(p.sites, site)

		// Descend by hand: the closure's own sites are enclosed by this one,
		// while anything in the subtest name is a sibling of it.
		p.collect(pass, call.Args[0], lexParent)
		p.collect(pass, run.lit.Body, site)
		return false
	})
}

// bindingSite returns the innermost site on lexParent's chain whose closure
// parameter declares obj, or nil when no enclosing closure declares it.
func bindingSite(lexParent *runSite, obj types.Object) *runSite {
	for s := lexParent; s != nil; s = s.lexParent {
		if s.run.cObj != nil && s.run.cObj == obj {
			return s
		}
	}
	return nil
}

// resolve decides which sites are reported and which of those get edits.
//
// Two shapes reach a *testing.T. The receiver may be the *qt.C parameter of
// an enclosing c.Run closure that this rule also rewrites, in which case it
// becomes the parameter that rewrite gives that closure; or it may be a
// variable declared as c := qt.New(t), in which case it becomes that t.
// Anything else — a *qt.C parameter of a helper, a struct field, a value from
// a factory — has no statically known *testing.T, so the rule leaves it be.
//
// A nested site inherits its enclosing site's answers. Reporting one whose
// enclosing site was declined would name a receiver that does not exist, and
// fixing one whose enclosing site was withheld would name a parameter that
// rewrite has not introduced.
func (p *runPlan) resolve(pass *analysis.Pass, onlyStableFixes bool) {
	for _, s := range p.sites {
		withheld := onlyStableFixes && closureUsesTestScopedMethod(pass, s.run)
		if s.outer != nil {
			s.recvText = s.outer.tName
			s.reported = s.outer.reported
			s.fixed = s.outer.fixed && !withheld
			continue
		}
		name, ok := targetFromQtNew(pass, p.origins, s.run)
		if !ok {
			continue
		}
		s.recvText = name
		s.reported = true
		s.fixed = !withheld
	}
}

// dropInfeasible withdraws the sites whose receiver declaration the plan
// would have to remove but cannot remove cleanly. There is no correct fix for
// such a site, and a reported site without a fix puts the repair back on the
// author, so the rule stays quiet about it instead.
//
// Withdrawing a site gives its receiver a use back, which can only turn a
// declaration that had to go into one that stays. So each round either ends
// the loop or removes at least one site from the reported set, and sites are
// never added back.
func (p *runPlan) dropInfeasible(pass *analysis.Pass) {
	for {
		blocked := make(map[types.Object]bool)
		for _, s := range p.sites {
			if !s.reported || blocked[s.run.recvObj] {
				continue
			}
			if _, ok := p.declEdits(pass, s.run.recvObj); !ok {
				blocked[s.run.recvObj] = true
			}
		}
		if len(blocked) == 0 {
			return
		}
		for _, s := range p.sites {
			if blocked[s.run.recvObj] || (s.outer != nil && !s.outer.reported) {
				s.reported, s.fixed = false, false
			}
			if s.outer != nil && !s.outer.fixed {
				s.fixed = false
			}
		}
	}
}

// report emits a diagnostic for every site the plan reports, carrying the
// edits for those it also fixes.
func (p *runPlan) report(pass *analysis.Pass) {
	for _, s := range p.sites {
		if !s.reported {
			continue
		}
		diag := analysis.Diagnostic{
			Pos:     s.call.Fun.Pos(),
			End:     s.call.Fun.End(),
			Message: "qtlint: use t.Run with a per-subtest qt.New instead of c.Run",
		}
		if edits, ok := p.edits(pass, s); s.fixed && ok {
			diag.SuggestedFixes = []analysis.SuggestedFix{{
				Message:   "Replace c.Run with t.Run and a per-subtest qt.New",
				TextEdits: edits,
			}}
		}
		pass.Report(diag)
	}
}

// edits renders the rewrite of one site. It reports false when the receiver's
// declaration cannot be removed cleanly, which dropInfeasible has already
// withdrawn every reported site for.
func (p *runPlan) edits(pass *analysis.Pass, s *runSite) ([]analysis.TextEdit, bool) {
	// The receiver may have had no other use, in which case its declaration
	// goes with the rewrite. Siblings sharing the receiver each carry the
	// same deletion; identical edits collapse when a driver applies them.
	edits, ok := p.declEdits(pass, s.run.recvObj)
	if !ok {
		return nil, false
	}

	edits = append(edits,
		analysis.TextEdit{
			Pos:     s.run.sel.X.Pos(),
			End:     s.run.sel.X.End(),
			NewText: []byte(s.recvText),
		},
		analysis.TextEdit{
			Pos:     s.run.param.Pos(),
			End:     s.run.param.End(),
			NewText: []byte(newRunParam(s.run, s.tName, p.testingName)),
		},
	)

	// The *qt.C is recreated only if the closure still needs one. A closure
	// whose only use of it was the receiver of a nested c.Run loses that use
	// to the nested rewrite, and a declaration with no uses does not compile.
	if s.run.cObj != nil && p.survivingUses(pass, s.run.lit, s.run.cObj) > 0 {
		edits = append(edits, prependStmtEdit(pass, s.run.lit.Body,
			fmt.Sprintf("%s := %s.New(%s)", s.run.cName, p.qtAlias, s.tName)))
	}
	return edits, true
}

// declEdits returns the edits that remove obj's declaration when the plan
// takes its last use, and no edits when it does not.
//
// It reports false when the declaration has to go but cannot be removed
// cleanly, because there is then no correct fix.
func (p *runPlan) declEdits(pass *analysis.Pass, obj types.Object) ([]analysis.TextEdit, bool) {
	origin, ok := p.origins[obj]
	if !ok || p.survivingUses(pass, p.root, obj) > 0 {
		return nil, true
	}
	start, end, ok := wholeLineSpan(pass, origin.decl)
	if !ok {
		return nil, false
	}
	return []analysis.TextEdit{{Pos: start, End: end}}, true
}

// survivingUses counts the references to obj within root that the plan leaves
// behind.
//
// A reference disappears when it is the receiver of a c.Run the plan actually
// rewrites — not merely one this rule recognizes. A site the plan declined or
// withheld still names its receiver, and the declaration must stay for it.
func (p *runPlan) survivingUses(pass *analysis.Pass, root ast.Node, obj types.Object) int {
	consumed := make(map[*ast.Ident]bool)
	for _, s := range p.sites {
		if s.fixed && s.run.recvObj == obj {
			consumed[s.run.recv] = true
		}
	}

	var count int
	ast.Inspect(root, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if ok && pass.TypesInfo.Uses[ident] == obj && !consumed[ident] {
			count++
		}
		return true
	})
	return count
}

// targetFromQtNew returns the name of the *testing.T behind a receiver
// declared as c := qt.New(t).
func targetFromQtNew(pass *analysis.Pass, origins map[types.Object]qtCOrigin, run qtCRun) (string, bool) {
	origin, ok := origins[run.recvObj]
	if !ok {
		return "", false
	}
	tIdent, ok := origin.arg.(*ast.Ident)
	if !ok {
		return "", false
	}
	tObj := pass.TypesInfo.Uses[tIdent]
	if tObj == nil || !isTestingTPtr(tObj.Type()) {
		return "", false
	}

	// The name has to still mean that object where the rewrite writes it.
	scope := pass.Pkg.Scope().Innermost(run.sel.Pos())
	if scope == nil {
		return "", false
	}
	if _, found := scope.LookupParent(tIdent.Name, run.sel.Pos()); found != tObj {
		return "", false
	}
	return tIdent.Name, true
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

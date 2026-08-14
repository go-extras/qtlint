package qtlint

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// deferredCMethods are quicktest's deferred-execution API, and they are the
// one shape where this rewrite can turn a passing test into a panicking one.
//
// (*C).Defer registers a cleanup that panics unless Done has run first.
// C.Run wraps the closure it calls in "defer c2.Done()"; a bare
// c := qt.New(t) does not, and nothing in the rewritten closure would. So a
// closure that calls Defer keeps its diagnostic and loses its fix whatever
// -only-stable-fixes says. Done travels with Defer because the two are one
// API, and withholding a fix nobody needed costs only a fix.
//
// Measured against quicktest v1.14.6: the c.Run form of a subtest calling
// c.Defer passes, and the t.Run plus qt.New form panics with "Done not
// called after Defer".
var deferredCMethods = map[string]bool{
	"Defer": true,
	"Done":  true,
}

// testScopedCMethods are the *qt.C methods that act on the test the *qt.C was
// made from rather than on the assertion at hand.
//
// The rewrite does not move them. C.Run builds the closure's *qt.C from the
// subtest's own *testing.T, so both forms reach the same test: measured
// against quicktest v1.14.6, Setenv, Unsetenv and Patch restore at the same
// point, Cleanup runs at the same point, and TempDir and Mkdir name the same
// subtest-scoped directory either way.
//
// They are still the calls that tie a subtest to a test's lifecycle rather
// than to an assertion, which is where a reader has to agree that the subtest
// is the scope that was meant. -only-stable-fixes withholds the fix for a
// closure that calls one so a project can migrate those by hand; the
// diagnostic still fires. That is a review gate, not a correctness one. The
// correctness one is deferredCMethods, and it is not optional.
var testScopedCMethods = map[string]bool{
	"Cleanup":  true,
	"Mkdir":    true,
	"Parallel": true,
	"Patch":    true,
	"Setenv":   true,
	"TempDir":  true,
	"Unsetenv": true,
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
		if !p.writesResolvableNames(pass, s.run) {
			continue
		}
		reach := closureCReach(pass, s.run)
		withheld := reach.deferred || (onlyStableFixes && reach.testScoped)
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

// writesResolvableNames reports whether the package names this site's rewrite
// would write still mean their packages at the positions it would write them.
//
// The rewrite emits three names. The receiver is checked where it is worked
// out: targetFromQtNew insists that the *testing.T's name still means that
// object in front of .Run, and the parameter an enclosing rewrite introduces
// is chosen by freeName, which keeps clear of every name in that closure. The
// other two are package qualifiers and are checked here — the testing one
// where the new parameter type goes, the quicktest one where the inserted
// qt.New goes.
//
// Neither qualifier is safe merely because the file imports it. A file can
// import a path twice, and importedPkgName answers with the first spelling it
// finds, which need not be the spelling that resolves at the insertion point.
//
// The quicktest qualifier is checked for every closure with a named *qt.C,
// including the few that turn out not to need a qt.New at all. Which of them
// do is settled later; declining one that would have been fine costs a fix,
// and writing a name that means something else costs a file that does not
// compile.
func (p *runPlan) writesResolvableNames(pass *analysis.Pass, run qtCRun) bool {
	if !packageQualifies(pass, p.testingName, testingPkgPath, run.param.Pos()) {
		return false
	}
	return run.cObj == nil ||
		packageQualifies(pass, p.qtAlias, quicktestPkgPath, run.lit.Body.Pos())
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

// cReach says what a closure can do to its own *qt.C.
type cReach struct {
	// deferred is set when the closure can reach quicktest's
	// deferred-execution API, and testScoped when it can reach a method whose
	// scope is the test rather than the assertion.
	deferred   bool
	testScoped bool
}

// closureCReach works out what run's closure can do to its own *qt.C.
//
// The question the withholding rules ask is not "does this closure write
// c.Defer" but "can Defer be called on this *qt.C", and the two come apart at
// every indirection: cc := c, helper(c), holder{c: c}. Matching method names
// against the closure's own parameter sees only the shapes that spell them
// out, so this works the other way round. It follows the *qt.C through the
// plain assignments it can see, classifies each use by the method that use
// selects, and treats a use it cannot see through as reaching anything.
//
// A selector is enough on its own: c.Cleanup taken as a method value and
// called later reaches exactly as far as calling it outright.
func closureCReach(pass *analysis.Pass, run qtCRun) cReach {
	if run.cObj == nil {
		return cReach{}
	}
	held := heldQtCObjects(pass, run.lit, run.cObj)

	var reach cReach
	inspectWithParent(run.lit, func(n, parent ast.Node) {
		ident, ok := n.(*ast.Ident)
		if !ok || !held[pass.TypesInfo.Uses[ident]] {
			return
		}
		if sel, ok := parent.(*ast.SelectorExpr); ok && sel.X == ident {
			switch {
			case deferredCMethods[sel.Sel.Name]:
				reach.deferred = true
			case testScopedCMethods[sel.Sel.Name]:
				reach.testScoped = true
			}
			return
		}
		if _, rhs, ok := soleAssign(parent); ok && rhs == ident {
			// The assignment that hands the *qt.C on is already followed.
			return
		}
		// The *qt.C goes somewhere this rule cannot follow: into a helper, a
		// struct, a slice. Anything at all could be called on it there, and
		// withholding a fix that was not needed costs only a fix.
		reach.deferred = true
		reach.testScoped = true
	})
	return reach
}

// heldQtCObjects returns cObj together with every variable inside lit that a
// plain one-to-one assignment hands it to. The set is closed under
// repetition, so cc := c followed by ccc := cc reaches ccc.
//
// A destination joins the set whatever its type. The alternative is to read
// the assignment as an escape, which would be the stricter answer for the one
// shape this exists to follow.
func heldQtCObjects(pass *analysis.Pass, lit *ast.FuncLit, cObj types.Object) map[types.Object]bool {
	held := map[types.Object]bool{cObj: true}
	for changed := true; changed; {
		changed = false
		ast.Inspect(lit, func(n ast.Node) bool {
			lhs, rhs, ok := soleAssign(n)
			if !ok {
				return true
			}
			src, ok := rhs.(*ast.Ident)
			if !ok || !held[pass.TypesInfo.Uses[src]] {
				return true
			}
			dst, ok := lhs.(*ast.Ident)
			if !ok {
				return true
			}
			obj := pass.TypesInfo.Defs[dst]
			if obj == nil {
				obj = pass.TypesInfo.Uses[dst]
			}
			if obj == nil || held[obj] {
				return true
			}
			held[obj] = true
			changed = true
			return true
		})
	}
	return held
}

// soleAssign returns the two sides of a node that assigns one value to one
// destination, which is the only shape a *qt.C is followed through.
func soleAssign(n ast.Node) (lhs, rhs ast.Expr, ok bool) {
	switch node := n.(type) {
	case *ast.AssignStmt:
		if len(node.Lhs) == 1 && len(node.Rhs) == 1 {
			return node.Lhs[0], node.Rhs[0], true
		}
	case *ast.ValueSpec:
		if len(node.Names) == 1 && len(node.Values) == 1 {
			return node.Names[0], node.Values[0], true
		}
	}
	return nil, nil, false
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

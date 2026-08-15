package qtlint

import (
	"fmt"
	"go/ast"
	"go/token"
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
//
// Chdir and Context reach a *qt.C through the embedded testing.TB rather than
// through C's own declarations, which is how they came to be left out: an
// inventory of what can be called on a *qt.C has to follow the embedding, and
// testing.TB gains methods with the language — both of these in Go 1.24. By
// this set's own criterion they belong to it. A working directory and a
// context last exactly as long as the test, and both bind to the subtest under
// either spelling, so like the rest of the set they cost only a fix.
var testScopedCMethods = map[string]bool{
	"Chdir":    true,
	"Cleanup":  true,
	"Context":  true,
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

	// namedAcross is set when a site nested inside this one writes a receiver
	// bound outside it, so that this site's own parameter must not hide that
	// receiver. keep holds the names from the source that must survive; the
	// names this plan introduces are handled by walking the enclosing sites,
	// whose parameters are already named by then.
	namedAcross bool
	keep        []string

	// tName is the name the rewrite gives this site's closure parameter, and
	// recvText what it writes in front of .Run.
	tName    string
	recvText string

	// reported says the rule reports this site at all; fixed says the report
	// carries edits. A site that is not reported is never fixed.
	reported bool
	fixed    bool

	// withheldReason is the clause the diagnostic appends when the site is
	// reported without a fix, naming what withheld it. A reported site with no
	// fix and no reason would leave the reader to work out which of a
	// closure's indirections the rule could not see through, which is the one
	// question the tool is in a position to answer.
	withheldReason string
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

	// requireQtCReceiver says the other opt-in rule is enabled in the same
	// run, so this plan has to account for the uses it will write.
	requireQtCReceiver bool

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
		root:               root,
		requireQtCReceiver: a.requireQtCReceiver,
		qtAlias:            importedPkgName(pass, file, quicktestPkgPath),
		testingName:        importedPkgName(pass, file, testingPkgPath),
	}
	if plan.qtAlias == "" || plan.testingName == "" {
		// The rewrite has to name both packages, so there is nothing to plan.
		return plan
	}
	plan.origins = collectQtCOrigins(pass, root)
	plan.collect(pass, root, nil)
	plan.nameParams(pass)
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

// nameParams gives every site the name its rewrite will write for the closure
// parameter it introduces.
//
// The name is t, and it shadows the enclosing test's t when the closure refers
// to one. The body is moving to the subtest, so a t inside it should address
// that subtest; renaming the parameter around the reference leaves those calls
// addressing the parent, which compiles and passes and is wrong.
//
// Shadowing is settled by Go's scoping and needs no analysis of what else is
// in the body. The one exception is not about the body at all — it is this
// rule's own rewrites colliding with each other.
//
// A site whose receiver is bound further out writes that receiver's name
// across the closures in between, and those closures are taking parameters of
// their own from this same plan. Three
// levels of c.Run where the middle one renames its *qt.C leaves the innermost
// site writing a "t" that the middle rewrite has just introduced, so the call
// binds to the middle subtest instead of the outer one. Both spellings compile
// and both pass; only the subtest's name moves, from outer/deep to
// outer/middle/deep, and with it every -run filter aimed at it.
//
// Only a site that is written across is kept clear of its enclosing names. A
// plain nest wants the shadowing — it is what lets each level of
// t.Run(…, func(t *testing.T)) call itself t.
//
// Two kinds of name are written across such a site. One is the parameter an
// enclosing site is about to introduce, which is why the sites are named
// outermost first: an enclosing name is final before an inner one is chosen.
// The other is a name from the source, written by a site whose receiver came
// from a c := qt.New(t) rather than from an enclosing closure; that one is
// known before any naming and is collected first.
func (p *runPlan) nameParams(pass *analysis.Pass) {
	for _, s := range p.sites {
		if s.outer != nil {
			for x := s.lexParent; x != nil && x != s.outer; x = x.lexParent {
				x.namedAcross = true
			}
			continue
		}
		name, ok := targetFromQtNew(pass, p.origins, s.run)
		if !ok {
			continue
		}
		for x := s.lexParent; x != nil; x = x.lexParent {
			x.keep = append(x.keep, name)
		}
	}

	for _, s := range p.sites {
		taken := map[string]bool{p.qtAlias: true, p.testingName: true}
		for _, name := range s.keep {
			taken[name] = true
		}
		if s.namedAcross {
			for x := s.lexParent; x != nil; x = x.lexParent {
				taken[x.tName] = true
			}
		}
		s.tName = freeName("t", taken)
	}
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
	// The package's function declarations are indexed once for the whole plan.
	// Building the index per escape would rebuild it for every subtest in
	// every file, which is the same answer at a cost that grows with the
	// package.
	callees := newCalleeReach(pass)
	for _, s := range p.sites {
		if !p.writesResolvableNames(pass, s.run) {
			continue
		}
		reach := closureCReach(pass, s.run, callees)
		withheld := reach.deferred || (onlyStableFixes && reach.testScoped)
		if withheld {
			s.withheldReason = reach.withholdReason(onlyStableFixes)
		}
		if !withheld && bodyRedeclares(s.run, s.tName) {
			withheld = true
			s.withheldReason = "; no fix: the closure body already declares " + s.tName +
				" in the scope the new parameter would occupy"
		}
		if s.outer != nil {
			s.recvText = s.outer.tName
			s.reported = s.outer.reported
			s.fixed = s.outer.fixed && !withheld
			if !s.fixed && s.withheldReason == "" {
				s.withheldReason = s.outer.withheldReason
			}
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
			Message: "qtlint: use t.Run with a per-subtest qt.New instead of c.Run" + s.withheldReason,
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
	// goes with the rewrite.
	edits, ok := p.declEdits(pass, s.run.recvObj)
	if !ok {
		return nil, false
	}

	// Every site sharing this receiver is rewritten by this one fix, not only
	// the site the diagnostic sits on.
	//
	// A SuggestedFix is the unit an editor applies. Carrying the declaration's
	// deletion in each sibling's fix separately is correct only for a driver
	// that applies all of them: accept one code action in gopls and the
	// declaration goes while the siblings that still name it stay, which does
	// not compile. Making each fix convert the whole group leaves every one of
	// them self-contained, and the group's fixes are then identical, so a
	// driver applying all of them collapses the duplicates exactly as before.
	for _, sibling := range p.sites {
		if !sibling.fixed || sibling.run.recvObj != s.run.recvObj {
			continue
		}
		edits = append(edits, p.siteEdits(pass, sibling)...)
	}
	return edits, true
}

// siteEdits renders the rewrite of one site, without the declaration removal
// the whole group shares.
func (p *runPlan) siteEdits(pass *analysis.Pass, s *runSite) []analysis.TextEdit {
	edits := []analysis.TextEdit{
		{
			Pos:     s.run.sel.X.Pos(),
			End:     s.run.sel.X.End(),
			NewText: []byte(s.recvText),
		},
		{
			Pos:     s.run.param.Pos(),
			End:     s.run.param.End(),
			NewText: []byte(newRunParam(s.run, s.tName, p.testingName)),
		},
	}

	// The *qt.C is recreated only if the closure still needs one. A closure
	// whose only use of it was the receiver of a nested c.Run loses that use
	// to the nested rewrite, and a declaration with no uses does not compile.
	if s.run.cObj != nil && p.survivingUses(pass, s.run.lit, s.run.cObj) > 0 {
		edits = append(edits, prependStmtEdit(pass, s.run.lit.Body,
			fmt.Sprintf("%s := %s.New(%s)", s.run.cName, p.qtAlias, s.tName)))
	}
	return edits
}

// declEdits returns the edits that remove obj's declaration when the plan
// takes its last use, and no edits when it does not.
//
// It reports false when the declaration has to go but cannot be removed
// cleanly, because there is then no correct fix.
func (p *runPlan) declEdits(pass *analysis.Pass, obj types.Object) ([]analysis.TextEdit, bool) {
	origin, ok := p.origins[obj]
	if !ok || p.survivingUses(pass, p.root, obj) > 0 || p.receiverRuleWillUse(pass, obj) {
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

// receiverRuleWillUse reports whether -require-qt-c-receiver, when it is also
// enabled, would rewrite a package-level assertion in this function into a use
// of obj.
//
// The two rules plan against the same receiver. This one deletes a declaration
// whose last use it consumed; the other turns qt.Assert(t, …) into
// c.Assert(…), which is a use of exactly that declaration. Neither is wrong
// alone, and applied together on a receiver whose only other use was the
// c.Run they produce a file that does not compile.
//
// So the deletion asks the other rule what it is about to write. The answer is
// conservative in the safe direction: a declaration kept when nothing ends up
// using it is a lint finding, and one removed while something does is a build
// failure.
func (p *runPlan) receiverRuleWillUse(pass *analysis.Pass, obj types.Object) bool {
	if !p.requireQtCReceiver {
		return false
	}
	origin, ok := p.origins[obj]
	if !ok {
		return false
	}
	tIdent, ok := origin.arg.(*ast.Ident)
	if !ok {
		return false
	}
	tObj := pass.TypesInfo.Uses[tIdent]
	if tObj == nil {
		return false
	}

	found := false
	ast.Inspect(p.root, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		assertion, ok := matchPackageLevelAssertion(pass, call)
		if !ok {
			return true
		}
		// Only an assertion the other rule would bind to THIS receiver
		// matters: it rewrites qt.Assert(t, …) into a use of the *qt.C built
		// from that same t.
		if assertion.tObj == tObj {
			found = true
		}
		return !found
	})
	return found
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

	// escape names the first use the analysis could not follow, and method the
	// first spelled-out method that decided the reach. Exactly one is usually
	// set, and both may be empty when nothing withholds the fix.
	//
	// They exist so a withheld fix can say what withheld it. Without them the
	// diagnostic reads the same whether the fix was applied or not, and a
	// reader has to work out which of a closure's several indirections was the
	// one the rule could not see through.
	escape string
	method string
}

// bodyRedeclares reports whether run's closure declares name directly in its
// body block, where the new parameter would land.
//
// Go declares a function's parameters in the body block rather than in a scope
// around it, so a `t := 1` written at the top of the closure does not shadow a
// parameter named t — it collides with it, and the rewritten file stops
// compiling. A `t` declared inside an if or a for or any nested block is a
// scope of its own and shadows as usual, which is why only the top level of
// the body is looked at.
//
// The answer is to withhold the fix, not to name the parameter around the
// collision. Renaming it would leave every t already in the body bound to the
// parent test while the closure runs as a subtest, which compiles and passes
// and is the defect this rule's naming exists to avoid. A closure this rule
// cannot convert without breaking is one an author converts by hand.
//
// A closure whose parameter is unnamed or blank takes no name at all, so
// nothing can collide with it.
func bodyRedeclares(run qtCRun, name string) bool {
	if run.cName == "" || run.lit.Body == nil {
		return false
	}
	for _, stmt := range run.lit.Body.List {
		if declaresNameAtTopLevel(stmt, name) {
			return true
		}
	}
	return false
}

// declaresNameAtTopLevel reports whether stmt declares name in the block it
// sits in.
//
// All four spellings count, because all four put the name in the same block
// the parameter is declared in: a short declaration, and a var, const or type
// declaration. Measured, a const collides as loudly as a var — "t redeclared
// in this block" — and only the := form is quiet enough to be mistaken for an
// assignment.
func declaresNameAtTopLevel(stmt ast.Stmt, name string) bool {
	switch stmt := stmt.(type) {
	case *ast.AssignStmt:
		if stmt.Tok != token.DEFINE {
			return false
		}
		for _, lhs := range stmt.Lhs {
			if ident, ok := lhs.(*ast.Ident); ok && ident.Name == name {
				return true
			}
		}
	case *ast.DeclStmt:
		decl, ok := stmt.Decl.(*ast.GenDecl)
		if !ok {
			return false
		}
		switch decl.Tok {
		case token.VAR, token.CONST, token.TYPE:
		default:
			return false
		}
		for _, spec := range decl.Specs {
			if declaresName(spec, name) {
				return true
			}
		}
	}
	return false
}

// declaresName reports whether spec introduces name.
func declaresName(spec ast.Spec, name string) bool {
	switch spec := spec.(type) {
	case *ast.ValueSpec:
		for _, ident := range spec.Names {
			if ident.Name == name {
				return true
			}
		}
	case *ast.TypeSpec:
		return spec.Name != nil && spec.Name.Name == name
	}
	return false
}

// withholdReason renders why a fix was withheld, as a clause a diagnostic can
// append to its message. It returns the empty string when nothing withholds.
func (r cReach) withholdReason(onlyStableFixes bool) string {
	switch {
	case r.escape != "":
		return "; no fix: the *qt.C is handed to " + r.escape +
			", so what it can reach includes (*qt.C).Defer, which panics unless Done() ran" +
			" — give that function a *testing.T instead and this converts"
	case r.deferred:
		return "; no fix: the closure calls c." + r.method +
			", and a bare qt.New(t) supplies no Done() the way C.Run does"
	case onlyStableFixes && r.testScoped:
		return "; no fix under -only-stable-fixes: the closure calls c." + r.method +
			", which binds to whichever test the *qt.C came from"
	}
	return ""
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
func closureCReach(pass *analysis.Pass, run qtCRun, callees *calleeReach) cReach {
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
				if reach.method == "" {
					reach.method = sel.Sel.Name
				}
			case testScopedCMethods[sel.Sel.Name]:
				reach.testScoped = true
				if reach.method == "" {
					reach.method = sel.Sel.Name
				}
			}
			return
		}
		if _, rhs, ok := soleAssign(parent); ok && rhs == ident {
			// The assignment that hands the *qt.C on is already followed.
			return
		}
		if inner, ok := followedCallReach(callees, parent, ident); ok {
			reach.merge(inner)
			return
		}
		// The *qt.C goes somewhere this rule cannot follow: into a helper, a
		// struct, a slice. Anything at all could be called on it there, and
		// withholding a fix that was not needed costs only a fix.
		reach.deferred = true
		reach.testScoped = true
		if reach.escape == "" {
			reach.escape = escapeDescription(parent)
		}
	})
	return reach
}

// escapeDescription names the construct a *qt.C escaped into, in the words a
// reader would use for it. The name is what makes a withheld fix actionable:
// "handed to helper(...)" points at the function whose signature to change,
// where "cannot fix" points at nothing.
func escapeDescription(parent ast.Node) string {
	switch p := parent.(type) {
	case *ast.CallExpr:
		if name := calleeName(p.Fun); name != "" {
			return name + "(...)"
		}
		return "a function call"
	case *ast.KeyValueExpr, *ast.CompositeLit:
		return "a composite literal"
	case *ast.ReturnStmt:
		return "the function's result"
	}
	return "an expression this rule cannot follow"
}

// calleeName renders the called function's name for an escape description,
// including the receiver or package qualifier when there is one.
func calleeName(fun ast.Expr) string {
	switch f := stripParens(fun).(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		if x, ok := stripParens(f.X).(*ast.Ident); ok {
			return x.Name + "." + f.Sel.Name
		}
		return f.Sel.Name
	}
	return ""
}

// heldQtCObjects returns cObj together with every variable inside lit that a
// plain one-to-one assignment hands it to. The set is closed under
// repetition, so cc := c followed by ccc := cc reaches ccc.
//
// A destination joins the set whatever its type. The alternative is to read
// the assignment as an escape, which would be the stricter answer for the one
// shape this exists to follow.
func heldQtCObjects(pass *analysis.Pass, lit *ast.FuncLit, cObj types.Object) map[types.Object]bool {
	return heldQtCObjectsIn(pass, lit, cObj)
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

package testingrunfix

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// The plain shape. c.Run was the receiver's only use, so its declaration goes
// with the rewrite; leaving it behind would not compile.
func TestPlain(t *testing.T) {
	c := qt.New(t)
	c.Run("sub", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Assert(1, qt.Equals, 1)
	})
}

// The receiver survives when the test still uses it for something else.
func TestReceiverSurvives(t *testing.T) {
	c := qt.New(t)
	c.Assert(0, qt.Equals, 0)

	c.Run("sub", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Assert(1, qt.Equals, 1)
	})
}

// Two subtests on one receiver: the declaration goes only once, and the
// rewrite of each is independent of the other.
func TestTwoSubtests(t *testing.T) {
	c := qt.New(t)
	c.Run("first", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Assert(1, qt.Equals, 1)
	})
	c.Run("second", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Assert(2, qt.Equals, 2)
	})
}

// The receiver and the closure parameter are matched on their type, not on
// the identifier c.
func TestOtherName(t *testing.T) {
	qc := qt.New(t)
	qc.Run("sub", func(inner *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		inner.Assert(1, qt.Equals, 1)
	})
}

// Nested subtests. The inner rewrite names the parameter the outer rewrite
// introduces, and the outer closure gets no qt.New because its only use of
// the *qt.C was the receiver the inner rewrite consumes.
func TestNested(t *testing.T) {
	c := qt.New(t)
	c.Run("outer", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Run("inner", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
			c.Assert(1, qt.Equals, 1)
		})
	})
}

// Nested subtests where the outer closure asserts as well: it keeps its own
// qt.New, because that use survives the inner rewrite.
func TestNestedOuterAsserts(t *testing.T) {
	c := qt.New(t)
	c.Run("outer", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Assert(0, qt.Equals, 0)
		c.Run("inner", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
			c.Assert(1, qt.Equals, 1)
		})
	})
}

// Nested subtests where the outer closure names the enclosing t. The outer
// rewrite takes that same name, so the reference becomes the outer subtest's
// own handle, and the inner call written as t.Run attaches to "outer" because
// that is what t means at that point.
func TestNestedShadowedOuter(t *testing.T) {
	c := qt.New(t)
	c.Run("outer", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		t.Log("parent")
		c.Run("inner", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
			c.Assert(1, qt.Equals, 1)
		})
	})
}

// The closure already refers to the enclosing test's t. The new parameter
// takes that name anyway: the closure is becoming a subtest, and a t inside it
// should address that subtest rather than the parent it was written against.
func TestOuterTReferenced(t *testing.T) {
	c := qt.New(t)
	c.Run("sub", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		t.Log("parent")
		c.Assert(1, qt.Equals, 1)
	})
}

// A blank parameter stays blank: nothing in the rewritten body could name it.
func TestBlankParam(t *testing.T) {
	c := qt.New(t)
	c.Run("sub", func(_ *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		t.Log("sub")
	})
}

// A closure calling a test-scoped *qt.C method is fixed by default. Only
// -only-stable-fixes withholds it; see the testingrunonlystable package.
func TestTestScopedMethod(t *testing.T) {
	c := qt.New(t)
	c.Run("sub", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Cleanup(func() {})
		c.Assert(1, qt.Equals, 1)
	})
}

// The acceptance control for Chdir and Context. Both are in the review-gate
// set, and a set entry only withholds under -only-stable-fixes: with no flag
// set the fix has to arrive, or the entry has quietly become a correctness
// rule that nobody asked for.
func TestChdirAndContextFixedByDefault(t *testing.T) {
	c := qt.New(t)
	c.Run("chdir", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Chdir("testdata")
	})
	c.Run("context", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		_ = c.Context()
	})
}

// A closure calling Defer is withheld with no flag set. C.Run calls Done on
// the *qt.C it hands the closure and a bare qt.New(t) does not, so this
// rewrite would replace a passing test with one that panics with "Done not
// called after Defer". The receiver's declaration stays, because the c.Run
// that was left alone still uses it.
func TestDeferWithheldByDefault(t *testing.T) {
	c := qt.New(t)
	c.Run("sub", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Defer(func() {})
		c.Assert(1, qt.Equals, 1)
	})
}

// Done is the other half of the same API and is withheld the same way.
func TestDoneWithheldByDefault(t *testing.T) {
	c := qt.New(t)
	c.Run("sub", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Done()
	})
}

// deferSomething reaches Defer through a parameter.
func deferSomething(c *qt.C) { c.Defer(func() {}) }

// Defer reached indirectly is withheld by default as well, which is the whole
// point of asking what can be called on the *qt.C rather than what is written
// next to it. The alias is followed; the helper call is not, and a use that
// cannot be followed is treated as reaching anything.
func TestIndirectDeferWithheldByDefault(t *testing.T) {
	c := qt.New(t)
	c.Run("alias", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		cc := c
		cc.Defer(func() {})
	})
	c.Run("helper", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		deferSomething(c)
	})
}

// The *qt.C is followed through a var declaration as well as through :=, and
// this is the case that pins which way round that matters. The alias only ever
// has Assert called on it, so nothing is withheld and the site keeps its fix.
// A rule that did not read "var cc = c" as handing the *qt.C on would see a use
// it could not follow, treat it as reaching anything, and withhold — quietly,
// in the safe direction, for a closure that does nothing unusual at all.
func TestVarAliasKeepsFix(t *testing.T) {
	c := qt.New(t)
	c.Run("sub", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		var cc = c
		cc.Assert(1, qt.Equals, 1)
	})
}

// The other direction: what is reached through a var alias is reached. Defer
// on the alias withholds the fix exactly as Defer on the parameter does.
func TestVarAliasDeferWithheld(t *testing.T) {
	c := qt.New(t)
	c.Run("sub", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		var cc = c
		cc.Defer(func() {})
		cc.Assert(1, qt.Equals, 1)
	})
}

// A one-line closure body: the opening brace, the statement and the closing
// brace share a line, so the rewrite must open a line for the statement it
// inserts instead of running the two together.
func TestSingleLineBody(t *testing.T) {
	c := qt.New(t)
	c.Run("sub", func(c *qt.C) { c.Assert(1, qt.Equals, 1) }) // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
}

// An ordinary half-migrated file. The third subtest's receiver is the same
// *qt.C the first one consumes, but the t it was made from is shadowed by the
// closure the third sits in, so the rule cannot name a *testing.T for it and
// declines it. The declaration has to stay for the site that was declined.
func TestDeclinedSiblingKeepsDecl(t *testing.T) {
	c := qt.New(t)
	c.Run("first", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Assert(1, qt.Equals, 1)
	})
	t.Run("second", func(t *testing.T) {
		c.Run("third", func(c *qt.C) {
			c.Assert(2, qt.Equals, 2)
		})
	})
}

// The same shape with a helper closure that takes its own *testing.T.
func TestDeclinedInHelperClosureKeepsDecl(t *testing.T) {
	c := qt.New(t)
	c.Run("first", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Assert(1, qt.Equals, 1)
	})
	run := func(t *testing.T) {
		c.Run("second", func(c *qt.C) {
			c.Assert(2, qt.Equals, 2)
		})
	}
	run(t)
}

// The declaration carries a trailing comment, so removing its line would take
// the comment with it: the rule declines the site rather than offer a fix
// that loses it. The nested subtest is declined with it, because rewriting
// the inner call alone attaches "inner" to the parent test rather than to
// "outer" — silently, since both spellings compile.
func TestUnremovableDeclDeclinesNest(t *testing.T) {
	c := qt.New(t) // the checker for this test
	c.Run("outer", func(c *qt.C) {
		c.Run("inner", func(c *qt.C) {
			c.Assert(1, qt.Equals, 1)
		})
	})
}

// Two subtests sharing one receiver whose declaration goes with them.
//
// Each site's fix carries the whole group, not only its own edits. A
// SuggestedFix is the unit an editor applies, so a fix that deleted the
// declaration and rewrote one site would leave the sibling naming a
// declaration that is gone — accepted alone in gopls, that does not compile.
func TestSiblingsShareOneFix(t *testing.T) {
	c := qt.New(t)
	c.Run("first", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Assert(1, qt.Equals, 1)
	})
	c.Run("second", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Assert(2, qt.Equals, 2)
	})
}

// A closure whose body declares t at its top level. Go declares a parameter in
// the body block, so the rewrite's own parameter would collide with this
// declaration rather than be shadowed by it, and the file would stop compiling.
// The fix is withheld and the diagnostic says so; the parameter is not renamed
// around the collision, because that would leave the t below bound to the
// parent while the closure runs as a subtest.
func TestBodyDeclaresT(t *testing.T) {
	c := qt.New(t)
	c.Run("shortdecl", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run; no fix: the closure body already declares t in the scope the new parameter would occupy"
		t := 1
		c.Assert(t, qt.Equals, 1)
	})
	c.Run("vardecl", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run; no fix: the closure body already declares t in the scope the new parameter would occupy"
		var t = 2
		c.Assert(t, qt.Equals, 2)
	})
	c.Run("constdecl", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run; no fix: the closure body already declares t in the scope the new parameter would occupy"
		const t = 3
		c.Assert(t, qt.Equals, 3)
	})
	c.Run("typedecl", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run; no fix: the closure body already declares t in the scope the new parameter would occupy"
		type t struct{ n int }
		c.Assert(t{n: 4}.n, qt.Equals, 4)
	})
}

// The same name declared inside a nested block is a scope of its own and
// shadows the parameter as usual, so this one is fixed. It is the control that
// keeps the check above from reading "the body mentions t anywhere".
func TestNestedBlockDeclaresT(t *testing.T) {
	c := qt.New(t)
	c.Run("sub", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		if true {
			t := 1
			c.Assert(t, qt.Equals, 1)
		}
	})
}

package testingrunonlystable

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// A stable and an unstable subtest share one *qt.C. Under -only-stable-fixes
// the unstable one keeps its c.Run, so the receiver still has a use and its
// declaration must stay — deleting it here would break the file.
func TestMixed(t *testing.T) {
	c := qt.New(t)

	c.Run("stable", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Assert(1, qt.Equals, 1)
	})

	c.Run("unstable", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Cleanup(func() {})
		c.Assert(2, qt.Equals, 2)
	})
}

// Every method in the withheld set, one subtest each. All are reported and
// none is rewritten under this flag.
//
// Chdir and Context are here for the same reason as the rest, and they are the
// two that reach a *qt.C through the embedded testing.TB rather than through
// C's own declarations. A stub that does not spell them out is a stub no
// fixture can use to ask about them.
func TestWithheldMethods(t *testing.T) {
	c := qt.New(t)

	c.Run("parallel", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Parallel()
	})
	c.Run("chdir", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Chdir("testdata")
	})
	c.Run("context", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		_ = c.Context()
	})
	c.Run("setenv", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Setenv("K", "V")
	})
	c.Run("tempdir", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Assert(c.TempDir(), qt.Not(qt.Equals), "")
	})
	c.Run("patch", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Patch(nil, nil)
	})
	c.Run("unsetenv", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Unsetenv("K")
	})
	c.Run("mkdir", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Assert(c.Mkdir(), qt.Not(qt.Equals), "")
	})
}

// holder puts a *qt.C somewhere the rule cannot follow it.
type holder struct{ c *qt.C }

// cleanUp reaches a test-scoped method through a parameter.
func cleanUp(c *qt.C) { c.Cleanup(func() {}) }

// The indirect routes to a test-scoped method are withheld too. Naming the
// methods and matching them against the closure's own parameter sees only the
// closures that spell them out; an alias, a helper and a struct field all
// reach the same methods without ever writing them next to c.
func TestIndirectReach(t *testing.T) {
	c := qt.New(t)

	c.Run("alias", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		cc := c
		cc.Cleanup(func() {})
	})
	c.Run("helper", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		cleanUp(c)
	})
	c.Run("field", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		h := holder{c: c}
		h.c.Setenv("K", "V")
	})
	c.Run("method value", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		f := c.Cleanup
		f(func() {})
	})
}

// An unstable outer withholds its fix, and the nested subtest withholds with
// it: rewriting the inner alone would name a t the outer has not introduced.
func TestNestedUnstableOuter(t *testing.T) {
	c := qt.New(t)
	c.Run("outer", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Setenv("K", "V")
		c.Run("inner", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
			c.Assert(1, qt.Equals, 1)
		})
	})
}

// scopedHelper takes the checker and calls a test-scoped method on it. The
// rule reads its body rather than guessing, so the closure that calls it is
// withheld for what the helper actually does.
func scopedHelper(c *qt.C) string { return c.TempDir() }

// plainHelper takes the checker and only asserts through it. Reading its body
// is what lets the closure that calls it keep its fix; before the callee was
// followed, handing the checker to any helper at all withheld one.
func plainHelper(c *qt.C) string {
	c.Assert(1, qt.Equals, 1)
	return "x"
}

// opaqueHolder gives the escape a shape the rule still cannot follow: a method
// has a receiver, so the binding is not a plain parameter of a package-level
// function and the body cannot be asked about it by name.
type opaqueHolder struct{}

func (opaqueHolder) take(c *qt.C) string { return c.TempDir() }

// A withheld site says what withheld it. The clause names the construct the
// *qt.C escaped into, because that is the one question the tool can answer and
// the reader cannot: which of a closure's indirections was the opaque one, and
// therefore whose signature to change.
func TestWithholdingNamesItsCause(t *testing.T) {
	c := qt.New(t)

	c.Run("scoped helper", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run; no fix under -only-stable-fixes: the closure calls c.TempDir, which binds to whichever test the \\*qt.C came from"
		c.Assert(scopedHelper(c), qt.Not(qt.Equals), "")
	})

	c.Run("opaque escape", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run; no fix: the \\*qt.C is handed to take\\(\\.\\.\\.\\), so what it can reach includes \\(\\*qt.C\\).Defer, which panics unless Done\\(\\) ran — give that function a \\*testing.T instead and this converts"
		c.Assert(opaqueHolder{}.take(c), qt.Not(qt.Equals), "")
	})

	c.Run("handle helper", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run; no fix under -only-stable-fixes: the closure hands its \\*qt.C to a parameter that is not one, so what it can reach there is testing.TB's own methods, which bind to whichever test the \\*qt.C came from"
		c.Assert(handleHelper(c), qt.Equals, "x")
	})

	c.Run("handle through a function value", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run; no fix under -only-stable-fixes: the closure hands its \\*qt.C to a parameter that is not one, so what it can reach there is testing.TB's own methods, which bind to whichever test the \\*qt.C came from"
		holder := opaqueHandleHolder{take: handleHelper}
		c.Assert(holder.take(c), qt.Equals, "x")
	})

	c.Run("handle through a named function type", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run; no fix under -only-stable-fixes: the closure hands its \\*qt.C to a parameter that is not one, so what it can reach there is testing.TB's own methods, which bind to whichever test the \\*qt.C came from"
		holder := namedHandlerHolder{take: handleHelper}
		c.Assert(holder.take(c), qt.Equals, "x")
	})

	c.Run("interface that declares Defer", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run; no fix: the closure calls c.Defer, and a bare qt.New\\(t\\) supplies no Done\\(\\) the way C.Run does"
		c.Assert(deferringHelper(c), qt.Equals, "x")
	})

	c.Run("alias behind a function value", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run; no fix: the \\*qt.C is handed to holder.take\\(\\.\\.\\.\\), so what it can reach includes \\(\\*qt.C\\).Defer, which panics unless Done\\(\\) ran — give that function a \\*testing.T instead and this converts"
		holder := opaqueAliasHolder{take: aliasDeferrer}
		c.Assert(holder.take(c), qt.Equals, "x")
	})

	c.Run("plain helper", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Assert(plainHelper(c), qt.Equals, "x")
	})

	c.Run("deferred", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run; no fix: the closure calls c.Defer, and a bare qt.New\\(t\\) supplies no Done\\(\\) the way C.Run does"
		c.Defer(func() {})
		c.Done()
	})
}

// The -only-stable-fixes branch of the explanation. A test-scoped method is
// not a correctness hazard the way Defer is, so the clause names the flag that
// withheld the fix rather than a panic that would not happen.
func TestOnlyStableFixesNamesTheFlag(t *testing.T) {
	c := qt.New(t)

	c.Run("cleanup", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run; no fix under -only-stable-fixes: the closure calls c.Cleanup, which binds to whichever test the \\*qt.C came from"
		c.Cleanup(func() {})
		c.Assert(1, qt.Equals, 1)
	})
}

// handleHelper takes the checker as a testing.TB, which is the case the type
// answer exists for: Defer and Done are C's own, so nothing inside can name
// them however the body is written, while Cleanup, TempDir and Setenv come
// from TB itself and keep the fix withheld under -only-stable-fixes.
func handleHelper(handle testing.TB) string { return "x" }

// opaqueHandleHolder hands the checker to a function value, which no
// body-reading can follow -- and does not need to, for the same reason.
type opaqueHandleHolder struct {
	take func(handle testing.TB) string
}

// deferrer declares Defer itself, so a *qt.C handed to it has Defer called on
// it without the callee ever naming *qt.C.
//
// This is why the type answer asks the method set rather than the identity. A
// rule asking "is this parameter a checker" answers no here, calls the hazard
// unreachable, and rewrites a subtest whose deferred function then never runs
// -- silently, because nothing fails.
type deferrer interface {
	Defer(f func())
	Done()
}

func deferringHelper(d deferrer) string {
	d.Defer(func() {})
	return "x"
}

// cAlias spells C by another name, so *cAlias IS *qt.C. The method set answers
// that correctly however the alias is written, where an identity check that
// did not resolve it would not.
type cAlias = qt.C

// aliasTaker puts the alias behind a function value, so the fixture turns on
// the type answer alone: there is no body to fall back on.
type aliasTaker func(c *cAlias) string

type opaqueAliasHolder struct {
	take aliasTaker
}

func aliasDeferrer(c *cAlias) string {
	c.Defer(func() {})
	return "x"
}

// Handler is a defined function type wrapping the same parameter shape
// opaqueHandleHolder's plain func field already exercises. paramTypeAt has
// to see through the name to its underlying signature to still answer by
// parameter type when the field's type is not itself a function literal
// type.
type Handler func(handle testing.TB) string

// namedHandlerHolder hands the checker to a function value held through a
// defined function type rather than an unnamed one.
type namedHandlerHolder struct {
	take Handler
}

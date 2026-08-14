package testingrun

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// The plain shape. c.Run was the receiver's only use, so its declaration goes
// with the rewrite.
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

// The receiver is matched on its type, not on the identifier c, and so is the
// closure's parameter.
func TestOtherName(t *testing.T) {
	qc := qt.New(t)
	qc.Run("sub", func(inner *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		inner.Assert(1, qt.Equals, 1)
	})
}

// Nested subtests. The inner rewrite names the parameter the outer rewrite
// introduces, so the two land as one consistent set of edits.
func TestNested(t *testing.T) {
	c := qt.New(t)
	c.Run("outer", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Run("inner", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
			c.Assert(1, qt.Equals, 1)
		})
	})
}

// The closure already refers to the enclosing test's t, so the new parameter
// takes a different name and that reference keeps meaning the parent.
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

// A closure calling a test-scoped *qt.C method is reported and still fixed by
// default; only -only-stable-fixes withholds it.
func TestTestScopedMethod(t *testing.T) {
	c := qt.New(t)
	c.Run("sub", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Cleanup(func() {})
		c.Assert(1, qt.Equals, 1)
	})
}

// A half-migrated shape: the outer subtest is already t.Run and the inner one
// is still c.Run, so the site sits inside a closure that is itself inside the
// test function.
//
// The rule plans the outermost function and nothing smaller, and this is the
// input that shows why. Planning each function encountered rather than only
// the outermost ones plans this site twice — once from the test function and
// once from the closure — and reports it twice. Every consumer that groups by
// position hides that: the text driver and analysistest both match on
// position, and -fix collapses the two identical edit sets. The -json output
// does not, and neither does a count of what was reported.
func TestHalfMigrated(t *testing.T) {
	t.Run("sub", func(t *testing.T) {
		c := qt.New(t)
		c.Run("inner", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
			c.Assert(1, qt.Equals, 1)
		})
	})
}

// Not reported: the subtest is a named function, whose func(*qt.C) signature
// t.Run will not take. Rewriting it means changing a declaration that may
// have callers elsewhere, so it is out of scope.
func namedSubtest(c *qt.C) {
	c.Assert(1, qt.Equals, 1)
}

func TestNamedFunc(t *testing.T) {
	c := qt.New(t)
	c.Run("sub", namedSubtest)
}

// Not reported: the *qt.C arrived as a parameter, so there is no statically
// known *testing.T to name.
func helper(c *qt.C) {
	c.Run("sub", func(c *qt.C) {
		c.Assert(1, qt.Equals, 1)
	})
}

// Not reported either, and the inner call is the point. Its receiver is the
// parameter the outer closure would be given, so whether it can be reported at
// all is the outer site's answer and not its own: the outer site was declined
// for having no *testing.T to name, and a diagnostic on the inner one would
// name a receiver that no rewrite is going to create. A reported site that
// carries no fix is exactly what this rule does not do.
func nestedHelper(c *qt.C) {
	c.Run("outer", func(c *qt.C) {
		c.Run("inner", func(c *qt.C) {
			c.Assert(1, qt.Equals, 1)
		})
	})
}

// Not reported: already the target form.
func TestAlreadyTestingRun(t *testing.T) {
	t.Run("sub", func(t *testing.T) {
		c := qt.New(t)
		c.Assert(1, qt.Equals, 1)
	})
}

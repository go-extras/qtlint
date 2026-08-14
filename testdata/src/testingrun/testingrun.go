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

// Not reported: already the target form.
func TestAlreadyTestingRun(t *testing.T) {
	t.Run("sub", func(t *testing.T) {
		c := qt.New(t)
		c.Assert(1, qt.Equals, 1)
	})
}

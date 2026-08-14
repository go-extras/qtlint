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

// Nested subtests where the outer closure names the enclosing t, so the outer
// rewrite has to take a different parameter name. The inner call must use
// that name: using the enclosing t instead would still compile and would
// attach the inner subtest to the parent test rather than to "outer".
func TestNestedRenamedOuter(t *testing.T) {
	c := qt.New(t)
	c.Run("outer", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		t.Log("parent")
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

// A closure calling a test-scoped *qt.C method is fixed by default. Only
// -only-stable-fixes withholds it; see the testingrunonlystable package.
func TestTestScopedMethod(t *testing.T) {
	c := qt.New(t)
	c.Run("sub", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Cleanup(func() {})
		c.Assert(1, qt.Equals, 1)
	})
}

// A one-line closure body: the opening brace, the statement and the closing
// brace share a line, so the rewrite must open a line for the statement it
// inserts instead of running the two together.
func TestSingleLineBody(t *testing.T) {
	c := qt.New(t)
	c.Run("sub", func(c *qt.C) { c.Assert(1, qt.Equals, 1) }) // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
}

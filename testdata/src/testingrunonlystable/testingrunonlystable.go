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
func TestWithheldMethods(t *testing.T) {
	c := qt.New(t)

	c.Run("parallel", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Parallel()
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
	c.Run("defer", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Defer(func() {})
	})
	c.Run("mkdir", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Assert(c.Mkdir("d"), qt.Equals, "d")
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

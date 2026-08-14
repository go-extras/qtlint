// Package testingrunacross is the fixture for a c.Run whose receiver is bound
// further out than the closure it sits in.
//
// The rewrite gives each closure a *testing.T parameter, and a site writes the
// name of the parameter its receiver's closure is given. When the two are the
// same closure the name is chosen against that closure's own identifiers and
// nothing else can hide it. When they are not, the name is written across the
// closures in between, and those are getting parameters from the same plan: a
// parameter introduced in between hides the one that was meant. Both spellings
// compile and both pass, so the only thing that moves is the subtest's name.
package testingrunacross

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// Three levels, where the middle closure renames its own *qt.C so that c in
// its body still means the outer one. Naming the middle closure's parameter t
// as well would leave "deep" reading the middle subtest's t, and the subtest
// would move from outer/deep to outer/middle/deep.
func TestGrandparentReceiver(t *testing.T) {
	c := qt.New(t)
	c.Run("outer", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Run("middle", func(mid *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
			mid.Assert(0, qt.Equals, 0)
			c.Run("deep", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
				c.Assert(1, qt.Equals, 1)
			})
		})
	})
}

// Four levels, so that two closures sit between the receiver and the site that
// names it. Keeping only the closest one clear of the outer name is not
// enough: the second would then be free to take that name back.
func TestGreatGrandparentReceiver(t *testing.T) {
	c := qt.New(t)
	c.Run("l1", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Run("l2", func(m2 *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
			m2.Assert(0, qt.Equals, 0)
			c.Run("l3", func(m3 *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
				m3.Assert(0, qt.Equals, 0)
				c.Run("deep", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
					c.Assert(1, qt.Equals, 1)
				})
			})
		})
	})
}

// The name written across an intermediate closure need not come from this
// plan. Here "deep" runs on a *qt.C made by its own qt.New, so the rewrite
// writes the test function's own t across the closure in between, and that
// closure must not be given a parameter called t either. Without that, "deep"
// moves two levels, from A/X plus a sibling deep to A/X/deep.
//
// The two closures both end up called t2, which is allowed to look odd: X is
// kept clear of t because a site inside it writes t, and of nothing else,
// because nothing inside it writes A's parameter. Keeping it clear of every
// enclosing name as well is the change that costs every plain nest its t.
func TestSourceNameAcross(t *testing.T) {
	c := qt.New(t)
	c.Run("A", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		d := qt.New(t)
		c.Run("X", func(mid *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
			mid.Assert(0, qt.Equals, 0)
			d.Run("deep", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
				c.Assert(1, qt.Equals, 1)
			})
		})
	})
}

// The acceptance control. Nothing is written across anything here, so every
// closure keeps the plain t: shadowing is what makes a nest of
// t.Run(…, func(t *testing.T)) read the way Go tests are written, and a rule
// that renamed unconditionally would take that away from every nest to fix the
// few that need it.
func TestPlainNestKeepsT(t *testing.T) {
	c := qt.New(t)
	c.Run("outer", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Assert(0, qt.Equals, 0)
		c.Run("inner", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
			c.Assert(1, qt.Equals, 1)
		})
	})
}

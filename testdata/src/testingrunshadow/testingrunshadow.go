// Package testingrunshadow holds inputs where a name the rewrite would write
// does not mean, at the point it would be written, what the file's import
// list says it means.
//
// Each input compiles: the shapes that break are the ones the original never
// spells out, so only the rewrite would introduce the reference.
package testingrunshadow

import (
	"testing"

	qt "github.com/frankban/quicktest"
	quicktest "github.com/frankban/quicktest"
)

// The new closure parameter is written as "*testing.T", so testing has to
// still name the package where the signature goes. The input compiles because
// its own closure signature never mentions testing — only the rewrite would.
func TestShadowedTestingName(t *testing.T) {
	testing := 1
	_ = testing
	c := qt.New(t)
	c.Run("sub", func(c *qt.C) {
		c.Assert(1, qt.Equals, 1)
	})
}

// The inserted "c := qt.New(t)" names quicktest under the first name the file
// imports it under, which need not be a name that means the package at the
// top of the closure body. Here the *qt.C is spelled through the file's other
// import of the same path, so the signature resolves and the insertion point
// does not.
func TestShadowedQuicktestName(t *testing.T) {
	qt := 1
	_ = qt
	c := quicktest.New(t)
	c.Run("sub", func(c *quicktest.C) {
		c.Assert(1, quicktest.Equals, 1)
	})
}

// The shadow is confined to a block that has closed again, so testing means
// the package where the new parameter type goes and the rewrite lands. A
// check that asked whether the function declares anything called testing,
// rather than what the name means where the rewrite writes it, would decline
// this one.
func TestBlockScopedTestingShadow(t *testing.T) {
	{
		testing := 1
		_ = testing
	}
	c := qt.New(t)
	c.Run("sub", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Assert(1, qt.Equals, 1)
	})
}

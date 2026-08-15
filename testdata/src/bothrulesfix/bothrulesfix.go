// Package bothrulesfix pins the rewrites of the two opt-in rules applied in
// one pass.
//
// The order test next door uses its own fixture and keeps the two rules from
// meeting on one receiver, so the combined FIX path had no coverage: each rule
// plans against the same declaration without seeing the other, and applied
// together they could produce a file that does not compile.
package bothrulesfix

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// The collision. The receiver's only other use is the c.Run, so
// -require-testing-run would remove its declaration — while
// -require-qt-c-receiver rewrites the qt.Assert above it into a use of exactly
// that declaration. The declaration has to survive.
func TestReceiverKeptForTheOtherRule(t *testing.T) {
	c := qt.New(t)
	qt.Assert(t, 1, qt.Equals, 1)                             // want "qtlint: use c.Assert\\(...\\) instead of qt.Assert\\(t, ...\\)"
	c.Run("sub", func(c *qt.C) { c.Assert(2, qt.Equals, 2) }) // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
}

// The control. With no package-level assertion to rewrite, nothing will use
// the declaration afterwards and it goes with the c.Run, as it did before.
func TestReceiverStillRemovedWithoutTheOtherRule(t *testing.T) {
	c := qt.New(t)
	c.Run("sub", func(c *qt.C) { c.Assert(3, qt.Equals, 3) }) // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
}

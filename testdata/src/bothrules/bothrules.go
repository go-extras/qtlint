// Package bothrules is the fixture for the two opt-in rules running together.
//
// Every other fixture turns on at most one of them, so the combined path — two
// house-style rules and the default set reporting into one pass — had no
// coverage at all. The order the diagnostics come out in is pinned by
// TestAnalyzer, which reads the reported sequence rather than the want
// comments: a want comment is matched by position and says nothing about the
// order its diagnostic arrived in.
package bothrules

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// The three rules fire on interleaved lines, and no two of them share a walk:
// the len/qt.Equals report comes from the default Preorder, the qt.Assert and
// qt.Check reports from -require-qt-c-receiver's WithStack, and the c.Run
// report from -require-testing-run's own walk of the file.
//
// c is asserted on directly as well, so -require-testing-run keeps its
// declaration and the two rules' rewrites do not contradict each other.
func TestBothRules(t *testing.T) {
	nums := []int{1, 2, 3}
	c := qt.New(t)
	c.Assert(0, qt.Equals, 0)

	qt.Assert(t, 1, qt.Equals, 1) // want "qtlint: use c.Assert\\(...\\) instead of qt.Assert\\(t, ...\\)"

	c.Run("sub", func(c *qt.C) { // want "qtlint: use t.Run with a per-subtest qt.New instead of c.Run"
		c.Assert(len(nums), qt.Equals, 3) // want "qtlint: use qt.HasLen instead of len\\(x\\), qt.Equals"
	})

	qt.Check(t, 3, qt.Equals, 3) // want "qtlint: use c.Check\\(...\\) instead of qt.Check\\(t, ...\\)"
}

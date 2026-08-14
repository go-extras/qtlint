package qtcreceiverfix

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// Case 1: an existing *qt.C built from this t is reused.
func TestExistingC(t *testing.T) {
	c := qt.New(t)
	c.Assert(0, qt.Equals, 0)

	qt.Assert(t, 1, qt.Equals, 1) // want "qtlint: use c.Assert\\(...\\) instead of qt.Assert\\(t, ...\\)"
	qt.Check(t, 2, qt.Equals, 2)  // want "qtlint: use c.Check\\(...\\) instead of qt.Check\\(t, ...\\)"
}

// Case 2: a *qt.C is created once, however many assertions need it.
func TestNoC(t *testing.T) {
	qt.Assert(t, 1, qt.Equals, 1)                        // want "qtlint: use c.Assert\\(...\\) instead of qt.Assert\\(t, ...\\)"
	qt.Check(t, 2, qt.Equals, 2)                         // want "qtlint: use c.Check\\(...\\) instead of qt.Check\\(t, ...\\)"
	qt.Assert(t, 3, qt.Equals, 3, qt.Commentf("third")) // want "qtlint: use c.Assert\\(...\\) instead of qt.Assert\\(t, ...\\)"
}

// Case 3: the helper gets its own *qt.C.
func assertOne(t *testing.T, got int) {
	qt.Assert(t, got, qt.Equals, 1) // want "qtlint: use c.Assert\\(...\\) instead of qt.Assert\\(t, ...\\)"
}

// The subtest closure binds t, so that is where the *qt.C is created.
func TestSubtest(t *testing.T) {
	c := qt.New(t)
	c.Assert(0, qt.Equals, 0)

	t.Run("sub", func(t *testing.T) {
		qt.Assert(t, 1, qt.Equals, 1) // want "qtlint: use c.Assert\\(...\\) instead of qt.Assert\\(t, ...\\)"
	})
}

// The name c is taken, so the next free one is used.
func TestNameTaken(t *testing.T) {
	c := 42
	qt.Assert(t, c, qt.Equals, 42) // want "qtlint: use c2.Assert\\(...\\) instead of qt.Assert\\(t, ...\\)"
}

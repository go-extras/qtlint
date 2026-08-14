package qtcreceiver

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// Case 1: a *qt.C created from this t is already in scope, so the fix reuses
// it rather than creating a second one.
func TestExistingC(t *testing.T) {
	c := qt.New(t)
	c.Assert(0, qt.Equals, 0)

	qt.Assert(t, 1, qt.Equals, 1) // want "qtlint: use c.Assert\\(...\\) instead of qt.Assert\\(t, ...\\)"
	qt.Check(t, 2, qt.Equals, 2)  // want "qtlint: use c.Check\\(...\\) instead of qt.Check\\(t, ...\\)"
}

// Case 2: no *qt.C is in scope, so the fix creates one in the function that
// binds t.
func TestNoC(t *testing.T) {
	qt.Assert(t, 1, qt.Equals, 1) // want "qtlint: use c.Assert\\(...\\) instead of qt.Assert\\(t, ...\\)"
	qt.Check(t, 2, qt.Equals, 2)  // want "qtlint: use c.Check\\(...\\) instead of qt.Check\\(t, ...\\)"
}

// Case 3: a helper taking its own *testing.T gets its own *qt.C; nothing is
// smuggled in from a caller.
func assertOne(t *testing.T, got int) {
	qt.Assert(t, got, qt.Equals, 1) // want "qtlint: use c.Assert\\(...\\) instead of qt.Assert\\(t, ...\\)"
}

// The insertion point is the function that binds the t being asserted
// against, which here is the subtest closure and not the parent test.
func TestSubtest(t *testing.T) {
	c := qt.New(t)
	c.Assert(0, qt.Equals, 0)

	t.Run("sub", func(t *testing.T) {
		qt.Assert(t, 1, qt.Equals, 1) // want "qtlint: use c.Assert\\(...\\) instead of qt.Assert\\(t, ...\\)"
	})
}

// The name c is taken by something that is not a *qt.C, so the fix picks the
// next free name instead of declining.
func TestNameTaken(t *testing.T) {
	c := 42
	qt.Assert(t, c, qt.Equals, 42) // want "qtlint: use c2.Assert\\(...\\) instead of qt.Assert\\(t, ...\\)"
}

// A *qt.C that is declared further down is not reused: the rewritten call
// would name it before its declaration.
func TestDeclaredLater(t *testing.T) {
	qt.Assert(t, 1, qt.Equals, 1) // want "qtlint: use c2.Assert\\(...\\) instead of qt.Assert\\(t, ...\\)"

	c := qt.New(t)
	c.Assert(2, qt.Equals, 2)
}

// A *qt.C built from a different *testing.T is not reused: c belongs to the
// subtest, and the assertion is against the parent's t.
func TestOtherT(t *testing.T) {
	t.Run("sub", func(inner *testing.T) {
		c := qt.New(inner)
		c.Assert(0, qt.Equals, 0)

		qt.Assert(t, 1, qt.Equals, 1) // want "qtlint: use c2.Assert\\(...\\) instead of qt.Assert\\(t, ...\\)"
	})
}

// Not reported: the assertion already goes through a *qt.C.
func TestAlreadyOnC(t *testing.T) {
	c := qt.New(t)
	c.Assert(1, qt.Equals, 1)
	c.Check(2, qt.Equals, 2)
}

// Not reported: the first argument is a *testing.B, not a *testing.T.
func BenchmarkOther(b *testing.B) {
	qt.Assert(b, 1, qt.Equals, 1)
}

// Not reported: the first argument is a testing.TB, not a *testing.T.
func assertTB(tb testing.TB) {
	qt.Assert(tb, 1, qt.Equals, 1)
}

// Not reported: the first argument is not an identifier, so there is no
// binding function to create a *qt.C in.
type harness struct {
	t *testing.T
}

func (h harness) assert() {
	qt.Assert(h.t, 1, qt.Equals, 1)
}

package eqistruefix

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestEqualityIsTrueFix(t *testing.T) {
	c := qt.New(t)
	x := 42
	y := 42

	qt.Assert(t, x == y, qt.IsTrue)    // want "qtlint: use qt.Equals instead of x == y, qt.IsTrue"
	c.Assert(x == y, qt.IsTrue)        // want "qtlint: use qt.Equals instead of x == y, qt.IsTrue"

	a := "hello"
	b := "hello"
	qt.Check(t, a == b, qt.IsTrue)     // want "qtlint: use qt.Equals instead of x == y, qt.IsTrue"
	c.Check(a == b, qt.IsTrue)         // want "qtlint: use qt.Equals instead of x == y, qt.IsTrue"

	qt.Assert(t, x == y, qt.IsFalse)   // want `qtlint: use qt.Not\(qt.Equals\) instead of x == y, qt.IsFalse`
	c.Assert(x == y, qt.IsFalse)       // want `qtlint: use qt.Not\(qt.Equals\) instead of x == y, qt.IsFalse`

	qt.Assert(t, x != y, qt.IsTrue)    // want `qtlint: use qt.Not\(qt.Equals\) instead of x != y, qt.IsTrue`
	c.Assert(x != y, qt.IsTrue)        // want `qtlint: use qt.Not\(qt.Equals\) instead of x != y, qt.IsTrue`

	qt.Assert(t, x != y, qt.IsFalse)   // want "qtlint: use qt.Equals instead of x != y, qt.IsFalse"
	c.Assert(x != y, qt.IsFalse)       // want "qtlint: use qt.Equals instead of x != y, qt.IsFalse"
}

package nilcmpfix

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestNilComparisonFix(t *testing.T) {
	c := qt.New(t)
	var x *int

	qt.Assert(t, x == nil, qt.IsTrue)  // want "qtlint: use qt.IsNil instead of x == nil, qt.IsTrue"
	c.Assert(x == nil, qt.IsTrue)      // want "qtlint: use qt.IsNil instead of x == nil, qt.IsTrue"

	qt.Assert(t, x == nil, qt.IsFalse) // want "qtlint: use qt.IsNotNil instead of x == nil, qt.IsFalse"
	c.Assert(x == nil, qt.IsFalse)     // want "qtlint: use qt.IsNotNil instead of x == nil, qt.IsFalse"

	qt.Check(t, x != nil, qt.IsTrue)   // want "qtlint: use qt.IsNotNil instead of x != nil, qt.IsTrue"
	c.Check(x != nil, qt.IsTrue)       // want "qtlint: use qt.IsNotNil instead of x != nil, qt.IsTrue"

	qt.Check(t, x != nil, qt.IsFalse)  // want "qtlint: use qt.IsNil instead of x != nil, qt.IsFalse"
	c.Check(x != nil, qt.IsFalse)      // want "qtlint: use qt.IsNil instead of x != nil, qt.IsFalse"
}

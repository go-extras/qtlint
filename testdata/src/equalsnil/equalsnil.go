package equalsnil

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

type sometype struct{}

// Test case: x, qt.Equals, nil should be replaced with x, qt.IsNil.
func TestEqualsNilAssert(t *testing.T) {
	c := qt.New(t)
	var x *int

	qt.Assert(t, x, qt.Equals, nil) // want "qtlint: use qt.IsNil instead of qt.Equals, nil"
	c.Assert(x, qt.Equals, nil)     // want "qtlint: use qt.IsNil instead of qt.Equals, nil"
}

// Test case: qt.Check should also be checked.
func TestEqualsNilCheck(t *testing.T) {
	c := qt.New(t)
	var x *int

	qt.Check(t, x, qt.Equals, nil) // want "qtlint: use qt.IsNil instead of qt.Equals, nil"
	c.Check(x, qt.Equals, nil)     // want "qtlint: use qt.IsNil instead of qt.Equals, nil"
}

// Test case: a typed nil is exactly the buggy comparison quicktest warns about.
func TestEqualsTypedNil(t *testing.T) {
	c := qt.New(t)

	qt.Assert(t, (*sometype)(nil), qt.Equals, nil) // want "qtlint: use qt.IsNil instead of qt.Equals, nil"
	c.Assert((*sometype)(nil), qt.Equals, nil)     // want "qtlint: use qt.IsNil instead of qt.Equals, nil"
}

// Test case: trailing arguments (e.g. qt.Commentf) are preserved.
func TestEqualsNilWithComment(t *testing.T) {
	c := qt.New(t)
	var x *int

	c.Assert(x, qt.Equals, nil, qt.Commentf("should be nil")) // want "qtlint: use qt.IsNil instead of qt.Equals, nil"
}

// Negative test cases: patterns that should NOT trigger the rule.

// Using qt.IsNil directly is correct.
func TestIsNilDirect(t *testing.T) {
	c := qt.New(t)
	var x *int

	qt.Assert(t, x, qt.IsNil)
	c.Assert(x, qt.IsNil)
}

// qt.Equals with a non-nil want is fine.
func TestEqualsNonNil(t *testing.T) {
	c := qt.New(t)
	x := 42

	qt.Assert(t, x, qt.Equals, 42)
	c.Assert(x, qt.Equals, 42)
}

// A different checker with nil is not flagged by this rule.
func TestDeepEqualsNil(t *testing.T) {
	c := qt.New(t)
	var x []int

	qt.Assert(t, x, qt.DeepEquals, nil)
	c.Assert(x, qt.DeepEquals, nil)
}

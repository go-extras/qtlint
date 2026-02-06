package nilcmp

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// Test case: x == nil, qt.IsTrue should be replaced with x, qt.IsNil
func TestEqNilIsTrue(t *testing.T) {
	c := qt.New(t)
	var x *int

	qt.Assert(t, x == nil, qt.IsTrue) // want "qtlint: use qt.IsNil instead of x == nil, qt.IsTrue"
	c.Assert(x == nil, qt.IsTrue)     // want "qtlint: use qt.IsNil instead of x == nil, qt.IsTrue"
}

// Test case: x == nil, qt.IsFalse should be replaced with x, qt.IsNotNil
func TestEqNilIsFalse(t *testing.T) {
	c := qt.New(t)
	var x *int

	qt.Assert(t, x == nil, qt.IsFalse) // want "qtlint: use qt.IsNotNil instead of x == nil, qt.IsFalse"
	c.Assert(x == nil, qt.IsFalse)     // want "qtlint: use qt.IsNotNil instead of x == nil, qt.IsFalse"
}

// Test case: x != nil, qt.IsTrue should be replaced with x, qt.IsNotNil
func TestNeqNilIsTrue(t *testing.T) {
	c := qt.New(t)
	var x *int

	qt.Assert(t, x != nil, qt.IsTrue) // want "qtlint: use qt.IsNotNil instead of x != nil, qt.IsTrue"
	c.Assert(x != nil, qt.IsTrue)     // want "qtlint: use qt.IsNotNil instead of x != nil, qt.IsTrue"
}

// Test case: x != nil, qt.IsFalse should be replaced with x, qt.IsNil
func TestNeqNilIsFalse(t *testing.T) {
	c := qt.New(t)
	var x *int

	qt.Assert(t, x != nil, qt.IsFalse) // want "qtlint: use qt.IsNil instead of x != nil, qt.IsFalse"
	c.Assert(x != nil, qt.IsFalse)     // want "qtlint: use qt.IsNil instead of x != nil, qt.IsFalse"
}

// Test case: nil == x (nil on left side)
func TestNilOnLeft(t *testing.T) {
	c := qt.New(t)
	var x *int

	qt.Assert(t, nil == x, qt.IsTrue) // want "qtlint: use qt.IsNil instead of x == nil, qt.IsTrue"
	c.Assert(nil == x, qt.IsFalse)    // want "qtlint: use qt.IsNotNil instead of x == nil, qt.IsFalse"
}

// Test case: qt.Check should also be checked
func TestCheckVariants(t *testing.T) {
	c := qt.New(t)
	var x *int

	qt.Check(t, x == nil, qt.IsTrue) // want "qtlint: use qt.IsNil instead of x == nil, qt.IsTrue"
	c.Check(x == nil, qt.IsFalse)    // want "qtlint: use qt.IsNotNil instead of x == nil, qt.IsFalse"
}

// Test case: field access expressions
func TestFieldAccess(t *testing.T) {
	type resolver struct {
		Dial *int
	}
	c := qt.New(t)
	r := resolver{}

	c.Assert(r.Dial == nil, qt.IsFalse) // want "qtlint: use qt.IsNotNil instead of x == nil, qt.IsFalse"
	c.Assert(r.Dial == nil, qt.IsTrue)  // want "qtlint: use qt.IsNil instead of x == nil, qt.IsTrue"
}

// Negative test cases: patterns that should NOT trigger the rule

// Test case: non-nil comparisons with IsTrue (should be caught by equality rule, not this one)
func TestNonNilComparison(t *testing.T) {
	c := qt.New(t)
	x := 42
	y := 42

	// These should NOT trigger the nil comparison rule
	c.Assert(x == y, qt.IsTrue)  // want "qtlint: use qt.Equals instead of x == y, qt.IsTrue"
	qt.Assert(t, x == y, qt.IsTrue) // want "qtlint: use qt.Equals instead of x == y, qt.IsTrue"
}

// Test case: already correct patterns
func TestCorrectPatterns(t *testing.T) {
	c := qt.New(t)
	var x *int

	// These are correct and should not trigger the rule
	qt.Assert(t, x, qt.IsNil)
	c.Assert(x, qt.IsNotNil)
	qt.Assert(t, x, qt.IsNotNil)
	c.Assert(x, qt.IsNil)
}

// Test case: boolean with IsTrue (not a comparison)
func TestBoolIsTrue(t *testing.T) {
	c := qt.New(t)
	x := true

	// Not a binary expression, should not trigger
	c.Assert(x, qt.IsTrue)
	c.Assert(x, qt.IsFalse)
}

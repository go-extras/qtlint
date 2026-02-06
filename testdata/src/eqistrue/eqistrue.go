package eqistrue

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// Test case: x == y, qt.IsTrue should be replaced with x, qt.Equals, y
func TestEqualityIsTrue(t *testing.T) {
	c := qt.New(t)
	x := 42
	y := 42

	qt.Assert(t, x == y, qt.IsTrue)    // want "qtlint: use qt.Equals instead of x == y, qt.IsTrue"
	c.Assert(x == y, qt.IsTrue)        // want "qtlint: use qt.Equals instead of x == y, qt.IsTrue"
}

// Test case: qt.Check should also be checked
func TestCheckEqualityIsTrue(t *testing.T) {
	c := qt.New(t)
	x := "hello"
	y := "hello"

	qt.Check(t, x == y, qt.IsTrue)    // want "qtlint: use qt.Equals instead of x == y, qt.IsTrue"
	c.Check(x == y, qt.IsTrue)        // want "qtlint: use qt.Equals instead of x == y, qt.IsTrue"
}

// Test case: different expressions
func TestEqualityIsTrueExpressions(t *testing.T) {
	c := qt.New(t)
	a := 1
	b := 2

	qt.Assert(t, a+1 == b, qt.IsTrue)  // want "qtlint: use qt.Equals instead of x == y, qt.IsTrue"
	c.Assert(a == b+1, qt.IsTrue)      // want "qtlint: use qt.Equals instead of x == y, qt.IsTrue"
}

// Negative test cases: patterns that should NOT trigger the rule

// Test case: using qt.Equals directly (correct pattern)
func TestEqualsCorrect(t *testing.T) {
	c := qt.New(t)
	x := 42

	// These are correct and should not trigger the rule
	qt.Assert(t, x, qt.Equals, 42)
	c.Assert(x, qt.Equals, 42)
}

// Test case: non-equality binary expressions with qt.IsTrue
func TestNonEqualityIsTrue(t *testing.T) {
	c := qt.New(t)
	x := 1
	y := 2

	// These should not trigger the rule (not == operator)
	qt.Assert(t, x < y, qt.IsTrue)
	c.Assert(x != y, qt.IsTrue)
}

// Test case: equality expression with other checkers
func TestEqualityOtherChecker(t *testing.T) {
	c := qt.New(t)
	x := true

	// These should not trigger the rule (not qt.IsTrue)
	qt.Assert(t, x, qt.IsFalse)
	c.Assert(x, qt.IsFalse)
}

// Test case: simple boolean with qt.IsTrue (not a comparison)
func TestBoolIsTrue(t *testing.T) {
	c := qt.New(t)
	x := true

	// These should not trigger the rule (not a binary expression)
	qt.Assert(t, x, qt.IsTrue)
	c.Assert(x, qt.IsTrue)
}

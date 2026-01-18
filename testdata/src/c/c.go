package c

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// Test case: allowed patterns that should NOT trigger warnings

// Using qt.IsNotNil directly is correct
func TestIsNotNil(t *testing.T) {
	c := qt.New(t)
	var x *int
	c.Assert(x, qt.IsNotNil)
	qt.Assert(t, x, qt.IsNotNil)
}

// Using qt.IsFalse directly is correct
func TestIsFalse(t *testing.T) {
	c := qt.New(t)
	value := false
	c.Assert(value, qt.IsFalse)
	qt.Assert(t, value, qt.IsFalse)
}

// Using qt.IsTrue directly is correct
func TestIsTrue(t *testing.T) {
	c := qt.New(t)
	value := true
	c.Assert(value, qt.IsTrue)
	qt.Assert(t, value, qt.IsTrue)
}

// Using qt.Not with other checkers is allowed
func TestNotWithOtherCheckers(t *testing.T) {
	c := qt.New(t)
	c.Assert(42, qt.Not(qt.Equals), 0)
	c.Assert([]int{1, 2}, qt.Not(qt.DeepEquals), []int{3, 4})
}

// Using qt.IsNil is correct
func TestIsNil(t *testing.T) {
	c := qt.New(t)
	var x *int
	c.Assert(x, qt.IsNil)
	qt.Assert(t, x, qt.IsNil)
}


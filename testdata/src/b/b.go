package b

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// Test case: method calls with different receiver types
func TestMethodCalls(t *testing.T) {
	c := qt.New(t)
	var x *int

	// Method call on c
	c.Assert(x, qt.Not(qt.IsNil)) // want "qtlint: use qt.IsNotNil instead of qt.Not\\(qt.IsNil\\)"

	// Nested in Run
	c.Run("subtest", func(c *qt.C) {
		c.Assert(x, qt.Not(qt.IsNil)) // want "qtlint: use qt.IsNotNil instead of qt.Not\\(qt.IsNil\\)"
	})
}

// Test case: multiple Not patterns in same function
func TestMultiplePatterns(t *testing.T) {
	c := qt.New(t)
	var x *int
	value := false

	c.Assert(x, qt.Not(qt.IsNil))      // want "qtlint: use qt.IsNotNil instead of qt.Not\\(qt.IsNil\\)"
	c.Assert(value, qt.Not(qt.IsTrue)) // want "qtlint: use qt.IsFalse instead of qt.Not\\(qt.IsTrue\\)"
}


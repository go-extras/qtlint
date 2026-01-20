package haslen

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// Test case: len(x), qt.Equals should be replaced with x, qt.HasLen
func TestLenEquals(t *testing.T) {
	c := qt.New(t)
	x := []int{1, 2, 3}
	
	qt.Assert(t, len(x), qt.Equals, 3) // want "qtlint: use qt.HasLen instead of len\\(x\\), qt.Equals"
	c.Assert(len(x), qt.Equals, 3)     // want "qtlint: use qt.HasLen instead of len\\(x\\), qt.Equals"
}

// Test case: qt.Check should also be checked
func TestCheckLenEquals(t *testing.T) {
	c := qt.New(t)
	x := "hello"
	
	qt.Check(t, len(x), qt.Equals, 5) // want "qtlint: use qt.HasLen instead of len\\(x\\), qt.Equals"
	c.Check(len(x), qt.Equals, 5)     // want "qtlint: use qt.HasLen instead of len\\(x\\), qt.Equals"
}

// Test case: different variable names
func TestLenEqualsVariableNames(t *testing.T) {
	c := qt.New(t)
	mySlice := []string{"a", "b"}
	myMap := map[string]int{"key": 1}

	qt.Assert(t, len(mySlice), qt.Equals, 2) // want "qtlint: use qt.HasLen instead of len\\(x\\), qt.Equals"
	c.Assert(len(myMap), qt.Equals, 1)       // want "qtlint: use qt.HasLen instead of len\\(x\\), qt.Equals"
}

// Negative test cases: patterns that should NOT trigger the rule

// Test case: using qt.HasLen directly (correct pattern)
func TestHasLenCorrect(t *testing.T) {
	c := qt.New(t)
	x := []int{1, 2, 3}

	// These are correct and should not trigger the rule
	qt.Assert(t, x, qt.HasLen, 3)
	c.Assert(x, qt.HasLen, 3)
}

// Test case: len(x) with checkers other than qt.Equals
func TestLenWithOtherCheckers(t *testing.T) {
	c := qt.New(t)
	x := []int{1, 2, 3}

	// These should not trigger the rule
	qt.Assert(t, len(x), qt.DeepEquals, 3)
	c.Assert(len(x), qt.Not(qt.Equals), 0)
}

// Test case: non-len expressions with qt.Equals
func TestNonLenWithEquals(t *testing.T) {
	c := qt.New(t)
	x := 42

	// These should not trigger the rule
	qt.Assert(t, x, qt.Equals, 42)
	c.Assert(x+1, qt.Equals, 43)
}

// Test case: user-defined len function
func TestUserDefinedLen(t *testing.T) {
	c := qt.New(t)

	// User-defined len function
	len := func(s string) int {
		return 100
	}

	// This should not trigger the rule (not builtin len)
	qt.Assert(t, len("test"), qt.Equals, 100)
	c.Assert(len("test"), qt.Equals, 100)
}


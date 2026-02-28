package strcontains

import (
	"slices"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
)

// Test case: strings.Contains(x, y), qt.IsTrue should be replaced with x, qt.Contains, y
func TestStringsContainsIsTrue(t *testing.T) {
	c := qt.New(t)
	str := "hello world"

	qt.Assert(t, strings.Contains(str, "world"), qt.IsTrue) // want "qtlint: use qt.Contains instead of strings.Contains\\(x, y\\), qt.IsTrue"
	c.Assert(strings.Contains(str, "world"), qt.IsTrue)     // want "qtlint: use qt.Contains instead of strings.Contains\\(x, y\\), qt.IsTrue"
}

// Test case: strings.Contains(x, y), qt.IsFalse should be replaced with x, qt.Not(qt.Contains), y
func TestStringsContainsIsFalse(t *testing.T) {
	c := qt.New(t)
	str := "hello world"

	qt.Assert(t, strings.Contains(str, "foo"), qt.IsFalse) // want "qtlint: use qt.Not\\(qt.Contains\\) instead of strings.Contains\\(x, y\\), qt.IsFalse"
	c.Assert(strings.Contains(str, "foo"), qt.IsFalse)     // want "qtlint: use qt.Not\\(qt.Contains\\) instead of strings.Contains\\(x, y\\), qt.IsFalse"
}

// Test case: qt.Check should also be checked
func TestCheckStringsContains(t *testing.T) {
	c := qt.New(t)
	str := "test string"

	qt.Check(t, strings.Contains(str, "test"), qt.IsTrue) // want "qtlint: use qt.Contains instead of strings.Contains\\(x, y\\), qt.IsTrue"
	c.Check(strings.Contains(str, "test"), qt.IsTrue)     // want "qtlint: use qt.Contains instead of strings.Contains\\(x, y\\), qt.IsTrue"
	qt.Check(t, strings.Contains(str, "foo"), qt.IsFalse) // want "qtlint: use qt.Not\\(qt.Contains\\) instead of strings.Contains\\(x, y\\), qt.IsFalse"
	c.Check(strings.Contains(str, "foo"), qt.IsFalse)     // want "qtlint: use qt.Not\\(qt.Contains\\) instead of strings.Contains\\(x, y\\), qt.IsFalse"
}

// Test case: different variable names and expressions
func TestStringsContainsVariableNames(t *testing.T) {
	c := qt.New(t)
	myString := "example text"
	substring := "text"

	qt.Assert(t, strings.Contains(myString, substring), qt.IsTrue) // want "qtlint: use qt.Contains instead of strings.Contains\\(x, y\\), qt.IsTrue"
	c.Assert(strings.Contains(myString, "example"), qt.IsTrue)     // want "qtlint: use qt.Contains instead of strings.Contains\\(x, y\\), qt.IsTrue"
	qt.Assert(t, strings.Contains("literal", "lit"), qt.IsTrue)    // want "qtlint: use qt.Contains instead of strings.Contains\\(x, y\\), qt.IsTrue"
}

// Test case: slices.Contains(x, y), qt.IsTrue should be replaced with x, qt.Contains, y
func TestSlicesContainsIsTrue(t *testing.T) {
	c := qt.New(t)
	slice := []int{1, 2, 3}

	qt.Assert(t, slices.Contains(slice, 2), qt.IsTrue) // want "qtlint: use qt.Contains instead of slices.Contains\\(x, y\\), qt.IsTrue"
	c.Assert(slices.Contains(slice, 2), qt.IsTrue)     // want "qtlint: use qt.Contains instead of slices.Contains\\(x, y\\), qt.IsTrue"
}

// Test case: slices.Contains(x, y), qt.IsFalse should be replaced with x, qt.Not(qt.Contains), y
func TestSlicesContainsIsFalse(t *testing.T) {
	c := qt.New(t)
	slice := []string{"a", "b", "c"}

	qt.Assert(t, slices.Contains(slice, "d"), qt.IsFalse) // want "qtlint: use qt.Not\\(qt.Contains\\) instead of slices.Contains\\(x, y\\), qt.IsFalse"
	c.Assert(slices.Contains(slice, "d"), qt.IsFalse)     // want "qtlint: use qt.Not\\(qt.Contains\\) instead of slices.Contains\\(x, y\\), qt.IsFalse"
}

// Test case: slices.Contains with qt.Check
func TestCheckSlicesContains(t *testing.T) {
	c := qt.New(t)
	slice := []int{10, 20, 30}

	qt.Check(t, slices.Contains(slice, 20), qt.IsTrue)  // want "qtlint: use qt.Contains instead of slices.Contains\\(x, y\\), qt.IsTrue"
	c.Check(slices.Contains(slice, 20), qt.IsTrue)      // want "qtlint: use qt.Contains instead of slices.Contains\\(x, y\\), qt.IsTrue"
	qt.Check(t, slices.Contains(slice, 99), qt.IsFalse) // want "qtlint: use qt.Not\\(qt.Contains\\) instead of slices.Contains\\(x, y\\), qt.IsFalse"
	c.Check(slices.Contains(slice, 99), qt.IsFalse)     // want "qtlint: use qt.Not\\(qt.Contains\\) instead of slices.Contains\\(x, y\\), qt.IsFalse"
}

// Negative test cases: patterns that should NOT trigger the rule

// Test case: using strings.Contains with other checkers (correct pattern)
func TestContainsCorrect(t *testing.T) {
	c := qt.New(t)
	str := "hello world"

	// These are correct and should not trigger the rule
	// (using strings.Contains with checkers other than IsTrue/IsFalse)
	_ = c
	_ = str
}

// Test case: strings.Contains with checkers other than qt.IsTrue/qt.IsFalse
func TestStringsContainsWithOtherCheckers(t *testing.T) {
	c := qt.New(t)
	str := "hello world"

	// These should not trigger the rule
	qt.Assert(t, strings.Contains(str, "world"), qt.Equals, true)
	c.Assert(strings.Contains(str, "world"), qt.DeepEquals, true)
}

// Test case: other functions named Contains
func TestOtherContainsFunctions(t *testing.T) {
	c := qt.New(t)

	// User-defined Contains function
	Contains := func(s, substr string) bool {
		return false
	}

	// This should not trigger the rule (not strings.Contains)
	qt.Assert(t, Contains("test", "test"), qt.IsTrue)
	c.Assert(Contains("test", "test"), qt.IsTrue)
}

// Test case: strings.Contains with wrong number of arguments
func TestStringsContainsWrongArgs(t *testing.T) {
	_ = t

	// This is invalid Go code but shouldn't panic the linter
	// (commented out because it won't compile)
	// qt.Assert(t, strings.Contains("test"), qt.IsTrue)
}

package strcontainsfix

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

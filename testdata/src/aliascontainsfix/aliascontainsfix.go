package aliascontainsfix

import (
	"testing"

	myslices "slices"
	mystrings "strings"

	qt "github.com/frankban/quicktest"
)

// Test case: strings.Contains with alias
func TestStringsContainsWithAlias(t *testing.T) {
	c := qt.New(t)
	str := "hello world"
	
	qt.Assert(t, mystrings.Contains(str, "world"), qt.IsTrue) // want "qtlint: use qt.Contains instead of mystrings.Contains\\(x, y\\), qt.IsTrue"
	c.Assert(mystrings.Contains(str, "foo"), qt.IsFalse)      // want "qtlint: use qt.Not\\(qt.Contains\\) instead of mystrings.Contains\\(x, y\\), qt.IsFalse"
}

// Test case: slices.Contains with alias
func TestSlicesContainsWithAlias(t *testing.T) {
	c := qt.New(t)
	slice := []int{1, 2, 3}
	
	qt.Assert(t, myslices.Contains(slice, 2), qt.IsTrue)   // want "qtlint: use qt.Contains instead of myslices.Contains\\(x, y\\), qt.IsTrue"
	c.Assert(myslices.Contains(slice, 99), qt.IsFalse)     // want "qtlint: use qt.Not\\(qt.Contains\\) instead of myslices.Contains\\(x, y\\), qt.IsFalse"
}


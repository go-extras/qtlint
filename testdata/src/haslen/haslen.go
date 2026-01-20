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


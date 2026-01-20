package haslenfix

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestLenEqualsFix(t *testing.T) {
	c := qt.New(t)
	x := []int{1, 2, 3}
	
	qt.Assert(t, len(x), qt.Equals, 3) // want "qtlint: use qt.HasLen instead of len\\(x\\), qt.Equals"
	c.Assert(len(x), qt.Equals, 3)     // want "qtlint: use qt.HasLen instead of len\\(x\\), qt.Equals"
	
	mySlice := []string{"a", "b"}
	qt.Check(t, len(mySlice), qt.Equals, 2) // want "qtlint: use qt.HasLen instead of len\\(x\\), qt.Equals"
	c.Check(len(mySlice), qt.Equals, 2)     // want "qtlint: use qt.HasLen instead of len\\(x\\), qt.Equals"
}


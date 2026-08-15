package pkg

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// TestZlast is a violation in the module that runs last.
func TestZlast(t *testing.T) {
	c := qt.New(t)
	var x *int
	c.Assert(x, qt.Not(qt.IsNil))
}

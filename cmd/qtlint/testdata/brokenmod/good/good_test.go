package good

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// TestGood is a plain violation in a module that loads correctly.
func TestGood(t *testing.T) {
	c := qt.New(t)
	var x *int
	c.Assert(x, qt.Not(qt.IsNil))
}

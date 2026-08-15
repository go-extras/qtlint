package sub

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// TestSub carries the nested module's unconstrained violation.
func TestSub(t *testing.T) {
	c := qt.New(t)
	var x *int
	c.Assert(x, qt.Not(qt.IsNil))
}

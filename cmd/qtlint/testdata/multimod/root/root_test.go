package root

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// TestRoot carries the outer module's unconstrained violation, so every run
// reports it whatever the build tags are.
func TestRoot(t *testing.T) {
	c := qt.New(t)
	var x *int
	c.Assert(x, qt.Not(qt.IsNil))
}

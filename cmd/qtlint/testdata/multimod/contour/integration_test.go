//go:build integration

package contour

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// TestOnlyTagged is reached only when the integration tag is set.
func TestOnlyTagged(t *testing.T) {
	c := qt.New(t)
	var x *int
	c.Assert(x, qt.Not(qt.IsNil))
}

package equalsboolfix

import (
	"testing"

	quick "github.com/frankban/quicktest"
)

// The fix writes the checker under whatever name the import has.
func TestAlias(t *testing.T) {
	c := quick.New(t)
	b := true

	c.Assert(b, quick.Equals, true)  // want `qt\.IsTrue instead of qt\.Equals, true`
	c.Assert(false, quick.Equals, b) // want `x, qt\.IsFalse instead of false, qt\.Equals, x`
}

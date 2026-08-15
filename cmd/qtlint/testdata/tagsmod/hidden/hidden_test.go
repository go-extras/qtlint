//go:build qtprobe

package hidden

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestHidden(t *testing.T) {
	c := qt.New(t)
	var x *int
	c.Assert(x, qt.Not(qt.IsNil))
}

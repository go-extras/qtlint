//go:build qtprobe

package quiet

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestQuiet(t *testing.T) {
	c := qt.New(t)
	var x *int
	c.Assert(x, qt.Not(qt.IsNil))
}

package plain

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestPlain(t *testing.T) {
	c := qt.New(t)
	var x *int
	c.Assert(x, qt.Not(qt.IsNil))
}

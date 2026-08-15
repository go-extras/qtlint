//go:build qtbeta

package beta

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func Testbeta(t *testing.T) {
	c := qt.New(t)
	var x *int
	c.Assert(x, qt.Not(qt.IsNil))
}

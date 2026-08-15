//go:build integration

package hush

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// TestHush is the only violation in its module, and it is behind a tag.
func TestHush(t *testing.T) {
	c := qt.New(t)
	var x *int
	c.Assert(x, qt.Not(qt.IsNil))
}

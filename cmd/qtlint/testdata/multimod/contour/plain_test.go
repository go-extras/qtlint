//go:build !integration

package contour

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// TestOnlyUntagged is dropped from the build as soon as the integration tag is
// set. It is why a tagged run cannot stand in for a plain one: a build tag only
// ever adds files to a build, so satisfying it removes this file from view.
func TestOnlyUntagged(t *testing.T) {
	c := qt.New(t)
	var x *int
	c.Assert(x, qt.Not(qt.IsNil))
}

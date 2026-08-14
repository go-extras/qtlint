package testingrunoff

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// This package is the flag gate's control. Every c.Run below is exactly what
// -require-testing-run reports, and none of them carries a want comment: the
// test runs it through a default analyzer, so a diagnostic here is a failure.
// If the rule ever fires without its flag, this package goes red.
func TestNotReportedByDefault(t *testing.T) {
	c := qt.New(t)
	c.Run("sub", func(c *qt.C) {
		c.Assert(1, qt.Equals, 1)
	})
}

func TestNestedNotReportedByDefault(t *testing.T) {
	c := qt.New(t)
	c.Run("outer", func(c *qt.C) {
		c.Run("inner", func(c *qt.C) {
			c.Cleanup(func() {})
			c.Assert(1, qt.Equals, 1)
		})
	})
}

package qtcreceiveroff

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// This package is the flag gate's control. Every call below is exactly what
// -require-qt-c-receiver reports, and none of them carries a want comment:
// the test runs it with a default analyzer, so a diagnostic here is a
// failure. If the rule ever fires without its flag, this package goes red.
func TestNotReportedByDefault(t *testing.T) {
	qt.Assert(t, 1, qt.Equals, 1)
	qt.Check(t, 2, qt.Equals, 2)
}

func assertOne(t *testing.T, got int) {
	qt.Assert(t, got, qt.Equals, 1)
}

func TestSubtestNotReportedByDefault(t *testing.T) {
	c := qt.New(t)
	c.Assert(0, qt.Equals, 0)

	t.Run("sub", func(t *testing.T) {
		qt.Assert(t, 1, qt.Equals, 1)
	})
}

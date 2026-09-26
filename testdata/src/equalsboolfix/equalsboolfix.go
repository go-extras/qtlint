package equalsboolfix

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

type flag bool

func TestFix(t *testing.T) {
	c := qt.New(t)
	b := true
	m := map[string]any{"enabled": true}
	f := flag(true)

	c.Assert(b, qt.Equals, true)                         // want `qt\.IsTrue instead of qt\.Equals, true`
	qt.Check(t, b, qt.Equals, false)                     // want `qt\.IsFalse instead of qt\.Equals, false`
	c.Assert(true, qt.Equals, b)                         // want `x, qt\.IsTrue instead of true, qt\.Equals, x`
	qt.Assert(t, false, qt.Equals, b)                    // want `x, qt\.IsFalse instead of false, qt\.Equals, x`
	c.Assert(m["enabled"], qt.Equals, true)              // want `qt\.IsTrue instead of qt\.Equals, true`
	c.Assert(f, qt.Equals, true)                         // want `flag is not bool`
	c.Assert(b, qt.Equals, true, qt.Commentf("set"))     // want `qt\.IsTrue instead of qt\.Equals, true`
	c.Assert(true, qt.Equals, b, qt.Commentf("set"))     // want `x, qt\.IsTrue instead of true, qt\.Equals, x`
	c.Check(len(m) > 0, qt.Equals, true)                 // want `qt\.IsTrue instead of qt\.Equals, true`
	c.Check(true, qt.Equals, m["enabled"] == any(false)) // want `x, qt\.IsTrue instead of true, qt\.Equals, x`
}

// A call spread over several lines keeps its layout.
func TestMultiline(t *testing.T) {
	c := qt.New(t)
	b := true

	c.Assert(
		b,
		qt.Equals, // want `qt\.IsTrue instead of qt\.Equals, true`
		true,
	)
	c.Assert(
		false, // want `x, qt\.IsFalse instead of false, qt\.Equals, x`
		qt.Equals,
		b,
	)
}

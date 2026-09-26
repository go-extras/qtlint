package equalsbool

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

type flag bool

type boolean = bool

// Test case: the literal as want, in every assertion form.
func TestLiteralWant(t *testing.T) {
	c := qt.New(t)
	b := true

	c.Assert(b, qt.Equals, true)                                      // want `qtlint: use qt\.IsTrue instead of qt\.Equals, true$`
	c.Check(b, qt.Equals, false)                                      // want `qtlint: use qt\.IsFalse instead of qt\.Equals, false$`
	qt.Assert(t, b, qt.Equals, true)                                  // want `qtlint: use qt\.IsTrue instead of qt\.Equals, true$`
	qt.Check(t, b, qt.Equals, false)                                  // want `qtlint: use qt\.IsFalse instead of qt\.Equals, false$`
	c.Assert(len("x") > 0, qt.Equals, true, qt.Commentf("non-empty")) // want `qtlint: use qt\.IsTrue instead of qt\.Equals, true$`
}

// Test case: the literal as got; the value under test is want.
func TestLiteralGot(t *testing.T) {
	c := qt.New(t)
	b := true

	c.Assert(true, qt.Equals, b)     // want `qtlint: use x, qt\.IsTrue instead of true, qt\.Equals, x$`
	c.Check(false, qt.Equals, b)     // want `qtlint: use x, qt\.IsFalse instead of false, qt\.Equals, x$`
	qt.Assert(t, true, qt.Equals, b) // want `qtlint: use x, qt\.IsTrue instead of true, qt\.Equals, x$`
	qt.Check(t, false, qt.Equals, b) // want `qtlint: use x, qt\.IsFalse instead of false, qt\.Equals, x$`
}

// Test case: an interface or a type parameter may hold a bool, and an alias of
// bool is bool.
func TestOperandTypes(t *testing.T) {
	c := qt.New(t)
	m := map[string]any{"enabled": true}
	var v boolean = true

	c.Assert(m["enabled"], qt.Equals, true) // want `qtlint: use qt\.IsTrue instead of qt\.Equals, true$`
	c.Assert(v, qt.Equals, true)            // want `qtlint: use qt\.IsTrue instead of qt\.Equals, true$`
}

func assertSet[T ~bool](c *qt.C, v T) {
	c.Assert(v, qt.Equals, true) // want `qtlint: use qt\.IsTrue instead of qt\.Equals, true$`
}

// Test case: a named bool type never equals the literal, which is a bool.
func TestNamedBool(t *testing.T) {
	c := qt.New(t)
	f := flag(true)

	c.Assert(f, qt.Equals, true)  // want `qtlint: use qt\.IsTrue instead of qt\.Equals, true: flag is not bool, so qt\.Equals always fails`
	c.Assert(false, qt.Equals, f) // want `qtlint: use x, qt\.IsFalse instead of false, qt\.Equals, x: flag is not bool, so qt\.Equals always fails`
}

// Negative test cases: patterns that should NOT trigger the rule.

// A local named true is not the predeclared one.
func TestShadowed(t *testing.T) {
	c := qt.New(t)
	b := false
	true := false

	c.Assert(b, qt.Equals, true)
}

// With a literal on both sides there is no value under test.
func TestBothLiterals(t *testing.T) {
	c := qt.New(t)

	c.Assert(true, qt.Equals, true)
}

// A value that is not of bool kind fails either way; IsTrue would only make it
// a bad check.
func TestNotBool(t *testing.T) {
	c := qt.New(t)

	c.Assert(1, qt.Equals, true)
	c.Assert("true", qt.Equals, true)
}

// A spread argument is the slice of wants, not a want.
func TestSpread(t *testing.T) {
	c := qt.New(t)
	wants := []any{true}

	c.Assert(true, qt.Equals, wants...)
}

// Only the predeclared literals count, not a variable holding one.
func TestVariable(t *testing.T) {
	c := qt.New(t)
	b, yes := true, true

	c.Assert(b, qt.Equals, yes)
}

// Other checkers are not this rule's business.
func TestOtherCheckers(t *testing.T) {
	c := qt.New(t)
	b := true

	c.Assert(b, qt.IsTrue)
	c.Assert(b, qt.DeepEquals, true)
}

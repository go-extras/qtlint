package a

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

// Test case: qt.Not(qt.IsNil) should be replaced with qt.IsNotNil
func TestNotIsNil(t *testing.T) {
	c := qt.New(t)
	var x *int
	qt.Assert(t, x, qt.Not(qt.IsNil)) // want "qtlint: use qt.IsNotNil instead of qt.Not\\(qt.IsNil\\)"
	c.Assert(x, qt.Not(qt.IsNil))     // want "qtlint: use qt.IsNotNil instead of qt.Not\\(qt.IsNil\\)"
}

// Test case: qt.Not(qt.IsTrue) should be replaced with qt.IsFalse
func TestNotIsTrue(t *testing.T) {
	c := qt.New(t)
	value := false
	qt.Assert(t, value, qt.Not(qt.IsTrue)) // want "qtlint: use qt.IsFalse instead of qt.Not\\(qt.IsTrue\\)"
	c.Assert(value, qt.Not(qt.IsTrue))     // want "qtlint: use qt.IsFalse instead of qt.Not\\(qt.IsTrue\\)"
}

// Test case: qt.Not(qt.IsFalse) should be replaced with qt.IsTrue
func TestNotIsFalse(t *testing.T) {
	c := qt.New(t)
	value := true
	qt.Assert(t, value, qt.Not(qt.IsFalse)) // want "qtlint: use qt.IsTrue instead of qt.Not\\(qt.IsFalse\\)"
	c.Assert(value, qt.Not(qt.IsFalse))     // want "qtlint: use qt.IsTrue instead of qt.Not\\(qt.IsFalse\\)"
}

// Test case: qt.Check should also be checked
func TestCheckNotIsNil(t *testing.T) {
	c := qt.New(t)
	var x *int
	qt.Check(t, x, qt.Not(qt.IsNil)) // want "qtlint: use qt.IsNotNil instead of qt.Not\\(qt.IsNil\\)"
	c.Check(x, qt.Not(qt.IsNil))     // want "qtlint: use qt.IsNotNil instead of qt.Not\\(qt.IsNil\\)"
}


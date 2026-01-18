package fix

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestFix(t *testing.T) {
	c := qt.New(t)
	var x *int
	
	qt.Assert(t, x, qt.Not(qt.IsNil)) // want "qtlint: use qt.IsNotNil instead of qt.Not\\(qt.IsNil\\)"
	c.Assert(x, qt.Not(qt.IsNil))     // want "qtlint: use qt.IsNotNil instead of qt.Not\\(qt.IsNil\\)"
	
	value := false
	qt.Assert(t, value, qt.Not(qt.IsTrue)) // want "qtlint: use qt.IsFalse instead of qt.Not\\(qt.IsTrue\\)"
	c.Assert(value, qt.Not(qt.IsTrue))     // want "qtlint: use qt.IsFalse instead of qt.Not\\(qt.IsTrue\\)"
	
	value2 := true
	qt.Assert(t, value2, qt.Not(qt.IsFalse)) // want "qtlint: use qt.IsTrue instead of qt.Not\\(qt.IsFalse\\)"
	c.Assert(value2, qt.Not(qt.IsFalse))     // want "qtlint: use qt.IsTrue instead of qt.Not\\(qt.IsFalse\\)"
}


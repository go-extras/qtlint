// Package quicktest is a stub for the -tags end-to-end fixture. It is not the
// real quicktest package and declares only what the fixture files below it
// need to type-check.
package quicktest

import "testing"

// C is a quicktest checker.
type C struct {
	TB testing.TB
}

// New returns a new checker instance.
func New(t testing.TB) *C {
	return &C{TB: t}
}

// Assert runs the given check and stops execution in case of failure.
func (c *C) Assert(got any, checker Checker, args ...any) bool {
	return true
}

// Checker is the interface implemented by quicktest checkers.
type Checker interface {
	Check(got any, args []any) error
}

type checkerFunc struct{}

func (checkerFunc) Check(got any, args []any) error { return nil }

// IsNil checks that a value is nil.
var IsNil Checker = checkerFunc{}

// IsNotNil checks that a value is not nil.
var IsNotNil Checker = checkerFunc{}

// Not negates a checker.
func Not(c Checker) Checker { return c }

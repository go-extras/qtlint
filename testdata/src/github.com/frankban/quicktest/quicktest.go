// Package quicktest is a stub for testing purposes.
// This is not the real quicktest package.
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
func (c *C) Assert(got interface{}, checker Checker, args ...interface{}) bool {
	return true
}

// Check runs the given check and continues execution in case of failure.
func (c *C) Check(got interface{}, checker Checker, args ...interface{}) bool {
	return true
}

// Run runs f as a subtest.
func (c *C) Run(name string, f func(c *C)) bool {
	return true
}

// Assert runs the given check using the provided t and stops execution in case of failure.
func Assert(t testing.TB, got interface{}, checker Checker, args ...interface{}) bool {
	return true
}

// Check runs the given check using the provided t and continues execution in case of failure.
func Check(t testing.TB, got interface{}, checker Checker, args ...interface{}) bool {
	return true
}

// Checker is implemented by types used as part of Check/Assert invocations.
type Checker interface{}

// Checkers
var (
	IsNil     Checker
	IsNotNil  Checker
	IsTrue    Checker
	IsFalse   Checker
	Equals    Checker
	DeepEquals Checker
)

// Not returns a Checker negating the given Checker.
func Not(checker Checker) Checker {
	return nil
}


// Package quicktest is a stub for testing purposes.
// This is not the real quicktest package.
//
// The rules that reason about *qt.C methods decide what to do from the method
// name, so a name this stub gets wrong or leaves out is a case the fixtures
// cannot reach. Signatures are copied from quicktest v1.14.6; the real C
// embeds testing.TB and inherits Cleanup, TempDir and the rest from it, which
// this stub spells out instead.
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

// Cleanup registers f to be called when the test is done.
func (c *C) Cleanup(f func()) {}

// Defer registers f to be called when the test's Done method is called.
func (c *C) Defer(f func()) {}

// Done calls the functions registered by Defer, in reverse order.
func (c *C) Done() {}

// Mkdir makes a temporary directory and returns its name.
func (c *C) Mkdir() string {
	return ""
}

// Parallel signals that the test is to be run in parallel.
func (c *C) Parallel() {}

// Patch sets a variable to a temporary value for the duration of the test.
func (c *C) Patch(dest, value any) {}

// Setenv sets an environment variable for the duration of the test.
func (c *C) Setenv(key, value string) {}

// Unsetenv unsets an environment variable for the duration of the test.
func (c *C) Unsetenv(name string) {}

// TempDir returns a temporary directory removed when the test is done.
func (c *C) TempDir() string {
	return ""
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
	IsNil      Checker
	IsNotNil   Checker
	IsTrue     Checker
	IsFalse    Checker
	Equals     Checker
	DeepEquals Checker
	HasLen     Checker
	Contains   Checker
	ErrorIs    Checker
	ErrorAs    Checker
)

// Not returns a Checker negating the given Checker.
func Not(checker Checker) Checker {
	return nil
}

// Comment is a comment associated with an assertion.
type Comment struct{}

// Commentf returns a Comment with the given formatted message.
func Commentf(format string, args ...interface{}) Comment {
	return Comment{}
}

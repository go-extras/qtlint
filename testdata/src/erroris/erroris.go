package erroris

import (
	"errors"
	"io/fs"
	"testing"

	qt "github.com/frankban/quicktest"
)

var ErrSentinel = errors.New("sentinel")

type customErr struct{ msg string }

func (e *customErr) Error() string { return e.msg }

func makeErr() error {
	return ErrSentinel
}

// Test case: errors.Is(err, target), qt.IsTrue should be replaced with err, qt.ErrorIs, target.
func TestErrorsIsIsTrue(t *testing.T) {
	c := qt.New(t)
	err := makeErr()

	qt.Assert(t, errors.Is(err, ErrSentinel), qt.IsTrue) // want "qtlint: use qt.ErrorIs instead of errors.Is\\(err, target\\), qt.IsTrue"
	c.Assert(errors.Is(err, ErrSentinel), qt.IsTrue)     // want "qtlint: use qt.ErrorIs instead of errors.Is\\(err, target\\), qt.IsTrue"
}

// Test case: errors.Is(err, target), qt.IsFalse should be replaced with err, qt.Not(qt.ErrorIs), target.
func TestErrorsIsIsFalse(t *testing.T) {
	c := qt.New(t)
	err := makeErr()

	qt.Assert(t, errors.Is(err, fs.ErrNotExist), qt.IsFalse) // want "qtlint: use qt.Not\\(qt.ErrorIs\\) instead of errors.Is\\(err, target\\), qt.IsFalse"
	c.Assert(errors.Is(err, fs.ErrNotExist), qt.IsFalse)     // want "qtlint: use qt.Not\\(qt.ErrorIs\\) instead of errors.Is\\(err, target\\), qt.IsFalse"
}

// Test case: qt.Check should also be checked.
func TestCheckErrorsIs(t *testing.T) {
	c := qt.New(t)
	err := makeErr()

	qt.Check(t, errors.Is(err, ErrSentinel), qt.IsTrue)     // want "qtlint: use qt.ErrorIs instead of errors.Is\\(err, target\\), qt.IsTrue"
	c.Check(errors.Is(err, ErrSentinel), qt.IsTrue)         // want "qtlint: use qt.ErrorIs instead of errors.Is\\(err, target\\), qt.IsTrue"
	qt.Check(t, errors.Is(err, fs.ErrNotExist), qt.IsFalse) // want "qtlint: use qt.Not\\(qt.ErrorIs\\) instead of errors.Is\\(err, target\\), qt.IsFalse"
	c.Check(errors.Is(err, fs.ErrNotExist), qt.IsFalse)     // want "qtlint: use qt.Not\\(qt.ErrorIs\\) instead of errors.Is\\(err, target\\), qt.IsFalse"
}

// Test case: errors.As(err, &target), qt.IsTrue should be replaced with err, qt.ErrorAs, &target.
func TestErrorsAsIsTrue(t *testing.T) {
	c := qt.New(t)
	err := makeErr()
	var ce *customErr

	qt.Assert(t, errors.As(err, &ce), qt.IsTrue) // want "qtlint: use qt.ErrorAs instead of errors.As\\(err, target\\), qt.IsTrue"
	c.Assert(errors.As(err, &ce), qt.IsTrue)     // want "qtlint: use qt.ErrorAs instead of errors.As\\(err, target\\), qt.IsTrue"
}

// Test case: errors.As(err, &target), qt.IsFalse should be replaced with err, qt.Not(qt.ErrorAs), &target.
func TestErrorsAsIsFalse(t *testing.T) {
	c := qt.New(t)
	err := makeErr()
	var ce *customErr

	qt.Assert(t, errors.As(err, &ce), qt.IsFalse) // want "qtlint: use qt.Not\\(qt.ErrorAs\\) instead of errors.As\\(err, target\\), qt.IsFalse"
	c.Assert(errors.As(err, &ce), qt.IsFalse)     // want "qtlint: use qt.Not\\(qt.ErrorAs\\) instead of errors.As\\(err, target\\), qt.IsFalse"
}

// Test case: complex target expressions are preserved verbatim.
func TestErrorsIsComplexTarget(t *testing.T) {
	c := qt.New(t)
	err := makeErr()
	targets := []error{ErrSentinel, fs.ErrNotExist}

	qt.Assert(t, errors.Is(err, targets[0]), qt.IsTrue) // want "qtlint: use qt.ErrorIs instead of errors.Is\\(err, target\\), qt.IsTrue"
	c.Assert(errors.Is(err, targets[1]), qt.IsFalse)    // want "qtlint: use qt.Not\\(qt.ErrorIs\\) instead of errors.Is\\(err, target\\), qt.IsFalse"
}

// Negative test cases: patterns that should NOT trigger the rule.

// Using qt.ErrorIs / qt.ErrorAs directly is correct.
func TestErrorIsDirect(t *testing.T) {
	c := qt.New(t)
	err := makeErr()
	var ce *customErr

	c.Assert(err, qt.ErrorIs, ErrSentinel)
	qt.Assert(t, err, qt.ErrorIs, ErrSentinel)
	c.Assert(err, qt.ErrorAs, &ce)
	qt.Assert(t, err, qt.ErrorAs, &ce)
}

// Using errors.Is/errors.As with checkers other than qt.IsTrue/qt.IsFalse is not flagged.
func TestErrorsIsWithOtherCheckers(t *testing.T) {
	c := qt.New(t)
	err := makeErr()

	qt.Assert(t, errors.Is(err, ErrSentinel), qt.Equals, true)
	c.Assert(errors.Is(err, ErrSentinel), qt.DeepEquals, true)
}

// User-defined functions named Is/As must not be flagged.
func TestUserDefinedIsAs(t *testing.T) {
	c := qt.New(t)

	Is := func(err, target error) bool { return false }
	As := func(err error, target any) bool { return false }

	err := makeErr()
	var ce *customErr
	qt.Assert(t, Is(err, ErrSentinel), qt.IsTrue)
	c.Assert(As(err, &ce), qt.IsTrue)
}

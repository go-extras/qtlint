package errorisfix

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

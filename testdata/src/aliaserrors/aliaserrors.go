package aliaserrors

import (
	"testing"

	myerrors "errors"

	qt "github.com/frankban/quicktest"
)

var ErrSentinel = myerrors.New("sentinel")

type customErr struct{ msg string }

func (e *customErr) Error() string { return e.msg }

// Test case: errors.Is with alias.
func TestErrorsIsWithAlias(t *testing.T) {
	c := qt.New(t)
	err := ErrSentinel

	qt.Assert(t, myerrors.Is(err, ErrSentinel), qt.IsTrue) // want "qtlint: use qt.ErrorIs instead of myerrors.Is\\(err, target\\), qt.IsTrue"
	c.Assert(myerrors.Is(err, ErrSentinel), qt.IsFalse)    // want "qtlint: use qt.Not\\(qt.ErrorIs\\) instead of myerrors.Is\\(err, target\\), qt.IsFalse"
}

// Test case: errors.As with alias.
func TestErrorsAsWithAlias(t *testing.T) {
	c := qt.New(t)
	err := ErrSentinel
	var ce *customErr

	qt.Assert(t, myerrors.As(err, &ce), qt.IsTrue) // want "qtlint: use qt.ErrorAs instead of myerrors.As\\(err, target\\), qt.IsTrue"
	c.Assert(myerrors.As(err, &ce), qt.IsFalse)    // want "qtlint: use qt.Not\\(qt.ErrorAs\\) instead of myerrors.As\\(err, target\\), qt.IsFalse"
}

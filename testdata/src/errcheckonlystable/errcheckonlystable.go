package errcheckonlystable

import (
	"errors"
	"testing"

	qt "github.com/frankban/quicktest"
)

func returnsErr() error {
	return errors.New("boom")
}

// Stable: single-err Fatal collapses to a bare c.Assert.
func TestStableFatal(t *testing.T) {
	c := qt.New(t)

	err := returnsErr()
	if err != nil { // want "qtlint: use c.Assert.*instead of t.Fatal"
		t.Fatal(err)
	}

	c.Assert(1, qt.Equals, 1)
}

// Stable: Fatalf with a literal format string maps directly to qt.Commentf.
func TestStableFatalfLiteral(t *testing.T) {
	c := qt.New(t)

	err := returnsErr()
	if err != nil { // want "qtlint: use c.Assert.*instead of t.Fatalf"
		t.Fatalf("unexpected: %v", err)
	}

	c.Assert(1, qt.Equals, 1)
}

// Unstable (suppressed under --only-stable-fixes): multi-arg non-formatted.
func TestUnstableMultiArgFatal(t *testing.T) {
	c := qt.New(t)

	err := returnsErr()
	if err != nil { // want "qtlint: use c.Assert.*instead of t.Fatal"
		t.Fatal("unexpected:", err, 123)
	}

	c.Assert(1, qt.Equals, 1)
}

// Unstable (suppressed under --only-stable-fixes): non-literal format.
func TestUnstableNonLiteralFormat(t *testing.T) {
	c := qt.New(t)

	err := returnsErr()
	fmtStr := "got: %v"
	if err != nil { // want "qtlint: use c.Assert.*instead of t.Fatalf"
		t.Fatalf(fmtStr, err)
	}

	c.Assert(1, qt.Equals, 1)
}

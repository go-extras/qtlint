package errcheckfmtalias

import (
	f "fmt"
	"testing"

	qt "github.com/frankban/quicktest"
)

func returnsErr() error {
	return f.Errorf("boom")
}

func TestFatalSpreadWithFmtAlias(t *testing.T) {
	c := qt.New(t)

	err := returnsErr()
	args := []any{"unexpected:", err}
	if err != nil { // want "qtlint: use c.Assert.*instead of t.Fatal"
		t.Fatal(args...)
	}

	c.Assert(1, qt.Equals, 1)
}

func TestFatalfSpreadWithFmtAlias(t *testing.T) {
	c := qt.New(t)

	err := returnsErr()
	args := []any{err, 123}
	if err != nil { // want "qtlint: use c.Assert.*instead of t.Fatalf"
		t.Fatalf("unexpected: %v %v", args...)
	}

	c.Assert(1, qt.Equals, 1)
}

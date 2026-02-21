package errcheckfix

import (
	"errors"
	"testing"

	qt "github.com/frankban/quicktest"
)

func returnsErr() error {
	return errors.New("boom")
}

func returnsTwo() (int, error) {
	return 42, errors.New("boom")
}

func TestFatalAfterAssign(t *testing.T) {
	c := qt.New(t)

	err := returnsErr()
	if err != nil { // want "qtlint: use c.Assert.*instead of t.Fatal"
		t.Fatal(err)
	}

	c.Assert(1, qt.Equals, 1)
}

func TestErrorAfterAssign(t *testing.T) {
	c := qt.New(t)

	err := returnsErr()
	if err != nil { // want "qtlint: use c.Check.*instead of t.Errorf"
		t.Errorf("unexpected: %v", err)
	}

	c.Assert(1, qt.Equals, 1)
}

func TestMultiValueAssign(t *testing.T) {
	c := qt.New(t)

	v, err := returnsTwo()
	if err != nil { // want "qtlint: use c.Assert.*instead of t.Fatalf"
		t.Fatalf("unexpected: %v", err)
	}
	_ = v

	c.Assert(1, qt.Equals, 1)
}

func TestIfInit(t *testing.T) {
	c := qt.New(t)

	if err := returnsErr(); err != nil { // want "qtlint: use c.Assert.*instead of t.Fatal"
		t.Fatal(err)
	}

	c.Assert(1, qt.Equals, 1)
}

func TestFatalMultipleArgs(t *testing.T) {
	c := qt.New(t)

	err := returnsErr()
	if err != nil { // want "qtlint: use c.Assert.*instead of t.Fatal"
		t.Fatal("unexpected:", err, 123)
	}

	c.Assert(1, qt.Equals, 1)
}

func TestErrorMultipleArgs(t *testing.T) {
	c := qt.New(t)

	err := returnsErr()
	if err != nil { // want "qtlint: use c.Check.*instead of t.Error"
		t.Error("unexpected:", err, 123)
	}

	c.Assert(1, qt.Equals, 1)
}

func TestErrorfMultipleArgs(t *testing.T) {
	c := qt.New(t)

	err := returnsErr()
	if err != nil { // want "qtlint: use c.Check.*instead of t.Errorf"
		t.Errorf("unexpected: %v %v", err, 123)
	}

	c.Assert(1, qt.Equals, 1)
}

func TestFatalfSpread(t *testing.T) {
	c := qt.New(t)

	err := returnsErr()
	args := []any{err, 123}
	if err != nil { // want "qtlint: use c.Assert.*instead of t.Fatalf"
		t.Fatalf("unexpected: %v %v", args...)
	}

	c.Assert(1, qt.Equals, 1)
}

func TestFatalSpread(t *testing.T) {
	c := qt.New(t)

	err := returnsErr()
	args := []any{"unexpected:", err, 123}
	if err != nil { // want "qtlint: use c.Assert.*instead of t.Fatal"
		t.Fatal(args...)
	}

	c.Assert(1, qt.Equals, 1)
}

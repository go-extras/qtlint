package datarows

import (
	"errors"
	"testing"

	qt "github.com/frankban/quicktest"
)

var errBoom = errors.New("boom")

// A table row carries data. A checker in a row is the branch the table was
// meant to remove, written out one row at a time.
func TestRowCarriesAChecker(t *testing.T) {
	tests := []struct {
		name    string
		wantErr func(c *qt.C, err error) // want "qtlint: a table row carries data, not a checker; give the row the value that varies, or split the table into the tests its rows are asserting differently"
	}{
		{
			name:    "no error",
			wantErr: func(c *qt.C, err error) { c.Assert(err, qt.IsNil) },
		},
		{
			name:    "an error",
			wantErr: func(c *qt.C, err error) { c.Assert(err, qt.Equals, errBoom) },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := qt.New(t)
			test.wantErr(c, nil)
		})
	}
}

// A row doing more of the work is not an exemption. Rows whose assertions
// differ in shape are two tests sharing one table, and the style this enforces
// asks for them to be separate.
func TestRowDoingSeveralThings(t *testing.T) {
	tests := []struct {
		name   string
		assert func(c *qt.C, err error, out string) // want "qtlint: a table row carries data, not a checker; give the row the value that varies, or split the table into the tests its rows are asserting differently"
	}{
		{
			name: "one assertion",
			assert: func(c *qt.C, err error, _ string) {
				c.Assert(err, qt.IsNil)
			},
		},
		{
			name: "two assertions",
			assert: func(c *qt.C, err error, out string) {
				c.Assert(err, qt.IsNil)
				c.Assert(out, qt.Equals, "x")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := qt.New(t)
			test.assert(c, nil, "x")
		})
	}
}

// The handle rather than the checker is the same defect: what reaches the row
// is still the means to assert.
func TestRowCarriesAHandle(t *testing.T) {
	tests := []struct {
		name   string
		assert func(tb testing.TB, err error) // want "qtlint: a table row carries data, not a checker; give the row the value that varies, or split the table into the tests its rows are asserting differently"
	}{
		{
			name:   "row",
			assert: func(tb testing.TB, err error) { qt.New(tb).Assert(err, qt.IsNil) },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assert(t, nil)
		})
	}
}

// A row field holding a function that asserts nothing is data the test builds
// with, and is left alone. This is the control that keeps the rule from
// reading "no function fields".
//
// It takes a parameter deliberately. A zero-parameter function never reaches
// the type check at all, so a control written that way would pass against a
// rule that had stopped looking at types — which is what it exists to catch.
func TestRowCarriesAFunctionThatIsData(t *testing.T) {
	tests := []struct {
		name string
		args func(dir string) []string
		want int
	}{
		{name: "none", args: func(dir string) []string { return nil }, want: 0},
		{name: "two", args: func(dir string) []string { return []string{dir, "b"} }, want: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := qt.New(t)
			c.Assert(test.args("a"), qt.HasLen, test.want)
		})
	}
}

// An unnamed parameter, and a row spelling the handle differently from the
// declaration. Neither is an exemption: the rule matches the parameter TYPE,
// so no name has to agree with any other name for it to fire.
func TestUnnamedAndRenamedHandles(t *testing.T) {
	tests := []struct {
		name    string
		wantErr func(*qt.C, error) // want "qtlint: a table row carries data, not a checker; give the row the value that varies, or split the table into the tests its rows are asserting differently"
	}{
		{
			name:    "the row names it something else",
			wantErr: func(check *qt.C, err error) { check.Assert(err, qt.IsNil) },
		},
		{
			name:    "and again",
			wantErr: func(other *qt.C, err error) { other.Assert(err, qt.Equals, errBoom) },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := qt.New(t)
			test.wantErr(c, nil)
		})
	}
}

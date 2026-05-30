package equalsnilfix

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

type sometype struct{}

func TestEqualsNilFix(t *testing.T) {
	c := qt.New(t)
	var x *int

	qt.Assert(t, x, qt.Equals, nil) // want "qtlint: use qt.IsNil instead of qt.Equals, nil"
	c.Assert(x, qt.Equals, nil)     // want "qtlint: use qt.IsNil instead of qt.Equals, nil"

	qt.Check(t, x, qt.Equals, nil) // want "qtlint: use qt.IsNil instead of qt.Equals, nil"
	c.Check(x, qt.Equals, nil)     // want "qtlint: use qt.IsNil instead of qt.Equals, nil"

	qt.Assert(t, (*sometype)(nil), qt.Equals, nil) // want "qtlint: use qt.IsNil instead of qt.Equals, nil"
	c.Assert((*sometype)(nil), qt.Equals, nil)     // want "qtlint: use qt.IsNil instead of qt.Equals, nil"

	c.Assert(x, qt.Equals, nil, qt.Commentf("should be nil")) // want "qtlint: use qt.IsNil instead of qt.Equals, nil"
}

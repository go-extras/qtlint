package equalsuncomparable

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

type config struct {
	Names []string
}

type ids []int

type handler func()

type point struct {
	X, Y int
}

type pair[T any] struct {
	A, B T
}

// Test case: a slice, a map, and every assertion form.
func TestSliceAndMap(t *testing.T) {
	c := qt.New(t)
	got, want := []int{1}, []int{1}

	c.Assert(got, qt.Equals, want)     // want `qtlint: use qt\.DeepEquals instead of qt\.Equals: \[\]int is not comparable, so qt\.Equals always fails`
	c.Check(got, qt.Equals, want)      // want `qtlint: use qt\.DeepEquals instead of qt\.Equals: \[\]int is not comparable`
	qt.Assert(t, got, qt.Equals, want) // want `qtlint: use qt\.DeepEquals instead of qt\.Equals: \[\]int is not comparable`
	qt.Check(t, got, qt.Equals, want)  // want `qtlint: use qt\.DeepEquals instead of qt\.Equals: \[\]int is not comparable`

	c.Assert(map[string]int{"a": 1}, qt.Equals, map[string]int{"a": 1}) // want `map\[string\]int is not comparable`
}

// Test case: an array or struct holding a slice is just as uncomparable, and
// a named type is reported by its name.
func TestComposite(t *testing.T) {
	c := qt.New(t)

	c.Assert(config{}, qt.Equals, config{})               // want `config is not comparable`
	c.Assert([2][]int{}, qt.Equals, [2][]int{})           // want `\[2\]\[\]int is not comparable`
	c.Assert(ids{1}, qt.Equals, ids{1})                   // want `ids is not comparable`
	c.Assert(ids{1}, qt.Equals, ids{1}, qt.Commentf("x")) // want `ids is not comparable`
}

// Test case: only one side needs a known uncomparable type.
func TestOneSide(t *testing.T) {
	c := qt.New(t)
	var got any = []int{1}

	c.Assert(got, qt.Equals, []int{1}) // want `\[\]int is not comparable`
}

// Test case: a func is reported without suggesting qt.DeepEquals.
func TestFunc(t *testing.T) {
	c := qt.New(t)
	f := func() {}
	var h handler

	c.Assert(f, qt.Equals, f)            // want `qtlint: qt\.Equals always fails on func type func\(\); a func can only be checked with qt\.IsNil or qt\.IsNotNil`
	c.Assert(h, qt.Equals, handler(nil)) // want `qt\.Equals always fails on func type handler`
}

// Test case: a slice of a type parameter is a slice whatever T is.
func assertSameSlice[T any](c *qt.C, got, want []T) {
	c.Assert(got, qt.Equals, want) // want `\[\]T is not comparable`
}

// Negative test cases: patterns that should NOT trigger the rule.

// Comparable types are what qt.Equals is for.
func TestComparable(t *testing.T) {
	c := qt.New(t)
	s := []int{1}

	c.Assert(point{1, 2}, qt.Equals, point{1, 2})
	c.Assert(&s, qt.Equals, &s)
	c.Assert(make(chan int), qt.Equals, make(chan int))
	c.Assert(len(s), qt.Equals, 1) // want `qtlint: use qt\.HasLen instead of len\(x\), qt\.Equals`
}

// Interface operands hold whatever they hold at run time.
func TestInterfaces(t *testing.T) {
	c := qt.New(t)
	var got, want any = []int{1}, []int{1}

	c.Assert(got, qt.Equals, want)
}

// A type parameter may be instantiated with a comparable type, and so may a
// struct built from one.
func assertSame[T any](c *qt.C, got, want T) {
	c.Assert(got, qt.Equals, want)
	c.Assert(pair[T]{got, want}, qt.Equals, pair[T]{got, want})
}

// A nil on either side belongs to the qt.IsNil rule, or to no rule.
func TestNil(t *testing.T) {
	c := qt.New(t)
	var s []int

	c.Assert(s, qt.Equals, nil) // want `qtlint: use qt\.IsNil instead of qt\.Equals, nil`
	c.Assert(nil, qt.Equals, s)
}

// A spread argument is the slice of wants, not a want.
func TestSpread(t *testing.T) {
	c := qt.New(t)
	wants := []any{1}

	c.Assert(1, qt.Equals, wants...)
}

// Other checkers are not this rule's business.
func TestOtherCheckers(t *testing.T) {
	c := qt.New(t)
	s := []int{1}

	c.Assert(s, qt.DeepEquals, []int{1})
	c.Assert(s, qt.Not(qt.IsNil)) // want `qtlint: use qt\.IsNotNil instead of qt\.Not\(qt\.IsNil\)`
	c.Assert(s, qt.HasLen, 1)
}

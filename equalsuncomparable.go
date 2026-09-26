package qtlint

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"
)

// checkEqualsUncomparablePattern reports got, qt.Equals, want where got or want
// has a static type that == cannot compare: a slice, a map, a func, or an array
// or struct holding one. It suggests qt.DeepEquals.
//
// The Equals checker compares the two values boxed as interface{}. Two boxed
// values of one uncomparable type make == panic, which Equals recovers into a
// failure, and values of two different types are unequal. So the assertion
// cannot pass, whatever the values are.
//
// An operand whose static type is an interface or a type parameter is not a
// reason to report: what it holds is decided at run time. Neither is an
// untyped nil on either side: x, qt.Equals, nil is checkEqualsNilPattern's,
// which suggests qt.IsNil.
//
// A func is reported without a fix: go-cmp, which is what qt.DeepEquals runs,
// reports two funcs equal only when both are nil, so DeepEquals would not make
// the assertion work either.
//
// Every other fix is best-effort unless go-cmp can compare every operand of
// known type down to its leaves; see cmpComparesFully. Otherwise DeepEquals may
// still fail on equal values, and -only-stable-fixes withholds the fix.
func (a *analyzer) checkEqualsUncomparablePattern(pass *analysis.Pass, call *ast.CallExpr) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}

	gotArgIndex := 0
	if isPackageQualified(pass, sel) {
		gotArgIndex = 1
	}
	checkerIndex := gotArgIndex + 1
	wantIndex := gotArgIndex + 2

	// With want... the last argument is the slice of wants, and its type says
	// nothing about the values compared.
	if len(call.Args) < wantIndex+1 || call.Ellipsis.IsValid() {
		return
	}

	checkerSel, ok := call.Args[checkerIndex].(*ast.SelectorExpr)
	if !ok || checkerSel.Sel.Name != "Equals" || !isPackageQualified(pass, checkerSel) {
		return
	}

	gotArg, wantArg := call.Args[gotArgIndex], call.Args[wantIndex]
	if isNilIdent(gotArg) || isNilIdent(wantArg) {
		return
	}

	var known []types.Type
	var culprit types.Type
	for _, arg := range []ast.Expr{gotArg, wantArg} {
		t := pass.TypesInfo.TypeOf(arg)
		if t == nil {
			return
		}
		// A type parameter's underlying type is its constraint, so this skips
		// type parameters too.
		if types.IsInterface(t) {
			continue
		}
		known = append(known, t)
		if culprit == nil && uncomparable(t) {
			culprit = t
		}
	}
	if culprit == nil {
		return
	}

	typeText := types.TypeString(culprit, types.RelativeTo(pass.Pkg))

	if _, isFunc := culprit.Underlying().(*types.Signature); isFunc {
		pass.Report(analysis.Diagnostic{
			Pos: checkerSel.Pos(),
			End: checkerSel.End(),
			Message: fmt.Sprintf("qtlint: qt.Equals always fails on func type %s; "+
				"a func can only be checked with qt.IsNil or qt.IsNotNil", typeText),
		})
		return
	}

	diag := analysis.Diagnostic{
		Pos: checkerSel.Pos(),
		End: checkerSel.End(),
		Message: fmt.Sprintf("qtlint: use qt.DeepEquals instead of qt.Equals: "+
			"%s is not comparable, so qt.Equals always fails", typeText),
	}

	if !a.onlyStableFixes || cmpComparesAll(known) {
		diag.SuggestedFixes = []analysis.SuggestedFix{{
			Message: "Replace with qt.DeepEquals",
			TextEdits: []analysis.TextEdit{{
				Pos:     checkerSel.Sel.Pos(),
				End:     checkerSel.Sel.End(),
				NewText: []byte("DeepEquals"),
			}},
		}}
	}

	pass.Report(diag)
}

// uncomparable reports whether == panics on two values of type t, whatever
// type arguments t is instantiated with.
//
// This is not what types.Comparable answers. That calls a type parameter
// comparable only when its constraint says so, which would report a struct
// with a field of type T even though == works for every comparable T.
func uncomparable(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Slice, *types.Map, *types.Signature:
		return true
	case *types.Array:
		return uncomparable(u.Elem())
	case *types.Struct:
		for field := range u.Fields() {
			if uncomparable(field.Type()) {
				return true
			}
		}
	}
	return false
}

// cmpComparesAll reports whether cmpComparesFully holds for each of ts.
//
// Only operands of known type are passed in. An operand of interface type does
// not change the answer: go-cmp reports values of two different dynamic types
// unequal without looking inside either, so the comparison goes down to the
// leaves only when both hold the type of the known operand.
func cmpComparesAll(ts []types.Type) bool {
	var seen typeutil.Map
	for _, t := range ts {
		if !cmpComparesFully(t, &seen) {
			return false
		}
	}
	return true
}

// cmpComparesFully reports whether go-cmp with no options, which is what
// qt.DeepEquals runs, compares two values of type t without meeting
//
//   - an unexported struct field: go-cmp panics, and DeepEquals reports a bad
//     check;
//   - a func: go-cmp reports two funcs equal only when both are nil;
//   - an interface: what it holds may be either of the above.
//
// go-cmp compares a type with a suitable Equal method by calling it, and looks
// no further. It matches map keys with == rather than comparing them, so only
// map values are walked.
//
// The seen map breaks cycles through recursive types: a type already on the
// walk answers true, and the rest of the walk decides.
func cmpComparesFully(t types.Type, seen *typeutil.Map) bool {
	if seen.At(t) != nil {
		return true
	}
	seen.Set(t, true)

	// reflect lists an interface's methods without a receiver, so go-cmp never
	// finds an Equal method on an interface type.
	if types.IsInterface(t) {
		return false
	}
	if hasCmpEqualMethod(t) {
		return true
	}

	switch u := t.Underlying().(type) {
	case *types.Basic, *types.Chan:
		return true
	case *types.Pointer:
		return cmpComparesFully(u.Elem(), seen)
	case *types.Slice:
		return cmpComparesFully(u.Elem(), seen)
	case *types.Array:
		return cmpComparesFully(u.Elem(), seen)
	case *types.Map:
		return cmpComparesFully(u.Elem(), seen)
	case *types.Struct:
		for field := range u.Fields() {
			if field.Name() == "_" {
				continue
			}
			if !field.Exported() || !cmpComparesFully(field.Type(), seen) {
				return false
			}
		}
		return true
	}
	return false
}

// hasCmpEqualMethod reports whether t has an Equal method go-cmp calls instead
// of comparing the value itself: (T) Equal(T) bool, or Equal(I) bool for an I
// that T is assignable to.
func hasCmpEqualMethod(t types.Type) bool {
	sel := types.NewMethodSet(t).Lookup(nil, "Equal")
	if sel == nil {
		return false
	}
	sig, ok := sel.Type().(*types.Signature)
	if !ok || sig.Params().Len() != 1 || sig.Results().Len() != 1 {
		return false
	}
	return types.Identical(sig.Results().At(0).Type(), types.Typ[types.Bool]) &&
		types.AssignableTo(t, sig.Params().At(0).Type())
}

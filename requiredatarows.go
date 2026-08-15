package qtlint

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// assertionField describes a struct field whose type is a function taking a
// test handle, together with every closure the table's rows assign to it.
type assertionField struct {
	// name is the field's name and field its declaration.
	name  string
	field *ast.Field
	// param is the handle parameter every row's closure asserts through, and
	// index its position in the signature.
	param *ast.Field
	index int
	// bodies are the row closures, in source order.
	bodies []*ast.FuncLit
}

// checkRequireDataRows reports a table-row function field whose every row holds
// a single assertion, because the field is carrying data as code.
//
// A table-driven test that forbids conditionals in its body pushes the varying
// part into the rows. When the varying part is a value, the row carries a
// value. When a project reaches for a closure instead, the branch it could not
// write as an `if` reappears one row at a time, spelled out:
//
//	tests := []struct {
//		name   string
//		assert func(c *qt.C, err error)
//	}{
//		{name: "…", assert: func(c *qt.C, err error) {
//			c.Assert(err, qt.ErrorMatches, `table "public.users" is declared more than once;.*`)
//		}},
//		// fifty more, each three lines carrying one string
//	}
//
// As data that row is `wantErr: "…"` and the table reads as a table.
//
// The rule reports only when EVERY row for a field holds exactly one assertion
// through the handle. A field whose closures differ in shape is doing work no
// data field can, and reporting it would be telling an author to write worse
// code. That is the discriminating condition, and it is why the rule counts
// rows rather than matching a name like `assert`.
//
// No fix is suggested. Replacing the field means naming the data it stands for,
// rewriting every row, and turning the call site into an assertion — three
// decisions that belong to the author. The diagnostic exists because the shape
// grows silently: every row is locally reasonable, and only the count shows it.
func (*analyzer) checkRequireDataRows(pass *analysis.Pass) {
	for _, file := range pass.Files {
		for _, root := range outermostFuncs(file) {
			for _, spec := range structTypesIn(root) {
				for _, field := range assertionFieldsOf(pass, spec) {
					collectRowBodies(pass, root, spec, field)
					reportDataRowField(pass, field)
				}
			}
		}
	}
}

// structTypesIn returns every struct type literal under root.
func structTypesIn(root ast.Node) []*ast.StructType {
	var out []*ast.StructType
	ast.Inspect(root, func(n ast.Node) bool {
		if s, ok := n.(*ast.StructType); ok {
			out = append(out, s)
		}
		return true
	})
	return out
}

// assertionFieldsOf returns the fields of spec whose type is a function taking
// a test handle.
func assertionFieldsOf(pass *analysis.Pass, spec *ast.StructType) []*assertionField {
	if spec.Fields == nil {
		return nil
	}

	var out []*assertionField
	for _, field := range spec.Fields.List {
		sig, ok := field.Type.(*ast.FuncType)
		if !ok || len(field.Names) != 1 || sig.Params == nil {
			continue
		}
		param, index, ok := handleParam(pass, sig)
		if !ok {
			continue
		}
		out = append(out, &assertionField{
			name:  field.Names[0].Name,
			field: field,
			param: param,
			index: index,
		})
	}
	return out
}

// handleParam finds the sole *qt.C or testing.TB parameter of a signature.
func handleParam(pass *analysis.Pass, sig *ast.FuncType) (*ast.Field, int, bool) {
	index := 0
	for _, field := range sig.Params.List {
		width := len(field.Names)
		if width == 0 {
			width = 1
		}
		typ := pass.TypesInfo.TypeOf(field.Type)
		if isTestHandleType(typ) {
			if width != 1 {
				return nil, 0, false
			}
			return field, index, true
		}
		index += width
	}
	return nil, 0, false
}

// collectRowBodies gathers the closures the table's rows assign to the field.
//
// The rows are found from the declaration NODE rather than from its type. Two
// anonymous struct types of the same shape are identical to go/types, so
// matching on the type hands one table the rows of another that happens to
// declare the same fields — a one-row table then reports on a two-row
// neighbour's closures, and the same closures can be reported twice at two
// field positions. Bounding the search to one function narrows that window
// without closing it.
//
// The AST already says which literal belongs to which declaration: the rows
// are the elements of the composite literal whose type expression IS this
// struct type node, directly or as the element of a slice or array of it.
func collectRowBodies(_ *analysis.Pass, root ast.Node, spec *ast.StructType, field *assertionField) {
	ast.Inspect(root, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok || !litDeclaredBy(lit, spec) {
			return true
		}
		for _, elt := range lit.Elts {
			collectFieldBody(elt, field)
			// A slice literal's elements are the rows; a bare struct literal
			// is one row and its elements are the fields.
			if row, ok := elt.(*ast.CompositeLit); ok {
				for _, inner := range row.Elts {
					collectFieldBody(inner, field)
				}
			}
		}
		return true
	})
}

// litDeclaredBy reports whether lit's rows are values of the struct type
// written at spec, by node identity rather than by type.
//
// The table is written as []struct{…}{…}, so the rows sit in the elements of
// the outer literal and each element is itself a literal with no type of its
// own. Both shapes reach here: the outer one carries an array type whose
// element is spec, and an element carries no type expression at all and is
// matched through its parent.
func litDeclaredBy(lit *ast.CompositeLit, spec *ast.StructType) bool {
	switch typ := lit.Type.(type) {
	case *ast.StructType:
		return typ == spec
	case *ast.ArrayType:
		inner, ok := typ.Elt.(*ast.StructType)
		return ok && inner == spec
	}
	return false
}

// reportDataRowField reports the field.
//
// Every one of them: a table row carries data, and a checker in a row is a
// branch the table was supposed to remove. It does not matter how much the
// closure does. A row whose assertions differ from its neighbours in shape is
// not a row that has earned a closure — it is a second test wearing the first
// one's table, and the style guide that forbids the conditional also asks for
// the happy path and the failure path to be separate tests.
//
// So there is no threshold and no row count to reach. Carrying a checker into
// a row is the defect.
func reportDataRowField(pass *analysis.Pass, field *assertionField) {
	pass.Report(analysis.Diagnostic{
		Pos: field.field.Pos(),
		End: field.field.End(),
		Message: "qtlint: a table row carries data, not a checker" +
			"; give the row the value that varies, or split the table into the tests its rows are asserting differently",
	})
}

// isTestHandleType reports whether typ is a way to assert: a *qt.C, a
// *testing.T, or the testing.TB a checker is built from.
//
// The field's NAME is not consulted. A row carrying the means to assert is the
// defect whatever it is called, and matching a name like "assert" would find
// the tidy cases and miss the rest.
func isTestHandleType(typ types.Type) bool {
	if typ == nil {
		return false
	}
	if isQuicktestCType(typ) || isTestingTPtr(typ) {
		return true
	}
	named, ok := types.Unalias(typ).(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj != nil && obj.Pkg() != nil &&
		obj.Pkg().Path() == testingPkgPath &&
		(obj.Name() == "TB" || obj.Name() == "B" || obj.Name() == "F")
}

// collectFieldBody records elt when it assigns a closure to the field.
func collectFieldBody(elt ast.Expr, field *assertionField) {
	kv, ok := elt.(*ast.KeyValueExpr)
	if !ok {
		return
	}
	key, ok := kv.Key.(*ast.Ident)
	if !ok || key.Name != field.name {
		return
	}
	if body, ok := kv.Value.(*ast.FuncLit); ok {
		field.bodies = append(field.bodies, body)
	}
}

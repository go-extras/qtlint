package qtlint

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// calleeReach answers what a helper does with a *qt.C handed to it, so that
// handing the checker to a helper is not by itself a reason to withhold a fix.
//
// The escape rule this refines is deliberately blunt: a *qt.C passed anywhere
// the rule could not follow was treated as reaching everything, including
// (*qt.C).Defer, because a rewrite that turns a passing test into a panicking
// one is worse than a fix that never arrives. That is the right default and it
// stays the default. What it is not is a reason to stop looking, and the cost
// of not looking is concrete: measured on a project whose helpers take the
// checker, every one of 191 reported subtests was withheld, and not one of the
// helpers named went anywhere near Defer.
//
// So a call is followed when the callee is a plain function declared in the
// package under analysis. Its body is asked the same question about the
// parameter the checker arrives in, transitively, and the answer replaces the
// blanket one. Everything else keeps the blunt answer: a method, a function
// value, a callee from another package, a parameter reached through variadic
// packing. Those are the shapes where the body is either absent or the binding
// is not a simple one, and guessing at them is how a rewrite starts panicking.
//
// Recursion terminates on a visited set keyed by the callee's object, so a
// helper that calls itself is answered once rather than forever.
type calleeReach struct {
	// pass is the analysis pass and decls its package-level function bodies,
	// indexed by the object each declares.
	pass  *analysis.Pass
	decls map[types.Object]*ast.FuncDecl
	// visiting guards against a cycle in the call graph.
	visiting map[types.Object]bool
}

// newCalleeReach indexes the package's function declarations once.
func newCalleeReach(pass *analysis.Pass) *calleeReach {
	decls := make(map[types.Object]*ast.FuncDecl)
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Body == nil {
				continue
			}
			if obj := pass.TypesInfo.Defs[fn.Name]; obj != nil {
				decls[obj] = fn
			}
		}
	}
	return &calleeReach{pass: pass, decls: decls, visiting: make(map[types.Object]bool)}
}

// follow reports what the callee of call does with the argument at index, and
// whether the question could be answered at all.
//
// A false second result means the caller keeps its blunt answer. It is
// returned for every shape whose binding is not a plain parameter of a
// package-level function this pass can read.
func (r *calleeReach) follow(call *ast.CallExpr, index int) (cReach, bool) {
	// A parameter whose type cannot name the deferred methods answers the
	// question by its type alone.
	//
	// Defer and Done are C's own. A callee that receives the checker as a
	// testing.TB cannot name them, so the hazard is unreachable there whatever
	// the body does, and the body does not have to be readable for that to
	// hold. It closes the shapes no body-reading can: a function value in a
	// struct field, a parameter of a callee from another package, a locally
	// bound closure.
	//
	// The question is the parameter's method set, not its identity. An
	// interface or type parameter that declares Defer itself is satisfied by a
	// *qt.C and lets the body call the method by name whether or not the
	// argument is one underneath, so asking "is this a *qt.C" would answer
	// "safe" there and drop the one hazard this rule exists to keep -- silently,
	// because a subtest whose deferred function stops running does not fail.
	// Asking the method set also covers *Alias where the alias resolves to C,
	// and makes a separate identity check unnecessary, because C names Defer
	// itself.
	//
	// A single deferred method is enough to withhold. Requiring the parameter
	// to name the whole API would let `interface{ Defer(func()) }` back through
	// the shortcut, and one unrun deferred function is the whole hazard.
	//
	// What a parameter type bounds is what the callee can name THROUGH it. A
	// callee that converts the value back to *qt.C names the concrete type
	// itself, and no parameter type bounds that; the rule treats a conversion
	// it cannot follow here as it does everywhere else.
	//
	// The test-scoped methods are answered the same way rather than assumed:
	// Cleanup, TempDir and Setenv belong to testing.TB, so a TB-typed parameter
	// reaches them and -only-stable-fixes must still say so, while a parameter
	// that names none of them withholds nothing.
	if param, ok := r.paramTypeAt(call, index); ok && !namesAnyMethod(param, deferredCMethods) {
		return cReach{testScoped: namesAnyMethod(param, testScopedCMethods), handedOn: true}, true
	}

	ident, ok := stripParens(call.Fun).(*ast.Ident)
	if !ok {
		return cReach{}, false
	}
	obj := r.pass.TypesInfo.Uses[ident]
	if obj == nil {
		return cReach{}, false
	}
	fn, ok := r.decls[obj]
	if !ok {
		return cReach{}, false
	}
	sig, ok := r.pass.TypesInfo.TypeOf(ident).(*types.Signature)
	if !ok || sig.Variadic() {
		// A variadic call packs its tail into a slice, so the checker is held
		// by an element rather than by a parameter, and the body cannot be
		// asked about it by name.
		return cReach{}, false
	}
	param := paramFieldAt(fn, index)
	if param == nil {
		return cReach{}, false
	}
	paramObj := r.pass.TypesInfo.Defs[param]
	if paramObj == nil {
		return cReach{}, false
	}

	if r.visiting[obj] {
		// Already on the stack: this call adds nothing the outer visit will not
		// already have seen.
		return cReach{}, true
	}
	r.visiting[obj] = true
	defer delete(r.visiting, obj)

	return r.bodyReach(fn, paramObj)
}

// namesAnyMethod reports whether any of names is in t's method set, so a
// parameter is asked what can be called through it rather than what it is.
//
// The check is by name only, matching how bodyReach itself decides a use is
// deferredCMethods or testScopedCMethods: this rule has never matched a
// method's signature, only its name.
//
// A nil pkg restricts the lookup to exported names, which is what every name
// here is: these are quicktest's and testing.TB's own API.
func namesAnyMethod(t types.Type, names map[string]bool) bool {
	if t == nil {
		return false
	}
	for name := range names {
		if obj, _, _ := types.LookupFieldOrMethod(t, true, nil, name); obj != nil {
			if _, isMethod := obj.(*types.Func); isMethod {
				return true
			}
		}
	}
	return false
}

// paramFieldAt returns the identifier naming the parameter at position index,
// or nil when that position is unnamed, blank, or out of range.
func paramFieldAt(fn *ast.FuncDecl, index int) *ast.Ident {
	if fn.Type.Params == nil {
		return nil
	}
	position := 0
	for _, field := range fn.Type.Params.List {
		width := len(field.Names)
		if width == 0 {
			// An unnamed parameter cannot be referred to, so nothing in the
			// body reaches the checker through it.
			if position == index {
				return nil
			}
			position++
			continue
		}
		for _, name := range field.Names {
			if position == index {
				if name.Name == "_" {
					return nil
				}
				return name
			}
			position++
		}
	}
	return nil
}

// bodyReach asks what fn's body does with the checker bound to paramObj.
func (r *calleeReach) bodyReach(fn *ast.FuncDecl, paramObj types.Object) (cReach, bool) {
	held := heldQtCObjectsIn(r.pass, fn.Body, paramObj)

	var reach cReach
	answered := true
	inspectWithParent(fn.Body, func(n, parent ast.Node) {
		ident, ok := n.(*ast.Ident)
		if !ok || !held[r.pass.TypesInfo.Uses[ident]] {
			return
		}
		if sel, ok := parent.(*ast.SelectorExpr); ok && sel.X == ident {
			switch {
			case deferredCMethods[sel.Sel.Name]:
				reach.deferred = true
				if reach.method == "" {
					reach.method = sel.Sel.Name
				}
			case testScopedCMethods[sel.Sel.Name]:
				reach.testScoped = true
				if reach.method == "" {
					reach.method = sel.Sel.Name
				}
			}
			return
		}
		if _, rhs, ok := soleAssign(parent); ok && rhs == ident {
			return
		}
		if call, ok := parent.(*ast.CallExpr); ok {
			if index, ok := argumentIndex(call, ident); ok {
				if inner, ok := r.follow(call, index); ok {
					reach.merge(inner)
					return
				}
			}
		}
		// The checker leaves this body somewhere unfollowable, so the question
		// is unanswered and the caller keeps its blunt answer.
		answered = false
	})
	if !answered {
		return cReach{}, false
	}
	return reach, true
}

// merge folds another body's answer into this one, keeping the first method
// name so a diagnostic names a call a reader can find.
func (r *cReach) merge(other cReach) {
	r.deferred = r.deferred || other.deferred
	r.testScoped = r.testScoped || other.testScoped
	r.handedOn = r.handedOn || other.handedOn
	if r.method == "" {
		r.method = other.method
	}
}

// argumentIndex reports the position at which ident is passed to call, and
// false when it is the callee rather than an argument.
func argumentIndex(call *ast.CallExpr, ident *ast.Ident) (int, bool) {
	for i, arg := range call.Args {
		if stripParens(arg) == ast.Expr(ident) {
			return i, true
		}
	}
	return 0, false
}

// heldQtCObjectsIn returns every object holding the *qt.C that seed names,
// following the assignments that hand it on.
//
// It takes any node rather than a closure literal so that a helper's own
// aliases are followed exactly as a subtest closure's are; heldQtCObjects is
// the closure-shaped caller.
func heldQtCObjectsIn(pass *analysis.Pass, root ast.Node, seed types.Object) map[types.Object]bool {
	held := make(map[types.Object]bool, 1)
	held[seed] = true
	for changed := true; changed; {
		changed = false
		ast.Inspect(root, func(n ast.Node) bool {
			lhs, rhs, ok := soleAssign(n)
			if !ok {
				return true
			}
			src, ok := rhs.(*ast.Ident)
			if !ok || !held[pass.TypesInfo.Uses[src]] {
				return true
			}
			dst, ok := lhs.(*ast.Ident)
			if !ok {
				return true
			}
			obj := pass.TypesInfo.Defs[dst]
			if obj == nil {
				obj = pass.TypesInfo.Uses[dst]
			}
			if obj == nil || held[obj] {
				return true
			}
			held[obj] = true
			changed = true
			return true
		})
	}
	return held
}

// followedCallReach answers what happens to a *qt.C passed as an argument,
// when the callee is one this pass can read.
//
// It is the narrow opening in the blunt escape rule: a helper declared in this
// package is read rather than guessed at, and everything else -- a method, a
// function value, another package -- falls through to the conservative answer
// its caller already had.
func followedCallReach(callees *calleeReach, parent ast.Node, ident *ast.Ident) (cReach, bool) {
	call, ok := parent.(*ast.CallExpr)
	if !ok {
		return cReach{}, false
	}
	index, ok := argumentIndex(call, ident)
	if !ok {
		return cReach{}, false
	}
	return callees.follow(call, index)
}

// paramTypeAt returns the declared type of the parameter the argument at index
// binds to, and false when the callee has no signature or the position runs
// into a variadic tail.
func (r *calleeReach) paramTypeAt(call *ast.CallExpr, index int) (types.Type, bool) {
	typ := r.pass.TypesInfo.TypeOf(call.Fun)
	if typ == nil {
		return nil, false
	}
	// Underlying rather than a direct assertion, because a defined function
	// type -- the shape a struct field holding a helper usually has -- is a
	// *types.Named that Unalias leaves alone, and the argument still binds to
	// its underlying signature's parameter the same way. Rejecting it would
	// leave every call through such a field unanswered for no reason. A
	// *types.Signature is its own underlying type, so the one call covers both.
	sig, ok := types.Unalias(typ).Underlying().(*types.Signature)
	if !ok {
		return nil, false
	}
	params := sig.Params()
	if params == nil || index >= params.Len() {
		return nil, false
	}
	if sig.Variadic() && index >= params.Len()-1 {
		// Packed into a slice, so the parameter's declared type is not what the
		// argument binds to.
		return nil, false
	}
	return params.At(index).Type(), true
}

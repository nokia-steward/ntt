package interpreter

// class.go implements the runtime semantics of TTCN-3 object
// orientation (ETSI ES 201 873-1 clause 5.1): constructing class
// instances (`C.create(...)`), dispatching methods and reading
// fields (`obj.method(...)`, `obj.field`), and the `select class`
// is-a test. Class definitions are bound as *runtime.ClassDesc; a
// constructed object is a *runtime.ClassInstance whose Fields hold
// the union of own and inherited members.

import (
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// newMethodEnv builds the closure scope for a method/constructor
// body declared on defClass running against inst. It binds `this`
// (the receiver) and, when defClass has a parent, `super` - a view
// of the same instance whose class pointer is the parent, so
// `super.method(...)` dispatches against the inherited definition
// while still reading and writing the receiver's shared field map
// (ETSI 5.1.1.9).
func newMethodEnv(defClass *runtime.ClassDesc, inst *runtime.ClassInstance) *runtime.Env {
	env := runtime.NewEnv(defClass.Env)
	env.Set("this", inst)
	if parent := resolveParentClass(defClass); parent != nil {
		env.Set("super", &runtime.ClassInstance{Class: parent, Fields: inst.Fields})
	}
	// Nested classes are visible by their bare name inside the
	// enclosing class's methods, bound to the running instance so a
	// `Child.create()` captures `this` as the enclosing object
	// (ETSI 5.1.1.10).
	if defClass.Decl != nil {
		for _, d := range defClass.Decl.Defs {
			if d == nil {
				continue
			}
			if ctd, ok := d.Def.(*syntax.ClassTypeDecl); ok && ctd.Name != nil {
				name := ctd.Name.String()
				if nc := nestedClassDesc(defClass, name, inst); nc != nil {
					env.Set(name, nc)
				}
			}
		}
	}
	return env
}

// classExtendsName returns the parent class name from an `extends`
// reference expression, or "".
func classExtendsName(e syntax.Expr) string {
	switch x := e.(type) {
	case *syntax.Ident:
		if x != nil {
			return x.String()
		}
	case *syntax.SelectorExpr:
		if id, ok := x.Sel.(*syntax.Ident); ok && id != nil {
			return id.String()
		}
	}
	return ""
}

// resolveParentClass returns the ClassDesc that cd extends, or nil
// when cd has no (resolvable) parent.
func resolveParentClass(cd *runtime.ClassDesc) *runtime.ClassDesc {
	if cd == nil || cd.Decl == nil || len(cd.Decl.Extends) == 0 || cd.Env == nil {
		return nil
	}
	name := classExtendsName(cd.Decl.Extends[0])
	if name == "" {
		return nil
	}
	if v, ok := cd.Env.Get(name); ok {
		if pc, ok := forceThunk(v).(*runtime.ClassDesc); ok {
			return pc
		}
	}
	return nil
}

// classChainBaseFirst returns the inheritance chain from the root
// ancestor down to cd inclusive. A cycle (malformed `extends`) is
// broken by the visited set.
func classChainBaseFirst(cd *runtime.ClassDesc) []*runtime.ClassDesc {
	var chain []*runtime.ClassDesc
	seen := map[*runtime.ClassDesc]bool{}
	for c := cd; c != nil && !seen[c]; c = resolveParentClass(c) {
		seen[c] = true
		chain = append([]*runtime.ClassDesc{c}, chain...)
	}
	return chain
}

// nestedClassDecl finds a class declared directly inside parent's
// body (ETSI 5.1.1.10 nested classes), or nil when there is none.
func nestedClassDecl(parent *runtime.ClassDesc, name string) *syntax.ClassTypeDecl {
	if parent == nil || parent.Decl == nil || name == "" {
		return nil
	}
	for _, d := range parent.Decl.Defs {
		if d == nil {
			continue
		}
		if ctd, ok := d.Def.(*syntax.ClassTypeDecl); ok && ctd.Name != nil && ctd.Name.String() == name {
			return ctd
		}
	}
	return nil
}

// nestedClassDesc builds a ClassDesc for a class nested in parent. When
// outer is non-nil (the enclosing instance, as in
// `v_parent.Child.create()`), the nested class's closure scope binds a
// snapshot of the outer instance's fields so the inner methods can read
// the enclosing object's members by their bare names; with outer nil it
// is a plain type reference (`Parent.Child` in a declaration or a
// `select class` case).
func nestedClassDesc(parent *runtime.ClassDesc, name string, outer *runtime.ClassInstance) *runtime.ClassDesc {
	decl := nestedClassDecl(parent, name)
	if decl == nil {
		return nil
	}
	env := parent.Env
	if outer != nil && env != nil {
		scope := runtime.NewEnv(env)
		for k, v := range outer.Fields {
			scope.Set(k, v)
		}
		env = scope
	}
	return &runtime.ClassDesc{Name: name, Decl: decl, Env: env}
}

// instanceIsA reports whether inst's runtime class is name or a
// subclass of name (the `select class` / cast subsumption test,
// ETSI 5.1.2.4 / 5.1.2.6).
func instanceIsA(inst *runtime.ClassInstance, name string) bool {
	if inst == nil || name == "" {
		return false
	}
	for c := inst.Class; c != nil; c = resolveParentClass(c) {
		if c.Name == name {
			return true
		}
	}
	return false
}

// constructClassInstance builds a fresh object of class cd: it
// default-initialises every field along the inheritance chain
// (base-first) and then runs the constructor.
func constructClassInstance(cd *runtime.ClassDesc, n *syntax.CallExpr, env runtime.Scope) runtime.Object {
	if cd == nil || cd.Decl == nil {
		return runtime.Errorf("create: not a class type")
	}
	inst := &runtime.ClassInstance{Class: cd, Fields: map[string]runtime.Object{}}
	for _, c := range classChainBaseFirst(cd) {
		initClassFields(inst, c)
	}

	var argExprs []syntax.Expr
	if n != nil && n.Args != nil {
		argExprs = n.Args.List
	}
	if err := runConstructorOn(inst, cd, argExprs, env); err != nil {
		return err
	}
	return inst
}

// runConstructorOn runs class cd's constructor against an existing
// instance: the explicit `create(...)` declared directly on cd when
// present, otherwise the implicit constructor that binds the actual
// arguments positionally to the field list (inherited fields first).
// Returns a non-nil runtime error only on argument-evaluation
// failure.
func runConstructorOn(inst *runtime.ClassInstance, cd *runtime.ClassDesc, argExprs []syntax.Expr, env runtime.Scope) runtime.Object {
	if ctor := findOwnConstructor(cd); ctor != nil {
		cenv := newMethodEnv(cd, inst)
		fn := &runtime.Function{Params: ctor.Params, Body: ctor.Body, Env: cenv}
		args := evalCallArgsLazy(fn, argExprs, env)
		if len(args) == 1 && runtime.IsError(args[0]) {
			return args[0]
		}
		// Bind the formals into the constructor scope so a `: Super(args)`
		// init list (ETSI 5.1.1.6) can reference them, then run the super
		// constructor before the constructor body (base-first init).
		bindParamsInto(cenv, ctor.Params, args)
		if ctor.Init != nil {
			if e := applySuperInit(inst, ctor.Init, cenv); e != nil && runtime.IsError(e) {
				return e
			}
		}
		applyFunctionWithCallSite(fn, args, argExprs)
		return nil
	}

	args := evalExprList(argExprs, env)
	if len(args) == 1 && runtime.IsError(args[0]) {
		return args[0]
	}
	names := classFieldNames(classChainBaseFirst(cd))
	for i, a := range args {
		if i < len(names) && a != nil && !runtime.IsError(a) {
			inst.Fields[names[i]] = a
		}
	}
	return nil
}

// bindParamsInto positionally binds evaluated actual arguments to the
// formal parameter names in env. Used to make constructor formals
// visible to a `: Super(args)` init list before the body runs.
func bindParamsInto(env *runtime.Env, params *syntax.FormalPars, args []runtime.Object) {
	if env == nil || params == nil {
		return
	}
	for i, p := range params.List {
		if p == nil || p.Name == nil || i >= len(args) {
			continue
		}
		env.Set(p.Name.String(), args[i])
	}
}

// applySuperInit handles the `: Super(args)` constructor init list
// that follows a `C.create(...)` call. y is the super-constructor
// reference expression (a CallExpr naming the parent class). The
// super class is resolved by name when given, otherwise it falls
// back to cd's direct parent.
func applySuperInit(inst *runtime.ClassInstance, y syntax.Expr, env runtime.Scope) runtime.Object {
	if inst == nil {
		return nil
	}
	var (
		superName string
		argExprs  []syntax.Expr
	)
	if call, ok := y.(*syntax.CallExpr); ok {
		superName = classExtendsName(call.Fun)
		if call.Args != nil {
			argExprs = call.Args.List
		}
	} else {
		superName = classExtendsName(y)
	}

	super := resolveParentClass(inst.Class)
	if superName != "" {
		for c := resolveParentClass(inst.Class); c != nil; c = resolveParentClass(c) {
			if c.Name == superName {
				super = c
				break
			}
		}
	}
	if super == nil {
		return nil
	}
	return runConstructorOn(inst, super, argExprs, env)
}

// isCreateCall reports whether e is a `something.create(...)` call,
// the left operand of a constructor init list.
func isCreateCall(e syntax.Expr) bool {
	call, ok := e.(*syntax.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*syntax.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.Sel.(*syntax.Ident)
	return ok && id != nil && id.String() == "create"
}

// initClassFields default-initialises the fields declared directly
// on cd (not inherited) into inst. A field with an initializer gets
// the evaluated value (with `this` visible); otherwise Undefined.
func initClassFields(inst *runtime.ClassInstance, cd *runtime.ClassDesc) {
	if cd == nil || cd.Decl == nil {
		return
	}
	for _, md := range cd.Decl.Defs {
		if md == nil {
			continue
		}
		vd, ok := md.Def.(*syntax.ValueDecl)
		if !ok || vd == nil {
			continue
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil {
				continue
			}
			var val runtime.Object = runtime.Undefined
			if dec.Value != nil {
				fenv := runtime.NewEnv(cd.Env)
				fenv.Set("this", inst)
				if v := eval(dec.Value, fenv); !runtime.IsError(v) && v != nil {
					val = v
				}
			}
			inst.Fields[dec.Name.String()] = val
		}
	}
}

// classFieldNames returns the field names across the chain in
// declaration order (base-first), used for implicit-constructor
// positional binding.
func classFieldNames(chain []*runtime.ClassDesc) []string {
	var names []string
	for _, c := range chain {
		if c.Decl == nil {
			continue
		}
		for _, md := range c.Decl.Defs {
			if md == nil {
				continue
			}
			vd, ok := md.Def.(*syntax.ValueDecl)
			if !ok || vd == nil {
				continue
			}
			for _, dec := range vd.Decls {
				if dec != nil && dec.Name != nil {
					names = append(names, dec.Name.String())
				}
			}
		}
	}
	return names
}

// findOwnConstructor returns the explicit constructor declared
// directly on cd (not inherited), or nil. A subclass without its own
// constructor uses the implicit positional one over the full field
// list rather than borrowing the parent's constructor.
func findOwnConstructor(cd *runtime.ClassDesc) *syntax.ConstructorDecl {
	if cd == nil || cd.Decl == nil {
		return nil
	}
	for _, md := range cd.Decl.Defs {
		if md == nil {
			continue
		}
		if ctor, ok := md.Def.(*syntax.ConstructorDecl); ok && ctor != nil {
			return ctor
		}
	}
	return nil
}

// findMethod returns the nearest method named name walking up from
// cd, plus the declaring class (used as the body closure so the
// method resolves its module-level siblings and types).
func findMethod(cd *runtime.ClassDesc, name string) (*syntax.FuncDecl, *runtime.ClassDesc) {
	for c := cd; c != nil; c = resolveParentClass(c) {
		if c.Decl == nil {
			continue
		}
		for _, md := range c.Decl.Defs {
			if md == nil {
				continue
			}
			if fn, ok := md.Def.(*syntax.FuncDecl); ok && fn != nil &&
				fn.Name != nil && fn.Name.String() == name {
				return fn, c
			}
		}
	}
	return nil, nil
}

// evalSelectClassStmt implements `select class (obj) { case (T) {...} }`
// (ETSI 5.1.2.4): the first case whose class T the object is an
// instance of (its runtime class or a superclass) is executed; an
// `else` case is the fallback.
func evalSelectClassStmt(n *syntax.SelectStmt, env runtime.Scope) runtime.Object {
	tag := eval(n.Tag, env)
	if runtime.IsError(tag) {
		return tag
	}
	inst, _ := tag.(*runtime.ClassInstance)

	var elseCase *syntax.CaseClause
	for _, cc := range n.Body {
		if cc == nil {
			continue
		}
		if cc.Case == nil {
			if elseCase == nil {
				elseCase = cc
			}
			continue
		}
		for _, p := range cc.Case.List {
			name := classExtendsName(p)
			if name != "" && inst != nil && instanceIsA(inst, name) {
				if cc.Body != nil {
					return eval(cc.Body, env)
				}
				return nil
			}
		}
	}
	if elseCase != nil && elseCase.Body != nil {
		return eval(elseCase.Body, env)
	}
	return nil
}

// dispatchClassMethod evaluates `inst.name(args)`. It returns
// (result, true) when name resolves to a method (with a body), and
// (nil, false) otherwise so the caller can fall through to other
// selector handling.
func dispatchClassMethod(inst *runtime.ClassInstance, name string, n *syntax.CallExpr, env runtime.Scope) (runtime.Object, bool) {
	if inst == nil || inst.Class == nil {
		return nil, false
	}
	method, defClass := findMethod(inst.Class, name)
	if method == nil || method.Body == nil {
		return nil, false
	}
	menv := newMethodEnv(defClass, inst)
	fn := &runtime.Function{Params: method.Params, Body: method.Body, Env: menv}

	var argExprs []syntax.Expr
	if n != nil && n.Args != nil {
		argExprs = n.Args.List
	}
	args := evalCallArgsLazy(fn, argExprs, env)
	if len(args) == 1 && runtime.IsError(args[0]) {
		return args[0], true
	}
	indexSnapshot := snapshotLHSIndices(fn, argExprs, env)
	ret, fenv, stopped := applyFunctionWithCallSite(fn, args, argExprs)
	if !stopped {
		writebackInoutParamsWithSnapshot(fn, argExprs, indexSnapshot, fenv, env)
	}
	return ret, true
}

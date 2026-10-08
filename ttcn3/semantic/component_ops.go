// component_ops.go implements the static portion of ETSI ES 201 873-1
// clause 21.3 ("test component operations") - the bits we can decide
// without modelling the live alive/done/killed state of every PTC.
//
// Three checks live here:
//
//  1. create-arg-type: `Comp.create(name [, location])` requires
//     charstring arguments (21.3.1). Passing an integer / boolean /
//     bit-string literal is a hard reject.
//
//  2. start-no-port-timer-default: a function passed to
//     `ref.start(funcCall(args))` must not declare ports, timers, or
//     defaults in its formal-parameter list - 21.3.2 forbids passing
//     port references, timer references or default references into a
//     started behaviour.
//
//  3. start-runs-on-compat: the started function's `runs on Comp`
//     clause must match the declared component type of the receiver
//     variable. We don't yet model `extends`-chains so the check is
//     exact-name equality with the obvious safe exits (unknown
//     receiver type, unresolved function).
//
// All three checks rely on the same module-level scans the connect /
// map compat code already does (component-port table, var ->
// component-type table); we reuse the existing helpers.
package semantic

import (
	"fmt"
	"strings"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkComponentOps(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic

	compPorts := collectComponentPorts(mod)
	portTypes := collectPortTypes(mod)
	funcs := collectFuncMeta(mod, portTypes)
	classes := collectClassNames(mod)
	abstracts := collectAbstractClasses(mod)
	compParents := collectComponentParents(mod)

	diags = append(diags, checkClassSubtyping(mod, classes)...)
	diags = append(diags, checkClassMemberShadowing(mod)...)

	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		varTypes := collectVarComponentTypes(fn.Body, compPorts)
		runsOn := ""
		if fn.RunsOn != nil && fn.RunsOn.Comp != nil {
			runsOn = syntax.Name(fn.RunsOn.Comp)
		}
		diags = append(diags, checkComponentOpsInBody(
			fn.Body, runsOn, varTypes, funcs,
			compPorts, classes, abstracts, compParents,
		)...)
	}
	return diags
}

// collectAbstractClasses returns the names of class declarations
// tagged with the `@abstract` modifier (ETSI ES 203 022 5.1.1.2).
// Abstract classes cannot be directly instantiated via .create().
func collectAbstractClasses(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		ct, ok := d.Def.(*syntax.ClassTypeDecl)
		if !ok || ct.Name == nil || ct.Modif == nil {
			continue
		}
		if strings.EqualFold(ct.Modif.String(), "@abstract") {
			out[ct.Name.String()] = true
		}
	}
	return out
}

// checkClassMemberShadowing flags class members whose names collide
// with members of the component listed in the class's `runs on`
// clause. ETSI ES 203 022 5.1.1.1 forbids reusing component member
// identifiers as class members or as formal parameter / local names
// inside class methods.
func checkClassMemberShadowing(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	compMembers := collectComponentMemberNames(mod)
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		ct, ok := d.Def.(*syntax.ClassTypeDecl)
		if !ok || ct.Name == nil || ct.RunsOn == nil || ct.RunsOn.Comp == nil {
			continue
		}
		compName := syntax.Name(ct.RunsOn.Comp)
		names := compMembers[compName]
		if len(names) == 0 {
			continue
		}
		for _, member := range ct.Defs {
			if member == nil || member.Def == nil {
				continue
			}
			if name := nameOfModuleDef(member); name != "" && names[name] {
				diags = append(diags, Diagnostic{
					Code:     "class-member-shadows-component",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"class %q member %q shadows %q's member of the same name",
						ct.Name.String(), name, compName),
					Node: member,
					Span: syntax.SpanOf(member),
				})
			}
			if fn, ok := member.Def.(*syntax.FuncDecl); ok && fn.Params != nil {
				for _, p := range fn.Params.List {
					if p == nil || p.Name == nil {
						continue
					}
					pname := p.Name.String()
					if names[pname] {
						diags = append(diags, Diagnostic{
							Code:     "class-method-param-shadows-component",
							Severity: SeverityError,
							Message: fmt.Sprintf(
								"method %q parameter %q shadows %q's member of the same name",
								funcName(fn), pname, compName),
							Node: p,
							Span: syntax.SpanOf(p),
						})
					}
				}
			}
		}
	}
	return diags
}

// collectComponentMemberNames returns a map of component-type name ->
// set-of-member-names declared in that component. Used by the class
// shadowing check.
func collectComponentMemberNames(mod *syntax.Module) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		ct, ok := d.Def.(*syntax.ComponentTypeDecl)
		if !ok || ct.Name == nil || ct.Body == nil {
			continue
		}
		names := map[string]bool{}
		syntax.Inspect(ct.Body, func(n syntax.Node) bool {
			switch x := n.(type) {
			case *syntax.ValueDecl:
				for _, d := range x.Decls {
					if d != nil && d.Name != nil {
						names[d.Name.String()] = true
					}
				}
				return false
			case *syntax.FuncDecl:
				if x.Name != nil {
					names[x.Name.String()] = true
				}
				return false
			}
			return true
		})
		out[ct.Name.String()] = names
	}
	return out
}

func nameOfModuleDef(m *syntax.ModuleDef) string {
	if m == nil || m.Def == nil {
		return ""
	}
	switch x := m.Def.(type) {
	case *syntax.ValueDecl:
		if len(x.Decls) > 0 && x.Decls[0] != nil && x.Decls[0].Name != nil {
			return x.Decls[0].Name.String()
		}
	case *syntax.FuncDecl:
		return funcName(x)
	}
	return ""
}

func funcName(fn *syntax.FuncDecl) string {
	if fn == nil || fn.Name == nil {
		return ""
	}
	return fn.Name.String()
}

// checkClassSubtyping flags `type <className> Alias;` constructs where
// `<className>` is a class type. ETSI ES 203 022 5.1.1.0 reserves
// class type definitions for the `type class` syntax; aliasing them
// via the normal sub-type mechanism is forbidden.
func checkClassSubtyping(mod *syntax.Module, classes map[string]bool) []Diagnostic {
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		st, ok := d.Def.(*syntax.SubTypeDecl)
		if !ok || st.Field == nil {
			continue
		}
		ref, ok := st.Field.Type.(*syntax.RefSpec)
		if !ok || ref.X == nil {
			continue
		}
		baseID, ok := ref.X.(*syntax.Ident)
		if !ok {
			continue
		}
		if !classes[baseID.String()] {
			continue
		}
		name := ""
		if st.Field.Name != nil {
			name = st.Field.Name.String()
		}
		diags = append(diags, Diagnostic{
			Code:     "class-subtype-forbidden",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"%q aliases class %q via the sub-type mechanism, which is not allowed",
				name, baseID.String()),
			Node: st,
			Span: syntax.SpanOf(st),
		})
	}
	return diags
}

// collectClassNames returns the set of class-type names declared in
// the module. Used so the `Comp.create(...)` validator can avoid
// flagging OO class constructor calls, which share the surface
// syntax but obey different argument rules (any value goes).
func collectClassNames(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		ct, ok := d.Def.(*syntax.ClassTypeDecl)
		if !ok || ct.Name == nil {
			continue
		}
		out[ct.Name.String()] = true
	}
	return out
}

// collectComponentParents maps each component type to its direct
// `extends` parents. The runs-on compat check then walks the chain to
// determine whether a function's runs-on component is a (transitive)
// ancestor of the receiver's type, which 6.3.3 makes the compat
// criterion.
func collectComponentParents(mod *syntax.Module) map[string][]string {
	out := map[string][]string{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		ct, ok := d.Def.(*syntax.ComponentTypeDecl)
		if !ok || ct.Name == nil {
			continue
		}
		var parents []string
		for _, e := range ct.Extends {
			if e == nil {
				continue
			}
			name := syntax.Name(e)
			if name != "" {
				parents = append(parents, name)
			}
		}
		out[ct.Name.String()] = parents
	}
	return out
}

// extendsOrEqual returns true when `child` is `parent` or any
// transitive parent reached via the extends-chain.
func extendsOrEqual(child, parent string, parents map[string][]string) bool {
	if child == parent {
		return true
	}
	seen := map[string]bool{}
	var walk func(n string) bool
	walk = func(n string) bool {
		if seen[n] {
			return false
		}
		seen[n] = true
		for _, p := range parents[n] {
			if p == parent {
				return true
			}
			if walk(p) {
				return true
			}
		}
		return false
	}
	return walk(child)
}

// funcMeta is the trimmed view of a FuncDecl the component-op checks
// need: the runs-on component name (empty if unspecified) and the
// formal-parameter kinds that decide whether the function can be
// started on a non-alive PTC.
type funcMeta struct {
	runsOn     string
	formals    []formalParam
	returnKind syntax.Kind // TIMER / PORT / IDENT / etc.
	returnName string      // type identifier (e.g. "default", "MyPortType")
}

type formalParam struct {
	name     string
	typeKind syntax.Kind // TIMER / PORT / IDENT etc.
	typeName string      // type identifier (e.g. "default", "MyPortType")
}

func collectFuncMeta(mod *syntax.Module, portTypes map[string]*portDirs) map[string]*funcMeta {
	out := map[string]*funcMeta{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}
		meta := &funcMeta{}
		if fn.RunsOn != nil && fn.RunsOn.Comp != nil {
			meta.runsOn = syntax.Name(fn.RunsOn.Comp)
		}
		if fn.Params != nil {
			for _, p := range fn.Params.List {
				if p == nil || p.Name == nil {
					continue
				}
				fp := formalParam{
					name: p.Name.String(),
				}
				if id, ok := p.Type.(*syntax.Ident); ok {
					fp.typeName = id.String()
					if id.Tok != nil {
						fp.typeKind = id.Tok.Kind()
					}
				}
				meta.formals = append(meta.formals, fp)
			}
		}
		// Promote "type is a declared port type" to a synthetic
		// PORT kind so the start-arg check doesn't have to chase
		// the port-type table again.
		for i := range meta.formals {
			if _, isPort := portTypes[meta.formals[i].typeName]; isPort {
				meta.formals[i].typeKind = syntax.PORT
			}
		}
		if fn.Return != nil && fn.Return.Type != nil {
			if id, ok := fn.Return.Type.(*syntax.Ident); ok {
				meta.returnName = id.String()
				if id.Tok != nil {
					meta.returnKind = id.Tok.Kind()
				}
				if _, isPort := portTypes[meta.returnName]; isPort {
					meta.returnKind = syntax.PORT
				}
			}
		}
		out[fn.Name.String()] = meta
	}
	return out
}

func checkComponentOpsInBody(
	body *syntax.BlockStmt,
	runsOn string,
	varTypes map[string]string,
	funcs map[string]*funcMeta,
	compPorts map[string]map[string]string,
	classes map[string]bool,
	abstracts map[string]bool,
	compParents map[string][]string,
) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(body, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		if rx, ok := n.(*syntax.RedirectExpr); ok {
			diags = append(diags, checkIndexRedirect(rx)...)
			return true
		}
		ce, ok := n.(*syntax.CallExpr)
		if !ok {
			return true
		}
		sel, ok := ce.Fun.(*syntax.SelectorExpr)
		if !ok {
			return true
		}
		op, ok := sel.Sel.(*syntax.Ident)
		if !ok {
			return true
		}
		switch op.String() {
		case "create":
			if recv, ok := sel.X.(*syntax.Ident); ok && classes[recv.String()] {
				// `AbstractClass.create()` is illegal
				// (ETSI ES 203 022 5.1.1.2).
				if abstracts[recv.String()] {
					diags = append(diags, Diagnostic{
						Code:     "abstract-class-create",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"%q is an abstract class and cannot be instantiated",
							recv.String()),
						Node: ce,
						Span: syntax.SpanOf(ce),
					})
				}
				// Other class constructors share the
				// surface syntax (`MyClass.create(<ctor-args>)`)
				// but follow different rules; skip the
				// component-create arg check.
				return true
			}
			diags = append(diags, checkCreateArgs(ce)...)
		case "start", "call":
			diags = append(diags, checkStartArgs(
				ce, sel, runsOn, varTypes, funcs, compParents,
			)...)
		}
		return true
	})
	return diags
}

// componentArrayOps is the set of TTCN-3 component-operation method
// names that ETSI 21.3.{5,6,7,8} allow combining with the `any from
// <array>` qualifier. Used by checkIndexRedirect to confirm that any
// `@index value` redirection sits on top of an actual array iteration
// rather than a single component reference (where index-redirect is
// nonsense).
var componentArrayOps = map[string]bool{
	"alive":   true,
	"running": true,
	"done":    true,
	"killed":  true,
}

// checkIndexRedirect flags `<comp>.{alive,running,done,killed} ->
// @index value v` when the receiver is *not* an `any from <array>`
// expression. ETSI ES 201 873-1 21.3.5 restriction (c) (and the
// equivalent clauses in 21.3.{6,7,8}) say the index redirection is
// only valid in the array form; single-component receivers reject it
// at compile time. We only diagnose when we can syntactically prove
// the receiver is a single reference - anything ambiguous (function
// call result, parenthesised expression, ...) gets the benefit of the
// doubt.
func checkIndexRedirect(rx *syntax.RedirectExpr) []Diagnostic {
	if rx == nil || rx.IndexTok == nil {
		return nil
	}
	sel, ok := rx.X.(*syntax.SelectorExpr)
	if !ok {
		return nil
	}
	op, ok := sel.Sel.(*syntax.Ident)
	if !ok {
		return nil
	}
	if !componentArrayOps[op.String()] {
		return nil
	}
	// `any from <expr>.op -> @index value v` - the inner X is a
	// FromExpr; that's the legal form, do not diagnose.
	if _, ok := sel.X.(*syntax.FromExpr); ok {
		return nil
	}
	return []Diagnostic{{
		Code:     "index-redirect-not-any-from",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"%q with @index value redirection requires an `any from <array>` receiver",
			op.String()),
		Node: rx,
		Span: syntax.SpanOf(rx),
	}}
}

// checkCreateArgs flags `Comp.create(arg)` calls whose first argument
// (the name) is obviously not a charstring. We only catch the literal
// cases - any expression that *might* evaluate to a charstring is
// passed through silently. Same for the second argument (the
// location), which must also be a charstring per 21.3.1.
func checkCreateArgs(ce *syntax.CallExpr) []Diagnostic {
	if ce.Args == nil || len(ce.Args.List) == 0 {
		return nil
	}
	var diags []Diagnostic
	for i, arg := range ce.Args.List {
		if i >= 2 {
			break
		}
		if !looksDefinitelyNonCharstring(arg) {
			continue
		}
		slot := "name"
		if i == 1 {
			slot = "location"
		}
		diags = append(diags, Diagnostic{
			Code:     "create-arg-not-charstring",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"create(): %s argument must be a charstring, got %s",
				slot, literalDescription(arg)),
			Node: arg,
			Span: syntax.SpanOf(arg),
		})
	}
	return diags
}

// looksDefinitelyNonCharstring identifies a small set of literal /
// reference forms whose value cannot be a charstring under any
// circumstance. We err on the side of silence for identifiers and
// composite expressions - the conformance gate cares about avoiding
// false positives more than catching every possible misuse.
func looksDefinitelyNonCharstring(e syntax.Expr) bool {
	switch v := e.(type) {
	case *syntax.ValueLiteral:
		if v.Tok == nil {
			return false
		}
		switch v.Tok.Kind() {
		case syntax.INT, syntax.FLOAT, syntax.TRUE, syntax.FALSE,
			syntax.BSTRING:
			return true
		}
	case *syntax.UnaryExpr:
		// `create(-, ...)` - a bare dash sneaks through here as
		// a UnaryExpr with a SUB op and no operand.  Don't trip
		// on that; the dash is the not-used marker that 21.3.1
		// allows in the name slot (means "let the runtime pick a
		// name").
		if v.Op != nil && v.Op.Kind() == syntax.SUB && v.X == nil {
			return false
		}
	}
	return false
}

func literalDescription(e syntax.Expr) string {
	switch v := e.(type) {
	case *syntax.ValueLiteral:
		if v.Tok != nil {
			switch v.Tok.Kind() {
			case syntax.INT, syntax.FLOAT:
				return "numeric literal"
			case syntax.TRUE, syntax.FALSE:
				return "boolean literal"
			case syntax.BSTRING:
				return "bit/hex/octet string literal"
			}
		}
	}
	return "non-charstring expression"
}

// checkStartArgs validates a `ref.start(funcCall(args))` invocation:
//
//   - the function called inside .start(...) must not declare a port,
//     timer, or default parameter (21.3.2);
//   - the function's runs-on component must match the receiver's
//     declared component type (21.3.2 / 6.3.3).
//
// We resolve the receiver via the same var-type table connect/map
// uses and look the function up by simple name in the module-local
// funcs table. Both are best-effort: unresolved names just skip the
// check.
func checkStartArgs(
	ce *syntax.CallExpr,
	sel *syntax.SelectorExpr,
	runsOn string,
	varTypes map[string]string,
	funcs map[string]*funcMeta,
	compParents map[string][]string,
) []Diagnostic {
	if ce.Args == nil || len(ce.Args.List) != 1 {
		return nil
	}
	inner, ok := ce.Args.List[0].(*syntax.CallExpr)
	if !ok {
		return nil
	}
	fnIdent, ok := inner.Fun.(*syntax.Ident)
	if !ok {
		return nil
	}
	meta := funcs[fnIdent.String()]
	if meta == nil {
		return nil
	}

	var diags []Diagnostic
	for _, fp := range meta.formals {
		switch {
		case fp.typeKind == syntax.PORT:
			diags = append(diags, startForbiddenParam(ce, fp.name, "port"))
		case fp.typeKind == syntax.TIMER:
			diags = append(diags, startForbiddenParam(ce, fp.name, "timer"))
		case fp.typeName == "default":
			diags = append(diags, startForbiddenParam(ce, fp.name, "default"))
		}
	}
	switch {
	case meta.returnKind == syntax.PORT:
		diags = append(diags, startForbiddenReturn(ce, fnIdent.String(), "port"))
	case meta.returnKind == syntax.TIMER || meta.returnName == "timer":
		diags = append(diags, startForbiddenReturn(ce, fnIdent.String(), "timer"))
	case meta.returnName == "default":
		diags = append(diags, startForbiddenReturn(ce, fnIdent.String(), "default"))
	}

	recvType := receiverComponentType(sel.X, runsOn, varTypes)
	if recvType != "" && meta.runsOn != "" &&
		!extendsOrEqual(recvType, meta.runsOn, compParents) {
		diags = append(diags, Diagnostic{
			Code:     "start-runs-on-incompatible",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"start(%s()): receiver is %s but function runs on %s",
				fnIdent.String(), recvType, meta.runsOn),
			Node: ce,
			Span: syntax.SpanOf(ce),
		})
	}
	return diags
}

func startForbiddenReturn(ce *syntax.CallExpr, name, kind string) Diagnostic {
	return Diagnostic{
		Code:     "start-forbidden-return-kind",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"start()/call(): function %q returns %s, which is forbidden (ETSI 21.3.2 / 21.3.10)",
			name, kind),
		Node: ce,
		Span: syntax.SpanOf(ce),
	}
}

func startForbiddenParam(ce *syntax.CallExpr, name, kind string) Diagnostic {
	return Diagnostic{
		Code:     "start-forbidden-param-kind",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"start(): cannot pass %s parameter %q to a started behaviour",
			kind, name),
		Node: ce,
		Span: syntax.SpanOf(ce),
	}
}

// receiverComponentType resolves the component-type name of the
// receiver expression `recv` in `recv.start(...)`. Falls back to the
// enclosing function's runs-on for the common `self` / `mtc` / `system`
// case. Returns "" when the receiver is too dynamic to resolve
// statically.
func receiverComponentType(
	recv syntax.Expr,
	runsOn string,
	varTypes map[string]string,
) string {
	id, ok := recv.(*syntax.Ident)
	if !ok {
		return ""
	}
	switch id.String() {
	case "self", "mtc":
		return runsOn
	}
	return varTypes[id.String()]
}

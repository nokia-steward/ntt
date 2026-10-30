// constructor_rules.go enforces ETSI ES 203 022 5.1.1.5 on class
// constructors and class field initialisers:
//
//   - explicit `create(...)` constructors cannot declare `out` or
//     `inout` formal parameters; only `in` (and the default
//     direction, which is `in`) are permitted.
//   - a class field initialiser cannot reference the field being
//     initialised (direct self-reference is a cyclic init).
//   - a class field initialiser cannot reference a sibling field
//     that itself has no initialiser - that sibling is unbound
//     at construction time and would propagate the unbound value.
//   - a class field initialiser cannot invoke a member function of
//     its own class - methods are not yet bound to a self.
//   - a constructor body cannot reassign a const/template field
//     whose declaration already supplied the one allowed init.
//   - a constructor body cannot invoke a member function of its
//     own class on `this` (or as an unqualified call resolving to
//     a member). The object is not yet fully constructed.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkConstructorRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		cd, ok := n.(*syntax.ConstructorDecl)
		if !ok || cd == nil || cd.Params == nil {
			return true
		}
		for _, p := range cd.Params.List {
			if p == nil || p.Direction == nil {
				continue
			}
			dir := p.Direction.String()
			if dir != "out" && dir != "inout" {
				continue
			}
			name := ""
			if p.Name != nil {
				name = p.Name.String()
			}
			diags = append(diags, Diagnostic{
				Code:     "constructor-out-inout-param",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"constructor parameter %q has direction %s; only `in` parameters are allowed (ETSI 5.1.1.5)",
					name, dir),
				Node: p,
				Span: syntax.SpanOf(p),
			})
		}
		return true
	})
	diags = append(diags, checkClassFieldInitRefs(mod)...)
	diags = append(diags, checkClassFieldInitMemberCalls(mod)...)
	diags = append(diags, checkClassConstructorBody(mod)...)
	return diags
}

// classFieldKinds collects per-class information needed by the
// constructor-body and field-initialiser rules.
type classFieldKinds struct {
	// initializedConsts is the set of field names declared as
	// `const T x := ...` (initialiser present).
	initializedConsts map[string]bool
	// initializedTemplates is the set of field names declared as
	// `template T x := ...` (initialiser present).
	initializedTemplates map[string]bool
	// methods is the set of class member function / altstep /
	// testcase names.
	methods map[string]bool
}

func collectClassFieldKinds(ct *syntax.ClassTypeDecl) classFieldKinds {
	out := classFieldKinds{
		initializedConsts:    map[string]bool{},
		initializedTemplates: map[string]bool{},
		methods:              map[string]bool{},
	}
	for _, member := range ct.Defs {
		if member == nil || member.Def == nil {
			continue
		}
		switch d := member.Def.(type) {
		case *syntax.ValueDecl:
			kind := ""
			// `timer` declarations carry a nil KindTok (the kind is
			// implied), so guard before reading it.
			if d.KindTok == nil {
				continue
			}
			if d.KindTok.Kind() == syntax.CONST {
				kind = "const"
			} else if d.KindTok.Kind() == syntax.TEMPLATE {
				kind = "template"
			}
			if kind == "" {
				continue
			}
			for _, dec := range d.Decls {
				if dec == nil || dec.Name == nil || dec.Value == nil {
					continue
				}
				name := dec.Name.String()
				if kind == "const" {
					out.initializedConsts[name] = true
				} else {
					out.initializedTemplates[name] = true
				}
			}
		case *syntax.TemplateDecl:
			if d == nil || d.Name == nil || d.Value == nil {
				continue
			}
			out.initializedTemplates[d.Name.String()] = true
		case *syntax.FuncDecl:
			if d == nil || d.Name == nil {
				continue
			}
			out.methods[d.Name.String()] = true
		}
	}
	return out
}

// checkClassFieldInitMemberCalls flags field initialisers that
// invoke a member function of the enclosing class.
// ETSI 5.1.1.5: "The initialization of a member field shall not
// invoke any member function in the object being initialized."
func checkClassFieldInitMemberCalls(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ct, ok := n.(*syntax.ClassTypeDecl)
		if !ok || ct == nil {
			return true
		}
		kinds := collectClassFieldKinds(ct)
		if len(kinds.methods) == 0 {
			return true
		}
		className := ""
		if ct.Name != nil {
			className = ct.Name.String()
		}
		for _, member := range ct.Defs {
			if member == nil || member.Def == nil {
				continue
			}
			vd, ok := member.Def.(*syntax.ValueDecl)
			if !ok || vd == nil {
				continue
			}
			for _, dec := range vd.Decls {
				if dec == nil || dec.Value == nil {
					continue
				}
				fieldName := ""
				if dec.Name != nil {
					fieldName = dec.Name.String()
				}
				calls := collectMemberCalls(dec.Value, kinds.methods)
				for _, c := range calls {
					diags = append(diags, Diagnostic{
						Code:     "class-field-init-calls-member",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"initialiser of class %q field %q invokes member function %q; member functions cannot be called during field initialisation (ETSI 5.1.1.5)",
							className, fieldName, c),
						Node: dec.Value,
						Span: syntax.SpanOf(dec.Value),
					})
				}
			}
		}
		return true
	})
	return diags
}

// checkClassConstructorBody enforces two rules on constructor
// bodies, per ETSI 5.1.1.5:
//
//   - constants and templates already initialised in their
//     declaration cannot be reassigned (initialised exactly once).
//   - the constructor body cannot invoke a member function of the
//     same class (the object is not yet fully constructed).
func checkClassConstructorBody(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	byName := classDeclsByName(mod)
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ct, ok := n.(*syntax.ClassTypeDecl)
		if !ok || ct == nil {
			return true
		}
		kinds := collectClassFieldKinds(ct)
		className := ""
		if ct.Name != nil {
			className = ct.Name.String()
		}
		for _, member := range ct.Defs {
			if member == nil || member.Def == nil {
				continue
			}
			cd, ok := member.Def.(*syntax.ConstructorDecl)
			if !ok || cd == nil || cd.Body == nil {
				continue
			}
			diags = append(diags, constructorBodyDiags(cd.Body, kinds, className)...)
			diags = append(diags, constructorExternalAssignDiags(cd, ct, byName, className)...)
		}
		return true
	})
	return diags
}

// classDeclsByName indexes every class declaration in the module by
// name so the inheritance chain can be walked.
func classDeclsByName(mod *syntax.Module) map[string]*syntax.ClassTypeDecl {
	out := map[string]*syntax.ClassTypeDecl{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		if ct, ok := d.Def.(*syntax.ClassTypeDecl); ok && ct != nil && ct.Name != nil {
			out[ct.Name.String()] = ct
		}
	}
	return out
}

// allClassFieldNames returns the set of field names visible in ct,
// including those inherited from superclasses, plus a flag reporting
// whether the whole chain was resolvable. When a superclass is not
// present in the module (e.g. imported), the flag is false and callers
// should skip field-based checks to avoid false positives.
func allClassFieldNames(ct *syntax.ClassTypeDecl, byName map[string]*syntax.ClassTypeDecl) (map[string]bool, bool) {
	out := map[string]bool{}
	seen := map[string]bool{}
	cur := ct
	for cur != nil {
		nm := ""
		if cur.Name != nil {
			nm = cur.Name.String()
		}
		if seen[nm] {
			return out, true
		}
		seen[nm] = true
		for _, m := range cur.Defs {
			if m == nil || m.Def == nil {
				continue
			}
			if vd, ok := m.Def.(*syntax.ValueDecl); ok && vd != nil {
				for _, dec := range vd.Decls {
					if dec != nil && dec.Name != nil {
						out[dec.Name.String()] = true
					}
				}
			}
		}
		if len(cur.Extends) == 0 {
			return out, true
		}
		parent, ok := byName[identName(cur.Extends[0])]
		if !ok {
			return out, false
		}
		cur = parent
	}
	return out, true
}

// constructorExternalAssignDiags flags an assignment in a constructor
// body whose target is a bare identifier that is neither local to the
// constructor (a parameter or a variable declared in the body) nor an
// accessible field of the class (own or inherited). ETSI 5.1.1.5: a
// constructor "shall not assign to variables that are not local to the
// constructor body or accessible fields of the class".
func constructorExternalAssignDiags(cd *syntax.ConstructorDecl, ct *syntax.ClassTypeDecl, byName map[string]*syntax.ClassTypeDecl, className string) []Diagnostic {
	if cd == nil || cd.Body == nil {
		return nil
	}
	fields, complete := allClassFieldNames(ct, byName)
	if !complete {
		return nil
	}
	locals := map[string]bool{}
	if cd.Params != nil {
		for _, p := range cd.Params.List {
			if p != nil && p.Name != nil {
				locals[p.Name.String()] = true
			}
		}
	}
	syntax.Inspect(cd.Body, func(n syntax.Node) bool {
		switch d := n.(type) {
		case *syntax.ValueDecl:
			for _, dec := range d.Decls {
				if dec != nil && dec.Name != nil {
					locals[dec.Name.String()] = true
				}
			}
		case *syntax.TemplateDecl:
			if d != nil && d.Name != nil {
				locals[d.Name.String()] = true
			}
		}
		return true
	})
	var diags []Diagnostic
	syntax.Inspect(cd.Body, func(n syntax.Node) bool {
		be, ok := n.(*syntax.BinaryExpr)
		if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
			return true
		}
		id, ok := be.X.(*syntax.Ident)
		if !ok || id == nil {
			return true
		}
		name := id.String()
		if name == "" || name == "this" || locals[name] || fields[name] {
			return true
		}
		diags = append(diags, Diagnostic{
			Code:     "constructor-external-assign",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"constructor of class %q assigns to %q, which is neither local to the constructor nor a field of the class (ETSI 5.1.1.5)",
				className, name),
			Node: be,
			Span: syntax.SpanOf(be),
		})
		return true
	})
	return diags
}

func constructorBodyDiags(body *syntax.BlockStmt, kinds classFieldKinds, className string) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(body, func(n syntax.Node) bool {
		if be, ok := n.(*syntax.BinaryExpr); ok && be != nil && be.Op != nil &&
			be.Op.Kind() == syntax.ASSIGN {
			if name, viaThis := thisFieldName(be.X); name != "" && viaThis {
				if kinds.initializedConsts[name] {
					diags = append(diags, Diagnostic{
						Code:     "class-constructor-reassigns-const",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"constructor of class %q reassigns const field %q (already initialised at declaration; ETSI 5.1.1.5)",
							className, name),
						Node: be,
						Span: syntax.SpanOf(be),
					})
				}
				if kinds.initializedTemplates[name] {
					diags = append(diags, Diagnostic{
						Code:     "class-constructor-reassigns-template",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"constructor of class %q reassigns template field %q (already initialised at declaration; ETSI 5.1.1.5)",
							className, name),
						Node: be,
						Span: syntax.SpanOf(be),
					})
				}
			}
		}
		if ce, ok := n.(*syntax.CallExpr); ok && ce != nil {
			if name, viaThis := memberCallName(ce); name != "" && kinds.methods[name] {
				if viaThis {
					diags = append(diags, Diagnostic{
						Code:     "class-constructor-calls-member",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"constructor of class %q invokes member function %q via `this`; member functions cannot be called from a constructor (ETSI 5.1.1.5)",
							className, name),
						Node: ce,
						Span: syntax.SpanOf(ce),
					})
				}
			}
		}
		// ETSI 5.1.1.5: a constructor body shall not use blocking
		// operations (timer timeout, port receive/trigger, procedure
		// getcall/getreply/catch, component done/killed).
		if sel, ok := n.(*syntax.SelectorExpr); ok && sel != nil {
			if op := identOf(sel.Sel); blockingClassOps[op] {
				diags = append(diags, Diagnostic{
					Code:     "class-constructor-blocking-op",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"constructor of class %q uses blocking operation %q; constructor bodies must not block (ETSI 5.1.1.5)",
						className, op),
					Node: sel,
					Span: syntax.SpanOf(sel),
				})
			}
		}
		return true
	})
	return diags
}

// blockingClassOps lists the operation names that block a TTCN-3
// behaviour and are therefore forbidden in a constructor body.
var blockingClassOps = map[string]bool{
	"timeout":  true,
	"receive":  true,
	"trigger":  true,
	"getcall":  true,
	"getreply": true,
	"catch":    true,
	"done":     true,
	"killed":   true,
}

// identOf returns the identifier text of e, or "".
func identOf(e syntax.Expr) string {
	if id, ok := e.(*syntax.Ident); ok && id != nil {
		return id.String()
	}
	return ""
}

// thisFieldName returns ("name", true) when e is `this.name`, and
// ("name", false) when e is a bare identifier `name`. Anything else
// returns ("", false).
func thisFieldName(e syntax.Expr) (string, bool) {
	if e == nil {
		return "", false
	}
	if sel, ok := e.(*syntax.SelectorExpr); ok && sel != nil {
		if id, ok := sel.X.(*syntax.Ident); ok && id != nil && id.String() == "this" {
			if selId, ok := sel.Sel.(*syntax.Ident); ok && selId != nil {
				return selId.String(), true
			}
		}
	}
	if id, ok := e.(*syntax.Ident); ok && id != nil {
		return id.String(), false
	}
	return "", false
}

// memberCallName returns the method name and a flag indicating
// whether the call goes through `this`. Bare-identifier method
// calls return (name, false); `this.m(...)` returns (m, true).
func memberCallName(ce *syntax.CallExpr) (string, bool) {
	if ce == nil || ce.Fun == nil {
		return "", false
	}
	if sel, ok := ce.Fun.(*syntax.SelectorExpr); ok && sel != nil {
		if id, ok := sel.X.(*syntax.Ident); ok && id != nil && id.String() == "this" {
			if selId, ok := sel.Sel.(*syntax.Ident); ok && selId != nil {
				return selId.String(), true
			}
		}
	}
	if id, ok := ce.Fun.(*syntax.Ident); ok && id != nil {
		return id.String(), false
	}
	return "", false
}

// collectMemberCalls walks expr and returns the names of every
// CallExpr whose target resolves to one of methods (either as a
// bare identifier or as `this.<name>`).
func collectMemberCalls(expr syntax.Expr, methods map[string]bool) []string {
	var out []string
	syntax.Inspect(expr, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok || ce == nil {
			return true
		}
		name, _ := memberCallName(ce)
		if name != "" && methods[name] {
			out = append(out, name)
		}
		return true
	})
	return out
}

// checkClassFieldInitRefs walks every class definition and
// validates each `var T name := <init>` / `const T name := <init>`
// / `template T name := <init>` field declaration:
//
//   - the initialiser must not reference the field itself
//   - the initialiser must not reference a sibling field whose
//     declaration carries no initialiser.
//
// We only diagnose references that are bare Idents matching a
// known sibling field name; anything else (function calls,
// `this.x`, complex expressions) falls through.
func checkClassFieldInitRefs(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ct, ok := n.(*syntax.ClassTypeDecl)
		if !ok || ct == nil {
			return true
		}
		fields := collectClassFieldInitState(ct)
		if len(fields) == 0 {
			return true
		}
		for _, member := range ct.Defs {
			if member == nil || member.Def == nil {
				continue
			}
			vd, ok := member.Def.(*syntax.ValueDecl)
			if !ok || vd == nil {
				continue
			}
			for _, dec := range vd.Decls {
				if dec == nil || dec.Name == nil || dec.Value == nil {
					continue
				}
				selfName := dec.Name.String()
				syntax.Inspect(dec.Value, func(in syntax.Node) bool {
					id, ok := in.(*syntax.Ident)
					if !ok || id == nil {
						return true
					}
					refName := id.String()
					if refName == selfName {
						diags = append(diags, Diagnostic{
							Code:     "class-field-self-init",
							Severity: SeverityError,
							Message: fmt.Sprintf(
								"initialiser of class field %q references itself (ETSI 5.1.1.5)",
								selfName),
							Node: id,
							Span: syntax.SpanOf(id),
						})
						return true
					}
					if hasInit, isField := fields[refName]; isField && !hasInit {
						diags = append(diags, Diagnostic{
							Code:     "class-field-uninit-ref",
							Severity: SeverityError,
							Message: fmt.Sprintf(
								"initialiser of class field %q references sibling field %q which has no initialiser (ETSI 5.1.1.5)",
								selfName, refName),
							Node: id,
							Span: syntax.SpanOf(id),
						})
					}
					return true
				})
			}
		}
		return true
	})
	return diags
}

// collectClassFieldInitState returns a map of field name to "has
// initialiser" for every var/const/template member of the class.
// Constructor params and method declarations are skipped.
func collectClassFieldInitState(ct *syntax.ClassTypeDecl) map[string]bool {
	out := map[string]bool{}
	for _, member := range ct.Defs {
		if member == nil || member.Def == nil {
			continue
		}
		vd, ok := member.Def.(*syntax.ValueDecl)
		if !ok || vd == nil {
			continue
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil {
				continue
			}
			out[dec.Name.String()] = dec.Value != nil
		}
	}
	return out
}

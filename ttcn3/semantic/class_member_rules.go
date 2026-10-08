// class_member_rules.go enforces the static well-formedness rules on
// class definitions from ETSI ES 201 873-1 clause 5.1.1 that become
// observable once the interpreter actually constructs objects and
// dispatches methods (without these checks the negative tests would
// execute to a pass verdict instead of being rejected):
//
//   - 5.1.1.0: class member names are unique within a class body
//     (no field/method/field-vs-method name clash).
//   - 5.1.1.0: a class method shall not carry a `runs on`, `mtc` or
//     `system` clause (those belong on the class, not its methods).
//   - 5.1.1.0: the `runs on` type of a class shall be runs-on
//     compatible with the creating behaviour, and a subclass's
//     `runs on` / `mtc` / `system` types shall be compatible with
//     the superclass's.
//   - 5.1.1.7/5.1.1.8: an overriding method shall not be more
//     visible-restrictive than the method it overrides, and shall
//     keep the same return type and the same formal parameters in
//     the same order.
//   - 5.1.1.9: class fields are private or protected only (never
//     public), and a private field is only accessible from inside
//     its own class.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// classMeta is the per-class information the member rules need.
type classMeta struct {
	decl     *syntax.ClassTypeDecl
	parent   string
	runsOn   string
	mtc      string
	system   string
	external bool                        // `type external class`
	methods  map[string]*syntax.FuncDecl // own methods, by name
	methVis  map[string]string           // own method visibility
}

func (a *Analyzer) checkClassMemberRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	classes := collectClassMeta(mod)
	if len(classes) == 0 {
		return nil
	}
	ext := collectComponentExtensionGraph(mod)

	var diags []Diagnostic
	for _, cm := range classes {
		diags = append(diags, classUniqueMemberDiags(cm)...)
		diags = append(diags, classMemberClauseDiags(cm)...)
		diags = append(diags, classFieldVisibilityDiags(cm)...)
		diags = append(diags, classOverrideDiags(cm, classes)...)
		diags = append(diags, classRunsOnInheritanceDiags(cm, classes, ext)...)
		diags = append(diags, classExternalExtendsDiags(cm, classes)...)
	}
	diags = append(diags, classCreateRunsOnDiags(mod, classes, ext)...)
	diags = append(diags, classPrivateFieldAccessDiags(mod, classes)...)
	diags = append(diags, classAnytypeFieldDiags(mod, classes)...)
	return diags
}

// classAnytypeFieldDiags flags storing a class instance into an
// `anytype` value: `v.<ClassName> := ...` where v is an anytype
// variable and ClassName is a class type. A class type shall not be the
// contained value of an anytype (ETSI 5.1.1.0).
func classAnytypeFieldDiags(mod *syntax.Module, classes map[string]classMeta) []Diagnostic {
	if len(classes) == 0 {
		return nil
	}
	anytypeVars := map[string]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil || vd.Type == nil {
			return true
		}
		if identName(vd.Type) != "anytype" {
			return true
		}
		for _, dec := range vd.Decls {
			if dec != nil && dec.Name != nil {
				anytypeVars[dec.Name.String()] = true
			}
		}
		return true
	})
	if len(anytypeVars) == 0 {
		return nil
	}
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		be, ok := n.(*syntax.BinaryExpr)
		if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
			return true
		}
		sel, ok := be.X.(*syntax.SelectorExpr)
		if !ok || sel == nil {
			return true
		}
		base, ok := sel.X.(*syntax.Ident)
		if !ok || base == nil || !anytypeVars[base.String()] {
			return true
		}
		field := identOf(sel.Sel)
		if _, isClass := classes[field]; !isClass {
			return true
		}
		diags = append(diags, Diagnostic{
			Code:     "anytype-contains-class",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"class type %q cannot be the contained value of an anytype (ETSI 5.1.1.0)",
				field),
			Node: be,
			Span: syntax.SpanOf(be),
		})
		return true
	})
	return diags
}

// collectClassMeta builds the class registry for the current module.
func collectClassMeta(mod *syntax.Module) map[string]classMeta {
	out := map[string]classMeta{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		ct, ok := d.Def.(*syntax.ClassTypeDecl)
		if !ok || ct == nil || ct.Name == nil {
			continue
		}
		cm := classMeta{
			decl:     ct,
			external: ct.ExternalTok != nil,
			methods:  map[string]*syntax.FuncDecl{},
			methVis:  map[string]string{},
		}
		if len(ct.Extends) > 0 {
			cm.parent = identName(ct.Extends[0])
		}
		if ct.RunsOn != nil {
			cm.runsOn = identName(ct.RunsOn.Comp)
		}
		if ct.Mtc != nil {
			cm.mtc = identName(ct.Mtc.Comp)
		}
		if ct.System != nil {
			cm.system = identName(ct.System.Comp)
		}
		for _, member := range ct.Defs {
			if member == nil || member.Def == nil {
				continue
			}
			if fn, ok := member.Def.(*syntax.FuncDecl); ok && fn != nil && fn.Name != nil {
				cm.methods[fn.Name.String()] = fn
				cm.methVis[fn.Name.String()] = memberVisibility(member, "protected")
			}
		}
		out[ct.Name.String()] = cm
	}
	return out
}

// memberVisibility returns the visibility keyword of a class member,
// defaulting to def when none is written.
func memberVisibility(member *syntax.ModuleDef, def string) string {
	if member != nil && member.Visibility != nil {
		switch member.Visibility.Kind() {
		case syntax.PUBLIC:
			return "public"
		case syntax.PRIVATE:
			return "private"
		}
	}
	return def
}

func visibilityRank(v string) int {
	switch v {
	case "public":
		return 2
	case "protected":
		return 1
	case "private":
		return 0
	}
	return 1
}

// classUniqueMemberDiags flags a class member name reused by another
// member of the same class (ETSI 5.1.1.0). Constructors are excluded
// (their name is the keyword `create`).
func classUniqueMemberDiags(cm classMeta) []Diagnostic {
	var diags []Diagnostic
	seen := map[string]bool{}
	flag := func(name string, n syntax.Node) {
		if name == "" {
			return
		}
		if seen[name] {
			diags = append(diags, Diagnostic{
				Code:     "class-duplicate-member",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"class member %q is declared more than once; class member names must be unique (ETSI 5.1.1.0)",
					name),
				Node: n,
				Span: syntax.SpanOf(n),
			})
			return
		}
		seen[name] = true
	}
	for _, member := range cm.decl.Defs {
		if member == nil || member.Def == nil {
			continue
		}
		switch d := member.Def.(type) {
		case *syntax.ValueDecl:
			for _, dec := range d.Decls {
				if dec != nil && dec.Name != nil {
					flag(dec.Name.String(), dec)
				}
			}
		case *syntax.TemplateDecl:
			if d.Name != nil {
				flag(d.Name.String(), d)
			}
		case *syntax.FuncDecl:
			if d.Name != nil {
				flag(d.Name.String(), d)
			}
		}
	}
	return diags
}

// classMemberClauseDiags flags a class method that carries a `runs
// on`, `mtc` or `system` clause (ETSI 5.1.1.0).
func classMemberClauseDiags(cm classMeta) []Diagnostic {
	var diags []Diagnostic
	for _, member := range cm.decl.Defs {
		if member == nil {
			continue
		}
		fn, ok := member.Def.(*syntax.FuncDecl)
		if !ok || fn == nil {
			continue
		}
		clause := ""
		switch {
		case fn.RunsOn != nil:
			clause = "runs on"
		case fn.Mtc != nil:
			clause = "mtc"
		case fn.System != nil:
			clause = "system"
		}
		if clause == "" {
			continue
		}
		name := ""
		if fn.Name != nil {
			name = fn.Name.String()
		}
		diags = append(diags, Diagnostic{
			Code:     "class-method-runs-on-clause",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"class method %q must not declare a %q clause (ETSI 5.1.1.0)",
				name, clause),
			Node: fn,
			Span: syntax.SpanOf(fn),
		})
	}
	return diags
}

// classFieldVisibilityDiags flags a class field declared `public`
// (fields may be private or protected only, ETSI 5.1.1.9).
func classFieldVisibilityDiags(cm classMeta) []Diagnostic {
	var diags []Diagnostic
	for _, member := range cm.decl.Defs {
		if member == nil || member.Def == nil || member.Visibility == nil {
			continue
		}
		if member.Visibility.Kind() != syntax.PUBLIC {
			continue
		}
		var name string
		switch d := member.Def.(type) {
		case *syntax.ValueDecl:
			if len(d.Decls) > 0 && d.Decls[0].Name != nil {
				name = d.Decls[0].Name.String()
			}
		case *syntax.TemplateDecl:
			if d.Name != nil {
				name = d.Name.String()
			}
		default:
			continue
		}
		diags = append(diags, Diagnostic{
			Code:     "class-public-field",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"class field %q cannot be public; fields may be private or protected only (ETSI 5.1.1.9)",
				name),
			Node: member.Def,
			Span: syntax.SpanOf(member.Def),
		})
	}
	return diags
}

// classOverrideDiags checks each method that overrides an inherited
// method of the same name: visibility must not narrow, and the
// signature (return type + ordered formal parameters) must match
// (ETSI 5.1.1.7 / 5.1.1.8).
func classOverrideDiags(cm classMeta, classes map[string]classMeta) []Diagnostic {
	var diags []Diagnostic
	for name, fn := range cm.methods {
		base, baseCm, found := lookupInheritedMethod(cm, name, classes)
		if !found {
			continue
		}
		subVis := cm.methVis[name]
		baseVis := baseCm.methVis[name]
		if visibilityRank(subVis) < visibilityRank(baseVis) {
			diags = append(diags, Diagnostic{
				Code:     "class-override-visibility",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"overriding method %q is %s but the overridden method is %s; an override must not be more restrictive (ETSI 5.1.1.8)",
					name, subVis, baseVis),
				Node: fn,
				Span: syntax.SpanOf(fn),
			})
		}
		if r1, r2 := returnTypeName(fn), returnTypeName(base); r1 != r2 {
			diags = append(diags, Diagnostic{
				Code:     "class-override-return-type",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"overriding method %q returns %q but the overridden method returns %q; the return type must be the same (ETSI 5.1.1.7)",
					name, r1, r2),
				Node: fn,
				Span: syntax.SpanOf(fn),
			})
		}
		if !sameFormalPars(fn.Params, base.Params) {
			diags = append(diags, Diagnostic{
				Code:     "class-override-signature",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"overriding method %q has different formal parameters than the overridden method; they must match in order, name, type and direction (ETSI 5.1.1.8)",
					name),
				Node: fn,
				Span: syntax.SpanOf(fn),
			})
		}
	}
	return diags
}

// lookupInheritedMethod walks cm's ancestors and returns the first
// non-private method named name together with the class that declares
// it. Private methods are not inherited and so do not establish an
// override relationship (ETSI 5.1.1.10: a private method may reappear
// with a different signature in another class of the hierarchy).
func lookupInheritedMethod(cm classMeta, name string, classes map[string]classMeta) (*syntax.FuncDecl, classMeta, bool) {
	seen := map[string]bool{}
	for p := cm.parent; p != "" && !seen[p]; {
		seen[p] = true
		pc, ok := classes[p]
		if !ok {
			break
		}
		if fn, ok := pc.methods[name]; ok && pc.methVis[name] != "private" {
			return fn, pc, true
		}
		p = pc.parent
	}
	return nil, classMeta{}, false
}

func returnTypeName(fn *syntax.FuncDecl) string {
	if fn == nil || fn.Return == nil {
		return ""
	}
	return identName(fn.Return.Type)
}

// sameFormalPars reports whether two formal-parameter lists match in
// arity, and per position in direction, type and name.
func sameFormalPars(a, b *syntax.FormalPars) bool {
	la, lb := 0, 0
	if a != nil {
		la = len(a.List)
	}
	if b != nil {
		lb = len(b.List)
	}
	if la != lb {
		return false
	}
	for i := 0; i < la; i++ {
		if formalParSig(a.List[i]) != formalParSig(b.List[i]) {
			return false
		}
	}
	return true
}

func formalParSig(p *syntax.FormalPar) string {
	if p == nil {
		return ""
	}
	dir := "in"
	if p.Direction != nil {
		dir = p.Direction.String()
	}
	name := ""
	if p.Name != nil {
		name = p.Name.String()
	}
	return dir + " " + identName(p.Type) + " " + name
}

// classRunsOnInheritanceDiags flags a subclass whose `runs on` /
// `mtc` / `system` component type is not compatible with the
// corresponding type of its superclass (ETSI 5.1.1.0).
func classRunsOnInheritanceDiags(cm classMeta, classes map[string]classMeta, ext map[string]map[string]bool) []Diagnostic {
	if cm.parent == "" {
		return nil
	}
	pc, ok := classes[cm.parent]
	if !ok {
		return nil
	}
	var diags []Diagnostic
	check := func(kind, sub, sup string) {
		if sub == "" || sup == "" {
			return
		}
		if !componentCompatible(sub, sup, ext) {
			diags = append(diags, Diagnostic{
				Code:     "class-inherit-runs-on-incompatible",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"subclass %s type %q is not compatible with superclass %s type %q (ETSI 5.1.1.0)",
					kind, sub, kind, sup),
				Node: cm.decl,
				Span: syntax.SpanOf(cm.decl),
			})
		}
	}
	check("runs on", cm.runsOn, pc.runsOn)
	check("mtc", cm.mtc, pc.mtc)
	check("system", cm.system, pc.system)
	return diags
}

// classExternalExtendsDiags flags an internal (non-external) class that
// extends an external class. An external class may extend a non-external
// one, but not the other way round (ETSI 5.1.1.3, restriction d).
func classExternalExtendsDiags(cm classMeta, classes map[string]classMeta) []Diagnostic {
	if cm.external || cm.parent == "" {
		return nil
	}
	pc, ok := classes[cm.parent]
	if !ok || !pc.external {
		return nil
	}
	return []Diagnostic{{
		Code:     "class-internal-extends-external",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"internal class %q shall not extend external class %q (ETSI 5.1.1.3)",
			cm.decl.Name.String(), cm.parent),
		Node: cm.decl,
		Span: syntax.SpanOf(cm.decl),
	}}
}

// classCreateRunsOnDiags flags `C.create(...)` invoked from a
// behaviour whose `runs on` component type is not compatible with the
// class's `runs on` type (ETSI 5.1.1.0).
func classCreateRunsOnDiags(mod *syntax.Module, classes map[string]classMeta, ext map[string]map[string]bool) []Diagnostic {
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil || fn.RunsOn == nil {
			continue
		}
		ctx := identName(fn.RunsOn.Comp)
		if ctx == "" {
			continue
		}
		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			ce, ok := n.(*syntax.CallExpr)
			if !ok || ce == nil {
				return true
			}
			cls := createCallClassName(ce)
			if cls == "" {
				return true
			}
			cm, ok := classes[cls]
			if !ok || cm.runsOn == "" {
				return true
			}
			if !componentCompatible(ctx, cm.runsOn, ext) {
				diags = append(diags, Diagnostic{
					Code:     "class-create-runs-on-incompatible",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"class %q runs on %q which is not compatible with the creating behaviour's runs-on type %q (ETSI 5.1.1.0)",
						cls, cm.runsOn, ctx),
					Node: ce,
					Span: syntax.SpanOf(ce),
				})
			}
			return true
		})
	}
	return diags
}

// createCallClassName returns the class name of a `Class.create(...)`
// call, or "".
func createCallClassName(ce *syntax.CallExpr) string {
	if ce == nil {
		return ""
	}
	sel, ok := ce.Fun.(*syntax.SelectorExpr)
	if !ok || sel == nil {
		return ""
	}
	id, ok := sel.Sel.(*syntax.Ident)
	if !ok || id == nil || id.String() != "create" {
		return ""
	}
	return identName(sel.X)
}

// classPrivateFieldAccessDiags flags `obj.f` reads/writes of a
// private class field from outside the declaring class - i.e. from a
// module-level function/testcase/altstep body (ETSI 5.1.1.9). Only
// explicitly `private` fields are diagnosed; method-call selectors
// (`obj.m(...)`) are not fields and are ignored.
func classPrivateFieldAccessDiags(mod *syntax.Module, classes map[string]classMeta) []Diagnostic {
	priv := map[string]map[string]bool{}
	for name, cm := range classes {
		for _, member := range cm.decl.Defs {
			if member == nil || member.Visibility == nil ||
				member.Visibility.Kind() != syntax.PRIVATE {
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
				if priv[name] == nil {
					priv[name] = map[string]bool{}
				}
				priv[name][dec.Name.String()] = true
			}
		}
	}
	if len(priv) == 0 {
		return nil
	}

	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			continue
		}
		varTypes := collectFuncBodyVarTypes(fn.Body)
		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			sel, ok := n.(*syntax.SelectorExpr)
			if !ok || sel == nil {
				return true
			}
			base := identName(sel.X)
			field := identName(sel.Sel)
			if base == "" || field == "" {
				return true
			}
			cls := varTypes[base]
			if cls == "" {
				return true
			}
			if fields, ok := priv[cls]; ok && fields[field] {
				diags = append(diags, Diagnostic{
					Code:     "class-private-field-access",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"private field %q of class %q cannot be accessed from outside the class (ETSI 5.1.1.9)",
						field, cls),
					Node: sel,
					Span: syntax.SpanOf(sel),
				})
			}
			return true
		})
	}
	return diags
}

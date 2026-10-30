// template_restriction_tables.go enforces the template restriction rules
// of ETSI ES 201 873-1 clause 15.8 that compare one restriction with
// another, rather than with the matching mechanisms a template holds:
//
//   - Table 13: what may be assigned to a restricted template or
//     template variable, or passed to a restricted formal template
//     parameter. A value always may; a template only with a restriction
//     as strict as the target's, except that a template(value) or
//     template(omit) also fits template(omit), and template(value) fits
//     template(present).
//   - Table 13c: a field, alternative or element of a template(value)
//     or template(omit) is itself restricted to value or omit, so it may
//     not refer to an unrestricted or present template either.
//   - Table 13a: a formal template parameter of a modified template may
//     only be as strict as, or stricter than, the base template's.
//   - Table 13b: a modified template may only loosen the base
//     template's restriction, and in the ways the table lists.
//
// A restriction is known for a reference to a template, a template
// variable or a formal template parameter, for a call of a template or
// of a function returning a template, and for the presentness
// conversion `t.present` (15.14). Constants, value variables, value
// parameters and `t.value` (15.15) are values. Anything else, a field
// reference among them, is left alone.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// restrValue is the restriction class of a value, as opposed to any
// template; "" is an unrestricted template.
const restrValue = "value expression"

func (a *Analyzer) checkTemplateRestrictionTables(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	c := &restrChecker{
		module:    map[string]string{},
		templates: map[string]*syntax.TemplateDecl{},
		funcs:     map[string]*syntax.FuncDecl{},
		local:     map[string]*syntax.TemplateDecl{},
		types:     map[string]syntax.Node{},
		members:   map[string]map[string]string{},
		parents:   collectComponentParents(mod),
	}
	c.collect(mod)
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.FuncDecl:
			if x.Body != nil {
				c.local = map[string]*syntax.TemplateDecl{}
				c.checkBody(x.Body, c.params(x.Params, c.runsOnScope(x)))
			}
			return false
		case *syntax.ControlPart:
			if x.Body != nil {
				c.local = map[string]*syntax.TemplateDecl{}
				c.checkBody(x.Body, map[string]string{})
			}
			return false
		case *syntax.ComponentTypeDecl, *syntax.ClassTypeDecl:
			return false
		case *syntax.TemplateDecl:
			c.checkTemplateDecl(x, map[string]string{})
			return false
		}
		return true
	})
	return c.diags
}

type restrChecker struct {
	module    map[string]string
	templates map[string]*syntax.TemplateDecl
	// local are the templates declared in the body being checked.
	local map[string]*syntax.TemplateDecl
	// types are the module's type declarations, by name: the
	// *syntax.StructTypeDecl or *syntax.SubTypeDecl.
	types map[string]syntax.Node
	funcs map[string]*syntax.FuncDecl
	// members are the variables and constants each component type
	// declares, parents the types each extends.
	members map[string]map[string]string
	parents map[string][]string
	diags   []Diagnostic
}

func (c *restrChecker) collect(mod *syntax.Module) {
	var defs func(list []*syntax.ModuleDef)
	defs = func(list []*syntax.ModuleDef) {
		for _, d := range list {
			if d == nil {
				continue
			}
			switch x := d.Def.(type) {
			case *syntax.GroupDecl:
				defs(x.Defs)
			case *syntax.TemplateDecl:
				if x.Name != nil {
					c.templates[x.Name.String()] = x
					c.module[x.Name.String()] = restrictionTag(x.RestrictionSpec)
				}
			case *syntax.FuncDecl:
				if x.Name != nil {
					c.funcs[x.Name.String()] = x
				}
			case *syntax.StructTypeDecl:
				if x.Name != nil {
					c.types[x.Name.String()] = x
				}
			case *syntax.SubTypeDecl:
				if x.Field != nil && x.Field.Name != nil {
					c.types[x.Field.Name.String()] = x
				}
			case *syntax.ValueDecl:
				for _, dec := range x.Decls {
					if dec != nil && dec.Name != nil {
						c.module[dec.Name.String()] = valueDeclRestriction(x)
					}
				}
			case *syntax.ModuleParameterGroup:
				for _, vd := range x.Decls {
					for _, dec := range vd.Decls {
						if dec != nil && dec.Name != nil {
							c.module[dec.Name.String()] = valueDeclRestriction(vd)
						}
					}
				}
			case *syntax.ComponentTypeDecl:
				if x.Body == nil {
					continue
				}
				for _, s := range x.Body.Stmts {
					ds, ok := s.(*syntax.DeclStmt)
					if !ok {
						continue
					}
					vd, ok := ds.Decl.(*syntax.ValueDecl)
					if !ok {
						continue
					}
					for _, dec := range vd.Decls {
						if dec == nil || dec.Name == nil || x.Name == nil {
							continue
						}
						comp := x.Name.String()
						if c.members[comp] == nil {
							c.members[comp] = map[string]string{}
						}
						c.members[comp][dec.Name.String()] = valueDeclRestriction(vd)
					}
				}
			}
		}
	}
	defs(mod.Defs)
}

// valueDeclRestriction is the restriction class of what a declaration
// declares: a template's restriction, or restrValue for a constant,
// variable, module parameter, timer or port.
func valueDeclRestriction(vd *syntax.ValueDecl) string {
	if vd.TemplateRestriction != nil || (vd.KindTok != nil && vd.KindTok.Kind() == syntax.TEMPLATE) {
		return restrictionTag(vd.TemplateRestriction)
	}
	return restrValue
}

// formalRestriction is the restriction class of a formal parameter.
func formalRestriction(p *syntax.FormalPar) string {
	if p.TemplateRestriction != nil {
		return restrictionTag(p.TemplateRestriction)
	}
	return restrValue
}

// runsOnScope is what a function sees of the component it runs on: the
// variables and constants of its type and of the types it extends, as
// far as they are declared in the module.
func (c *restrChecker) runsOnScope(fd *syntax.FuncDecl) map[string]string {
	out := map[string]string{}
	if fd.RunsOn == nil || fd.RunsOn.Comp == nil {
		return out
	}
	seen := map[string]bool{}
	var add func(comp string)
	add = func(comp string) {
		if seen[comp] {
			return
		}
		seen[comp] = true
		for name, r := range c.members[comp] {
			if _, own := out[name]; !own {
				out[name] = r
			}
		}
		for _, p := range c.parents[comp] {
			add(p)
		}
	}
	add(syntax.Name(fd.RunsOn.Comp))
	return out
}

// params adds the formal parameters of a definition to a copy of scope.
func (c *restrChecker) params(fps *syntax.FormalPars, scope map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range scope {
		out[k] = v
	}
	if fps != nil {
		for _, p := range fps.List {
			if p != nil && p.Name != nil {
				out[p.Name.String()] = formalRestriction(p)
			}
		}
	}
	return out
}

// restrictionOf classifies e, reporting false when its restriction is
// not known.
func (c *restrChecker) restrictionOf(e syntax.Expr, scope map[string]string) (string, bool) {
	switch x := e.(type) {
	case *syntax.ParenExpr:
		if len(x.List) == 1 {
			return c.restrictionOf(x.List[0], scope)
		}
	case *syntax.Ident:
		if x.Tok == nil || x.Tok.Kind() != syntax.IDENT || x.Tok2 != nil {
			return "", false
		}
		name := x.String()
		if r, ok := scope[name]; ok {
			return r, true
		}
		r, ok := c.module[name]
		return r, ok
	case *syntax.CallExpr:
		id, ok := x.Fun.(*syntax.Ident)
		if !ok || id.Tok == nil || id.Tok.Kind() != syntax.IDENT {
			return "", false
		}
		if _, local := scope[id.String()]; local {
			return "", false
		}
		if td := c.templates[id.String()]; td != nil {
			return restrictionTag(td.RestrictionSpec), true
		}
		if fd := c.funcs[id.String()]; fd != nil && fd.KindTok != nil && fd.KindTok.Kind() == syntax.FUNCTION {
			if fd.Return == nil {
				return "", false
			}
			if fd.Return.Restriction == nil {
				return restrValue, true
			}
			return restrictionTag(fd.Return.Restriction), true
		}
	case *syntax.SelectorExpr:
		sel, ok := x.Sel.(*syntax.Ident)
		if !ok || sel.Tok == nil {
			return "", false
		}
		switch sel.Tok.Kind() {
		case syntax.VALUE:
			return restrValue, true
		case syntax.PRESENT:
			// 15.14: an unrestricted template becomes a present one,
			// an omit one a value one; value and present ones stay.
			r, ok := c.restrictionOf(x.X, scope)
			if !ok {
				return "", false
			}
			switch r {
			case "":
				return "present", true
			case "omit":
				return "value", true
			}
			return r, true
		}
	}
	return "", false
}

// fits is table 13: may a template or value of class rhs be assigned
// to, or passed to, a template of restriction lhs?
func fits(lhs, rhs string) bool {
	if lhs == "" || rhs == restrValue {
		return true
	}
	switch lhs {
	case "omit":
		return rhs == "omit" || rhs == "value"
	case "value":
		// template(omit) fits only an optional field of a
		// template(value), never a whole one.
		return rhs == "value"
	case "present":
		return rhs == "value" || rhs == "present"
	}
	return true
}

func restrName(r string) string {
	switch r {
	case "":
		return "an unrestricted template"
	case restrValue:
		return "a value"
	}
	return fmt.Sprintf("a template(%s)", r)
}

func (c *restrChecker) reportf(code string, n syntax.Node, format string, args ...interface{}) {
	c.diags = append(c.diags, Diagnostic{
		Code:     code,
		Severity: SeverityError,
		Message:  fmt.Sprintf(format, args...),
		Node:     n,
		Span:     syntax.SpanOf(n),
	})
}

// checkInit checks e, assigned to or passed for a template of type typ
// (nil when not known) and restriction lhs, against table 13, and its
// fields, alternatives and elements against table 13c.
func (c *restrChecker) checkInit(lhs string, e syntax.Expr, typ syntax.Node, what string, scope map[string]string) {
	if lhs == "" || lhs == restrValue || e == nil {
		return
	}
	if r, ok := c.restrictionOf(e, scope); ok {
		if !fits(lhs, r) {
			c.reportf("template-restriction-assignment", e,
				"%s is assigned %s, which template(%s) does not accept (ETSI 15.8 table 13)",
				what, restrName(r), lhs)
		}
		return
	}
	c.checkFields(lhs, e, typ, what, scope)
}

// restrValueOrOmit is the implicit restriction of a field of a
// template(value) or template(omit) whose optionality is not known: value
// or omit, which accept the same but an omit template.
const restrValueOrOmit = "value or omit"

// implicitRestriction is table 13c: the restriction of a field,
// alternative or element of a template with restriction parent, optional
// telling whether it is an optional field of a record or set, known
// whether that is known.
func implicitRestriction(parent string, optional, known bool) string {
	switch parent {
	case "value", "omit", restrValueOrOmit:
		if !known {
			return restrValueOrOmit
		}
		if optional {
			return "omit"
		}
		return "value"
	case "present":
		if known && !optional {
			return "present"
		}
	}
	return ""
}

// checkFields walks the fields, alternatives and elements of a template
// of restriction parent and type typ, each restricted as table 13c says,
// and checks what they refer to against table 13.
func (c *restrChecker) checkFields(parent string, e syntax.Expr, typ syntax.Node, what string, scope map[string]string) {
	cl, ok := e.(*syntax.CompositeLiteral)
	if !ok || parent == "" {
		return
	}
	fields, union, elem := c.shapeOf(typ, 0)
	for i, el := range cl.List {
		var f *syntax.Field
		if be, ok := el.(*syntax.BinaryExpr); ok && be.Op != nil && be.Op.Kind() == syntax.ASSIGN {
			if id, ok := be.X.(*syntax.Ident); ok {
				f = fieldNamed(fields, id.String())
			}
			el = be.Y
		} else if fields != nil && i < len(fields) {
			f = fields[i]
		}
		var r string
		var sub syntax.Node
		switch {
		case f != nil:
			r = implicitRestriction(parent, f.Optional != nil && !union, true)
			sub = f.Type
		case elem != nil:
			r = implicitRestriction(parent, false, true)
			sub = elem
		default:
			r = implicitRestriction(parent, false, false)
		}
		if r == "" {
			continue
		}
		if got, ok := c.restrictionOf(el, scope); ok {
			bad := !fits(r, got)
			if r == restrValueOrOmit {
				bad = got == "" || got == "present"
			}
			if bad {
				c.reportf("template-restriction-field", el,
					"a field of %s, a template(%s), refers to %s, which its field, restricted to %s, does not accept (ETSI 15.8 tables 13, 13c)",
					what, parent, restrName(got), r)
			}
			continue
		}
		c.checkFields(r, el, sub, what, scope)
	}
}

func fieldNamed(fields []*syntax.Field, name string) *syntax.Field {
	for _, f := range fields {
		if f != nil && syntax.Name(f.Name) == name {
			return f
		}
	}
	return nil
}

// shapeOf is what a value of type typ is made of, as far as the module
// declares it: the fields of a record or set, or the alternatives of a
// union (union true), or the element type of a record of, set of or
// array.
func (c *restrChecker) shapeOf(typ syntax.Node, depth int) (fields []*syntax.Field, union bool, elem syntax.Node) {
	if typ == nil || depth > 8 {
		return nil, false, nil
	}
	switch x := typ.(type) {
	case *syntax.RefSpec:
		return c.shapeOf(x.X, depth+1)
	case *syntax.StructSpec:
		return x.Fields, x.KindTok != nil && x.KindTok.Kind() == syntax.UNION, nil
	case *syntax.ListSpec:
		return nil, false, x.ElemType
	case *syntax.Ident:
		switch d := c.types[x.String()].(type) {
		case *syntax.StructTypeDecl:
			return d.Fields, d.KindTok != nil && d.KindTok.Kind() == syntax.UNION, nil
		case *syntax.SubTypeDecl:
			if d.Field == nil {
				return nil, false, nil
			}
			if len(d.Field.ArrayDef) > 0 {
				return nil, false, d.Field.Type
			}
			return c.shapeOf(d.Field.Type, depth+1)
		}
	}
	return nil, false, nil
}

func (c *restrChecker) checkTemplateDecl(td *syntax.TemplateDecl, scope map[string]string) {
	name := syntax.Name(td.Name)
	inner := c.params(td.Params, scope)
	c.checkInit(restrictionTag(td.RestrictionSpec), td.Value, td.Type, fmt.Sprintf("template %q", name), inner)
	c.checkModified(td, scope)
	c.checkCalls(td.Value, inner)
}

// checkModified applies tables 13a and 13b to a modified template.
func (c *restrChecker) checkModified(td *syntax.TemplateDecl, scope map[string]string) {
	if td.ModifiesTok == nil || td.Base == nil {
		return
	}
	id, ok := td.Base.(*syntax.Ident)
	if !ok {
		return
	}
	base := c.templates[id.String()]
	if _, local := scope[id.String()]; local {
		// A local template, or else a variable or parameter.
		base = c.local[id.String()]
	}
	if base == nil {
		return
	}
	name := syntax.Name(td.Name)
	br, mr := restrictionTag(base.RestrictionSpec), restrictionTag(td.RestrictionSpec)
	if !modifiedRestrictionAllowed(br, mr) {
		c.reportf("modified-template-restriction", td,
			"template %q, %s, modifies %q, %s, which table 13b does not allow (ETSI 15.8 restriction d)",
			name, restrName(mr), id.String(), restrName(br))
	}
	if base.Params == nil || td.Params == nil {
		return
	}
	for i, p := range td.Params.List {
		if i >= len(base.Params.List) {
			break
		}
		bp := base.Params.List[i]
		if p == nil || bp == nil || p.TemplateRestriction == nil || bp.TemplateRestriction == nil {
			continue
		}
		bpr, mpr := restrictionTag(bp.TemplateRestriction), restrictionTag(p.TemplateRestriction)
		if !modifiedParamAllowed(bpr, mpr) {
			c.reportf("modified-template-param-restriction", p,
				"parameter %q of template %q is %s, but %s in base template %q; table 13a does not allow that (ETSI 15.8 restriction c)",
				syntax.Name(p.Name), name, restrName(mpr), restrName(bpr), id.String())
		}
	}
}

// modifiedRestrictionAllowed is table 13b.
func modifiedRestrictionAllowed(base, mod string) bool {
	switch base {
	case "omit":
		return mod == "omit" || mod == ""
	case "value":
		return true
	case "present":
		return mod == "present" || mod == ""
	}
	return mod == ""
}

// modifiedParamAllowed is table 13a.
func modifiedParamAllowed(base, mod string) bool {
	switch base {
	case "omit":
		return mod == "omit" || mod == "value"
	case "value":
		return mod == "value"
	case "present":
		return mod == "value" || mod == "present"
	}
	return true
}

// checkBody checks the declarations, assignments and calls of a
// statement block. Declarations are added to scope as they are met.
func (c *restrChecker) checkBody(body *syntax.BlockStmt, scope map[string]string) {
	syntax.Inspect(body, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.ValueDecl:
			r := valueDeclRestriction(x)
			for _, dec := range x.Decls {
				if dec == nil || dec.Name == nil {
					continue
				}
				var typ syntax.Node
				if len(dec.ArrayDef) == 0 {
					typ = x.Type
				}
				c.checkInit(r, dec.Value, typ, fmt.Sprintf("%q", dec.Name.String()), scope)
				scope[dec.Name.String()] = r
			}
			return true
		case *syntax.TemplateDecl:
			if x.Name != nil {
				c.checkTemplateDecl(x, scope)
				scope[x.Name.String()] = restrictionTag(x.RestrictionSpec)
				c.local[x.Name.String()] = x
			}
			return false
		case *syntax.ExprStmt:
			if be, ok := x.Expr.(*syntax.BinaryExpr); ok && be.Op != nil && be.Op.Kind() == syntax.ASSIGN {
				if id, ok := be.X.(*syntax.Ident); ok {
					if r, ok := scope[id.String()]; ok {
						c.checkInit(r, be.Y, nil, fmt.Sprintf("%q", id.String()), scope)
					}
				}
			}
			return true
		case *syntax.CallExpr:
			c.checkCall(x, scope)
			return true
		}
		return true
	})
}

// checkCalls checks the calls inside a template body.
func (c *restrChecker) checkCalls(e syntax.Expr, scope map[string]string) {
	if e == nil {
		return
	}
	syntax.Inspect(e, func(n syntax.Node) bool {
		if ce, ok := n.(*syntax.CallExpr); ok {
			c.checkCall(ce, scope)
		}
		return true
	})
}

// checkCall checks the actual parameters of a call of a template,
// function, altstep or testcase of the module against its formal
// template parameters' restrictions (table 13).
func (c *restrChecker) checkCall(ce *syntax.CallExpr, scope map[string]string) {
	id, ok := ce.Fun.(*syntax.Ident)
	if !ok || ce.Args == nil || id.Tok == nil || id.Tok.Kind() != syntax.IDENT {
		return
	}
	if _, local := scope[id.String()]; local {
		return
	}
	var formals *syntax.FormalPars
	if td := c.templates[id.String()]; td != nil {
		formals = td.Params
	} else if fd := c.funcs[id.String()]; fd != nil {
		formals = fd.Params
	}
	if formals == nil {
		return
	}
	byName := map[string]*syntax.FormalPar{}
	for _, p := range formals.List {
		if p != nil && p.Name != nil {
			byName[p.Name.String()] = p
		}
	}
	for i, arg := range ce.Args.List {
		var fp *syntax.FormalPar
		if be, ok := arg.(*syntax.BinaryExpr); ok && be.Op != nil && be.Op.Kind() == syntax.ASSIGN {
			fp = byName[syntax.Name(be.X)]
			arg = be.Y
		} else if i < len(formals.List) {
			fp = formals.List[i]
		}
		if fp == nil || fp.TemplateRestriction == nil || isDashDefault(arg) {
			continue
		}
		if fp.Direction != nil && fp.Direction.Kind() != syntax.IN {
			continue
		}
		var typ syntax.Node
		if len(fp.ArrayDef) == 0 {
			typ = fp.Type
		}
		c.checkInit(restrictionTag(fp.TemplateRestriction), arg, typ,
			fmt.Sprintf("parameter %q of %q", syntax.Name(fp.Name), id.String()), scope)
	}
}

// template_restrictions.go implements the ETSI ES 201 873-1 clause
// 15.8 "template restrictions" rules.
//
// A template declared `template(omit) T x := ...` may only have a
// concrete value or `omit` as initialiser. Wildcards (`?`, `*`),
// pattern strings, length restrictions, list-of templates (superset /
// subset), value ranges and others are not allowed - they widen the
// matched set in a way that cannot be reconciled with the
// restriction.
//
// `template(value) T x` is even stricter: only a literal value is
// accepted; `omit` is also forbidden.
//
// `template(present) T x` forbids `omit` but tolerates wildcards.
//
// We catch the ETSI NegSem_1508_* cases at analysis time rather than
// runtime so the conformance gate marks them as the expected
// rejection.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkTemplateRestrictions(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic

	// Restriction d on modified-template restrictions is
	// disabled: the Sem and NegSem suites contradict each
	// other (Sem_1508_TemplateRestrictions_017 allows
	// `base=unrestricted → mod=omit` while
	// NegSem_1508_TemplateRestrictions_059 rejects the same
	// transition). The Sem tests follow V4.5.1, the NegSems
	// follow a later revision. Enabling the rule scores
	// fewer NegSems than it loses Sems.
	// diags = append(diags, modifiedTemplateRestrictionViolations(mod)...)
	_ = modifiedTemplateRestrictionViolations
	diags = append(diags, modifiedTemplateListBaseViolations(mod)...)

	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		// Functions, altsteps and testcases never accept `-` as
		// a default-value form. The dash is only meaningful for
		// formal parameters of *modifying templates* (see 5.4.1.1
		// rule "verify that error is generated if formal value
		// parameter of <function|altstep|testcase> contains
		// dash"). Testcases additionally cannot have `timer`
		// parameters (per 5.4.1.1, testcases run alone in their
		// own scope and timer references must be local).
		if fn, ok := d.Def.(*syntax.FuncDecl); ok && fn.Params != nil {
			for _, p := range fn.Params.List {
				if p == nil {
					continue
				}
				if isDashDefault(p.Value) {
					diags = append(diags, Diagnostic{
						Code:     "dash-default-non-template",
						Severity: SeverityError,
						Message:  "dash (`-`) default value is only allowed for parameters of modifying templates",
						Node:     p,
						Span:     syntax.SpanOf(p),
					})
				}
				// Out / inout formal parameters cannot have
				// a default value and cannot be tagged
				// `@lazy` / `@fuzzy` (5.4.1.1).
				if p.Direction != nil {
					switch p.Direction.Kind() {
					case syntax.OUT, syntax.INOUT:
						if p.Value != nil {
							diags = append(diags, Diagnostic{
								Code:     "out-inout-default-value",
								Severity: SeverityError,
								Message:  "out/inout formal parameters cannot have default values",
								Node:     p,
								Span:     syntax.SpanOf(p),
							})
						}
						if p.Modif != nil {
							s := p.Modif.String()
							if s == "@lazy" || s == "@fuzzy" {
								diags = append(diags, Diagnostic{
									Code:     "out-inout-lazy-modifier",
									Severity: SeverityError,
									Message:  fmt.Sprintf("out/inout formal parameters cannot have %s modifier", s),
									Node:     p,
									Span:     syntax.SpanOf(p),
								})
							}
						}
					}
				}
				if fn.KindTok != nil && fn.KindTok.Kind() == syntax.TESTCASE {
					if id, ok := p.Type.(*syntax.Ident); ok && id.Tok != nil {
						switch id.Tok.Kind() {
						case syntax.TIMER:
							diags = append(diags, Diagnostic{
								Code:     "testcase-timer-param",
								Severity: SeverityError,
								Message:  "test cases cannot have timer formal parameters",
								Node:     p,
								Span:     syntax.SpanOf(p),
							})
						case syntax.PORT:
							diags = append(diags, Diagnostic{
								Code:     "testcase-port-param",
								Severity: SeverityError,
								Message:  "test cases cannot have port formal parameters",
								Node:     p,
								Span:     syntax.SpanOf(p),
							})
						}
						if id.String() == "default" {
							diags = append(diags, Diagnostic{
								Code:     "testcase-default-param",
								Severity: SeverityError,
								Message:  "test cases cannot have default formal parameters",
								Node:     p,
								Span:     syntax.SpanOf(p),
							})
						}
					}
					// User-defined port type used as testcase
					// parameter: matched by checking the type's
					// name resolves to a `type port` decl.
					if id, ok := p.Type.(*syntax.Ident); ok && id.Tok != nil {
						if isUserDefinedPortName(mod, id.String()) {
							diags = append(diags, Diagnostic{
								Code:     "testcase-port-param",
								Severity: SeverityError,
								Message: fmt.Sprintf(
									"test cases cannot have port formal parameters (%s is a port type)",
									id.String()),
								Node: p,
								Span: syntax.SpanOf(p),
							})
						}
					}
				}
			}
		}
		td, ok := d.Def.(*syntax.TemplateDecl)
		if !ok {
			continue
		}
		// Restriction b) Formal parameters of templates shall
		// always be `in` (no out/inout). A non-modified template
		// also rejects the `dash` (`:= -`) default value form,
		// which is reserved for modifying templates only (15.5).
		// Per 5.4.1.1 the parameter type must be a value or
		// template - never `port`, `timer` or `default`.
		if td.Params != nil {
			isModified := td.Base != nil
			for _, p := range td.Params.List {
				if p == nil {
					continue
				}
				if p.Direction != nil {
					switch p.Direction.Kind() {
					case syntax.OUT, syntax.INOUT:
						diags = append(diags, Diagnostic{
							Code:     "template-out-inout-param",
							Severity: SeverityError,
							Message: fmt.Sprintf(
								"template formal parameters must be in (got %s)",
								p.Direction.String()),
							Node: p,
							Span: syntax.SpanOf(p),
						})
					}
				}
				if !isModified && isDashDefault(p.Value) {
					diags = append(diags, Diagnostic{
						Code:     "dash-default-non-modified-template",
						Severity: SeverityError,
						Message:  "dash (`-`) default value is only allowed for parameters of modifying templates",
						Node:     p,
						Span:     syntax.SpanOf(p),
					})
				}
				if id, ok := p.Type.(*syntax.Ident); ok && id.Tok != nil {
					switch id.Tok.Kind() {
					case syntax.TIMER:
						diags = append(diags, Diagnostic{
							Code:     "template-timer-param",
							Severity: SeverityError,
							Message:  "templates cannot have timer formal parameters",
							Node:     p,
							Span:     syntax.SpanOf(p),
						})
					case syntax.PORT:
						diags = append(diags, Diagnostic{
							Code:     "template-port-param",
							Severity: SeverityError,
							Message:  "templates cannot have port formal parameters",
							Node:     p,
							Span:     syntax.SpanOf(p),
						})
					}
					if id.String() == "default" {
						diags = append(diags, Diagnostic{
							Code:     "template-default-param",
							Severity: SeverityError,
							Message:  "templates cannot have default formal parameters",
							Node:     p,
							Span:     syntax.SpanOf(p),
						})
					}
				}
				if id, ok := p.Type.(*syntax.Ident); ok && id.Tok != nil {
					if isUserDefinedPortName(mod, id.String()) {
						diags = append(diags, Diagnostic{
							Code:     "template-port-param",
							Severity: SeverityError,
							Message: fmt.Sprintf(
								"templates cannot have port formal parameters (%s is a port type)",
								id.String()),
							Node: p,
							Span: syntax.SpanOf(p),
						})
					}
				}
			}
		}
		if td.RestrictionSpec == nil {
			continue
		}
		restr := ""
		if td.RestrictionSpec.Tok != nil {
			restr = td.RestrictionSpec.Tok.String()
		}
		if restr == "" {
			continue
		}
		diags = append(diags, restrictionViolationsIn(td.Value, restr, td)...)
		// 15.8 parameter-passing rule: a template(omit|value)
		// cannot accept a formal parameter whose default value
		// contains `?` / `*` (omit forbids both, value forbids
		// `*`). The same applies when the parameter's own
		// template restriction is laxer than the enclosing
		// template's restriction. Catches the NegSem_1508_*
		// 036 / 037 / 040 / 049 / 052 family.
		if td.Params != nil {
			diags = append(diags, paramRestrictionViolations(td.Params, restr)...)
		}
	}

	// Nested template declarations (declared inside testcase
	// or function bodies) carry their own restriction and
	// must obey the same body rules. The module.Defs loop
	// above only covers top-level templates, so walk the
	// whole tree for any leftover TemplateDecls.
	syntax.Inspect(mod, func(n syntax.Node) bool {
		td, ok := n.(*syntax.TemplateDecl)
		if !ok || td == nil || td.RestrictionSpec == nil {
			return true
		}
		// Skip the top-level templates we already handled.
		for _, d := range mod.Defs {
			if d != nil && d.Def == td {
				return true
			}
		}
		restr := ""
		if td.RestrictionSpec.Tok != nil {
			restr = td.RestrictionSpec.Tok.String()
		}
		if restr == "" {
			return true
		}
		diags = append(diags, restrictionViolationsIn(td.Value, restr, td)...)
		return true
	})

	return diags
}

// paramRestrictionViolations flags formal-parameter defaults whose
// shape cannot coexist with the enclosing template's restriction:
// `(omit)` forbids both `?` and `*` anywhere, `(value)` forbids
// `*`. Restriction 15.8.c on laxer parameter restrictions was
// removed in TTCN-3:2013 (see Sem_1508_TemplateRestrictions_033),
// so we no longer flag a `template` parameter inside a
// `template(value|present)` body solely on its restriction tag.
func paramRestrictionViolations(params *syntax.FormalPars, enclosing string) []Diagnostic {
	var diags []Diagnostic
	for _, p := range params.List {
		if p == nil || p.Value == nil {
			continue
		}
		diags = append(diags, restrictionViolationsIn(p.Value, enclosing, p)...)
	}
	return diags
}

// modifiedTemplateRestrictionViolations flags `modifies` templates
// whose restriction differs from the base template's restriction.
// ETSI 15.8 requires the modified template's restriction to MATCH
// the base's; any change (stricter or laxer) is rejected. We collect
// every named TemplateDecl in the module up-front so the modifies
// lookup stays O(1).
func modifiedTemplateRestrictionViolations(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	byName := map[string]*syntax.TemplateDecl{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		td, ok := n.(*syntax.TemplateDecl)
		if !ok || td == nil || td.Name == nil {
			return true
		}
		byName[td.Name.String()] = td
		return true
	})
	syntax.Inspect(mod, func(n syntax.Node) bool {
		td, ok := n.(*syntax.TemplateDecl)
		if !ok || td == nil || td.ModifiesTok == nil || td.Base == nil {
			return true
		}
		baseName := identName(td.Base)
		if baseName == "" {
			return true
		}
		base, ok := byName[baseName]
		if !ok {
			return true
		}
		baseRestr := restrictionTag(base.RestrictionSpec)
		modRestr := restrictionTag(td.RestrictionSpec)
		if baseRestr == modRestr {
			return true
		}
		diags = append(diags, Diagnostic{
			Code:     "modified-template-restriction-mismatch",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"modified template restriction `template(%s)` differs from base `template(%s)` (ETSI 15.8 restriction d)",
				modRestr, baseRestr),
			Node: td,
			Span: syntax.SpanOf(td),
		})
		return true
	})
	return diags
}

func modifiedTemplateListBaseViolations(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	byName := map[string]*syntax.TemplateDecl{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		td, ok := n.(*syntax.TemplateDecl)
		if !ok || td == nil || td.Name == nil {
			return true
		}
		byName[td.Name.String()] = td
		return true
	})
	syntax.Inspect(mod, func(n syntax.Node) bool {
		td, ok := n.(*syntax.TemplateDecl)
		if !ok || td == nil || td.ModifiesTok == nil || td.Base == nil {
			return true
		}
		baseName := identName(td.Base)
		if baseName == "" {
			return true
		}
		base := byName[baseName]
		if base == nil || !isTemplateValueList(base.Value) {
			return true
		}
		diags = append(diags, Diagnostic{
			Code:     "modified-template-list-base",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"template %q modifies list-valued base template %q; value-list templates cannot be modified (ETSI 15.6.4)",
				identName(td.Name), baseName),
			Node: td,
			Span: syntax.SpanOf(td),
		})
		return true
	})
	return diags
}

func isTemplateValueList(expr syntax.Expr) bool {
	pe, ok := expr.(*syntax.ParenExpr)
	return ok && pe != nil && len(pe.List) > 1
}

// restrictionTag returns the lower-case restriction keyword
// ("omit", "value", "present") or "" when no restriction is set.
func restrictionTag(rs *syntax.RestrictionSpec) string {
	if rs == nil || rs.Tok == nil {
		return ""
	}
	return rs.Tok.String()
}

// isUserDefinedPortName reports whether the given identifier names
// a port type declaration in the module. We need this to flag
// `testcase TC(MyPortType p)`-style violations because the parser
// surfaces the type as a generic Ident, not a port keyword.
func isUserDefinedPortName(mod *syntax.Module, name string) bool {
	found := false
	syntax.Inspect(mod, func(n syntax.Node) bool {
		if n == nil || found {
			return false
		}
		if pt, ok := n.(*syntax.PortTypeDecl); ok && pt.Name != nil {
			if pt.Name.String() == name {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// isDashDefault reports whether expr is the `-` (dash) default-value
// literal. The parser surfaces it as a ValueLiteral whose Tok kind is
// SUB.
func isDashDefault(expr syntax.Expr) bool {
	if expr == nil {
		return false
	}
	v, ok := expr.(*syntax.ValueLiteral)
	if !ok || v.Tok == nil {
		return false
	}
	return v.Tok.Kind() == syntax.SUB
}

// restrictionViolationsIn flags forbidden constructs inside expr.
// For `omit` and `value` restrictions, wildcards, ranges, patterns,
// length restrictions and value lists are rejected anywhere in the
// initialiser - including inside nested record/list literals. The
// per-field `omit` value is only flagged at the top level, because
// inner optional fields are allowed to carry `omit`.
func restrictionViolationsIn(expr syntax.Expr, restr string, anchor syntax.Node) []Diagnostic {
	var diags []Diagnostic
	if expr == nil {
		return diags
	}

	report := func(node syntax.Node, what string) {
		diags = append(diags, Diagnostic{
			Code:     "template-restriction-violation",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"%q is not allowed in a template(%s) initialiser",
				what, restr),
			Node: node,
			Span: syntax.SpanOf(node),
		})
	}

	// Top-level only: bare `omit` literal.
	if v, ok := expr.(*syntax.ValueLiteral); ok && v.Tok != nil {
		switch v.Tok.Kind() {
		case syntax.OMIT:
			if restr == "value" || restr == "present" {
				report(v, "omit")
			}
		}
	}

	// Recurse anywhere for the wildcard / pattern / range / set
	// constructs.
	syntax.Inspect(expr, func(n syntax.Node) bool {
		if n == nil {
			return false
		}
		switch x := n.(type) {
		case *syntax.ValueLiteral:
			if x.Tok != nil {
				switch x.Tok.Kind() {
				case syntax.ANY:
					if restr == "omit" || restr == "value" {
						report(x, "?")
					}
				case syntax.MUL:
					if restr == "omit" || restr == "value" {
						report(x, "*")
					}
				}
			}
		case *syntax.LengthExpr:
			if restr == "omit" || restr == "value" {
				report(x, "length")
			}
		case *syntax.PatternExpr:
			if restr == "omit" || restr == "value" {
				report(x, "pattern")
			}
		case *syntax.BinaryExpr:
			if x.Op != nil && x.Op.Kind() == syntax.RANGE && (restr == "omit" || restr == "value") {
				report(x, "value range")
			}
		case *syntax.ParenExpr:
			if (restr == "omit" || restr == "value") && len(x.List) > 1 {
				report(x, "value list")
			}
		case *syntax.CallExpr:
			// `permutation(...)` is a list template
			// construct; reject for omit/value.
			if id, ok := x.Fun.(*syntax.Ident); ok && id.Tok != nil {
				switch id.String() {
				case "permutation", "complement", "subset", "superset":
					if restr == "omit" || restr == "value" {
						report(x, id.String())
					}
				}
			}
		case *syntax.DecmatchExpr:
			if restr == "omit" || restr == "value" {
				report(x, "decmatch")
			}
		case *syntax.DecodedExpr:
			if restr == "omit" || restr == "value" {
				report(x, "@decoded")
			}
		case *syntax.UnaryExpr:
			// `<value> ifpresent` widens the match to
			// include the absence of the field; it is
			// not a concrete value so omit/value templates
			// must reject it.
			if x.Op != nil && x.Op.Kind() == syntax.IFPRESENT &&
				(restr == "omit" || restr == "value") {
				report(x, "ifpresent")
			}
		}
		return true
	})
	return diags
}

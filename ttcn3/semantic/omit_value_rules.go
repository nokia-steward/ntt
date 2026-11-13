// omit_value_rules.go enforces two closely-related ETSI clauses
// on template-only value literals:
//
//   - B.1.2.8 restriction A (`omit`): "omit can be assigned to
//     templates of any type as a whole or to optional fields of
//     set or record templates."
//   - B.1.2.4 restriction A (`*` AnyValueOrNone): "* can only
//     appear in optional fields of structured templates."
//
// Cases the runtime currently lets through:
//
//   - var-of-omit: `var T v := omit;` -- a plain variable is not
//     a template; emitted as omit-on-non-template.
//   - mandatory-field-omit: a `field := omit` entry in a
//     CompositeLiteral / TemplateDecl whose target field is not
//     declared `optional`. Emitted as omit-on-mandatory-field.
//   - mandatory-field-any-or-none: same shape with `*` instead of
//     `omit`. Emitted as any-or-none-on-mandatory-field. The walker
//     applies equally to union alternatives (which are never
//     optional, so any `alt := omit` or `alt := *` inside a union
//     composite is illegal).
//
// We only inspect literal RHS expressions of these two
// templating tokens; constants, function returns, parameters etc.
// fall through silently.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// structFieldOpt records the optional flag for every named field
// of a named struct (record / set / union) type.
type structFieldOpt map[string]bool

// structFieldTyp records the *declared type name* of every field
// in a named struct type. Used to recurse into nested composite
// initialisers and look up the inner type's optional table.
type structFieldTyp map[string]string

// checkOmitValueRules is wired into Analyze; emits the three
// diagnostics described at the top of this file.
func (a *Analyzer) checkOmitValueRules(mod *syntax.Module) []Diagnostic {
	fieldOpt := collectStructFieldOptionality(mod)
	fieldTyp := collectStructFieldDeclTypes(mod)
	var diags []Diagnostic
	walk := func(n syntax.Node) {
		syntax.Inspect(n, func(sn syntax.Node) bool {
			switch x := sn.(type) {
			case *syntax.ValueDecl:
				diags = append(diags, checkVarOfOmit(x)...)
				diags = append(diags, checkValueDeclCompositeOmit(x, fieldOpt, fieldTyp)...)
			case *syntax.TemplateDecl:
				diags = append(diags, checkTemplateDeclCompositeOmit(x, fieldOpt, fieldTyp)...)
			case *syntax.BinaryExpr:
				if x.Op != nil && x.Op.Kind() == syntax.ASSIGN {
					if cl, ok := x.Y.(*syntax.CompositeLiteral); ok {
						diags = append(diags,
							checkCompositeOmit(cl, fieldOpt, fieldTyp,
								"", "assignment")...)
					}
				}
			}
			return true
		})
	}
	// Function bodies (covers var/template decls in local scope
	// plus assignment-form omit injections).
	syntax.Inspect(mod, func(n syntax.Node) bool {
		fn, ok := n.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			return true
		}
		walk(fn.Body)
		return false
	})
	// Module-level definitions: `const T x := { f := omit }` and
	// `template T t := { ... }` at the top of the file land here.
	// Without this pass, NegSyn_060201_RecordTypeValues_001 etc.
	// silently slipped through because the walker only descended
	// into FuncDecl.Body.
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		switch md := d.Def.(type) {
		case *syntax.ValueDecl:
			diags = append(diags, checkValueDeclCompositeOmit(md, fieldOpt, fieldTyp)...)
		case *syntax.TemplateDecl:
			diags = append(diags, checkTemplateDeclCompositeOmit(md, fieldOpt, fieldTyp)...)
		}
	}
	return diags
}

// collectStructFieldDeclTypes records each struct field's
// declared type name (the RefSpec Ident form); inline struct /
// list / map specs map to "" and disable nested recursion for
// those entries.
func collectStructFieldDeclTypes(mod *syntax.Module) map[string]structFieldTyp {
	out := map[string]structFieldTyp{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		st, ok := d.Def.(*syntax.StructTypeDecl)
		if !ok || st.Name == nil {
			continue
		}
		ft := structFieldTyp{}
		for _, f := range st.Fields {
			if f == nil || f.Name == nil {
				continue
			}
			if rs, ok := f.Type.(*syntax.RefSpec); ok && rs != nil {
				ft[f.Name.String()] = identName(rs.X)
			}
		}
		out[st.Name.String()] = ft
	}
	return out
}

// checkVarOfOmit flags `var T v := omit;` -- omit is template-
// only, never assignable as a whole to a plain variable.
//
// We also exempt `template T v := omit;` so the existing
// templated-value flow that legitimately uses omit stays clean.
func checkVarOfOmit(vd *syntax.ValueDecl) []Diagnostic {
	if vd == nil || vd.KindTok == nil || vd.KindTok.Kind() != syntax.VAR {
		return nil
	}
	// `var template(...) T v := omit;` is a template-typed
	// variable; omit is then exactly the "template as a whole"
	// case restriction A explicitly allows.
	if vd.TemplateRestriction != nil {
		return nil
	}
	var diags []Diagnostic
	for _, dec := range vd.Decls {
		if dec == nil || dec.Value == nil {
			continue
		}
		if !isOmitLiteral(dec.Value) {
			continue
		}
		diags = append(diags, Diagnostic{
			Code:     "omit-on-non-template",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"%q is a value (not a template); omit can only be assigned to a template as a whole or to an optional field",
				dec.Name.String()),
			Node: dec.Value,
			Span: syntax.SpanOf(dec.Value),
		})
	}
	return diags
}

// checkValueDeclCompositeOmit recurses into CompositeLiteral
// initializers of var / const decls and emits the
// field-omit-on-mandatory diagnostic at each violation site.
func checkValueDeclCompositeOmit(
	vd *syntax.ValueDecl,
	fieldOpt map[string]structFieldOpt,
	fieldTyp map[string]structFieldTyp,
) []Diagnostic {
	if vd == nil {
		return nil
	}
	typeName := identName(vd.Type)
	var diags []Diagnostic
	for _, dec := range vd.Decls {
		if dec == nil || dec.Value == nil {
			continue
		}
		cl, ok := dec.Value.(*syntax.CompositeLiteral)
		if !ok {
			continue
		}
		diags = append(diags, checkCompositeOmit(cl, fieldOpt, fieldTyp, typeName, dec.Name.String())...)
	}
	return diags
}

// checkTemplateDeclCompositeOmit walks a `template T name := { ... };`
// declaration and applies the same field-omit-on-mandatory rule.
// Templates are parsed as TemplateDecl, not ValueDecl, so they need
// a separate dispatch.
func checkTemplateDeclCompositeOmit(
	td *syntax.TemplateDecl,
	fieldOpt map[string]structFieldOpt,
	fieldTyp map[string]structFieldTyp,
) []Diagnostic {
	if td == nil || td.Value == nil {
		return nil
	}
	typeName := identName(td.Type)
	cl, ok := td.Value.(*syntax.CompositeLiteral)
	if !ok {
		return nil
	}
	name := ""
	if td.Name != nil {
		name = td.Name.String()
	}
	return checkCompositeOmit(cl, fieldOpt, fieldTyp, typeName, name)
}

// checkCompositeOmit walks a CompositeLiteral assigning to a value
// of `typeName` and flags `field := omit` entries whose field is
// not declared `optional`. Nested CompositeLiterals are recursed
// into using the declared field type as the new context.
func checkCompositeOmit(
	cl *syntax.CompositeLiteral,
	fieldOpt map[string]structFieldOpt,
	fieldTyp map[string]structFieldTyp,
	typeName string,
	context string,
) []Diagnostic {
	if cl == nil {
		return nil
	}
	opts := fieldOpt[typeName]
	types := fieldTyp[typeName]
	var diags []Diagnostic
	for _, item := range cl.List {
		be, ok := item.(*syntax.BinaryExpr)
		if !ok || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
			continue
		}
		fieldName := identName(be.X)
		if fieldName == "" {
			continue
		}
		if isOmitLiteral(be.Y) {
			if opts != nil {
				if isOpt, known := opts[fieldName]; known && !isOpt {
					diags = append(diags, Diagnostic{
						Code:     "omit-on-mandatory-field",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"field %q of type %q is not declared optional; cannot assign omit (context: %s)",
							fieldName, typeName, context),
						Node: be,
						Span: syntax.SpanOf(be),
					})
				}
			}
			continue
		}
		if isAnyOrNoneLiteral(be.Y) {
			if opts != nil {
				if isOpt, known := opts[fieldName]; known && !isOpt {
					diags = append(diags, Diagnostic{
						Code:     "any-or-none-on-mandatory-field",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"field %q of type %q is not declared optional; cannot assign * (context: %s)",
							fieldName, typeName, context),
						Node: be,
						Span: syntax.SpanOf(be),
					})
				}
			}
			continue
		}
		// Nested composite literal: recurse with the field's
		// declared type as the new typeName context.
		if nested, ok := be.Y.(*syntax.CompositeLiteral); ok {
			nestedType := ""
			if types != nil {
				nestedType = types[fieldName]
			}
			if nestedType != "" {
				diags = append(diags, checkCompositeOmit(
					nested, fieldOpt, fieldTyp, nestedType,
					context+"."+fieldName)...)
			}
		}
	}
	return diags
}

// isOmitLiteral reports whether e is exactly the `omit` token.
func isOmitLiteral(e syntax.Expr) bool {
	lit, ok := e.(*syntax.ValueLiteral)
	if !ok || lit == nil || lit.Tok == nil {
		return false
	}
	return lit.Tok.Kind() == syntax.OMIT
}

// isAnyOrNoneLiteral reports whether e is exactly the `*` token
// (AnyValueOrNone) used as a template matching mechanism. The
// scanner uses MUL for both the multiplicative operator and the
// AnyValueOrNone literal; only the bare ValueLiteral form is
// AnyValueOrNone, so this check is sufficient for the rule.
func isAnyOrNoneLiteral(e syntax.Expr) bool {
	lit, ok := e.(*syntax.ValueLiteral)
	if !ok || lit == nil || lit.Tok == nil {
		return false
	}
	return lit.Tok.Kind() == syntax.MUL
}

// collectStructFieldOptionality walks every top-level struct type
// and records each field's `optional` flag.
func collectStructFieldOptionality(mod *syntax.Module) map[string]structFieldOpt {
	out := map[string]structFieldOpt{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		st, ok := d.Def.(*syntax.StructTypeDecl)
		if !ok || st.Name == nil {
			continue
		}
		opts := structFieldOpt{}
		for _, f := range st.Fields {
			if f == nil || f.Name == nil {
				continue
			}
			opts[f.Name.String()] = f.Optional != nil
		}
		out[st.Name.String()] = opts
	}
	return out
}

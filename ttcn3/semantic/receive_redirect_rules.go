// receive_redirect_rules.go enforces port-redirect rules from
// ETSI ES 201 873-1 clause 22.2.2 / 22.2.3:
//
//   - The `@index` redirection (`-> @index value v`) is only
//     valid on `any from` / `all from` port-array operations.
//     Plain `p.receive(...)` and `any port.receive(...)` are
//     rejected.
//
//   - A value redirection (`-> value v`) on a receive that
//     specifies no template / type argument has no source type
//     to redirect from. The receive must include a template
//     parameter or be a typed receive.
//
// We catch these statically by walking every RedirectExpr in the
// module and inspecting its `X` (the receive / trigger / getcall
// / getreply / catch CallExpr or SelectorExpr).
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkReceiveRedirectRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	varTypes := collectModuleVarTypes(mod)
	structFieldTypes := collectStructFieldTypes(mod)
	syntax.Inspect(mod, func(n syntax.Node) bool {
		r, ok := n.(*syntax.RedirectExpr)
		if !ok || r == nil {
			return true
		}
		op := redirectOpName(r.X)
		if !isPortOperationName(op) {
			return true
		}

		if r.IndexTok != nil && !isFromPortReceiver(r.X) {
			diags = append(diags, Diagnostic{
				Code:     "index-redirect-without-any-from",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"@index redirection requires an `any from port-array` form (got plain %s)",
					op),
				Node: r,
				Span: syntax.SpanOf(r),
			})
		}

		// ETSI 22.{2,3}: `@index value v` redirects the index
		// of the matched port in a port-array `any from`/
		// `all from` operation; the target variable must be an
		// integer type (the highest valid index never overflows
		// integer). Conservative: only flag when the variable
		// type is a recognised primitive that is NOT integer.
		if r.IndexTok != nil && r.Index != nil {
			if target := identName(r.Index); target != "" {
				if ty, ok := varTypes[target]; ok && ty != "" && !isIntegerLikeTypeWithMod(ty, mod) {
					diags = append(diags, Diagnostic{
						Code:     "index-redirect-non-integer",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"@index redirect target %q has type %q; expected an integer-compatible type",
							target, ty),
						Node: r,
						Span: syntax.SpanOf(r),
					})
				}
			}
		}

		// ETSI 22.2.2 / 22.2.3: `-> value v` binds the matched
		// message to v. v must be type-compatible with the
		// receive's template-instance type (e.g.
		// `p.trigger(integer:?)` writes an integer, so v_str
		// declared as charstring is rejected). Conservative:
		// only flag when both sides are recognised primitives.
		if r.ValueTok != nil && len(r.Value) > 0 && (op == "receive" || op == "trigger" || op == "check") {
			if tyFromTemplate := templateInstanceType(r.X); tyFromTemplate != "" {
				if target := identName(r.Value[0]); target != "" {
					if ty, ok := varTypes[target]; ok && ty != "" &&
						isPrimitiveType(ty) && isPrimitiveType(tyFromTemplate) &&
						ty != tyFromTemplate {
						diags = append(diags, Diagnostic{
							Code:     "value-redirect-type-mismatch",
							Severity: SeverityError,
							Message: fmt.Sprintf(
								"value redirect target %q has type %q; expected %q to match the receive template",
								target, ty, tyFromTemplate),
							Node: r,
							Span: syntax.SpanOf(r),
						})
					}
				}
				// Field-destructuring shape:
				// `-> value ( v1 := f1, v2 := f2[..] )`
				// (ETSI 22.2.2 / 22.2.3). For each entry
				// we resolve the RHS field name to its
				// declared type and compare against the
				// LHS variable's type. Only fires on
				// recognised primitives so user-defined
				// types stay silent.
				if pe, ok := r.Value[0].(*syntax.ParenExpr); ok && pe != nil {
					fields := structFieldTypes[tyFromTemplate]
					for _, item := range pe.List {
						be, ok := item.(*syntax.BinaryExpr)
						if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
							continue
						}
						target := identName(be.X)
						if target == "" {
							continue
						}
						ty, hasTy := varTypes[target]
						if !hasTy || ty == "" || !isPrimitiveType(ty) {
							continue
						}
						fieldTy := fieldRefType(be.Y, fields)
						if fieldTy == "" || !isPrimitiveType(fieldTy) || fieldTy == ty {
							continue
						}
						diags = append(diags, Diagnostic{
							Code:     "value-redirect-field-type-mismatch",
							Severity: SeverityError,
							Message: fmt.Sprintf(
								"value redirect %q: target type %q is not compatible with %q field type %q (ETSI 22.2.2 / 22.2.3)",
								target, ty, tyFromTemplate, fieldTy),
							Node: be,
							Span: syntax.SpanOf(be),
						})
					}
				}
			}
		}

		if r.ValueTok != nil && len(r.Value) > 0 && !receiveHasTemplateArg(r.X) &&
			(op == "receive" || op == "trigger" || op == "check") {
			diags = append(diags, Diagnostic{
				Code:     "value-redirect-without-template",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"value redirection on `%s` requires a template / type argument to determine the source type",
					op),
				Node: r,
				Span: syntax.SpanOf(r),
			})
		}

		// ETSI 22.3.2: `getcall` does NOT yield a value (the
		// signature's return value belongs to `getreply`). A
		// `-> value v` redirect on getcall therefore has
		// nothing to bind. Same for `raise` which is a
		// send-shape operation. We allow `param(...)`,
		// `sender ...`, `@index ...` and the assorted decode
		// redirects - only the bare-value form is rejected.
		if r.ValueTok != nil && len(r.Value) > 0 && (op == "getcall" || op == "raise") {
			diags = append(diags, Diagnostic{
				Code:     "value-redirect-forbidden-op",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"value redirection is not allowed on `%s` (ETSI 22.3)",
					op),
				Node: r,
				Span: syntax.SpanOf(r),
			})
		}
		return true
	})
	return diags
}

// templateInstanceType extracts the type name from a port-operation
// CallExpr whose first argument is a typed template instance such as
// `integer:?` (a BinaryExpr with Op=":" and X=Ident("integer")).
// Returns "" for any other shape. `any from`/`all from` wrappers
// are unwrapped first so port-array shapes can be inspected too.
func templateInstanceType(x syntax.Expr) string {
	if from, ok := x.(*syntax.FromExpr); ok && from != nil {
		x = from.X
	}
	ce, ok := x.(*syntax.CallExpr)
	if !ok || ce == nil || ce.Args == nil || len(ce.Args.List) == 0 {
		return ""
	}
	be, ok := ce.Args.List[0].(*syntax.BinaryExpr)
	if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.COLON {
		return ""
	}
	id, ok := be.X.(*syntax.Ident)
	if !ok || id == nil {
		return ""
	}
	return id.String()
}

// collectModuleVarTypes returns a name -> declared-type-name map for
// every `var T name`, `var template T name`, and formal parameter in
// the module. Type names are taken verbatim from the source so we
// can compare them against `templateInstanceType` strings.
func collectModuleVarTypes(mod *syntax.Module) map[string]string {
	out := map[string]string{}
	record := func(name, ty string) {
		if name == "" || ty == "" {
			return
		}
		if _, ok := out[name]; !ok {
			out[name] = ty
		}
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch v := n.(type) {
		case *syntax.ValueDecl:
			if v == nil {
				return true
			}
			ty := identName(v.Type)
			for _, d := range v.Decls {
				if d != nil && d.Name != nil {
					declTy := ty
					// `var integer v[1]` is an array
					// of integer, not a scalar
					// integer. Surface this so
					// @index-target type checks
					// reject array redirections per
					// ETSI 22.{2,3}.x restriction i.
					if len(d.ArrayDef) > 0 && declTy != "" {
						declTy = declTy + "[]"
					}
					record(d.Name.String(), declTy)
				}
			}
		case *syntax.FormalPar:
			if v == nil || v.Name == nil {
				return true
			}
			record(v.Name.String(), identName(v.Type))
		}
		return true
	})
	return out
}

// isIntegerLikeType reports whether ty is a primitive integer type
// name or a fixed-size array / record-of-integer suitable for
// multi-dimensional port-array @index redirection (ETSI 22.{2,3}
// restriction j).  Subtypes / typedefs that ultimately reduce to
// integer would also be valid, but we keep this conservative until
// type resolution is wired in.
func isIntegerLikeType(ty string) bool {
	switch ty {
	case "integer", "integer[]":
		return true
	}
	return false
}

// isIntegerLikeTypeWithMod is the module-aware variant: it also
// accepts a SubTypeDecl whose Field.Type is `record of integer` or
// `set of integer` (used as the index redirect target on a
// multi-dimensional port array).
func isIntegerLikeTypeWithMod(ty string, mod *syntax.Module) bool {
	if isIntegerLikeType(ty) {
		return true
	}
	if mod == nil || ty == "" {
		return false
	}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		switch x := d.Def.(type) {
		case *syntax.SubTypeDecl:
			if x == nil || x.Field == nil ||
				x.Field.Name == nil ||
				x.Field.Name.String() != ty {
				continue
			}
			ls, ok := x.Field.Type.(*syntax.ListSpec)
			if !ok || ls == nil {
				continue
			}
			if ls.KindTok == nil {
				continue
			}
			k := ls.KindTok.Kind()
			if k != syntax.RECORD && k != syntax.SET {
				continue
			}
			if typeSpecToName(ls.ElemType) == "integer" {
				return true
			}
		}
	}
	return false
}

// fieldRefType returns the declared primitive type name of an RHS
// expression that points at a field of the matched message.
// Supported shapes:
//
//   - Ident("field")              -> typeName of field
//   - IndexExpr{X=Ident("field")} -> typeName of field (array
//     indexing yields the element type, which for primitives
//     stored in `integer field[]` etc. is the same name)
//
// Anything else returns "".
func fieldRefType(expr syntax.Expr, fields fieldsOfStruct) string {
	switch v := expr.(type) {
	case *syntax.Ident:
		if v == nil {
			return ""
		}
		return fields[v.String()].typeName
	case *syntax.IndexExpr:
		if v == nil {
			return ""
		}
		base := identName(v.X)
		if base == "" {
			return ""
		}
		return fields[base].typeName
	}
	return ""
}

// isPrimitiveType reports whether ty is a name we recognise as one
// of TTCN-3's built-in primitive types. The value-redirect mismatch
// rule only fires when both the receive template and the target
// variable are recognised primitives - that avoids false positives
// on user-defined record / set / enum types we can't resolve.
func isPrimitiveType(ty string) bool {
	switch ty {
	case "integer", "float", "boolean", "charstring", "universal",
		"bitstring", "octetstring", "hexstring", "verdicttype":
		return true
	}
	return false
}

// redirectOpName returns the canonical port-operation name
// referenced by the X side of a RedirectExpr, e.g. "receive",
// "trigger", "getcall", or "" when X is not a port operation.
// `any from ports.OP(...)` and `all from ports.OP(...)` shapes
// are unwrapped through FromExpr.X first.
func redirectOpName(x syntax.Expr) string {
	if from, ok := x.(*syntax.FromExpr); ok && from != nil {
		x = from.X
	}
	switch v := x.(type) {
	case *syntax.CallExpr:
		if sel, ok := v.Fun.(*syntax.SelectorExpr); ok && sel != nil {
			return identName(sel.Sel)
		}
	case *syntax.SelectorExpr:
		if v != nil {
			return identName(v.Sel)
		}
	}
	return ""
}

func isPortOperationName(name string) bool {
	switch name {
	case "receive", "trigger", "getcall", "getreply", "catch", "check":
		return true
	}
	return false
}

// isFromPortReceiver reports whether x is an `any from` / `all from`
// port-array shape. The parser surfaces two flavours:
//   - the FromExpr wraps the whole CallExpr / SelectorExpr (newer
//     shape, e.g. `any from p.getreply(...)`)
//   - the FromExpr is the receiver of the SelectorExpr (legacy
//     shape, `any from p`.x)
//
// We accept either.
func isFromPortReceiver(x syntax.Expr) bool {
	if _, ok := x.(*syntax.FromExpr); ok {
		return true
	}
	var receiver syntax.Expr
	switch v := x.(type) {
	case *syntax.CallExpr:
		if sel, ok := v.Fun.(*syntax.SelectorExpr); ok && sel != nil {
			receiver = sel.X
		}
	case *syntax.SelectorExpr:
		if v != nil {
			receiver = v.X
		}
	}
	if receiver == nil {
		return false
	}
	_, ok := receiver.(*syntax.FromExpr)
	return ok
}

// receiveHasTemplateArg reports whether the receive-shaped call
// carries a template / type argument. A bare selector like
// `p.receive` (no parens at all) is treated as no-arg.
// `any from`/`all from` wrappers are unwrapped first.
func receiveHasTemplateArg(x syntax.Expr) bool {
	if from, ok := x.(*syntax.FromExpr); ok && from != nil {
		x = from.X
	}
	ce, ok := x.(*syntax.CallExpr)
	if !ok || ce == nil || ce.Args == nil {
		return false
	}
	return len(ce.Args.List) > 0
}

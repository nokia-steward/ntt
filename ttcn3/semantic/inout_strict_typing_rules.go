package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// checkInoutStrictTypingRules enforces ETSI 5.4.2: when an
// actual parameter is passed to an `inout` formal, the actual's
// declared type identifier must match the formal's type
// identifier verbatim. Structural compatibility is not enough;
// aliases (`type T1 T2;`) do not satisfy strong typing either.
//
// We only fire when both the formal and the actual carry a
// resolvable, named type, and the formal is declared `inout`.
// Templates and matchers fall through - their own rules cover
// the rest.
func (a *Analyzer) checkInoutStrictTypingRules(mod *syntax.Module) []Diagnostic {
	sigs := collectModuleSigs(mod)
	if len(sigs) == 0 {
		return nil
	}
	hasInout := false
	for _, list := range sigs {
		for _, p := range list {
			if p.dir == "inout" && p.typeName != "" {
				hasInout = true
				break
			}
		}
		if hasInout {
			break
		}
	}
	if !hasInout {
		return nil
	}
	varTypes := collectModuleVarTypes(mod)
	fieldTypes := collectSingleFieldTypes(mod)
	listElemTypes := collectListElementTypes(mod)
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok || ce == nil || ce.Args == nil {
			return true
		}
		callee := identName(ce.Fun)
		if callee == "" {
			return true
		}
		formals, has := sigs[callee]
		if !has {
			return true
		}
		for i, arg := range ce.Args.List {
			if i >= len(formals) {
				break
			}
			p := formals[i]
			if p.dir != "inout" || p.typeName == "" {
				continue
			}
			actualTy := actualParamStaticType(arg, varTypes, fieldTypes, listElemTypes)
			if actualTy == "" || actualTy == p.typeName {
				continue
			}
			diags = append(diags, Diagnostic{
				Code:     "inout-actual-type-mismatch",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"argument %d of %s: inout actual has type %q, expected %q (ETSI 5.4.2 strong typing requires identical type identifier)",
					i+1, callee, actualTy, p.typeName),
				Node: arg,
				Span: syntax.SpanOf(arg),
			})
		}
		return true
	})
	return diags
}

// actualParamStaticType resolves the syntactic shape of an
// actual parameter to the bare type-identifier it refers to.
// Returns "" when the shape is unsupported (literals, calls,
// complex composites). Idents look up varTypes; selectors strip
// the array suffix added by collectModuleVarTypes and dive into
// fieldTypes; index expressions strip array suffix from the
// underlying var.
func actualParamStaticType(e syntax.Expr, varTypes, fieldTypes, listElems map[string]string) string {
	switch x := e.(type) {
	case *syntax.Ident:
		if x == nil || x.Tok == nil {
			return ""
		}
		ty, ok := varTypes[x.String()]
		if !ok {
			return ""
		}
		return stripArraySuffix(ty)
	case *syntax.SelectorExpr:
		if x == nil || x.Sel == nil {
			return ""
		}
		id, ok := x.Sel.(*syntax.Ident)
		if !ok || id == nil {
			return ""
		}
		if ft, ok := fieldTypes[id.String()]; ok {
			return stripArraySuffix(ft)
		}
		return ""
	case *syntax.IndexExpr:
		if x == nil || x.X == nil {
			return ""
		}
		id, ok := x.X.(*syntax.Ident)
		if !ok || id == nil {
			return ""
		}
		ty, ok := varTypes[id.String()]
		if !ok {
			return ""
		}
		base := stripArraySuffix(ty)
		// `var T arr[N]; arr[i]` -> T
		if len(ty) > 2 && ty[len(ty)-2:] == "[]" {
			return base
		}
		// `var RoI x; x[i]` -> element type of RoI
		if elem, ok := listElems[base]; ok {
			return elem
		}
		return ""
	}
	return ""
}

// collectListElementTypes maps every `type record/set of T NAME;`
// declaration to its element-type name. Used so the inout
// strong-typing rule can resolve `var R x; x[i]` to T.
func collectListElementTypes(mod *syntax.Module) map[string]string {
	out := map[string]string{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		std, ok := n.(*syntax.SubTypeDecl)
		if !ok || std == nil || std.Field == nil || std.Field.Name == nil {
			return true
		}
		ls, ok := std.Field.Type.(*syntax.ListSpec)
		if !ok || ls == nil || ls.ElemType == nil {
			return true
		}
		ref, ok := ls.ElemType.(*syntax.RefSpec)
		if !ok || ref == nil {
			return true
		}
		out[std.Field.Name.String()] = identName(ref.X)
		return true
	})
	return out
}

func stripArraySuffix(ty string) string {
	if l := len(ty); l > 2 && ty[l-2:] == "[]" {
		return ty[:l-2]
	}
	return ty
}

// collectSingleFieldTypes maps each record/set/union field name
// to the single declared type identifier. When the same field
// name appears with different types in different structures we
// drop the entry; the inout strong-typing rule plays it safe by
// not flagging ambiguous shapes.
func collectSingleFieldTypes(mod *syntax.Module) map[string]string {
	type entry struct {
		ty     string
		clash  bool
	}
	tmp := map[string]*entry{}
	add := func(name, ty string) {
		if name == "" || ty == "" {
			return
		}
		if e, ok := tmp[name]; ok {
			if e.ty != ty {
				e.clash = true
			}
			return
		}
		tmp[name] = &entry{ty: ty}
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		std, ok := n.(*syntax.StructTypeDecl)
		if !ok || std == nil {
			return true
		}
		for _, f := range std.Fields {
			if f == nil || f.Name == nil {
				continue
			}
			add(f.Name.String(), describeFieldType(f.Type))
		}
		return true
	})
	out := map[string]string{}
	for name, e := range tmp {
		if e.clash || e.ty == "" {
			continue
		}
		out[name] = e.ty
	}
	return out
}

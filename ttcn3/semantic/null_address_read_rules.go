package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkNullAddressReadRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	addressTypes := collectAddressTypeNames(mod)
	if len(addressTypes) == 0 {
		return nil
	}
	recordAddressFields := collectAddressFields(mod, addressTypes)
	listAddressTypes := collectAddressListTypes(mod, addressTypes)
	if len(recordAddressFields) == 0 && len(listAddressTypes) == 0 {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			continue
		}
		diags = append(diags, checkNullAddressReadsInBlock(fn.Body, recordAddressFields, listAddressTypes)...)
	}
	return diags
}

func checkNullAddressReadsInBlock(
	body *syntax.BlockStmt,
	recordAddressFields map[string]map[string]bool,
	listAddressTypes map[string]bool,
) []Diagnostic {
	var diags []Diagnostic
	nullFields := map[string]map[string]bool{}
	nullIndexes := map[string]bool{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		declTy := identName(vd.Type)
		if declTy == "" {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil || dec.Value == nil {
				continue
			}
			name := dec.Name.String()
			if fields := nullAddressFieldsInInitializer(dec.Value, recordAddressFields[declTy]); len(fields) > 0 {
				nullFields[name] = fields
			}
			if listAddressTypes[declTy] && compositeHasNullIndex(dec.Value) {
				nullIndexes[name] = true
			}
			if ref := nullAddressReadRef(dec.Value, nullFields, nullIndexes); ref != "" {
				diags = append(diags, Diagnostic{
					Code:     "null-address-read",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"declaration initialiser reads null address value %s via field/index notation (ETSI 10 / 11)",
						ref),
					Node: dec.Value,
					Span: syntax.SpanOf(dec.Value),
				})
			}
		}
		return true
	})
	return diags
}

func collectAddressTypeNames(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		st, ok := d.Def.(*syntax.SubTypeDecl)
		if !ok || st == nil || st.Field == nil || st.Field.Name == nil {
			continue
		}
		if st.Field.Name.String() == "address" {
			out["address"] = true
		}
	}
	return out
}

func collectAddressFields(mod *syntax.Module, addressTypes map[string]bool) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		st, ok := d.Def.(*syntax.StructTypeDecl)
		if !ok || st == nil || st.Name == nil {
			continue
		}
		fields := map[string]bool{}
		for _, f := range st.Fields {
			if f == nil || f.Name == nil {
				continue
			}
			if addressTypes[typeSpecIdentName(f.Type)] {
				fields[f.Name.String()] = true
			}
		}
		if len(fields) > 0 {
			out[st.Name.String()] = fields
		}
	}
	return out
}

func collectAddressListTypes(mod *syntax.Module, addressTypes map[string]bool) map[string]bool {
	out := map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		st, ok := d.Def.(*syntax.SubTypeDecl)
		if !ok || st == nil || st.Field == nil || st.Field.Name == nil {
			continue
		}
		ls, ok := st.Field.Type.(*syntax.ListSpec)
		if !ok || ls == nil {
			continue
		}
		if addressTypes[typeSpecIdentName(ls.ElemType)] {
			out[st.Field.Name.String()] = true
		}
	}
	return out
}

func nullAddressFieldsInInitializer(expr syntax.Expr, addressFields map[string]bool) map[string]bool {
	out := map[string]bool{}
	if len(addressFields) == 0 {
		return out
	}
	cl, ok := expr.(*syntax.CompositeLiteral)
	if !ok || cl == nil {
		return out
	}
	for _, item := range cl.List {
		be, ok := item.(*syntax.BinaryExpr)
		if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
			continue
		}
		field := identName(be.X)
		if field == "" || !addressFields[field] {
			continue
		}
		if isNullLiteral(be.Y) {
			out[field] = true
		}
	}
	return out
}

func compositeHasNullIndex(expr syntax.Expr) bool {
	cl, ok := expr.(*syntax.CompositeLiteral)
	if !ok || cl == nil {
		return false
	}
	for _, item := range cl.List {
		be, ok := item.(*syntax.BinaryExpr)
		if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
			continue
		}
		if _, ok := be.X.(*syntax.IndexExpr); ok && isNullLiteral(be.Y) {
			return true
		}
	}
	return false
}

func nullAddressReadRef(
	expr syntax.Expr,
	nullFields map[string]map[string]bool,
	nullIndexes map[string]bool,
) string {
	switch x := expr.(type) {
	case *syntax.SelectorExpr:
		if x == nil {
			return ""
		}
		base := identName(x.X)
		field := identName(x.Sel)
		if base != "" && field != "" && nullFields[base][field] {
			return base + "." + field
		}
	case *syntax.IndexExpr:
		if x == nil {
			return ""
		}
		base := identName(x.X)
		if base != "" && nullIndexes[base] {
			return base + "[...]"
		}
	}
	return ""
}

func typeSpecIdentName(ts syntax.TypeSpec) string {
	ref, ok := ts.(*syntax.RefSpec)
	if !ok || ref == nil {
		return ""
	}
	return identName(ref.X)
}

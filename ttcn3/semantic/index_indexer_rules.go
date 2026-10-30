// index_indexer_rules.go enforces ETSI ES 201 873-1 clause 6.2.3
// / 6.2.7 short-hand index notation: when an array / record-of /
// set-of element is referenced by `arr[v]` where `v` is itself a
// structured value, `v` must be an array or `record of integer`
// constrained to a single fixed length. Other shapes are
// rejected:
//
//   - `set [length(N)] of integer` - set kind is unordered and
//     cannot describe a tuple of indices.
//   - `record of integer` with no length / a length range - the
//     dimensionality of the index would be undefined.
//   - non-integer element types (charstring, etc.).
//
// The check fires only when the indexer variable is declared in
// the current module so its type is statically resolvable.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkIndexIndexerRules(mod *syntax.Module) []Diagnostic {
	indexers := collectIndexerVarKinds(mod)
	if len(indexers) == 0 {
		return nil
	}
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ix, ok := n.(*syntax.IndexExpr)
		if !ok || ix == nil || ix.Index == nil {
			return true
		}
		id, ok := ix.Index.(*syntax.Ident)
		if !ok || id == nil {
			return true
		}
		info, ok := indexers[id.String()]
		if !ok || info.reason == "" {
			return true
		}
		diags = append(diags, Diagnostic{
			Code:     "indexer-invalid-shape",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"indexer %q used in `arr[%s]` is %s; multi-dim indexers must be `record of integer` (or integer array) constrained to a single fixed length (ETSI 6.2.3 / 6.2.7)",
				id.String(), id.String(), info.reason),
			Node: ix,
			Span: syntax.SpanOf(ix),
		})
		return true
	})
	return diags
}

// indexerInfo records the per-variable verdict for use as an
// index expression. reason == "" means the variable is acceptable
// (or not a candidate indexer at all, e.g. a scalar integer).
type indexerInfo struct {
	reason string
}

// collectIndexerVarKinds inspects every variable declaration that
// could appear as an `arr[v]` index value. Scalars and template
// variables are passed through (reason left empty); structured
// candidates are classified.
func collectIndexerVarKinds(mod *syntax.Module) map[string]indexerInfo {
	subtypes := collectIndexerSubtypeKinds(mod)
	out := map[string]indexerInfo{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil || vd.KindTok == nil ||
			vd.KindTok.Kind() != syntax.VAR {
			return true
		}
		if vd.TemplateRestriction != nil &&
			vd.TemplateRestriction.TemplateTok != nil &&
			vd.TemplateRestriction.TemplateTok.Kind() != syntax.ILLEGAL {
			return true
		}
		declType := identName(vd.Type)
		for _, d := range vd.Decls {
			if d == nil || d.Name == nil {
				continue
			}
			name := d.Name.String()
			// `var integer x[N]` is a valid indexer
			// when there is exactly one [N] dimension
			// with a single integer literal. Anything
			// else (multiple dims, range, non-integer
			// elem) is rejected.
			if len(d.ArrayDef) > 0 {
				if declType != "integer" {
					out[name] = indexerInfo{
						reason: fmt.Sprintf(
							"an array of %q (not integer)", declType),
					}
					continue
				}
				if len(d.ArrayDef) != 1 || !isSingleIntDimension(d.ArrayDef[0]) {
					out[name] = indexerInfo{
						reason: "a multi-dimensional or variable-size array",
					}
				}
				continue
			}
			if info, ok := subtypes[declType]; ok && info.reason != "" {
				out[name] = info
			}
		}
		return true
	})
	return out
}

// collectIndexerSubtypeKinds builds a map from subtype name to
// the indexer verdict for its base list type. Only subtypes whose
// base is a `record`/`set` of integer are emitted; everything
// else is omitted (the caller treats absent entries as "not a
// known indexer candidate").
func collectIndexerSubtypeKinds(mod *syntax.Module) map[string]indexerInfo {
	out := map[string]indexerInfo{}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		st, ok := d.Def.(*syntax.SubTypeDecl)
		if !ok || st == nil || st.Field == nil || st.Field.Name == nil {
			continue
		}
		ls, ok := st.Field.Type.(*syntax.ListSpec)
		if !ok || ls == nil || ls.ElemType == nil {
			continue
		}
		elemName := ""
		if rs, ok := ls.ElemType.(*syntax.RefSpec); ok && rs != nil {
			elemName = identName(rs.X)
		}
		if elemName != "integer" {
			// non-integer element types are also
			// invalid indexers, but flagging them in
			// every project would be too noisy; restrict
			// to integer-shaped list types only.
			continue
		}
		kind := ""
		if ls.KindTok != nil {
			kind = ls.KindTok.String()
		}
		switch kind {
		case "set":
			out[st.Field.Name.String()] = indexerInfo{
				reason: "a `set of integer` (unordered, not a valid index tuple)",
			}
		case "record":
			if !isSingleLenLengthExpr(ls.Length) {
				out[st.Field.Name.String()] = indexerInfo{
					reason: "a `record of integer` without a fixed single length",
				}
			}
		}
	}
	return out
}

// isSingleLenLengthExpr returns true if le is a LengthExpr whose
// Size is a single integer literal, e.g. `length(2)`. A range
// (`length(1..2)`) or missing length both return false.
func isSingleLenLengthExpr(le *syntax.LengthExpr) bool {
	if le == nil || le.Size == nil || le.Size.List == nil ||
		len(le.Size.List) != 1 {
		return false
	}
	item := le.Size.List[0]
	if be, ok := item.(*syntax.BinaryExpr); ok && be != nil && be.Op != nil &&
		be.Op.String() == ".." {
		return false
	}
	lit, ok := item.(*syntax.ValueLiteral)
	if !ok || lit == nil || lit.Tok == nil {
		return false
	}
	return lit.Tok.Kind() == syntax.INT
}

// isSingleIntDimension reports whether a single array dimension
// spec is `[N]` for a literal integer N.
func isSingleIntDimension(dim syntax.Expr) bool {
	pe, ok := dim.(*syntax.ParenExpr)
	if !ok || pe == nil || pe.List == nil || len(pe.List) != 1 {
		return false
	}
	lit, ok := pe.List[0].(*syntax.ValueLiteral)
	if !ok || lit == nil || lit.Tok == nil {
		return false
	}
	return lit.Tok.Kind() == syntax.INT
}

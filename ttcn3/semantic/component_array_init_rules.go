// component_array_init_rules.go enforces ETSI ES 201 873-1
// clauses 21.3.5 / 21.3.6 / 21.3.7 / 21.3.8:
//
//	"The ComponentArrayRef shall be a reference to a completely
//	 initialized component array."
//
// We flag the narrow, statically-decidable case of
//
//	var Comp v_arr[N];
//	v_arr[k] := Comp.create;        // only some indices assigned
//	any from v_arr.<op>             // <op> in alive/running/done/killed
//
// where at least one literal index 0..N-1 is never assigned to.
//
// To stay false-positive-free we restrict the rule to:
//   - declarations with a constant integer array-dimension
//     (`var T v[N]`),
//   - declarations with no initializer at the var site, and
//   - functions where every assignment to v[?] uses a literal int
//     index (any non-literal index disables the rule for v).
//
// Catches NegSem_210305_alive_operation_006 / NegSem_210306_
// running_operation_006 / NegSem_210307_done_operation_010 /
// NegSem_210308_killed_operation_001 and similar.
package semantic

import (
	"fmt"
	"strconv"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkComponentArrayInitRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		fn, ok := n.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			return true
		}
		decls := collectFixedArrayDecls(fn.Body)
		if len(decls) == 0 {
			return true
		}
		assigned, disabled := collectIndexedAssignments(fn.Body, decls)
		// Walk any-from comp-op usages of the array.
		syntax.Inspect(fn.Body, func(sn syntax.Node) bool {
			fe, ok := sn.(*syntax.FromExpr)
			if !ok || fe == nil || fe.KindTok == nil ||
				fe.KindTok.Kind() != syntax.ANYKW {
				return true
			}
			sel, ok := fe.X.(*syntax.SelectorExpr)
			if !ok || sel == nil {
				return true
			}
			op := identName(sel.Sel)
			if !isComponentArrayOp(op) {
				return true
			}
			name := identName(sel.X)
			if name == "" {
				return true
			}
			size, ok := decls[name]
			if !ok || disabled[name] {
				return true
			}
			set := assigned[name]
			for i := 0; i < size; i++ {
				if set[i] {
					continue
				}
				diags = append(diags, Diagnostic{
					Code:     "any-from-uninit-component-array",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"`any from %s.%s` requires a fully initialised component array; index %d is never assigned (ETSI 21.3.x)",
						name, op, i),
					Node: fe,
					Span: syntax.SpanOf(fe),
				})
				break
			}
			return true
		})
		return false
	})
	return diags
}

// isComponentArrayOp reports whether op is one of the component
// test operations that take a ComponentArrayRef on the left.
func isComponentArrayOp(op string) bool {
	switch op {
	case "alive", "running", "done", "killed":
		return true
	}
	return false
}

// collectFixedArrayDecls returns a map of variable name -> array
// size for every `var T v[N]` declaration without initializer in
// body. Anything more elaborate (multi-dim, non-literal dim, init
// value present) is skipped to keep the rule narrow.
func collectFixedArrayDecls(body syntax.Node) map[string]int {
	out := map[string]int{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil || vd.KindTok == nil {
			return true
		}
		if vd.KindTok.Kind() != syntax.VAR {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil {
				continue
			}
			if dec.Value != nil {
				// Initialized -> not statically a partial.
				continue
			}
			if len(dec.ArrayDef) != 1 {
				continue
			}
			size, ok := arrayDimSize(dec.ArrayDef[0])
			if !ok || size <= 0 {
				continue
			}
			out[dec.Name.String()] = size
		}
		return true
	})
	return out
}

// collectIndexedAssignments scans body for `v[k] := ...`
// assignments where v is one of the tracked arrays. Returns the
// set of literal indices assigned per array, plus a set of
// arrays whose tracking is disabled because they were touched in
// a way the rule cannot reason about (non-literal index, plain
// `v := ...`, &c).
func collectIndexedAssignments(body syntax.Node, decls map[string]int) (map[string]map[int]bool, map[string]bool) {
	assigned := make(map[string]map[int]bool, len(decls))
	disabled := make(map[string]bool, len(decls))
	for name := range decls {
		assigned[name] = map[int]bool{}
	}
	syntax.Inspect(body, func(n syntax.Node) bool {
		be, ok := n.(*syntax.BinaryExpr)
		if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
			return true
		}
		switch lhs := be.X.(type) {
		case *syntax.IndexExpr:
			name := identName(lhs.X)
			if _, tracked := decls[name]; !tracked {
				return true
			}
			idx, ok := literalIntValue(lhs.Index)
			if !ok {
				disabled[name] = true
				return true
			}
			assigned[name][idx] = true
		case *syntax.Ident:
			name := lhs.String()
			if _, tracked := decls[name]; tracked {
				// Whole-array reassignment - we cannot
				// classify the new shape, so disable.
				disabled[name] = true
			}
		}
		return true
	})
	return assigned, disabled
}

// arrayDimSize extracts the integer N from a `[N]` array-def
// ParenExpr. Returns (0,false) for any non-literal dimension.
func arrayDimSize(pe *syntax.ParenExpr) (int, bool) {
	if pe == nil || len(pe.List) != 1 {
		return 0, false
	}
	return literalIntValue(pe.List[0])
}

func literalIntValue(e syntax.Expr) (int, bool) {
	v, ok := e.(*syntax.ValueLiteral)
	if !ok || v == nil || v.Tok == nil {
		return 0, false
	}
	n, err := strconv.Atoi(v.Tok.String())
	if err != nil {
		return 0, false
	}
	return n, true
}

// index_redirect_array_rules.go checks `any from A.op` on component and
// timer arrays and its `-> @index value v` redirection (ETSI ES 201 873-1
// clauses 21.3.5 to 21.3.8 restrictions d/e and g/h, 23.5 c/d, 23.6 d/e):
//
//   - a single-dimensional array's index is stored in an integer
//     variable whose type allows storing the highest index;
//   - a multi-dimensional array's in an integer array or record of
//     integer of exactly as many elements as the array has dimensions,
//     whose type allows storing the highest index of all dimensions;
//   - the TimerArrayRef of `any from` is a timer array (23.5, 23.6).
//
// Arrays and targets resolve through the scope (LookupWithDB), not by
// name. What does not resolve, or has a size that is not a literal, is
// left alone. Port arrays are the dim rule's (index_redirect_dim_rules.go).
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkIndexRedirectArrayRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	add := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.FromExpr:
			id, op := anyFromOperand(x)
			if id == nil || (op != "running" && op != "timeout") {
				return true
			}
			if arr, ok := m.arrayOperand(id); ok && arr.timer && len(arr.dims) == 0 {
				add(x, "any-from-timer-not-array", fmt.Sprintf("any from %s.%s: %s is not a timer array (ETSI 23.5, 23.6)", id, op, id))
			}
		case *syntax.RedirectExpr:
			if x.IndexTok == nil || x.Index == nil {
				return true
			}
			from, ok := x.X.(*syntax.FromExpr)
			if !ok {
				return true
			}
			id, op := anyFromOperand(from)
			if id == nil {
				return true
			}
			arr, ok := m.arrayOperand(id)
			if !ok || len(arr.dims) == 0 || !arr.timer && !componentOp(op) || arr.timer && op != "running" && op != "timeout" {
				return true
			}
			clause := "21.3"
			if arr.timer {
				clause = "23"
			}
			target, ok := m.indexTarget(x.Index, mod)
			if !ok {
				return true
			}
			switch {
			case len(arr.dims) == 1 && target.dims > 0:
				add(x, "index-redirect-array-shape", fmt.Sprintf("@index value %s: %s is single-dimensional, so the index is stored in an integer variable (ETSI %s)", x.Index, id, clause))
				return true
			case len(arr.dims) > 1 && (target.dims > 1 || target.dims == 0 && !target.list):
				add(x, "index-redirect-array-shape", fmt.Sprintf("@index value %s: %s has %d dimensions, so the index is stored in an integer array or record of integer (ETSI %s)", x.Index, id, len(arr.dims), clause))
				return true
			case len(arr.dims) > 1 && target.size > 0 && target.size != len(arr.dims):
				add(x, "index-redirect-length-mismatch", fmt.Sprintf("@index value %s: %d elements, but %s has %d dimensions (ETSI %s)", x.Index, target.size, id, len(arr.dims), clause))
				return true
			}
			highest, ok := arr.highestIndex()
			if ok && target.ranged && !target.rng.contains(highest) {
				add(x, "index-redirect-type-range", fmt.Sprintf("@index value %s: its type cannot store index %d of %s (ETSI %s)", x.Index, highest, id, clause))
			}
		}
		return true
	})
	return diags
}

// componentOp reports whether op is a component operation an `any from`
// applies to a component array with.
func componentOp(op string) bool {
	switch op {
	case "alive", "running", "done", "killed":
		return true
	}
	return false
}

// anyFromOperand is the array A and the operation op of `any from A.op`
// or `any from A.op()`.
func anyFromOperand(x *syntax.FromExpr) (*syntax.Ident, string) {
	if x.KindTok == nil || x.KindTok.Kind() != syntax.ANYKW {
		return nil, ""
	}
	e := x.X
	if call, ok := e.(*syntax.CallExpr); ok {
		e = call.Fun
	}
	sel, ok := e.(*syntax.SelectorExpr)
	if !ok || sel.Sel == nil {
		return nil, ""
	}
	id, ok := sel.X.(*syntax.Ident)
	if !ok {
		return nil, ""
	}
	return id, syntax.Name(sel.Sel)
}

// arrayOperand is a timer or a variable of a component type: whether it is
// a timer, and its dimensions, each the range of its indices or unknown.
type arrayOperand struct {
	timer bool
	dims  []*intRange
}

// highestIndex is the highest index of all dimensions, if all are known.
func (a arrayOperand) highestIndex() (int64, bool) {
	var highest int64 = -1
	for _, d := range a.dims {
		if d == nil {
			return 0, false
		}
		highest = max(highest, d.hi)
	}
	return highest, highest >= 0
}

func (m typeModel) arrayOperand(id *syntax.Ident) (arrayOperand, bool) {
	vd, decl, def, ok := m.valueDeclOf(id)
	if !ok {
		return arrayOperand{}, false
	}
	var arr arrayOperand
	switch {
	case isTimerType(vd.Type):
		// `timer t[n]` is declared without a kind token.
		arr.timer = true
	case vd.KindTok != nil && vd.KindTok.Kind() == syntax.VAR:
		defs := def.Tree.LookupWithDB(vd.Type, m.db)
		if len(defs) != 1 {
			return arrayOperand{}, false
		}
		if _, ok := defs[0].Node.(*syntax.ComponentTypeDecl); !ok {
			return arrayOperand{}, false
		}
	default:
		return arrayOperand{}, false
	}
	for _, p := range decl.ArrayDef {
		arr.dims = append(arr.dims, arrayDimRange(p))
	}
	return arr, true
}

// arrayDimRange is the range of indices of an array dimension `[n]`
// (0..n-1) or `[lo..hi]`, both of literals; nil otherwise.
func arrayDimRange(p *syntax.ParenExpr) *intRange {
	if p == nil || len(p.List) != 1 {
		return nil
	}
	if n, ok := signedIntLiteral(p.List[0]); ok {
		if n <= 0 {
			return nil
		}
		return &intRange{0, n - 1}
	}
	if r, ok := literalIntRange(p.List[0]); ok && r.lo <= r.hi {
		return &r
	}
	return nil
}

// indexTargetInfo is the variable of an index redirection: its array
// dimensions, the size of its only one if a literal, whether it is of a
// record of type, and the range of literals its integer (element) type is
// restricted to.
type indexTargetInfo struct {
	dims, size int
	list       bool
	ranged     bool
	rng        intRange
}

func (m typeModel) indexTarget(e syntax.Expr, mod *syntax.Module) (indexTargetInfo, bool) {
	id, ok := e.(*syntax.Ident)
	if !ok {
		return indexTargetInfo{}, false
	}
	vd, decl, def, ok := m.valueDeclOf(id)
	if !ok || vd.KindTok == nil || vd.KindTok.Kind() != syntax.VAR || vd.Type == nil {
		return indexTargetInfo{}, false
	}
	t := indexTargetInfo{dims: len(decl.ArrayDef)}
	if t.dims == 1 {
		if r := arrayDimRange(decl.ArrayDef[0]); r != nil && r.lo == 0 {
			t.size = int(r.hi + 1)
		}
	}
	root, ok := m.typeRoot(def.Tree, vd.Type, 0)
	if !ok {
		return indexTargetInfo{}, false
	}
	switch root.kind {
	case "integer":
		// A subtype declared in this module (mod is m.tree's).
		if def.Tree == m.tree {
			t.rng, t.ranged = integerSubtypeRange(syntax.Name(vd.Type), mod)
		}
	case "record of", "set of":
		t.list = true
	default:
		return indexTargetInfo{}, false
	}
	return t, true
}

// valueDeclOf is the declaration of the value, timer or port id refers to,
// its declarator, and where it is.
func (m typeModel) valueDeclOf(id *syntax.Ident) (*syntax.ValueDecl, *syntax.Declarator, *ttcn3.Node, bool) {
	defs := m.tree.LookupWithDB(id, m.db)
	if len(defs) != 1 {
		return nil, nil, nil, false
	}
	def := defs[0]
	switch d := def.Node.(type) {
	case *syntax.ValueDecl:
		for _, x := range d.Decls {
			if x != nil && x.Name != nil && x.Name.String() == id.String() {
				return d, x, def, true
			}
		}
	case *syntax.Declarator:
		if vd, ok := def.ParentOf(d).(*syntax.ValueDecl); ok {
			return vd, d, def, true
		}
	}
	return nil, nil, nil, false
}

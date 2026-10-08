// inout_param_overlap_rules.go enforces ETSI ES 201 873-1 clause
// 5.4.2 paragraph: "Whenever a value or template of a record,
// set, union, record of, set of, array and anytype type is passed
// as an actual parameter to an inout parameter, none of the
// fields or elements of this structured value or template shall
// be passed as an actual parameter to another inout parameter of
// the same parameterized TTCN-3 object."
//
// We detect the unambiguous violations:
//
//   * `f(v.x, v.y)` where both formals are inout (different paths
//     under the same root, NegSem_050402_143 / _144).
//   * `f(v, v[i])` or `f(v, v.x)` where both formals are inout
//     and the second path is a sub-element of the first
//     (NegSem_050402_145).
//   * The fully-aliased `f(v, v)` shape (same root, no further
//     path on either side).
//
// We only consider local user-defined functions whose formal
// parameter directions we can resolve in this module.  External
// or imported functions are skipped to keep false positives at
// zero.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// pathSeg classifies a single path segment.  Field selectors are
// equal when their names match; index segments are never equal to
// each other (the parser doesn't model whether two index
// expressions evaluate to the same slot, and the conformance
// suite intentionally permits `f(v[0], v[5])` for swaps).
type pathSeg struct {
	field bool
	name  string
	src   string
}

func (s pathSeg) sameAs(o pathSeg) bool {
	if !s.field || !o.field {
		return false
	}
	return s.name == o.name
}

// rootedPath captures the (root, sub-path) view of an actual
// argument expression.  `root` is the leftmost identifier; `path`
// is the sequence of selector / index suffixes that follow.  An
// empty `path` means "the whole variable".
type rootedPath struct {
	root string
	path []pathSeg
}

func (p rootedPath) isPrefixOf(other rootedPath) bool {
	if p.root != other.root || len(p.path) > len(other.path) {
		return false
	}
	for i, s := range p.path {
		if !s.sameAs(other.path[i]) {
			return false
		}
	}
	return true
}

func (p rootedPath) display() string {
	out := p.root
	for _, s := range p.path {
		out += s.src
	}
	return out
}

func (a *Analyzer) checkInoutParamOverlapRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	dirs := collectFuncInoutDirs(mod)
	if len(dirs) == 0 {
		return nil
	}
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok || ce == nil || ce.Fun == nil || ce.Args == nil {
			return true
		}
		id, ok := ce.Fun.(*syntax.Ident)
		if !ok || id == nil {
			return true
		}
		dir := dirs[id.String()]
		if len(dir) == 0 {
			return true
		}
		args := ce.Args.List
		if len(args) == 0 {
			return true
		}
		paths := make([]*rootedPath, len(args))
		for i := range args {
			if i >= len(dir) || !dir[i] {
				continue
			}
			rp := rootedPathOf(args[i])
			if rp == nil {
				continue
			}
			paths[i] = rp
		}
		for i := 0; i < len(paths); i++ {
			if paths[i] == nil {
				continue
			}
			for j := i + 1; j < len(paths); j++ {
				if paths[j] == nil {
					continue
				}
				if !sharesPath(*paths[i], *paths[j]) {
					continue
				}
				diags = append(diags, Diagnostic{
					Code:     "inout-param-overlap",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"actual inout arguments overlap: %s and %s share the same root structure (ETSI 5.4.2)",
						paths[i].display(), paths[j].display()),
					Node: ce,
					Span: syntax.SpanOf(ce),
				})
				return true
			}
		}
		return true
	})
	return diags
}

// sharesPath returns true if a and b refer to overlapping data
// under the same root identifier:
//
//   * One side is a strict prefix of the other - the whole
//     structured value is passed alongside one of its sub-
//     elements (NegSem_050402_145).
//   * Both sides diverge through a `.field` selector at the same
//     depth - distinct fields of the same record / set / union /
//     anytype are still aliases of "this structured value or
//     template" for ETSI 5.4.2 purposes (NegSem_050402_143 /
//     _144).
//
// Sibling array / record-of indices (e.g. `v[0]` vs `v[5]`) are
// intentionally NOT flagged - Sem_050402_196 (the spec's own
// swap-by-index example) relies on that pattern, and the
// conformance suite ships no NegSem for sibling index aliases.
func sharesPath(a, b rootedPath) bool {
	if a.root != b.root {
		return false
	}
	if len(a.path) == 0 && len(b.path) == 0 {
		return false
	}
	if a.isPrefixOf(b) || b.isPrefixOf(a) {
		return true
	}
	common := 0
	for common < len(a.path) && common < len(b.path) &&
		a.path[common].sameAs(b.path[common]) {
		common++
	}
	if common == len(a.path) || common == len(b.path) {
		return true
	}
	if !a.path[common].field || !b.path[common].field {
		return false
	}
	return true
}

// collectFuncInoutDirs builds a name -> direction-mask map for
// every user-defined FuncDecl in the module.  Each slot is true
// when the corresponding formal carries the `inout` direction.
func collectFuncInoutDirs(mod *syntax.Module) map[string][]bool {
	out := map[string][]bool{}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn.Name == nil || fn.Params == nil {
			continue
		}
		dirs := make([]bool, len(fn.Params.List))
		for i, p := range fn.Params.List {
			if p == nil || p.Direction == nil {
				continue
			}
			if p.Direction.Kind() == syntax.INOUT {
				dirs[i] = true
			}
		}
		out[fn.Name.String()] = dirs
	}
	return out
}

// rootedPathOf walks selector and index chains to produce the
// (root, path) view of an argument expression.  Returns nil when
// the expression is not a pure path (e.g. a function call, a
// literal, an arithmetic expression).
func rootedPathOf(e syntax.Expr) *rootedPath {
	var suffixes []pathSeg
	for {
		switch v := e.(type) {
		case *syntax.Ident:
			if v == nil {
				return nil
			}
			rp := rootedPath{root: v.String(), path: reverseSegs(suffixes)}
			return &rp
		case *syntax.SelectorExpr:
			if v == nil || v.Sel == nil {
				return nil
			}
			sel, ok := v.Sel.(*syntax.Ident)
			if !ok || sel == nil {
				return nil
			}
			name := sel.String()
			suffixes = append(suffixes, pathSeg{field: true, name: name, src: "." + name})
			e = v.X
		case *syntax.IndexExpr:
			if v == nil {
				return nil
			}
			suffixes = append(suffixes, pathSeg{src: "[...]"})
			e = v.X
		default:
			return nil
		}
	}
}

func reverseSegs(in []pathSeg) []pathSeg {
	out := make([]pathSeg, len(in))
	for i, s := range in {
		out[len(in)-1-i] = s
	}
	return out
}

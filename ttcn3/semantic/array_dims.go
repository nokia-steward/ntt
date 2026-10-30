// array_dims.go implements ETSI ES 201 873-1 clause 6.2.7: array
// dimensions must be constant expressions that evaluate to a
// positive integer. Bound expressions in a range (`lo .. hi`) must
// both be non-negative.
//
// We perform a small interpretation pass for literals and named
// constants. Unknown expressions are not flagged.
package semantic

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkArrayDims(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic

	consts := collectIntegerConsts(mod)
	binds := collectIntegerBindings(mod)

	walk := func(arrayDefs []*syntax.ParenExpr) {
		for _, pe := range arrayDefs {
			if pe == nil {
				continue
			}
			for _, dim := range pe.List {
				diags = append(diags, checkOneArrayDim(dim, consts, binds)...)
			}
		}
	}

	syntax.Inspect(mod, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		switch x := n.(type) {
		case *syntax.Declarator:
			// `var T x[N]` and similar variable / const
			// declarators.
			walk(x.ArrayDef)
		case *syntax.Field:
			// `field T a[N]` inside a record/set/union, and
			// the top-level Field of a `type integer Arr[N]`
			// SubTypeDecl (the SubTypeDecl wraps a Field
			// rather than a Declarator).
			walk(x.ArrayDef)
		}
		return true
	})
	return diags
}

// bindingKind classifies an integer-typed identifier with respect to
// the array-dimension constness requirement.
type bindingKind int

const (
	bindUnknown   bindingKind = iota
	bindConstLit              // `const int N := <literal-expr>` - usable
	bindConstCall             // `const int N := f()` - non-constant init
	bindVar                   // `var int N` - mutable
	bindModulepar             // `modulepar int N` - runtime value
)

// checkOneArrayDim walks a single dimension expression and reports
// values that violate the positive-integer rule.
func checkOneArrayDim(expr syntax.Expr, consts map[string]int64, binds map[string]bindingKind) []Diagnostic {
	var diags []Diagnostic
	report := func(code, msg string, n syntax.Node) {
		diags = append(diags, Diagnostic{
			Code:     code,
			Severity: SeverityError,
			Message:  msg,
			Node:     n,
			Span:     syntax.SpanOf(n),
		})
	}
	checkIdent := func(e *syntax.Ident) {
		if e == nil || e.Tok == nil {
			return
		}
		switch binds[e.String()] {
		case bindVar:
			report("array-dim-non-const",
				fmt.Sprintf("array dimension %q is a variable; must be a constant expression", e.String()), e)
		case bindModulepar:
			report("array-dim-non-const",
				fmt.Sprintf("array dimension %q is a modulepar; must be a constant expression", e.String()), e)
		case bindConstCall:
			report("array-dim-non-const",
				fmt.Sprintf("array dimension %q is a const initialised by a function call; must be a compile-time expression", e.String()), e)
		}
	}
	switch e := expr.(type) {
	case *syntax.BinaryExpr:
		if e.Op != nil && e.Op.Kind() == syntax.RANGE {
			// ETSI ES 201 873-1 clause 6.2.7: an array
			// dimension must evaluate to a positive integer.
			// For a `lo..hi` range that means both bounds
			// must be strictly positive (NegSem_060207_017
			// rejects `[0..2]`) and `hi >= lo`.
			if v, ok := evalConstInt(e.X, consts); ok && v <= 0 {
				report("array-dim-non-positive",
					fmt.Sprintf("array range lower bound must be positive, got %d", v), e.X)
			}
			if v, ok := evalConstInt(e.Y, consts); ok && v <= 0 {
				report("array-dim-non-positive",
					fmt.Sprintf("array range upper bound must be positive, got %d", v), e.Y)
			}
			if lo, lok := evalConstInt(e.X, consts); lok {
				if hi, hok := evalConstInt(e.Y, consts); hok && hi < lo {
					report("array-dim-non-positive",
						fmt.Sprintf("array range upper bound %d is below lower bound %d",
							hi, lo), e)
				}
			}
			if id, ok := e.X.(*syntax.Ident); ok {
				checkIdent(id)
			}
			if id, ok := e.Y.(*syntax.Ident); ok {
				checkIdent(id)
			}
			return diags
		}
	case *syntax.Ident:
		checkIdent(e)
	}
	// Detect floats first - they're always invalid regardless of
	// value because TTCN-3 only allows integer array dimensions.
	if isFloatLiteral(expr) {
		report("array-dim-non-positive", "array dimension must be integer, got float", expr)
		return diags
	}
	if v, ok := evalConstInt(expr, consts); ok && v <= 0 {
		report("array-dim-non-positive",
			fmt.Sprintf("array dimension must be positive, got %d", v), expr)
	}
	return diags
}

// collectIntegerBindings classifies module-scope integer-typed
// declarations into the bindingKind taxonomy. Used by the array
// dimension constness check.
func collectIntegerBindings(mod *syntax.Module) map[string]bindingKind {
	out := map[string]bindingKind{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd.KindTok == nil {
			return true
		}
		var kind bindingKind
		switch vd.KindTok.Kind() {
		case syntax.CONST:
			kind = bindConstLit
		case syntax.MODULEPAR:
			kind = bindModulepar
		case syntax.VAR:
			kind = bindVar
		default:
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil {
				continue
			}
			thisKind := kind
			if kind == bindConstLit {
				// Refine: const initialised by a CallExpr
				// (or anything not reducible to a literal)
				// is non-constant for array-dim purposes.
				if dec.Value == nil {
					thisKind = bindConstCall
				} else if _, isCall := dec.Value.(*syntax.CallExpr); isCall {
					thisKind = bindConstCall
				} else if _, ok := evalConstInt(dec.Value, nil); !ok {
					// Not a literal we can fold - treat
					// as call-initialised so the array
					// dim check rejects.
					thisKind = bindConstCall
				}
			}
			out[dec.Name.String()] = thisKind
		}
		return true
	})
	return out
}

// collectIntegerConsts gathers the integer values of module-scope
// `const integer name := <literal>;` declarations. We only model
// literal integers and simple negation; unknown forms are skipped.
func collectIntegerConsts(mod *syntax.Module) map[string]int64 {
	out := map[string]int64{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd.KindTok == nil || vd.KindTok.Kind() != syntax.CONST {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil || dec.Value == nil {
				continue
			}
			if v, ok := evalConstInt(dec.Value, nil); ok {
				out[dec.Name.String()] = v
			}
		}
		return true
	})
	return out
}

// evalConstInt evaluates simple integer constant expressions:
// literal integers, named constants from the table, unary minus
// applied to one of those, and basic +/-/* binary ops. Returns
// (value, true) when the expression can be reduced.
func evalConstInt(expr syntax.Expr, consts map[string]int64) (int64, bool) {
	if expr == nil {
		return 0, false
	}
	switch e := expr.(type) {
	case *syntax.ValueLiteral:
		if e.Tok == nil {
			return 0, false
		}
		s := strings.TrimSpace(e.Tok.String())
		if s == "" {
			return 0, false
		}
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, false
		}
		return v, true
	case *syntax.Ident:
		if consts == nil || e.Tok == nil {
			return 0, false
		}
		v, ok := consts[e.String()]
		return v, ok
	case *syntax.UnaryExpr:
		if e.Op == nil {
			return 0, false
		}
		v, ok := evalConstInt(e.X, consts)
		if !ok {
			return 0, false
		}
		switch e.Op.Kind() {
		case syntax.SUB:
			return -v, true
		case syntax.ADD:
			return v, true
		}
	case *syntax.BinaryExpr:
		if e.Op == nil {
			return 0, false
		}
		l, lok := evalConstInt(e.X, consts)
		r, rok := evalConstInt(e.Y, consts)
		if !lok || !rok {
			return 0, false
		}
		switch e.Op.Kind() {
		case syntax.ADD:
			return l + r, true
		case syntax.SUB:
			return l - r, true
		case syntax.MUL:
			return l * r, true
		}
	case *syntax.ParenExpr:
		if len(e.List) == 1 {
			return evalConstInt(e.List[0], consts)
		}
	}
	return 0, false
}

// isFloatLiteral reports whether expr is a literal containing a
// `.` or exponent, i.e. a float value.
func isFloatLiteral(expr syntax.Expr) bool {
	if expr == nil {
		return false
	}
	if v, ok := expr.(*syntax.ValueLiteral); ok && v.Tok != nil {
		s := v.Tok.String()
		return strings.ContainsAny(s, ".eE")
	}
	return false
}

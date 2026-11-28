// timer_init_rules.go enforces ETSI ES 201 873-1 clause 12
// "Declaring timers": when a timer declaration carries a
// default duration it shall be a non-negative float literal.
// Notably,
//
//	timer t_timer := -1.0;
//
// is rejected because a timer duration must be >= 0.0 (the
// language doesn't support negative timer durations).
//
// We flag a directly visible negative literal (a UnaryExpr
// with a minus operator over a numeric ValueLiteral) and a
// float expression over literals and constants that folds to a
// negative value (`c_base - 1.0`). Expressions that compute the
// duration at runtime are left to the runtime semantic check.
package semantic

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkTimerInitRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		if !isTimerValueDecl(vd) {
			return true
		}
		for _, dc := range vd.Decls {
			if dc == nil || dc.Value == nil {
				continue
			}
			name := ""
			if dc.Name != nil {
				name = dc.Name.String()
			}
			// Timer arrays (`timer t[N] := { ... }`) carry a
			// value-list initialiser; validate it element-wise
			// instead of treating the whole `{...}` as a scalar
			// default duration.
			if len(dc.ArrayDef) > 0 {
				diags = append(diags, checkTimerArrayInit(dc, name)...)
				continue
			}
			if lit, ok := negativeNumericLiteral(dc.Value); ok {
				diags = append(diags, Diagnostic{
					Code:     "timer-negative-default",
					Severity: SeverityError,
					Message: "timer " + quoted(name) +
						": default duration " + lit +
						" must be non-negative (ETSI 12)",
					Node: dc,
					Span: syntax.SpanOf(dc),
				})
				continue
			}
			if tree != nil {
				if v, ok := m.constFloat(dc.Value, 0); ok && v < 0 {
					diags = append(diags, Diagnostic{
						Code:     "timer-negative-default",
						Severity: SeverityError,
						Message: "timer " + quoted(name) +
							": default duration " + sourceText(dc.Value) + " resolves to " +
							strconv.FormatFloat(v, 'g', -1, 64) + ", it must be non-negative (ETSI 12)",
						Node: dc,
						Span: syntax.SpanOf(dc),
					})
					continue
				}
			}
			if integerOnlyLiteral(dc.Value) {
				diags = append(diags, Diagnostic{
					Code:     "timer-non-float-default",
					Severity: SeverityError,
					Message: "timer " + quoted(name) +
						": default duration must be a float literal (e.g. 1.0), not an integer (ETSI 12)",
					Node: dc,
					Span: syntax.SpanOf(dc),
				})
			}
		}
		return true
	})
	return diags
}

// checkTimerArrayInit validates the initialiser of a timer array
// declarator `timer t[N] := <init>` against ETSI clause 12:
//
//   - the initialiser must be a value list `{ ... }`, not a single
//     value (`timer t[4] := 1.0` is rejected);
//   - the number of elements must match the declared dimension N;
//   - no element may be a negative duration literal.
func checkTimerArrayInit(dc *syntax.Declarator, name string) []Diagnostic {
	var diags []Diagnostic
	cl, ok := dc.Value.(*syntax.CompositeLiteral)
	if !ok {
		diags = append(diags, Diagnostic{
			Code:     "timer-array-non-list-init",
			Severity: SeverityError,
			Message: "timer array " + quoted(name) +
				": an array of timers must be initialised with a value list { ... }, not a single value (ETSI 12)",
			Node: dc,
			Span: syntax.SpanOf(dc),
		})
		return diags
	}
	if len(dc.ArrayDef) == 1 {
		if size, ok := arrayDimSize(dc.ArrayDef[0]); ok && size != len(cl.List) {
			diags = append(diags, Diagnostic{
				Code:     "timer-array-init-count",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"timer array %s: declared with %d element(s) but the initialiser supplies %d (ETSI 12)",
					quoted(name), size, len(cl.List)),
				Node: dc,
				Span: syntax.SpanOf(dc),
			})
		}
	}
	for _, e := range cl.List {
		if lit, ok := negativeNumericLiteral(e); ok {
			diags = append(diags, Diagnostic{
				Code:     "timer-negative-default",
				Severity: SeverityError,
				Message: "timer array " + quoted(name) +
					": default duration " + lit +
					" must be non-negative (ETSI 12)",
				Node: e,
				Span: syntax.SpanOf(e),
			})
		}
	}
	return diags
}

// constFloat is the value of the float expression e built from float
// literals and constants with + - * /, ok false when it is anything
// else.
func (m typeModel) constFloat(e syntax.Expr, depth int) (float64, bool) {
	if depth > 8 {
		return 0, false
	}
	switch x := e.(type) {
	case *syntax.ValueLiteral:
		if x.Tok == nil || x.Tok.Kind() != syntax.FLOAT {
			return 0, false
		}
		v, err := strconv.ParseFloat(x.Tok.String(), 64)
		return v, err == nil
	case *syntax.Ident:
		vd, dc, _, ok := m.valueDeclOf(x)
		if !ok || vd.KindTok == nil || vd.KindTok.Kind() != syntax.CONST || dc.Value == nil || len(dc.ArrayDef) > 0 {
			return 0, false
		}
		return m.constFloat(dc.Value, depth+1)
	case *syntax.ParenExpr:
		if len(x.List) == 1 {
			return m.constFloat(x.List[0], depth+1)
		}
	case *syntax.UnaryExpr:
		if x.Op == nil {
			return 0, false
		}
		v, ok := m.constFloat(x.X, depth+1)
		switch {
		case ok && x.Op.Kind() == syntax.SUB:
			return -v, true
		case ok && x.Op.Kind() == syntax.ADD:
			return v, true
		}
	case *syntax.BinaryExpr:
		if x.Op == nil {
			return 0, false
		}
		l, lok := m.constFloat(x.X, depth+1)
		r, rok := m.constFloat(x.Y, depth+1)
		if !lok || !rok {
			return 0, false
		}
		switch x.Op.Kind() {
		case syntax.ADD:
			return l + r, true
		case syntax.SUB:
			return l - r, true
		case syntax.MUL:
			return l * r, true
		case syntax.DIV:
			if r != 0 {
				return l / r, true
			}
		}
	}
	return 0, false
}

// isTimerValueDecl reports whether vd declares a timer. The
// parser stores `timer` either in `KindTok` (when a local
// declaration uses the kind-keyword form) or in `Type` as an
// identifier whose underlying token is TIMER (for the
// component-body shorthand `timer t := ...`).
func isTimerValueDecl(vd *syntax.ValueDecl) bool {
	if vd == nil {
		return false
	}
	if vd.KindTok != nil && vd.KindTok.Kind() == syntax.TIMER {
		return true
	}
	if id, ok := vd.Type.(*syntax.Ident); ok && id != nil && id.Tok != nil {
		if id.Tok.Kind() == syntax.TIMER {
			return true
		}
	}
	return false
}

// negativeNumericLiteral returns the printed form when expr is
// `-<number>` (UnaryExpr over a ValueLiteral that parses as a
// numeric token).
func negativeNumericLiteral(expr syntax.Expr) (string, bool) {
	ue, ok := expr.(*syntax.UnaryExpr)
	if !ok || ue == nil || ue.Op == nil {
		return "", false
	}
	if ue.Op.String() != "-" {
		return "", false
	}
	vl, ok := ue.X.(*syntax.ValueLiteral)
	if !ok || vl == nil || vl.Tok == nil {
		return "", false
	}
	s := vl.Tok.String()
	if !isNumericLiteralText(s) {
		return "", false
	}
	return "-" + s, true
}

// integerOnlyLiteral returns true when expr is a bare integer
// literal (no decimal point, no exponent) used as a timer
// default duration. Timer durations are floats in TTCN-3 and
// integers are not legal initialisers (ETSI 12).
func integerOnlyLiteral(expr syntax.Expr) bool {
	vl, ok := expr.(*syntax.ValueLiteral)
	if !ok || vl == nil || vl.Tok == nil {
		return false
	}
	s := vl.Tok.String()
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

func isNumericLiteralText(s string) bool {
	if s == "" {
		return false
	}
	hasDigit := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			hasDigit = true
			continue
		}
		if r == '.' || r == 'e' || r == 'E' || r == '+' || r == '-' {
			continue
		}
		return false
	}
	return hasDigit
}

func quoted(s string) string {
	if s == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteByte('"')
	sb.WriteString(s)
	sb.WriteByte('"')
	return sb.String()
}

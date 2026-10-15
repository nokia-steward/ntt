package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// checkTimerSyntaxRules catches a handful of malformed timer
// operation shapes that the recursive-descent parser tolerates
// by recovering as two unrelated statements:
//
//   - `t_timer stop;` parses as `t_timer; stop;` - a bare timer
//     ident as an expression statement is meaningless.
//   - `t_timer.start 1.0;` parses as `t_timer.start; 1.0;` - a
//     bare value-literal expression statement is meaningless.
//   - `timeout(t_timer);` parses as a function-style call on the
//     keyword `timeout` - which is a timer operation, not a
//     function. Same for `start(...)`, `stop(...)`, `read(...)`,
//     `running(...)` used as bare-identifier call targets.
//
// Each rule is intentionally narrow: it only fires when we can
// tie the misuse to a known timer identifier, so we don't blow
// up on look-alike constructs in unrelated cohorts.
func (a *Analyzer) checkTimerSyntaxRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	timers := collectModuleTimerIdents(mod)
	syntax.Inspect(mod, func(n syntax.Node) bool {
		fn, ok := n.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			return true
		}
		walkExprStmts(fn.Body, func(es *syntax.ExprStmt) {
			if es == nil || es.Expr == nil {
				return
			}
			// Bare timer ident at statement position
			// (e.g. `t_timer stop;` parses as two
			// statements; the first is just `t_timer`).
			if id, ok := es.Expr.(*syntax.Ident); ok && id != nil {
				name := id.String()
				if timers[name] {
					diags = append(diags, Diagnostic{
						Code:     "timer-bare-ident-stmt",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"bare timer reference `%s` is not a valid statement; missing `.start`/`.stop`/`.timeout`? (ETSI 23)",
							name),
						Node: es,
						Span: syntax.SpanOf(es),
					})
					return
				}
				// Bare `any timer` / `all timer` at
				// statement position (e.g. `any timer timeout;`
				// recovers as `any timer; timeout;`).
				if id.Tok != nil && id.Tok2 != nil {
					first, second := id.Tok.String(), id.Tok2.String()
					if (first == "any" || first == "all") && second == "timer" {
						diags = append(diags, Diagnostic{
							Code:     "timer-bare-ident-stmt",
							Severity: SeverityError,
							Message: fmt.Sprintf(
								"`%s timer` is not a valid statement on its own; missing `.timeout`/`.running`? (ETSI 23.7)",
								first),
							Node: es,
							Span: syntax.SpanOf(es),
						})
						return
					}
				}
				// Bare timer-op keyword as a stand-alone
				// statement (e.g. `running;`, `timeout;`,
				// `read;`). These are never valid TTCN-3.
				// `stop` is a valid statement (terminates
				// the current test component), so we skip
				// it; `start` is reserved for the start
				// statement and also skipped.
				switch name {
				case "running", "timeout", "read":
					diags = append(diags, Diagnostic{
						Code:     "timer-op-bare-stmt",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"bare `%s;` is not a valid statement; use `T.%s` on a timer reference (ETSI 23)",
							name, name),
						Node: es,
						Span: syntax.SpanOf(es),
					})
					return
				}
			}
			// Bare indexed timer reference at statement
			// position (e.g. `t_timers[1] start;` recovers as
			// `t_timers[1]; start;` and `t_timers[1] start(1.0);`
			// as `t_timers[1]; start(1.0);`). An array element
			// access standing alone is meaningless - it is the
			// missing-dot form of a timer operation.
			if ix, ok := es.Expr.(*syntax.IndexExpr); ok && ix != nil {
				if base, ok := ix.X.(*syntax.Ident); ok && base != nil && timers[base.String()] {
					diags = append(diags, Diagnostic{
						Code:     "timer-bare-ident-stmt",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"bare timer reference `%s[...]` is not a valid statement; missing `.start`/`.stop`/`.timeout`? (ETSI 23)",
							base.String()),
						Node: es,
						Span: syntax.SpanOf(es),
					})
					return
				}
			}
			// Bare value-literal at statement position
			// (e.g. `t_timer.start 1.0;` leaves `1.0;`
			// as a residual statement).
			if _, ok := es.Expr.(*syntax.ValueLiteral); ok {
				diags = append(diags, Diagnostic{
					Code:     "stray-value-literal-stmt",
					Severity: SeverityError,
					Message:  "stray literal at statement position; missing operator or punctuation",
					Node:     es,
					Span:     syntax.SpanOf(es),
				})
				return
			}
			// `timeout(t_timer);` etc - the timer ops
			// are NOT free-standing functions.
			if ce, ok := es.Expr.(*syntax.CallExpr); ok && ce != nil && ce.Fun != nil {
				if id, ok := ce.Fun.(*syntax.Ident); ok && id != nil {
					op := id.String()
					if isTimerOpKeyword(op) {
						diags = append(diags, Diagnostic{
							Code:     "timer-op-as-function",
							Severity: SeverityError,
							Message: fmt.Sprintf(
								"`%s(...)` is not a function; use `T.%s` on a timer reference (ETSI 23)",
								op, op),
							Node: ce,
							Span: syntax.SpanOf(ce),
						})
					}
				}
				// `T.start()` / `T[i].start()` with empty
				// parentheses is malformed: the start operation
				// takes either no parens (`T.start`) or a
				// duration (`T.start(d)`), never `()` (ETSI 23.2).
				if sel, ok := ce.Fun.(*syntax.SelectorExpr); ok && sel != nil {
					if op, ok := sel.Sel.(*syntax.Ident); ok && op != nil && op.String() == "start" {
						if (ce.Args == nil || len(ce.Args.List) == 0) && timerSelectorBase(sel.X, timers) {
							diags = append(diags, Diagnostic{
								Code:     "timer-start-empty-parens",
								Severity: SeverityError,
								Message:  "`start()` with empty parentheses is not valid; use `T.start` or `T.start(duration)` (ETSI 23.2)",
								Node:     ce,
								Span:     syntax.SpanOf(ce),
							})
						}
					}
				}
			}
			// `all.stop;` / `any.stop;` - the `all`/`any`
			// keyword must be qualified with `timer` (or
			// `component`/`port`) before a `.op` (ETSI 23.6).
			if sel, ok := es.Expr.(*syntax.SelectorExpr); ok && sel != nil {
				if base, ok := sel.X.(*syntax.Ident); ok && base != nil &&
					base.Tok != nil && base.Tok2 == nil {
					if bw := base.Tok.String(); bw == "all" || bw == "any" {
						if op, ok := sel.Sel.(*syntax.Ident); ok && op != nil {
							switch op.String() {
							case "stop", "timeout", "running", "read", "kill", "done", "killed", "alive":
								diags = append(diags, Diagnostic{
									Code:     "timer-all-unqualified",
									Severity: SeverityError,
									Message: fmt.Sprintf(
										"`%s.%s` is not valid; the `%s` keyword must be qualified, e.g. `%s timer.%s` (ETSI 23.6)",
										bw, op.String(), bw, bw, op.String()),
									Node: es,
									Span: syntax.SpanOf(es),
								})
							}
						}
					}
				}
			}
		})
		walkIfTimerTimeout(fn.Body, timers, &diags)
		return true
	})
	return diags
}

// walkIfTimerTimeout flags a `.timeout` timer operation used inside
// an `if` condition. The timeout operation is a standalone statement
// or alt guard and cannot be used as a boolean value (ETSI 23.6).
func walkIfTimerTimeout(n syntax.Node, timers map[string]bool, diags *[]Diagnostic) {
	syntax.Inspect(n, func(c syntax.Node) bool {
		ifs, ok := c.(*syntax.IfStmt)
		if !ok || ifs == nil || ifs.Cond == nil {
			return true
		}
		syntax.Inspect(ifs.Cond, func(e syntax.Node) bool {
			sel, ok := e.(*syntax.SelectorExpr)
			if !ok || sel == nil {
				return true
			}
			op, ok := sel.Sel.(*syntax.Ident)
			if !ok || op == nil || op.String() != "timeout" {
				return true
			}
			if timerSelectorBase(sel.X, timers) {
				*diags = append(*diags, Diagnostic{
					Code:     "timer-timeout-in-expression",
					Severity: SeverityError,
					Message:  "`.timeout` is a timer operation and cannot be used in a boolean expression (ETSI 23.6)",
					Node:     sel,
					Span:     syntax.SpanOf(sel),
				})
			}
			return true
		})
		return true
	})
}

// timerSelectorBase reports whether x is a reference to a known
// timer, either as a bare identifier (`t`) or an indexed array
// element (`t[i]`).
func timerSelectorBase(x syntax.Expr, timers map[string]bool) bool {
	switch v := x.(type) {
	case *syntax.Ident:
		return v != nil && timers[v.String()]
	case *syntax.IndexExpr:
		if v == nil {
			return false
		}
		if id, ok := v.X.(*syntax.Ident); ok && id != nil {
			return timers[id.String()]
		}
	}
	return false
}

// walkExprStmts visits every ExprStmt nested under n, depth-first.
// We need our own walker because syntax.Inspect on an ExprStmt's
// child Expr would also fire for sub-expressions that happen to
// be wrapped in an ExprStmt-like position - we only care about
// the actual statement nodes.
func walkExprStmts(n syntax.Node, visit func(*syntax.ExprStmt)) {
	if n == nil {
		return
	}
	syntax.Inspect(n, func(c syntax.Node) bool {
		if es, ok := c.(*syntax.ExprStmt); ok {
			visit(es)
		}
		return true
	})
}

// isTimerOpKeyword reports whether op is one of the bare timer
// operation names that get free-standing function-call abuse.
// `start` is excluded: it's already covered by the parser's
// own keyword handling for the start statement.
func isTimerOpKeyword(op string) bool {
	switch op {
	case "timeout", "stop", "read", "running":
		return true
	}
	return false
}

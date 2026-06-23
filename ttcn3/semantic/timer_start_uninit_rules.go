// timer_start_uninit_rules.go enforces ETSI ES 201 873-1
// clause 12 / 22.x: a `.start` operation invoked on a timer
// without a default duration must supply a duration argument.
// In other words, the timer must be initialised before it is
// started, either at declaration time (via `:= D`) or in the
// `start` call itself (via `t.start(D)`).
//
// Examples that are now rejected:
//
//	type component C { timer t; }   // no default duration
//	testcase TC() runs on C {
//	    t.start;                    // ← error, no duration
//	}
//
// Examples that remain accepted:
//
//	timer t := 1.0; t.start;        // default duration present
//	timer t;        t.start(2.0);   // explicit duration arg
//
// The check is intentionally narrow:
//   - Only resolves identifiers that are local-to-the-body
//     timer declarations OR component-typed timer members for
//     the caller's `runs on` component.
//   - Skips parameters, indexed timers (`t[0]`), qualified
//     references, and timers reached through `port.start` etc.
//   - Skips when the timer name can be re-assigned by another
//     `t := …` between declaration and start (we don't do
//     control-flow analysis; the spec requires the timer to be
//     statically defaulted or the start to carry an argument).
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkTimerStartUninitRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	compTimers := collectComponentTimers(mod)
	compArrTimers := collectComponentArrayTimers(mod)
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			continue
		}
		uninit := map[string]bool{}
		arrInfo := map[string]*timerArrayInfo{}
		if comp := runsOnComponent(fn); comp != "" {
			for name, hasDefault := range compTimers[comp] {
				if !hasDefault {
					uninit[name] = true
				}
			}
			for name, info := range compArrTimers[comp] {
				arrInfo[name] = info
			}
		}
		collectLocalTimers(fn.Body, uninit)
		collectLocalArrayTimers(fn.Body, arrInfo)
		walkTimerStarts(fn.Body, uninit, &diags)
		walkIndexedTimerStarts(fn.Body, arrInfo, &diags)
	}
	return diags
}

// timerArrayInfo records the initialisation state of a timer array
// so an indexed `.start` can be validated element-wise.
type timerArrayInfo struct {
	// wholeUninit is set when the array has no initialiser at all
	// (`timer t[N];`) - every element is then uninitialised.
	wholeUninit bool
	// dashElems holds the indices initialised with `-`
	// (`timer t[2] := {-, 1.0}`), which are uninitialised.
	dashElems map[int]bool
}

// timerArrayInfoFromDecl builds a timerArrayInfo for an array timer
// declarator, or returns nil when the declarator is not an array.
func timerArrayInfoFromDecl(dc *syntax.Declarator) *timerArrayInfo {
	if dc == nil || len(dc.ArrayDef) == 0 {
		return nil
	}
	info := &timerArrayInfo{dashElems: map[int]bool{}}
	if dc.Value == nil {
		info.wholeUninit = true
		return info
	}
	cl, ok := dc.Value.(*syntax.CompositeLiteral)
	if !ok {
		return info
	}
	for i, e := range cl.List {
		if isDashLiteral(e) {
			info.dashElems[i] = true
		}
	}
	return info
}

// isDashLiteral reports whether e is the `-` placeholder that leaves
// an array element uninitialised.
func isDashLiteral(e syntax.Expr) bool {
	vl, ok := e.(*syntax.ValueLiteral)
	if !ok || vl == nil || vl.Tok == nil {
		return false
	}
	return vl.Tok.String() == "-"
}

// collectComponentArrayTimers maps each component type to the array
// timer members it declares, keyed by member name.
func collectComponentArrayTimers(mod *syntax.Module) map[string]map[string]*timerArrayInfo {
	out := map[string]map[string]*timerArrayInfo{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		ctd, ok := d.Def.(*syntax.ComponentTypeDecl)
		if !ok || ctd == nil || ctd.Name == nil || ctd.Body == nil {
			continue
		}
		timers := map[string]*timerArrayInfo{}
		for _, st := range ctd.Body.Stmts {
			ds, ok := st.(*syntax.DeclStmt)
			if !ok || ds == nil {
				continue
			}
			vd, ok := ds.Decl.(*syntax.ValueDecl)
			if !ok || vd == nil || !isTimerValueDecl(vd) {
				continue
			}
			for _, dc := range vd.Decls {
				if dc == nil || dc.Name == nil {
					continue
				}
				if info := timerArrayInfoFromDecl(dc); info != nil {
					timers[dc.Name.String()] = info
				}
			}
		}
		out[ctd.Name.String()] = timers
	}
	return out
}

// collectLocalArrayTimers records array timer declarations found in a
// function body into arrInfo.
func collectLocalArrayTimers(body *syntax.BlockStmt, arrInfo map[string]*timerArrayInfo) {
	if body == nil {
		return
	}
	syntax.Inspect(body, func(n syntax.Node) bool {
		ds, ok := n.(*syntax.DeclStmt)
		if !ok || ds == nil {
			return true
		}
		vd, ok := ds.Decl.(*syntax.ValueDecl)
		if !ok || vd == nil || !isTimerValueDecl(vd) {
			return true
		}
		for _, dc := range vd.Decls {
			if dc == nil || dc.Name == nil {
				continue
			}
			if info := timerArrayInfoFromDecl(dc); info != nil {
				arrInfo[dc.Name.String()] = info
			}
		}
		return true
	})
}

// walkIndexedTimerStarts flags `t[k].start` where t[k] is an
// uninitialised element of a timer array (whole array undeclared-
// default, or element k explicitly `-`).
func walkIndexedTimerStarts(body *syntax.BlockStmt, arrInfo map[string]*timerArrayInfo, diags *[]Diagnostic) {
	if body == nil || len(arrInfo) == 0 {
		return
	}
	syntax.Inspect(body, func(n syntax.Node) bool {
		es, ok := n.(*syntax.ExprStmt)
		if !ok || es == nil || es.Expr == nil {
			return true
		}
		sel, ok := es.Expr.(*syntax.SelectorExpr)
		if !ok || sel == nil {
			return true
		}
		selID, ok := sel.Sel.(*syntax.Ident)
		if !ok || selID == nil || selID.String() != "start" {
			return true
		}
		ix, ok := sel.X.(*syntax.IndexExpr)
		if !ok || ix == nil {
			return true
		}
		base, ok := ix.X.(*syntax.Ident)
		if !ok || base == nil {
			return true
		}
		info := arrInfo[base.String()]
		if info == nil {
			return true
		}
		bad := info.wholeUninit
		if !bad {
			if idx, ok := literalIntValue(ix.Index); ok && info.dashElems[idx] {
				bad = true
			}
		}
		if !bad {
			return true
		}
		*diags = append(*diags, Diagnostic{
			Code:     "timer-start-uninitialised",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"timer array element %q has no default duration and the `.start` operation is invoked without an argument (ETSI 12)",
				base.String()),
			Node: es,
			Span: syntax.SpanOf(es),
		})
		return true
	})
}

func collectComponentTimers(mod *syntax.Module) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		ctd, ok := d.Def.(*syntax.ComponentTypeDecl)
		if !ok || ctd == nil || ctd.Name == nil || ctd.Body == nil {
			continue
		}
		timers := map[string]bool{}
		for _, st := range ctd.Body.Stmts {
			ds, ok := st.(*syntax.DeclStmt)
			if !ok || ds == nil {
				continue
			}
			vd, ok := ds.Decl.(*syntax.ValueDecl)
			if !ok || vd == nil || !isTimerValueDecl(vd) {
				continue
			}
			for _, dc := range vd.Decls {
				if dc == nil || dc.Name == nil {
					continue
				}
				timers[dc.Name.String()] = dc.Value != nil
			}
		}
		out[ctd.Name.String()] = timers
	}
	return out
}

func collectLocalTimers(body *syntax.BlockStmt, uninit map[string]bool) {
	if body == nil {
		return
	}
	syntax.Inspect(body, func(n syntax.Node) bool {
		ds, ok := n.(*syntax.DeclStmt)
		if !ok || ds == nil {
			return true
		}
		vd, ok := ds.Decl.(*syntax.ValueDecl)
		if !ok || vd == nil || !isTimerValueDecl(vd) {
			return true
		}
		for _, dc := range vd.Decls {
			if dc == nil || dc.Name == nil {
				continue
			}
			if dc.Value == nil {
				uninit[dc.Name.String()] = true
			} else {
				delete(uninit, dc.Name.String())
			}
		}
		return true
	})
}

func walkTimerStarts(body *syntax.BlockStmt, uninit map[string]bool, diags *[]Diagnostic) {
	if body == nil || len(uninit) == 0 {
		return
	}
	syntax.Inspect(body, func(n syntax.Node) bool {
		es, ok := n.(*syntax.ExprStmt)
		if !ok || es == nil || es.Expr == nil {
			return true
		}
		sel, ok := es.Expr.(*syntax.SelectorExpr)
		if !ok || sel == nil {
			return true
		}
		selID, ok := sel.Sel.(*syntax.Ident)
		if !ok || selID == nil || selID.String() != "start" {
			return true
		}
		base, ok := sel.X.(*syntax.Ident)
		if !ok || base == nil {
			return true
		}
		if !uninit[base.String()] {
			return true
		}
		*diags = append(*diags, Diagnostic{
			Code:     "timer-start-uninitialised",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"timer %q has no default duration and the `.start` operation is invoked without an argument (ETSI 12)",
				base.String()),
			Node: es,
			Span: syntax.SpanOf(es),
		})
		return true
	})
}

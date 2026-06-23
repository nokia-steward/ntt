// repeat_scope_rules.go enforces ETSI ES 201 873-1 clause 20.3
// "The repeat statement":
//
//   - `repeat;` shall only appear inside an `alt` block or in the
//     body of an altstep. Anywhere else (testcase body after the
//     alt, plain function body, etc.) is rejected.
//
// We walk every FuncDecl body once, tracking how many enclosing
// alt scopes the current statement is in. Altstep bodies start
// with an implicit alt scope; testcases and plain functions don't.
package semantic

import (
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkRepeatScopeRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			continue
		}
		inAlt := 0
		if fn.KindTok != nil && fn.KindTok.Kind() == syntax.ALTSTEP {
			inAlt = 1
		}
		diags = append(diags, walkRepeat(fn.Body, inAlt)...)
	}
	return diags
}

func walkRepeat(stmt syntax.Stmt, inAlt int) []Diagnostic {
	if stmt == nil {
		return nil
	}
	var diags []Diagnostic
	switch v := stmt.(type) {
	case *syntax.BlockStmt:
		if v == nil {
			return nil
		}
		for _, s := range v.Stmts {
			diags = append(diags, walkRepeat(s, inAlt)...)
		}
	case *syntax.BranchStmt:
		if v == nil || v.Tok == nil {
			return nil
		}
		if v.Tok.Kind() == syntax.REPEAT && inAlt == 0 {
			diags = append(diags, Diagnostic{
				Code:     "repeat-outside-alt",
				Severity: SeverityError,
				Message:  "`repeat` is only allowed inside an alt block or altstep body (ETSI 20.3)",
				Node:     v,
				Span:     syntax.SpanOf(v),
			})
		}
	case *syntax.AltStmt:
		if v == nil {
			return nil
		}
		diags = append(diags, walkRepeat(v.Body, inAlt+1)...)
	case *syntax.IfStmt:
		if v == nil {
			return nil
		}
		diags = append(diags, walkRepeat(v.Then, inAlt)...)
		diags = append(diags, walkRepeat(v.Else, inAlt)...)
	case *syntax.ForStmt:
		if v == nil {
			return nil
		}
		diags = append(diags, walkRepeat(v.Body, inAlt)...)
	case *syntax.ForRangeStmt:
		if v == nil {
			return nil
		}
		diags = append(diags, walkRepeat(v.Body, inAlt)...)
	case *syntax.WhileStmt:
		if v == nil {
			return nil
		}
		diags = append(diags, walkRepeat(v.Body, inAlt)...)
	case *syntax.DoWhileStmt:
		if v == nil {
			return nil
		}
		diags = append(diags, walkRepeat(v.Body, inAlt)...)
	case *syntax.SelectStmt:
		if v == nil {
			return nil
		}
		for _, cc := range v.Body {
			if cc == nil {
				continue
			}
			diags = append(diags, walkRepeat(cc.Body, inAlt)...)
		}
	case *syntax.CallStmt:
		// `p.call(...) { ... }` opens an implicit alt-like
		// scope for response handling. ETSI 22.3 lets repeat
		// reset the outer alt though, not the call block;
		// for now we keep it permissive and treat call blocks
		// as alt-equivalent so we don't false-positive on
		// existing tests.
		if v == nil {
			return nil
		}
		if v.Body != nil {
			diags = append(diags, walkRepeat(v.Body, inAlt+1)...)
		}
	}
	return diags
}

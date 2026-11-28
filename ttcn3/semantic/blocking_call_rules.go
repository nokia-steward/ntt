// blocking_call_rules.go enforces ETSI ES 201 873-1
// clause 14 / 22.3.1: a blocking signature (one that
// does NOT carry the \`noblock\` modifier) requires the
// associated \`.call(sig:value)\` invocation to either
// pair with a response block or to specify a timer
// argument. A bare statement \`port.call(sig:value);\` on
// a blocking signature is rejected.
//
// We only flag the cleanest, fully literal shape:
//   - the call is an ExprStmt (so the parser did not
//     attach a response BlockStmt to it),
//   - the call expression is \`X.call(SIG:VALUE)\` with
//     exactly one positional argument,
//   - the signature is declared in the same module and
//     does NOT carry the \`noblock\` modifier.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkBlockingCallRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	sigs := collectSignaturesBlocking(mod)
	if len(sigs) == 0 {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			continue
		}
		walkStmtsForBlockingCall(fn.Body, sigs, &diags)
	}
	return diags
}

func walkStmtsForBlockingCall(
	body *syntax.BlockStmt,
	sigs map[string]bool,
	diags *[]Diagnostic,
) {
	if body == nil {
		return
	}
	for _, st := range body.Stmts {
		switch s := st.(type) {
		case *syntax.ExprStmt:
			checkBareBlockingCall(s, sigs, diags)
		case *syntax.BlockStmt:
			walkStmtsForBlockingCall(s, sigs, diags)
		case *syntax.IfStmt:
			if s != nil && s.Then != nil {
				walkStmtsForBlockingCall(s.Then, sigs, diags)
			}
			if s != nil && s.Else != nil {
				if bs, ok := s.Else.(*syntax.BlockStmt); ok {
					walkStmtsForBlockingCall(bs, sigs, diags)
				}
			}
		case *syntax.ForStmt:
			if s != nil && s.Body != nil {
				walkStmtsForBlockingCall(s.Body, sigs, diags)
			}
		case *syntax.WhileStmt:
			if s != nil && s.Body != nil {
				walkStmtsForBlockingCall(s.Body, sigs, diags)
			}
		case *syntax.DoWhileStmt:
			if s != nil && s.Body != nil {
				walkStmtsForBlockingCall(s.Body, sigs, diags)
			}
		case *syntax.AltStmt:
			if s != nil && s.Body != nil {
				walkStmtsForBlockingCall(s.Body, sigs, diags)
			}
		case *syntax.SelectStmt:
			if s == nil {
				continue
			}
			for _, cc := range s.Body {
				if cc != nil && cc.Body != nil {
					walkStmtsForBlockingCall(cc.Body, sigs, diags)
				}
			}
		}
	}
}

func checkBareBlockingCall(
	es *syntax.ExprStmt,
	sigs map[string]bool,
	diags *[]Diagnostic,
) {
	if es == nil {
		return
	}
	ce, ok := es.Expr.(*syntax.CallExpr)
	if !ok || ce == nil || ce.Args == nil {
		return
	}
	se, ok := ce.Fun.(*syntax.SelectorExpr)
	if !ok || se == nil {
		return
	}
	sel, ok := se.Sel.(*syntax.Ident)
	if !ok || sel == nil || sel.String() != "call" {
		return
	}
	if len(ce.Args.List) != 1 {
		return
	}
	be, ok := ce.Args.List[0].(*syntax.BinaryExpr)
	if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.COLON {
		return
	}
	sig, ok := be.X.(*syntax.Ident)
	if !ok || sig == nil {
		return
	}
	blocking, known := sigs[sig.String()]
	if !known || !blocking {
		return
	}
	*diags = append(*diags, Diagnostic{
		Code:     "blocking-call-no-response",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"signature %q is blocking; the `.call(...)` invocation must specify a timer or pair with a response block (ETSI 14 / 22.3.1)",
			sig.String()),
		Node: es,
		Span: syntax.SpanOf(es),
	})
}

func collectSignaturesBlocking(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		sd, ok := d.Def.(*syntax.SignatureDecl)
		if !ok || sd == nil || sd.Name == nil {
			continue
		}
		out[sd.Name.String()] = sd.NoBlock == nil
	}
	return out
}

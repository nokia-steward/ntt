// mtc_system_clause_rules.go checks a map or unmap with a param clause in
// a function or altstep without a system clause (ETSI ES 201 873-1 clauses
// 21.1.1, 21.1.2): the type of the system component shall be known in the
// context of the operation, and without a system clause it is unknown
// there (16.1.0, 16.2.0).
//
// Other uses of mtc or system where their type is unknown are not
// reported: the suites read them both ways. ETSI's takes mtc:p in a
// function without an mtc clause for valid
// (Sem_210303_Stop_test_component_008), and the independent suite both
// rejects and accepts `var C v := mtc` and system:p there. A test case
// without a system clause has a system of its MTC's type (16.3).
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkMtcSystemClauseRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		fn, ok := n.(*syntax.FuncDecl)
		if !ok || fn.KindTok == nil || fn.Body == nil || fn.System != nil {
			return true
		}
		if kind := fn.KindTok.Kind(); kind != syntax.FUNCTION && kind != syntax.ALTSTEP {
			return true
		}
		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			x, ok := n.(*syntax.ParamExpr)
			if !ok {
				return true
			}
			call, ok := x.X.(*syntax.CallExpr)
			if !ok || call.Args == nil || !m.undefined(call.Fun) {
				return true
			}
			op := syntax.Name(call.Fun)
			if op != "map" && op != "unmap" {
				return true
			}
			for _, arg := range call.Args.List {
				b, ok := arg.(*syntax.BinaryExpr)
				if !ok || b.Op == nil || b.Op.Kind() != syntax.COLON {
					continue
				}
				if id, ok := b.X.(*syntax.Ident); ok && id.String() == "system" && m.undefined(id) {
					diags = append(diags, Diagnostic{
						Code:     "param-unknown-system-type",
						Severity: SeverityError,
						Message:  fmt.Sprintf("%s with param: without a system clause the type of system is unknown here (ETSI 21.1.1, 21.1.2)", op),
						Node:     arg,
						Span:     syntax.SpanOf(arg),
					})
				}
			}
			return true
		})
		return true
	})
	return diags
}

// operation_shape_rules.go checks the operands of configuration and
// communication operations, and calls of functions without a return
// value (ETSI ES 201 873-1):
//
//   - a function without a return clause is invoked directly, not in an
//     expression (16.1.1 a);
//   - `all component` is used with `all port` only, as the one argument of
//     a disconnect or unmap (21.1.2);
//   - a configuration operation names a component reference, not an
//     array of them (6.2.11);
//   - a sending operation has a port and a value (22.1.4.1);
//   - any port is not used with configuration operations, nor with
//     start, stop, clear and halt, and all port not with receiving
//     operations (table 26, 22.0);
//   - a port is not assigned: it has the semantics of a constant of its
//     port type (6.2.10.1).
//
// References resolve through the scope (LookupWithDB).
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkOperationShapeRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	add := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}
	// value reports a call of a function without return clause where e,
	// an expression, needs a value.
	value := func(e syntax.Expr) {
		call, ok := e.(*syntax.CallExpr)
		if !ok {
			return
		}
		if fn := m.funcDeclOf(call.Fun); fn != nil && fn.KindTok != nil && fn.KindTok.Kind() == syntax.FUNCTION && fn.Return == nil {
			add(call, "void-function-in-expression", fmt.Sprintf("%s returns no value: it is invoked directly, not in an expression (ETSI 16.1.1 a)", call.Fun))
		}
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.Declarator:
			if x.Value != nil {
				value(x.Value)
			}
		case *syntax.ReturnStmt:
			if x.Result != nil {
				value(x.Result)
			}
		case *syntax.BinaryExpr:
			if x.Op == nil {
				return true
			}
			switch x.Op.Kind() {
			case syntax.ASSIGN:
				value(x.Y)
				if _, ok := tree.ParentOf(x).(*syntax.ExprStmt); ok && m.isPortRef(x.X) {
					add(x, "port-assigned", fmt.Sprintf("%s is a port: it is not assigned, as a constant (ETSI 6.2.10.1)", x.X))
				}
			case syntax.COLON, syntax.RANGE:
			default:
				value(x.X)
				value(x.Y)
			}
		case *syntax.CallExpr:
			m.checkOperationCall(x, add)
		case *syntax.SelectorExpr:
			m.checkAnyAllPort(x, add)
		}
		return true
	})
	return diags
}

// funcDeclOf is the function, altstep or testcase e refers to.
func (m typeModel) funcDeclOf(e syntax.Expr) *syntax.FuncDecl {
	id, ok := e.(*syntax.Ident)
	if !ok {
		return nil
	}
	defs := m.tree.LookupWithDB(id, m.db)
	if len(defs) != 1 {
		return nil
	}
	fn, _ := defs[0].Node.(*syntax.FuncDecl)
	return fn
}

// undefined reports whether no definition of the name e is in scope, as
// for an operation keyword.
func (m typeModel) undefined(e syntax.Expr) bool {
	id, ok := e.(*syntax.Ident)
	return ok && len(m.tree.LookupWithDB(id, m.db)) == 0
}

// isPortRef reports whether e refers to a port.
func (m typeModel) isPortRef(e syntax.Expr) bool {
	id, ok := e.(*syntax.Ident)
	if !ok {
		return false
	}
	vd, _, _, ok := m.valueDeclOf(id)
	return ok && vd.KindTok != nil && vd.KindTok.Kind() == syntax.PORT
}

func (m typeModel) checkOperationCall(call *syntax.CallExpr, add func(syntax.Node, string, string)) {
	var args []syntax.Expr
	if call.Args != nil {
		args = call.Args.List
	}
	switch fun := call.Fun.(type) {
	case *syntax.Ident:
		name := fun.String()
		if !m.undefined(fun) {
			return
		}
		switch name {
		case "send", "call", "reply", "raise":
			add(call, "sending-operation-without-port", fmt.Sprintf("%s: a sending operation names the port it takes place at (ETSI 22.1.4.1)", name))
		case "connect", "disconnect", "map", "unmap":
			for _, arg := range args {
				b, ok := arg.(*syntax.BinaryExpr)
				if !ok || b.Op == nil || b.Op.Kind() != syntax.COLON {
					continue
				}
				comp, port := syntax.Name(b.X), syntax.Name(b.Y)
				if port == "any port" {
					add(arg, "any-port-not-allowed", fmt.Sprintf("%s: any port is not used with configuration operations (ETSI 22.0, table 26)", name))
					continue
				}
				if comp == "all component" && (name == "disconnect" || name == "unmap") {
					switch {
					case port != "all port":
						add(arg, "all-component-without-all-port", fmt.Sprintf("%s(all component:%s): all component is used with all port only (ETSI 21.1.2)", name, port))
					case len(args) > 1:
						add(arg, "all-component-not-alone", fmt.Sprintf("%s: all component:all port is its one and only argument (ETSI 21.1.2)", name))
					}
					continue
				}
				if id, ok := b.X.(*syntax.Ident); ok {
					if vd, decl, _, ok := m.valueDeclOf(id); ok && vd.KindTok != nil && vd.KindTok.Kind() == syntax.VAR && len(decl.ArrayDef) > 0 {
						add(arg, "configuration-on-component-array", fmt.Sprintf("%s: %s is an array of component references; the operation takes one (ETSI 6.2.11)", name, id))
					}
				}
			}
		}
	case *syntax.SelectorExpr:
		switch syntax.Name(fun.Sel) {
		case "send", "call", "reply":
			if len(args) == 0 && m.isPortRef(fun.X) {
				add(call, "sending-operation-without-value", fmt.Sprintf("%s.%s(): a sending operation transmits a value (ETSI 22.1.4.1)", fun.X, syntax.Name(fun.Sel)))
			}
		}
	}
}

// checkAnyAllPort reports any port with start, stop, clear or halt and all
// port with a receiving operation (22.0, table 26).
func (m typeModel) checkAnyAllPort(sel *syntax.SelectorExpr, add func(syntax.Node, string, string)) {
	op := syntax.Name(sel.Sel)
	switch syntax.Name(sel.X) {
	case "any port":
		switch op {
		case "start", "stop", "clear", "halt":
			add(sel, "any-port-not-allowed", fmt.Sprintf("any port.%s: any port is not used with %s; all port is (ETSI 22.0, table 26)", op, op))
		}
	case "all port":
		switch op {
		case "receive", "trigger", "getcall", "getreply", "catch", "check":
			add(sel, "all-port-not-allowed", fmt.Sprintf("all port.%s: all port is not used with receiving operations; any port is (ETSI 22.0, table 26)", op))
		}
	}
}

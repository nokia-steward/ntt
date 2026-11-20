// misc_static_rules.go enforces static rules of ETSI ES 201 873-1 that a
// look at the syntax decides:
//
//   - the number of actual parameters of a predefined function (annex C);
//   - getverdict and the port operations clear, halt and checkstate are
//     not allowed in a control part, execute not in a testcase (clauses
//     16.1.5, 19, 26.1, table 15; setverdict is control_part_ops.go's);
//   - start applies to no `all component`, stop and kill to no `any
//     component` (21.3);
//   - a union alternative is not optional, and not every alternative is
//     of the union's own type (6.2.5);
//   - an array dimension has an inclusive, finite upper bound (6.2.7);
//   - port, component and default types are not subtyped (6.1.0);
//   - not_a_number is not negated (6.1.0);
//   - the default duration of a timer is not infinity (12);
//   - an index redirection is used with any from only (21.3, 22, 23);
//   - no list branch follows the else branch of a select case (19.3.1);
//   - a range does not start with infinity (B.1.2.5);
//   - a template a receiving operation matches with is no
//     AnyValueOrNone, omit or ifpresent as a whole (B.1.2.4, B.1.2.8,
//     B.1.4.2).
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// predefinedArity is the number of actual parameters, min and max, the
// predefined functions take.
var predefinedArity = map[string][2]int{
	"int2char": {1, 1}, "int2unichar": {1, 1}, "int2bit": {2, 2}, "int2hex": {2, 2},
	"int2oct": {2, 2}, "int2str": {1, 1}, "int2float": {1, 1}, "float2int": {1, 1},
	"char2int": {1, 1}, "char2oct": {1, 1}, "unichar2int": {1, 1}, "unichar2char": {1, 1},
	"bit2int": {1, 1}, "bit2hex": {1, 1}, "bit2oct": {1, 1}, "bit2str": {1, 1},
	"hex2int": {1, 1}, "hex2bit": {1, 1}, "hex2oct": {1, 1}, "hex2str": {1, 1},
	"oct2int": {1, 1}, "oct2bit": {1, 1}, "oct2hex": {1, 1}, "oct2str": {1, 1},
	"oct2char": {1, 1}, "str2int": {1, 1}, "str2hex": {1, 1}, "str2oct": {1, 1},
	"str2float": {1, 1}, "enum2int": {1, 1}, "testcasename": {0, 0},
	"unichar2oct": {1, 2}, "oct2unichar": {1, 2}, "int2enum": {2, 2},
	"get_stringencoding": {1, 1}, "remove_bom": {1, 1}, "float2str": {1, 1},
}

func (a *Analyzer) checkMiscStaticRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	var diags []Diagnostic
	add := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}
	userFuncs := collectFuncDeclsByName(mod)
	// address is left out: a user-defined integer address may be
	// subtyped as integer is (6.3.0 NOTE).
	refTypes := map[string]bool{"default": true}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		switch x := d.Def.(type) {
		case *syntax.PortTypeDecl:
			if x.Name != nil {
				refTypes[x.Name.String()] = true
			}
		case *syntax.ComponentTypeDecl:
			if x.Name != nil {
				refTypes[x.Name.String()] = true
			}
		}
	}
	indexOK := indexRedirectsWithAnyFrom(mod)
	wholeOnly := wholeMatchingTemplates(mod)
	m := typeModel{tree: tree, db: a.db()}

	mod.Inspect(func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.CallExpr:
			// A template matching a received message is no AnyValueOrNone,
			// omit or ifpresent as a whole (B.1.2.4, B.1.2.8, B.1.4.2).
			if op := receivingOpName(x.Fun); op != "" && x.Args != nil && len(x.Args.List) > 0 {
				if ref, ok := unparen(x.Args.List[0]).(*syntax.Ident); ok && len(wholeOnly) > 0 {
					// The reference may be a parameter or local of
					// the same name: resolve it.
					what := ""
					if defs := tree.LookupWithDB(ref, a.db()); len(defs) == 1 {
						if t, ok := defs[0].Node.(*syntax.TemplateDecl); ok {
							what = wholeOnly[t]
						}
					}
					if what != "" {
						add(x, "whole-template-in-receive", fmt.Sprintf("template %s is %s as a whole: no template of a %s (ETSI B.1.2.4, B.1.2.8, B.1.4.2)", ref.String(), what, op))
					}
				}
			}
			id, ok := x.Fun.(*syntax.Ident)
			if !ok {
				return true
			}
			if ar, ok := predefinedArity[id.String()]; ok && userFuncs[id.String()] == nil {
				nargs, named := 0, false
				if x.Args != nil {
					nargs = len(x.Args.List)
					for _, e := range x.Args.List {
						if _, ok := namedArgName(e); ok {
							named = true
						}
					}
				}
				if !named && (nargs < ar[0] || nargs > ar[1]) {
					add(x, "predefined-arity", fmt.Sprintf("%s takes %s actual parameters, not %d (ETSI annex C)", id.String(), arityText(ar), nargs))
				}
			}
		case *syntax.ControlPart:
			// A selector names a field, not a function or operation.
			fieldSel := map[syntax.Node]bool{}
			syntax.Inspect(x.Body, func(n syntax.Node) bool {
				switch y := n.(type) {
				case *syntax.FuncDecl:
					return false
				case *syntax.Ident:
					if y.String() == "getverdict" && !fieldSel[y] && userFuncs["getverdict"] == nil {
						add(y, "control-part-verdict", "getverdict is not allowed in a control part (ETSI 16.1.5, table 15)")
					}
				case *syntax.SelectorExpr:
					fieldSel[y.Sel] = true
					switch syntax.Name(y.Sel) {
					case "clear", "halt", "checkstate":
						// A field of a record or set value of the
						// name is no port operation.
						if _, isValue := m.exprRoot(y.X); !isValue {
							add(y, "control-part-port-op", fmt.Sprintf("port operation %s is not allowed in a control part (ETSI table 15)", syntax.Name(y.Sel)))
						}
					}
				}
				return true
			})
			// The rules below apply in the control part too.
			return true
		case *syntax.FuncDecl:
			// Functions and altsteps may be called from the control part
			// and execute test cases (26.1 c); a testcase is test
			// behaviour.
			if x.KindTok != nil && x.KindTok.Kind() == syntax.TESTCASE && x.Body != nil {
				syntax.Inspect(x.Body, func(m syntax.Node) bool {
					if c, ok := m.(*syntax.CallExpr); ok && syntax.Name(c.Fun) == "execute" && userFuncs["execute"] == nil {
						add(c, "execute-in-test-behaviour", "execute is not allowed in test behaviour (ETSI 26.1, table 15)")
					}
					return true
				})
			}
		case *syntax.SelectorExpr:
			op := syntax.Name(x.Sel)
			switch base := syntax.Name(x.X); {
			case base == "all component" && op == "start":
				add(x, "all-component-start", "start is not applied to all component (ETSI 21.3.2)")
			case base == "any component" && (op == "stop" || op == "kill"):
				add(x, "any-component-stop-kill", fmt.Sprintf("%s is not applied to any component (ETSI 21.3.3, 21.3.4)", op))
			}
		case *syntax.StructTypeDecl:
			if x.KindTok == nil || x.KindTok.Kind() != syntax.UNION || x.Name == nil {
				return true
			}
			self := 0
			for _, f := range x.Fields {
				if f == nil {
					continue
				}
				if f.Optional != nil {
					add(f, "union-optional-alternative", "a union alternative is not optional (ETSI 6.2.5)")
				}
				if len(f.ArrayDef) == 0 && syntax.Name(f.Type) == x.Name.String() {
					self++
				}
			}
			// At least one alternative does not reference the union's
			// own type, else it recurses without end.
			if self > 0 && self == len(x.Fields) {
				add(x, "union-self-alternative", fmt.Sprintf("every alternative of union %s is of its own type (ETSI 6.2.5)", x.Name.String()))
			}
		case *syntax.Declarator:
			checkArrayDims(x.ArrayDef, add)
		case *syntax.Field:
			checkArrayDims(x.ArrayDef, add)
		case *syntax.SubTypeDecl:
			if x.Field != nil && x.Field.ValueConstraint != nil && refTypes[syntax.Name(x.Field.Type)] {
				add(x, "subtype-not-allowed", fmt.Sprintf("type %s is not subtyped (ETSI 6.1.0)", syntax.Name(x.Field.Type)))
			}
		case *syntax.UnaryExpr:
			if x.Op != nil && x.Op.Kind() == syntax.SUB {
				if v, ok := x.X.(*syntax.ValueLiteral); ok && v.Tok != nil && v.Tok.Kind() == syntax.NAN {
					add(x, "negated-not-a-number", "not_a_number is not negated (ETSI 6.1.0)")
				}
			}
		case *syntax.ValueDecl:
			if x.KindTok != nil && x.KindTok.Kind() == syntax.TIMER || hasLeadingTimerKeyword(x.Type) {
				for _, d := range x.Decls {
					if d != nil && d.Value != nil && isInfinityValue(d.Value) {
						add(d, "timer-infinite-duration", "the default duration of a timer is not infinity (ETSI 12)")
					}
				}
			}
		case *syntax.RedirectExpr:
			// A port operation's is receive_redirect_rules.go's.
			if x.Index != nil && !indexOK[x] && !isPortOperationName(redirectOpName(x.X)) {
				add(x, "index-redirect-without-any-from", "@index is used with any from only (ETSI 21.3, 22, 23)")
			}
		case *syntax.SelectStmt:
			elseSeen := false
			for _, c := range x.Body {
				if c == nil {
					continue
				}
				if c.Case == nil {
					elseSeen = true
				} else if elseSeen {
					add(c, "select-case-after-else", "no list branch follows the else branch (ETSI 19.3.1)")
				}
			}
		case *syntax.BinaryExpr:
			if x.Op != nil && x.Op.Kind() == syntax.RANGE && isInfinityValue(x.X) {
				add(x, "range-starts-with-infinity", "the lower boundary of a range is not infinity (ETSI B.1.2.5)")
			}
		}
		return true
	})
	return diags
}

func arityText(ar [2]int) string {
	if ar[0] == ar[1] {
		return fmt.Sprint(ar[0])
	}
	return fmt.Sprintf("%d to %d", ar[0], ar[1])
}

// isInfinityValue reports whether e is infinity, not negated.
func isInfinityValue(e syntax.Expr) bool {
	switch x := e.(type) {
	case *syntax.ValueLiteral:
		return x.Tok != nil && x.Tok.String() == "infinity"
	case *syntax.Ident:
		return x.String() == "infinity"
	}
	return false
}

// checkArrayDims reports array dimensions with an exclusive or infinite
// upper bound (6.2.7).
func checkArrayDims(dims []*syntax.ParenExpr, add func(syntax.Node, string, string)) {
	for _, d := range dims {
		if d == nil {
			continue
		}
		for _, e := range d.List {
			b, ok := e.(*syntax.BinaryExpr)
			if !ok || b.Op == nil || b.Op.Kind() != syntax.RANGE {
				continue
			}
			for _, bound := range []syntax.Expr{b.X, b.Y} {
				if u, ok := bound.(*syntax.UnaryExpr); ok && u.Op != nil && u.Op.Kind() == syntax.EXCL {
					add(b, "array-dimension-bound", "an array dimension has inclusive bounds (ETSI 6.2.7)")
				}
			}
			if isInfinityValue(b.Y) {
				add(b, "array-dimension-bound", "an array dimension is finite (ETSI 6.2.7)")
			}
		}
	}
}

// indexRedirectsWithAnyFrom are the redirects of operations used with
// any from: of the operation itself, or of the one a check checks.
func indexRedirectsWithAnyFrom(mod *syntax.Module) map[*syntax.RedirectExpr]bool {
	ok := map[*syntax.RedirectExpr]bool{}
	mod.Inspect(func(n syntax.Node) bool {
		r, isR := n.(*syntax.RedirectExpr)
		if isR {
			if _, any := r.X.(*syntax.FromExpr); any {
				ok[r] = true
			}
			return true
		}
		fe, isF := n.(*syntax.FromExpr)
		if !isF {
			return true
		}
		syntax.Inspect(fe.X, func(m syntax.Node) bool {
			if r, ok2 := m.(*syntax.RedirectExpr); ok2 {
				ok[r] = true
			}
			return true
		})
		return true
	})
	return ok
}

// receivingOpName is the receiving operation fun names, `p.receive` or
// `receive` inside a check, or "".
func receivingOpName(fun syntax.Expr) string {
	var name string
	switch f := fun.(type) {
	case *syntax.SelectorExpr:
		name = syntax.Name(f.Sel)
	case *syntax.Ident:
		name = f.String()
	}
	switch name {
	case "receive", "trigger", "getcall", "getreply":
		return name
	}
	return ""
}

// wholeMatchingTemplates are the templates without parameters that are
// AnyValueOrNone, omit or have ifpresent as a whole, and which.
func wholeMatchingTemplates(mod *syntax.Module) map[*syntax.TemplateDecl]string {
	out := map[*syntax.TemplateDecl]string{}
	mod.Inspect(func(n syntax.Node) bool {
		t, ok := n.(*syntax.TemplateDecl)
		if !ok || t.Name == nil || t.Params != nil || t.Base != nil || t.Value == nil {
			return true
		}
		switch v := t.Value.(type) {
		case *syntax.ValueLiteral:
			if v.Tok != nil && v.Tok.Kind() == syntax.MUL {
				out[t] = "AnyValueOrNone"
			}
			if v.Tok != nil && v.Tok.Kind() == syntax.OMIT {
				out[t] = "omit"
			}
		case *syntax.UnaryExpr:
			if v.Op != nil && v.Op.Kind() == syntax.IFPRESENT {
				out[t] = "ifpresent"
			}
		}
		return true
	})
	return out
}

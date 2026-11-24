// predefined_arg_type_rules.go checks the root types of the arguments of
// predefined functions and operators (ETSI ES 201 873-1 annex C, clauses
// 7.1.8.2, 7.3, 16.1.6), where the model knows them:
//
//   - the parameters of the predefined functions of table predefinedParams
//     (C.1, C.5; the general error causes of 16.1.2);
//   - the reason of the not implemented statement `???(reason)` is a
//     universal charstring (16.1.6);
//   - a decoded field reference `x => T` applies to a bitstring,
//     hexstring, octetstring, charstring or universal charstring, its
//     optional encoding format `x => (T, f)` is a charstring and is only
//     given for a universal charstring x (7.3);
//   - ischosen applies to a union field reference or an anytype
//     alternative, not to the field of a record or set (7.1.8.2).
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// predefinedParams gives, per predefined function, the root type of each
// parameter whose type is a basic or string type; "" for a parameter of
// any type.
var predefinedParams = map[string][]string{
	"int2char": {"integer"}, "int2unichar": {"integer"}, "int2str": {"integer"},
	"int2float": {"integer"}, "float2int": {"float"}, "float2str": {"float"},
	"int2bit": {"integer", "integer"}, "int2hex": {"integer", "integer"}, "int2oct": {"integer", "integer"},
	"char2int": {"charstring"}, "char2oct": {"charstring"},
	"unichar2int": {"universal charstring"}, "unichar2char": {"universal charstring"},
	"unichar2oct": {"universal charstring", "charstring"},
	"bit2int":     {"bitstring"}, "bit2hex": {"bitstring"}, "bit2oct": {"bitstring"}, "bit2str": {"bitstring"},
	"hex2int": {"hexstring"}, "hex2bit": {"hexstring"}, "hex2oct": {"hexstring"}, "hex2str": {"hexstring"},
	"oct2int": {"octetstring"}, "oct2bit": {"octetstring"}, "oct2hex": {"octetstring"},
	"oct2str": {"octetstring"}, "oct2char": {"octetstring"}, "oct2unichar": {"octetstring", "charstring"},
	"str2int": {"charstring"}, "str2hex": {"charstring"}, "str2oct": {"charstring"}, "str2float": {"charstring"},
	"any2unistr":       {"", "universal charstring"},
	"encvalue":         {"", "universal charstring", "universal charstring"},
	"decvalue":         {"bitstring", "", "universal charstring", "universal charstring"},
	"encvalue_unichar": {"", "charstring", "universal charstring", "universal charstring"},
	"decvalue_unichar": {"universal charstring", "", "charstring", "universal charstring", "universal charstring"},
}

func (a *Analyzer) checkPredefinedArgTypeRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	add := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}
	// known is the basic or string root type of e, if the model knows it.
	known := func(e syntax.Expr) (rootType, bool) {
		r, ok := m.exprRoot(e)
		return r, ok && r.primitive()
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.CallExpr:
			if x.Args == nil {
				return true
			}
			if v, ok := x.Fun.(*syntax.ValueLiteral); ok && v.Tok != nil && v.Tok.Kind() == syntax.NOTIMPL {
				if len(x.Args.List) == 1 {
					if r, ok := known(x.Args.List[0]); ok && !isCharKind(r.kind) {
						add(x.Args.List[0], "predefined-argument-type", fmt.Sprintf("???: the reason is of root type %s, not universal charstring (ETSI 16.1.6)", r.kind))
					}
				}
				return true
			}
			id, ok := x.Fun.(*syntax.Ident)
			if !ok || !m.undefined(id) {
				return true
			}
			name := id.String()
			if name == "ischosen" && len(x.Args.List) == 1 {
				if sel, ok := x.Args.List[0].(*syntax.SelectorExpr); ok {
					if r, ok := m.exprRoot(sel.X); ok && (r.kind == "record" || r.kind == "set") {
						add(sel, "ischosen-not-an-alternative", fmt.Sprintf("ischosen(%s): a field of a %s, not a union alternative (ETSI 7.1.8.2)", sel, r.kind))
					}
				}
				return true
			}
			params, ok := predefinedParams[name]
			if !ok {
				return true
			}
			for i, arg := range x.Args.List {
				if i >= len(params) || params[i] == "" {
					continue
				}
				if b, ok := arg.(*syntax.BinaryExpr); ok && b.Op != nil && b.Op.Kind() == syntax.ASSIGN {
					// Named arguments are not matched to parameters here.
					break
				}
				r, ok := known(arg)
				if ok && !assignable(rootType{kind: params[i]}, r) {
					add(arg, "predefined-argument-type", fmt.Sprintf("%s: argument %d is of root type %s, not %s (ETSI C, 16.1.2)", name, i+1, r.kind, params[i]))
				}
			}
		case *syntax.BinaryExpr:
			if x.Op == nil || x.Op.Kind() != syntax.DECODE {
				return true
			}
			operand, ok := known(x.X)
			if ok {
				switch operand.kind {
				case "bitstring", "hexstring", "octetstring", "charstring", "universal charstring":
				default:
					add(x.X, "decoded-field-reference-operand", fmt.Sprintf("=>: the operand is of root type %s, not a string type (ETSI 7.3)", operand.kind))
					return true
				}
			}
			p, isList := x.Y.(*syntax.ParenExpr)
			if !isList || len(p.List) != 2 {
				return true
			}
			if ok && operand.kind != "universal charstring" {
				add(p.List[1], "decoded-field-reference-format", fmt.Sprintf("=>: an encoding format is given only for a universal charstring operand, not for %s (ETSI 7.3)", operand.kind))
			} else if f, fok := known(p.List[1]); fok && !isCharKind(f.kind) {
				add(p.List[1], "decoded-field-reference-format", fmt.Sprintf("=>: the encoding format is of root type %s, not charstring (ETSI 7.3)", f.kind))
			}
		}
		return true
	})
	return diags
}

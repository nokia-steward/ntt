// addr_clause_type_rules.go enforces ETSI ES 201 873-1 clauses
// 22.2.2 / 22.2.3 / 22.3.x:
//
//	"All AddressRef items in the from / to clause and all
//	 VariableRef items in the sender clause shall be of type
//	 address, component or of the address type bound to the port
//	 type."
//
// We flag the narrow, statically-decidable cases where the
// referenced variable was declared in the enclosing function body
// with one of the built-in primitive types (integer, charstring,
// float, boolean, bitstring, octetstring, hexstring, verdicttype,
// universal charstring). User-defined types cannot be classified
// without full symbol resolution, so they are conservatively
// ignored - we never flag them, which keeps false positives at
// zero in the Sem* suite.
//
// Catches NegSem_220303_ReplyOperation_009 and NegSem_220302_
// getcall_operation_018 / 021 / 022.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// primitiveBuiltinAddrTypes is the closed set of TTCN-3 built-in
// types that cannot legally hold a component reference or an
// address-bound value. Anything outside this set is treated as
// "possibly an address" and skipped to avoid false positives.
var primitiveBuiltinAddrTypes = map[string]bool{
	"integer":             true,
	"charstring":          true,
	"universal charstring": true,
	"float":               true,
	"boolean":             true,
	"bitstring":           true,
	"octetstring":         true,
	"hexstring":           true,
	"verdicttype":         true,
	"anytype":             true,
}

func (a *Analyzer) checkAddrClauseTypeRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	// Primitive types declared as a port's `address` type are
	// exempt from this rule: `type port P { address integer }`
	// makes `integer` a legal sender / from / to target on P.
	// Conservatively, if ANY port type in the module binds a
	// primitive as its address, we skip flagging that primitive.
	exempt := collectPortAddressPrimitives(mod)
	for k := range collectModuleAddressAliasPrimitives(mod) {
		exempt[k] = true
	}
	modulePrim := collectModulePrimitiveConsts(mod)
	syntax.Inspect(mod, func(n syntax.Node) bool {
		fn, ok := n.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			return true
		}
		primTypeVars := collectPrimitiveTypeVars(fn)
		for k, v := range modulePrim {
			if _, hit := primTypeVars[k]; !hit {
				primTypeVars[k] = v
			}
		}
		if len(primTypeVars) == 0 {
			return true
		}
		syntax.Inspect(fn.Body, func(sn syntax.Node) bool {
			be, ok := sn.(*syntax.BinaryExpr)
			if !ok || be == nil || be.Op == nil {
				return true
			}
			var clause string
			switch be.Op.Kind() {
			case syntax.FROM:
				clause = "from"
			case syntax.TO:
				clause = "to"
			default:
				return true
			}
			for _, item := range addrRefItems(be.Y) {
				name := identName(item)
				if name == "" {
					continue
				}
				ty, isPrim := primTypeVars[name]
				if !isPrim {
					continue
				}
				if exempt[ty] {
					continue
				}
				diags = append(diags, Diagnostic{
					Code:     addrClausePrimitiveCode(clause),
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"%s %s: address reference of primitive type %q is not allowed (must be component or address type; ETSI 22.2.2 / 22.3.x)",
						clause, name, ty),
					Node: item,
					Span: syntax.SpanOf(item),
				})
			}
			return true
		})
		// `sender X` redirects are also subject to the same
		// type restriction. They appear as RedirectExpr.Sender.
		syntax.Inspect(fn.Body, func(sn syntax.Node) bool {
			re, ok := sn.(*syntax.RedirectExpr)
			if !ok || re == nil || re.Sender == nil {
				return true
			}
			name := identName(re.Sender)
			if name == "" {
				return true
			}
			ty, isPrim := primTypeVars[name]
			if !isPrim {
				return true
			}
			if exempt[ty] {
				return true
			}
			diags = append(diags, Diagnostic{
				Code:     "sender-clause-primitive-address",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"sender %s: redirect target of primitive type %q is not allowed (must be component or address type; ETSI 22.2.2 / 22.3.x)",
					name, ty),
				Node: re.Sender,
				Span: syntax.SpanOf(re.Sender),
			})
			return true
		})
		return false
	})
	return diags
}

// addrClausePrimitiveCode picks the diagnostic-code suffix
// matching the clause; we keep them distinct so downstream
// tooling can tell the two apart without parsing the message
// text.
func addrClausePrimitiveCode(clause string) string {
	if clause == "to" {
		return "to-clause-primitive-address"
	}
	return "from-clause-primitive-address"
}

// collectPrimitiveTypeVars returns a map of variable names ->
// declared type name for every `var T x` / `const T x` declaration
// inside the function whose type is one of the built-in primitive
// types listed in primitiveBuiltinAddrTypes. Function parameters
// of those types are also included.
func collectPrimitiveTypeVars(fn *syntax.FuncDecl) map[string]string {
	out := map[string]string{}
	// Parameters.
	if fn.Params != nil {
		for _, p := range fn.Params.List {
			if p == nil || p.Type == nil || p.Name == nil {
				continue
			}
			name := primitiveTypeName(p.Type)
			if name == "" {
				continue
			}
			out[p.Name.String()] = name
		}
	}
	// Local declarations.
	syntax.Inspect(fn.Body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil || vd.Type == nil {
			return true
		}
		// Templates / timers / ports do not declare an
		// address-like primitive value, so skip those.
		if vd.KindTok != nil {
			switch vd.KindTok.Kind() {
			case syntax.TIMER, syntax.PORT, syntax.TEMPLATE:
				return true
			}
		}
		name := primitiveTypeName(vd.Type)
		if name == "" {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil {
				continue
			}
			// Arrays of primitive type are still primitive
			// for our purposes - they are equally invalid
			// as a from/to/sender target.
			out[dec.Name.String()] = name
		}
		return true
	})
	return out
}

// collectModulePrimitiveConsts maps every module-level constant
// declared with a built-in primitive type to that type name. This
// is used by addr_clause_type_rules.go so a `to <const-name>`
// clause whose constant is a primitive (e.g. charstring) is flagged
// just like a local variable would be.
func collectModulePrimitiveConsts(mod *syntax.Module) map[string]string {
	out := map[string]string{}
	if mod == nil {
		return out
	}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		vd, ok := d.Def.(*syntax.ValueDecl)
		if !ok || vd == nil || vd.Type == nil ||
			vd.KindTok == nil {
			continue
		}
		if vd.KindTok.Kind() != syntax.CONST {
			continue
		}
		name := primitiveTypeName(vd.Type)
		if name == "" {
			continue
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil {
				continue
			}
			out[dec.Name.String()] = name
		}
	}
	return out
}

// collectPortAddressPrimitives walks every PortTypeDecl in mod and
// returns the set of primitive built-in type names declared as
// `address <type>` on any port. Once a primitive is exempt for
// any port, we no longer flag sender / from / to references of
// that type anywhere in the module - that errs on the side of
// false negatives (we won't catch a misuse on a *different* port
// that doesn't declare an address type) but eliminates the
// false positives that broke Sem_2204_the_check_operation_002.
// collectModuleAddressAliasPrimitives picks up a module-level
// declaration `type <Primitive> address;` that binds the special
// builtin "address" name to a primitive base type. Once the
// alias exists, any variable of that primitive can stand in for an
// address reference in sender/from/to clauses.
func collectModuleAddressAliasPrimitives(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	if mod == nil {
		return out
	}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		std, ok := d.Def.(*syntax.SubTypeDecl)
		if !ok || std == nil || std.Field == nil ||
			std.Field.Name == nil ||
			std.Field.Name.String() != "address" {
			continue
		}
		rs, ok := std.Field.Type.(*syntax.RefSpec)
		if !ok || rs == nil {
			continue
		}
		id, ok := rs.X.(*syntax.Ident)
		if !ok || id == nil {
			continue
		}
		name := id.String()
		if primitiveBuiltinAddrTypes[name] {
			out[name] = true
		}
	}
	return out
}

func collectPortAddressPrimitives(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ptd, ok := n.(*syntax.PortTypeDecl)
		if !ok || ptd == nil {
			return true
		}
		for _, a := range ptd.Attrs {
			pa, ok := a.(*syntax.PortAttribute)
			if !ok || pa == nil || pa.KindTok == nil {
				continue
			}
			if pa.KindTok.Kind() != syntax.ADDRESS {
				continue
			}
			for _, t := range pa.Types {
				if name := primitiveTypeName(t); name != "" {
					out[name] = true
				}
			}
		}
		return true
	})
	return out
}

// primitiveTypeName returns the canonical name of a built-in
// primitive type expression, or "" if the type is anything else
// (user-defined, parameterized, qualified, etc).
func primitiveTypeName(e syntax.Expr) string {
	id, ok := e.(*syntax.Ident)
	if !ok || id == nil || id.Tok == nil {
		return ""
	}
	name := id.Tok.String()
	if id.Tok2 != nil {
		name = name + " " + id.Tok2.String()
	}
	if primitiveBuiltinAddrTypes[name] {
		return name
	}
	return ""
}

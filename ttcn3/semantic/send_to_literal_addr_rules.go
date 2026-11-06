// send_to_literal_addr_rules.go enforces ETSI ES 201 873-1
// clause 22.2.1: all `to` items in a port.send(...) shall be of
// the address type bound to the port (`address X` clause of the
// port type). When the `to` value is a literal whose syntactic
// kind disagrees with the port's bound address primitive, the
// send is invalid.
//
// We only flag the cleanest shape:
//
//	p.send(...) to <literal>
//
// where:
//   - p is a port-typed component member;
//   - the port type binds a primitive `address` (integer,
//     charstring, ...);
//   - <literal> is a ValueLiteral whose token kind maps to a
//     different primitive.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkSendToLiteralAddrRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	portAddr := collectPortTypeAddressPrim(mod)
	if len(portAddr) == 0 {
		return nil
	}
	compPorts := collectComponentPortTypeAlt(mod)
	if len(compPorts) == 0 {
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
		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			be, ok := n.(*syntax.BinaryExpr)
			if !ok || be == nil || be.Op == nil ||
				be.Op.Kind() != syntax.TO {
				return true
			}
			ce, ok := be.X.(*syntax.CallExpr)
			if !ok || ce == nil {
				return true
			}
			sel, ok := ce.Fun.(*syntax.SelectorExpr)
			if !ok || sel == nil || sel.Sel == nil {
				return true
			}
			selID, ok := sel.Sel.(*syntax.Ident)
			if !ok || selID == nil || selID.String() != "send" {
				return true
			}
			portID, ok := sel.X.(*syntax.Ident)
			if !ok || portID == nil {
				return true
			}
			portTy := ""
			for _, ports := range compPorts {
				if t, hit := ports[portID.String()]; hit {
					portTy = t
					break
				}
			}
			if portTy == "" {
				return true
			}
			addrPrim, hasAddr := portAddr[portTy]
			if !hasAddr {
				return true
			}
			vl, ok := be.Y.(*syntax.ValueLiteral)
			if !ok || vl == nil || vl.Tok == nil {
				return true
			}
			litPrim := tokenKindToPrim(vl.Tok.Kind())
			if litPrim == "" || litPrim == addrPrim {
				return true
			}
			diags = append(diags, Diagnostic{
				Code:     "send-to-literal-address-type-mismatch",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"send `to` literal has primitive type %q but port %q binds address %q (ETSI 22.2.1)",
					litPrim, portID.String(), addrPrim),
				Node: be.Y,
				Span: syntax.SpanOf(be.Y),
			})
			return true
		})
	}
	return diags
}

func collectPortTypeAddressPrim(mod *syntax.Module) map[string]string {
	out := map[string]string{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		ptd, ok := d.Def.(*syntax.PortTypeDecl)
		if !ok || ptd == nil || ptd.Name == nil {
			continue
		}
		for _, attr := range ptd.Attrs {
			pa, ok := attr.(*syntax.PortAttribute)
			if !ok || pa == nil || pa.KindTok == nil {
				continue
			}
			if pa.KindTok.Kind() != syntax.ADDRESS {
				continue
			}
			for _, t := range pa.Types {
				if name := primitiveTypeName(t); name != "" {
					out[ptd.Name.String()] = name
				}
			}
		}
	}
	return out
}

func collectComponentPortTypeAlt(mod *syntax.Module) map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		cd, ok := d.Def.(*syntax.ComponentTypeDecl)
		if !ok || cd == nil || cd.Name == nil || cd.Body == nil {
			continue
		}
		ports := map[string]string{}
		for _, st := range cd.Body.Stmts {
			ds, ok := st.(*syntax.DeclStmt)
			if !ok || ds == nil {
				continue
			}
			vd, ok := ds.Decl.(*syntax.ValueDecl)
			if !ok || vd == nil || vd.KindTok == nil ||
				vd.KindTok.Kind() != syntax.PORT {
				continue
			}
			id, ok := vd.Type.(*syntax.Ident)
			if !ok || id == nil {
				continue
			}
			for _, dc := range vd.Decls {
				if dc != nil && dc.Name != nil {
					ports[dc.Name.String()] = id.String()
				}
			}
		}
		out[cd.Name.String()] = ports
	}
	return out
}

func tokenKindToPrim(k syntax.Kind) string {
	switch k {
	case syntax.STRING:
		return "charstring"
	case syntax.INT:
		return "integer"
	case syntax.FLOAT:
		return "float"
	case syntax.TRUE, syntax.FALSE:
		return "boolean"
	case syntax.BSTRING:
		return "bitstring"
	}
	return ""
}

package lsp

import (
	"context"
	"time"

	"github.com/nokia/ntt/internal/log"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (s *Server) documentSymbol(ctx context.Context, params *protocol.DocumentSymbolParams) ([]interface{}, error) {
	start := time.Now()
	defer func() {
		if err := recover(); err != nil {
			// Incomplete trees are common while the user is typing.
			log.Debugf("Panic: %#v\n", err)
		}
		log.Debugf("DocumentSymbol took %s.\n", time.Since(start))
	}()

	syms := DocumentSymbols(ttcn3.ParseFile(params.TextDocument.URI.SpanURI().Filename()))
	out := make([]interface{}, len(syms))
	for i := range syms {
		out[i] = syms[i]
	}
	return out, nil
}

// DocumentSymbols returns the outline of module-level definitions in tree.
func DocumentSymbols(tree *ttcn3.Tree) []protocol.DocumentSymbol {
	var syms []protocol.DocumentSymbol
	tree.Inspect(func(n syntax.Node) bool {
		if syntax.IsNil(n) {
			return false
		}
		got, descend := symbolsAt(n)
		syms = append(syms, got...)
		return descend
	})
	return syms
}

// symbolsAt returns the symbols introduced by n.
// descend is false when those symbols already include anything nested in n.
func symbolsAt(n syntax.Node) (syms []protocol.DocumentSymbol, descend bool) {
	switch n := n.(type) {
	case *syntax.FuncDecl:
		return functionSymbol(n), false
	case *syntax.ComponentTypeDecl:
		return componentSymbol(n), false
	case *syntax.PortTypeDecl:
		return portSymbol(n), false
	case *syntax.SignatureDecl:
		return signatureSymbol(n), false
	case *syntax.SubTypeDecl:
		return subTypeSymbol(n), false
	case *syntax.TemplateDecl:
		return templateSymbol(n), false
	case *syntax.ValueDecl:
		return valueSymbols(n), false
	case *syntax.EnumTypeDecl:
		return namedSymbol(n, n.Name, "enum type", protocol.Enum), false
	case *syntax.StructTypeDecl:
		return namedSymbol(n, n.Name, tokenText(n.KindTok)+" type", protocol.Struct), false
	case *syntax.MapTypeDecl:
		return namedSymbol(n, n.Name, "map type", protocol.Struct), false
	case *syntax.BehaviourTypeDecl:
		return namedSymbol(n, n.Name, " subtype", protocol.Operator), false
	default:
		return nil, true
	}
}

func namedSymbol(n syntax.Node, name *syntax.Ident, detail string, kind protocol.SymbolKind) []protocol.DocumentSymbol {
	if name == nil {
		return nil
	}
	return []protocol.DocumentSymbol{symbol(n, name.String(), detail, kind, nil)}
}

func functionSymbol(n *syntax.FuncDecl) []protocol.DocumentSymbol {
	if n.Name == nil {
		return nil
	}

	kind := protocol.Function
	var children []protocol.DocumentSymbol
	if n.RunsOn != nil && n.RunsOn.Comp != nil {
		kind = protocol.Method
		children = append(children, symbol(n.RunsOn.Comp, "runs on", syntax.Name(n.RunsOn.Comp), protocol.Class, nil))
	}
	if n.System != nil && n.System.Comp != nil {
		kind = protocol.Method
		children = append(children, symbol(n.System.Comp, "system", syntax.Name(n.System.Comp), protocol.Class, nil))
	}
	if n.Return != nil && n.Return.Type != nil {
		children = append(children, symbol(n.Return.Type, "return", syntax.Name(n.Return.Type), protocol.Struct, nil))
	}

	detail := "definition"
	if n.KindTok != nil {
		detail = n.KindTok.String() + " " + detail
	}
	if n.External != nil {
		detail = "external " + detail
	}
	return []protocol.DocumentSymbol{symbol(n, n.Name.String(), detail, kind, children)}
}

func componentSymbol(n *syntax.ComponentTypeDecl) []protocol.DocumentSymbol {
	if n.Name == nil {
		return nil
	}

	var children []protocol.DocumentSymbol
	if len(n.Extends) > 0 {
		children = append(children, extendsSymbol(n.Extends))
	}
	if n.Body != nil {
		for _, stmt := range n.Body.Stmts {
			decl, ok := stmt.(*syntax.DeclStmt)
			if !ok {
				continue
			}
			val, ok := decl.Decl.(*syntax.ValueDecl)
			if !ok {
				continue
			}
			children = append(children, valueSymbols(val)...)
		}
	}
	return []protocol.DocumentSymbol{symbol(n, n.Name.String(), "component type", protocol.Class, children)}
}

func extendsSymbol(exprs []syntax.Expr) protocol.DocumentSymbol {
	children := make([]protocol.DocumentSymbol, len(exprs))
	for i, expr := range exprs {
		children[i] = symbol(expr, syntax.Name(expr), "", protocol.Class, nil)
	}
	return symbolBetween(syntax.Begin(exprs[0]), syntax.End(exprs[len(exprs)-1]), "extends", "", protocol.Array, children)
}

func portSymbol(n *syntax.PortTypeDecl) []protocol.DocumentSymbol {
	if n.Name == nil {
		return nil
	}

	var children []protocol.DocumentSymbol
	for _, attr := range n.Attrs {
		pa, ok := attr.(*syntax.PortAttribute)
		if !ok || pa.KindTok == nil {
			continue
		}
		switch pa.KindTok.Kind() {
		case syntax.ADDRESS:
			if len(pa.Types) > 0 {
				children = append(children, symbol(pa, "address", syntax.Name(pa.Types[0])+" type", protocol.Struct, nil))
			}
		case syntax.IN, syntax.OUT, syntax.INOUT:
			children = append(children, symbol(pa, pa.KindTok.String(), "", protocol.Array, typeSymbols(pa.Types)))
		}
	}
	return []protocol.DocumentSymbol{symbol(n, n.Name.String(), tokenText(n.KindTok)+" port type", protocol.Interface, children)}
}

func signatureSymbol(n *syntax.SignatureDecl) []protocol.DocumentSymbol {
	if n.Name == nil {
		return nil
	}

	detail := "blocking signature"
	if n.NoBlock != nil {
		detail = "non-blocking signature"
	}

	var children []protocol.DocumentSymbol
	if n.Return != nil && n.Return.Type != nil {
		children = append(children, symbol(n.Return.Type, syntax.Name(n.Return.Type), "return type", protocol.Struct, nil))
	}
	if n.Exception != nil && n.ExceptionTok != nil {
		children = append(children, symbolBetween(
			syntax.Begin(n.ExceptionTok), syntax.End(n.Exception),
			"Exceptions", "", protocol.Array, typeSymbols(n.Exception.List),
		))
	}
	return []protocol.DocumentSymbol{symbol(n, n.Name.String(), detail, protocol.Function, children)}
}

func subTypeSymbol(n *syntax.SubTypeDecl) []protocol.DocumentSymbol {
	if n.Field == nil || n.Field.Name == nil {
		return nil
	}

	detail, kind := "subtype", protocol.Struct
	var children []protocol.DocumentSymbol
	if list, ok := n.Field.Type.(*syntax.ListSpec); ok {
		detail, kind = tokenText(list.KindTok)+" of type", protocol.Array
		if ref, ok := list.ElemType.(*syntax.RefSpec); ok && ref != nil {
			children = []protocol.DocumentSymbol{symbol(ref, syntax.Name(ref), "element type", protocol.Struct, nil)}
		}
	}
	return []protocol.DocumentSymbol{symbol(n, n.Field.Name.String(), detail, kind, children)}
}

func templateSymbol(n *syntax.TemplateDecl) []protocol.DocumentSymbol {
	if n.Name == nil {
		return nil
	}

	var children []protocol.DocumentSymbol
	if n.ModifiesTok != nil {
		if name := syntax.Name(n.Base); name != "" {
			children = []protocol.DocumentSymbol{symbol(n.Base, name, "template", protocol.Constant, nil)}
		}
	}
	return []protocol.DocumentSymbol{symbol(n, n.Name.String(), "template "+syntax.Name(n.Type), protocol.Constant, children)}
}

func valueSymbols(n *syntax.ValueDecl) []protocol.DocumentSymbol {
	detail, kind := valueDetail(n)
	syms := make([]protocol.DocumentSymbol, len(n.Decls))
	for i, decl := range n.Decls {
		syms[i] = symbol(n, syntax.Name(decl), detail, kind, nil)
	}
	return syms
}

func valueDetail(n *syntax.ValueDecl) (string, protocol.SymbolKind) {
	detail := syntax.Name(n.Type)
	kind := protocol.Variable
	if detail == "timer" {
		kind = protocol.Event
	}
	if n.KindTok == nil {
		return detail, kind
	}
	detail = n.KindTok.String() + " " + detail
	switch n.KindTok.Kind() {
	case syntax.PORT:
		kind = protocol.Interface
	case syntax.CONST, syntax.TEMPLATE, syntax.MODULEPAR:
		kind = protocol.Constant
	}
	return detail, kind
}

func typeSymbols(types []syntax.Expr) []protocol.DocumentSymbol {
	var syms []protocol.DocumentSymbol
	for _, t := range types {
		if name := syntax.Name(t); name != "" {
			syms = append(syms, symbol(t, name, "type", protocol.Struct, nil))
		}
	}
	return syms
}

func symbol(n syntax.Node, name, detail string, kind protocol.SymbolKind, children []protocol.DocumentSymbol) protocol.DocumentSymbol {
	return symbolBetween(syntax.Begin(n), syntax.End(n), name, detail, kind, children)
}

func symbolBetween(begin, end syntax.Position, name, detail string, kind protocol.SymbolKind, children []protocol.DocumentSymbol) protocol.DocumentSymbol {
	rng := setProtocolRange(begin, end)
	return protocol.DocumentSymbol{
		Name:           name,
		Detail:         detail,
		Kind:           kind,
		Range:          rng,
		SelectionRange: rng,
		Children:       children,
	}
}

func tokenText(tok syntax.Token) string {
	if tok == nil {
		return ""
	}
	return tok.String()
}

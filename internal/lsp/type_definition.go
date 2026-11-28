package lsp

import (
	"context"

	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// typeDefinition implements textDocument/typeDefinition: instead of
// jumping to the declaration of the variable under the cursor (which is
// what `Definition` does), we jump to the declaration of *its type*.
//
// This reuses the typeOf machinery already exposed by Tree.TypeOf, which
// the hover provider also relies on.
func (s *Server) typeDefinition(ctx context.Context, params *protocol.TypeDefinitionParams) (interface{}, error) {
	if params == nil {
		return nil, nil
	}

	file := string(params.TextDocument.URI.SpanURI())
	tree := ttcn3.ParseFile(file)
	if tree == nil || tree.Root == nil {
		return nil, nil
	}

	line := int(params.Position.Line) + 1
	col := int(params.Position.Character) + 1
	x := tree.IdentifierAt(line, col)
	if x == nil {
		return nil, nil
	}

	var locs []protocol.Location
	for _, def := range tree.LookupWithDB(x, &s.db) {
		for _, typ := range tree.TypeOf(def.Node, &s.db) {
			anchor := typeAnchor(typ.Node)
			if anchor == nil {
				continue
			}
			locs = append(locs, location(syntax.SpanOf(anchor)))
		}
	}

	return unifyLocs(locs), nil
}

// typeAnchor returns the most useful "definition anchor" for a type
// node - usually the identifier that names the type. For anonymous types
// (like inline record specs) we fall back to the type node itself so the
// editor can still jump to the source span.
func typeAnchor(n syntax.Node) syntax.Node {
	switch t := n.(type) {
	case *syntax.StructTypeDecl:
		return t.Name
	case *syntax.EnumTypeDecl:
		return t.Name
	case *syntax.PortTypeDecl:
		return t.Name
	case *syntax.ComponentTypeDecl:
		return t.Name
	case *syntax.MapTypeDecl:
		return t.Name
	case *syntax.BehaviourTypeDecl:
		return t.Name
	case *syntax.SubTypeDecl:
		if t.Field != nil {
			return t.Field.Name
		}
		return t
	}
	return n
}

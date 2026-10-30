package lsp

import (
	"context"

	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// documentHighlight implements textDocument/documentHighlight, returning
// every occurrence of the identifier under the cursor inside the current
// file. Unlike references we never cross file boundaries: highlights are
// a within-document operation only.
//
// We mark the cursor position as Write only when the identifier is the
// declared name of a node (Ident.IsName), otherwise Read. This is enough
// for editors to render the "cursor declared this here" hint.
func (s *Server) documentHighlight(ctx context.Context, params *protocol.DocumentHighlightParams) ([]protocol.DocumentHighlight, error) {
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
	target, ok := tree.IdentifierAt(line, col).(*syntax.Ident)
	if !ok || target == nil || target.Tok == nil {
		return nil, nil
	}
	name := target.Tok.String()

	var hits []protocol.DocumentHighlight
	tree.Inspect(func(n syntax.Node) bool {
		id, ok := n.(*syntax.Ident)
		if !ok || id == nil {
			return true
		}
		if id.Tok != nil && id.Tok.String() == name {
			hits = append(hits, highlightForIdent(id, id.Tok))
		}
		if id.Tok2 != nil && id.Tok2.String() == name {
			hits = append(hits, highlightForIdent(id, id.Tok2))
		}
		return true
	})

	return hits, nil
}

func highlightForIdent(id *syntax.Ident, tok syntax.Token) protocol.DocumentHighlight {
	kind := protocol.Read
	if id.IsName {
		kind = protocol.Write
	}
	span := syntax.SpanOf(tok)
	return protocol.DocumentHighlight{
		Range: setProtocolRange(span.Begin, span.End),
		Kind:  kind,
	}
}

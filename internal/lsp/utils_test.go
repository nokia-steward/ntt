package lsp

import (
	"context"
	"testing"

	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/stretchr/testify/assert"
)

func TestUnifyLocs(t *testing.T) {
	loc := func(line uint32) protocol.Location {
		return protocol.Location{
			URI:   "file:///a.ttcn3",
			Range: protocol.Range{Start: protocol.Position{Line: line}},
		}
	}
	actual := unifyLocs([]protocol.Location{loc(3), loc(1), loc(3), loc(2), loc(1)})
	assert.Equal(t, []protocol.Location{loc(3), loc(1), loc(2)}, actual)
}

func TestRequestsOnUnreadableFile(t *testing.T) {
	ctx := context.Background()
	doc := protocol.TextDocumentIdentifier{URI: "file:///nonexistent/TestRequestsOnUnreadableFile.ttcn3"}
	pos := protocol.TextDocumentPositionParams{TextDocument: doc, Position: protocol.Position{Line: 1, Character: 1}}
	s := &Server{serverConfig: Config{SemanticTokensEnabled: true, InlayHintEnabled: true}}

	tree, x := identifierAt(doc, pos.Position)
	assert.Nil(t, tree)
	assert.Nil(t, x)

	def, err := s.definition(ctx, &protocol.DefinitionParams{TextDocumentPositionParams: pos})
	assert.NoError(t, err)
	assert.Nil(t, def)

	hover, err := ProcessHover(&protocol.HoverParams{TextDocumentPositionParams: pos}, &s.db, &MarkdownHover{})
	assert.NoError(t, err)
	assert.Nil(t, hover)

	refs, err := s.references(ctx, &protocol.ReferenceParams{TextDocumentPositionParams: pos})
	assert.NoError(t, err)
	assert.Nil(t, refs)

	toks, err := s.semanticTokens(ctx, &protocol.SemanticTokensParams{TextDocument: doc})
	assert.NoError(t, err)
	assert.Nil(t, toks)

	toks, err = s.semanticTokensRange(ctx, &protocol.SemanticTokensRangeParams{TextDocument: doc})
	assert.NoError(t, err)
	assert.Nil(t, toks)

	hints, err := s.inlayHint(ctx, &protocol.InlayHintParams{TextDocument: doc})
	assert.NoError(t, err)
	assert.Nil(t, hints)

	syms, err := s.documentSymbol(ctx, &protocol.DocumentSymbolParams{TextDocument: doc})
	assert.NoError(t, err)
	assert.Nil(t, syms)
}

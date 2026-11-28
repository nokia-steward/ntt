package lsp

import (
	"context"
	"testing"

	"github.com/nokia/ntt/internal/lsp/protocol"
)

func TestDocumentHighlight_FindsAllOccurrences(t *testing.T) {
	const src = `module M {
	function f() {
		var integer x := 1;
		x := x + 1;
	}
}`
	uri := setUpFakeFile(t, src)

	s := &Server{}
	got, err := s.documentHighlight(context.Background(), &protocol.DocumentHighlightParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 2, Character: 14}, // on the first `x`
		},
	})
	if err != nil {
		t.Fatalf("documentHighlight returned error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 highlights for `x`, got %d: %v", len(got), got)
	}
}

func TestDocumentHighlight_NoIdentifierAtPosition(t *testing.T) {
	const src = `module M { }`
	uri := setUpFakeFile(t, src)

	s := &Server{}
	got, err := s.documentHighlight(context.Background(), &protocol.DocumentHighlightParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
	})
	if err != nil {
		t.Fatalf("documentHighlight returned error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil result, got %v", got)
	}
}

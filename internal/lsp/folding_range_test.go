package lsp

import (
	"context"
	"testing"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/lsp/protocol"
)

func TestFoldingRange_EmptyOnSingleLine(t *testing.T) {
	const src = "module M { function f() { return; } }"
	uri := setUpFakeFile(t, src)

	s := &Server{}
	got, err := s.foldingRange(context.Background(), &protocol.FoldingRangeParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	if err != nil {
		t.Fatalf("foldingRange returned error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no folds on a single line, got %v", got)
	}
}

func TestFoldingRange_MultiLineBlocks(t *testing.T) {
	const src = `module M {
	function f() {
		return;
	}
}`
	uri := setUpFakeFile(t, src)

	s := &Server{}
	got, err := s.foldingRange(context.Background(), &protocol.FoldingRangeParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	if err != nil {
		t.Fatalf("foldingRange returned error: %v", err)
	}
	if len(got) < 2 {
		t.Fatalf("expected at least 2 fold ranges (module + function body), got %d: %v", len(got), got)
	}
	for _, r := range got {
		if r.EndLine <= r.StartLine {
			t.Errorf("invalid fold range %+v: end must be after start", r)
		}
	}
}

func setUpFakeFile(t *testing.T, src string) protocol.DocumentURI {
	t.Helper()
	uri := protocol.DocumentURI("file:///" + t.Name() + ".ttcn3")
	fs.SetContent(string(uri), []byte(src))
	return uri
}

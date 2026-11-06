package lsp

import (
	"testing"

	"github.com/nokia/ntt/internal/lsp/protocol"
)

func TestApplyIncrementalChange_Insert(t *testing.T) {
	const src = "hello\nworld\n"
	ch := protocol.TextDocumentContentChangeEvent{
		Range: &protocol.Range{
			Start: protocol.Position{Line: 0, Character: 5},
			End:   protocol.Position{Line: 0, Character: 5},
		},
		Text: " there",
	}
	got, ok := applyIncrementalChange([]byte(src), ch)
	if !ok {
		t.Fatal("applyIncrementalChange returned !ok")
	}
	want := "hello there\nworld\n"
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestApplyIncrementalChange_Replace(t *testing.T) {
	const src = "line one\nline two\nline three\n"
	ch := protocol.TextDocumentContentChangeEvent{
		Range: &protocol.Range{
			Start: protocol.Position{Line: 1, Character: 5},
			End:   protocol.Position{Line: 1, Character: 8},
		},
		Text: "TWO",
	}
	got, ok := applyIncrementalChange([]byte(src), ch)
	if !ok {
		t.Fatal("applyIncrementalChange returned !ok")
	}
	want := "line one\nline TWO\nline three\n"
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestApplyIncrementalChange_Delete(t *testing.T) {
	const src = "abc\nXYZ\n"
	ch := protocol.TextDocumentContentChangeEvent{
		Range: &protocol.Range{
			Start: protocol.Position{Line: 1, Character: 0},
			End:   protocol.Position{Line: 1, Character: 3},
		},
		Text: "",
	}
	got, ok := applyIncrementalChange([]byte(src), ch)
	if !ok {
		t.Fatal("applyIncrementalChange returned !ok")
	}
	want := "abc\n\n"
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPositionToOffset_ClampsPastEOF(t *testing.T) {
	const src = "abc"
	off, ok := positionToOffset([]byte(src), 5, 0)
	if !ok {
		t.Fatal("positionToOffset must always succeed")
	}
	if off != len(src) {
		t.Fatalf("got offset %d, want %d", off, len(src))
	}
}

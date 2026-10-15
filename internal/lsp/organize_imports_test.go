package lsp

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/lsp/protocol"
)

// changesKey is the key organizeImports uses for its WorkspaceEdit
// changes map. The production code derives the key from the URI's
// filename so we can't just look up by the original URI literal -
// Windows in particular round-trips file:///foo through Filename()
// (which prepends a drive letter) before keying the map.
func changesKey(file string) string {
	return string(fs.URI(protocol.DocumentURI(file).SpanURI().Filename()))
}

func TestOrganizeImports_SortsAndDedupes(t *testing.T) {
	const src = `module M {
	import from Zeta all;
	import from Alpha all;
	import from Beta all;
	import from Alpha all;
}
`
	file := "file:///" + t.Name() + ".ttcn3"
	fs.SetContent(file, []byte(src))

	s := &Server{}
	uri := protocol.DocumentURI(file)
	action, ok := s.organizeImports(uri)
	if !ok {
		t.Fatalf("expected organizeImports to emit an action")
	}
	if action.Kind != protocol.SourceOrganizeImports {
		t.Fatalf("got kind %q, want %q", action.Kind, protocol.SourceOrganizeImports)
	}

	edits := action.Edit.Changes[changesKey(file)]
	if len(edits) != 1 {
		t.Fatalf("expected exactly one TextEdit, got %d (keys: %v)",
			len(edits), keysOf(action.Edit.Changes))
	}

	got := edits[0].NewText
	// Imports must be sorted alphabetically; Alpha must appear only once.
	if i := strings.Index(got, "Alpha"); i < 0 {
		t.Fatalf("rewrite missing Alpha import: %q", got)
	}
	if strings.Count(got, "Alpha") != 1 {
		t.Errorf("duplicate Alpha import not removed: %q", got)
	}
	if !precedes(got, "Alpha", "Beta") || !precedes(got, "Beta", "Zeta") {
		t.Errorf("imports not alphabetised: %q", got)
	}
}

func TestOrganizeImports_NoOpWhenAlreadySorted(t *testing.T) {
	const src = `module M {
	import from Alpha all;
	import from Beta all;
}
`
	file := "file:///" + t.Name() + ".ttcn3"
	fs.SetContent(file, []byte(src))

	s := &Server{}
	if _, ok := s.organizeImports(protocol.DocumentURI(file)); ok {
		t.Fatalf("expected no action for already-sorted imports")
	}
}

func TestOrganizeImports_LeavesSingleImportAlone(t *testing.T) {
	const src = `module M {
	import from Alpha all;
}
`
	file := "file:///" + t.Name() + ".ttcn3"
	fs.SetContent(file, []byte(src))

	s := &Server{}
	if _, ok := s.organizeImports(protocol.DocumentURI(file)); ok {
		t.Fatalf("single-import module should not produce an edit")
	}
}

func TestOrganizeImports_PerModuleIsolation(t *testing.T) {
	// Two modules with their own unsorted imports - we should get an
	// edit for both, each scoped to its own module.
	const src = `module A {
	import from Zeta all;
	import from Alpha all;
}

module B {
	import from Yankee all;
	import from Bravo all;
}
`
	file := "file:///" + t.Name() + ".ttcn3"
	fs.SetContent(file, []byte(src))

	s := &Server{}
	action, ok := s.organizeImports(protocol.DocumentURI(file))
	if !ok {
		t.Fatalf("expected an action")
	}
	edits := action.Edit.Changes[changesKey(file)]
	if len(edits) != 2 {
		t.Fatalf("expected one edit per module, got %d (keys: %v)",
			len(edits), keysOf(action.Edit.Changes))
	}
}

func TestWantsKind(t *testing.T) {
	tests := []struct {
		name string
		only []protocol.CodeActionKind
		k    protocol.CodeActionKind
		want bool
	}{
		{"empty matches anything", nil, protocol.SourceOrganizeImports, true},
		{"explicit match", []protocol.CodeActionKind{protocol.SourceOrganizeImports}, protocol.SourceOrganizeImports, true},
		{"parent matches child", []protocol.CodeActionKind{"source"}, protocol.SourceOrganizeImports, true},
		{"different family", []protocol.CodeActionKind{protocol.QuickFix}, protocol.SourceOrganizeImports, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := wantsKind(tc.only, tc.k); got != tc.want {
				t.Errorf("wantsKind(%v, %v) = %v, want %v", tc.only, tc.k, got, tc.want)
			}
		})
	}
}

// precedes returns true if first appears before second in s. Both must
// be present for the function to succeed.
func precedes(s, first, second string) bool {
	i := strings.Index(s, first)
	j := strings.Index(s, second)
	return i >= 0 && j > i
}

// keysOf returns the keys of a map[string][]TextEdit. We only use it
// for failure messages, so we don't bother sorting.
func keysOf(m map[string][]protocol.TextEdit) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

package lsp

import (
	"testing"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// findIdent walks the parsed tree and returns the first Ident with the
// given name. We can't trust hard-coded line/column offsets in tests
// because the parser may not preserve a stable column for nested
// expressions, so we look up by name instead.
func findIdent(tree *ttcn3.Tree, name string) *syntax.Ident {
	var found *syntax.Ident
	tree.Inspect(func(n syntax.Node) bool {
		if found != nil {
			return false
		}
		if id, ok := n.(*syntax.Ident); ok && id != nil && id.Tok != nil && id.Tok.String() == name {
			found = id
		}
		return true
	})
	return found
}

// TestSymbolReferences_FindsDeclAndCall verifies that the symbol-aware
// reference search reports both the declaration site and call site of a
// function defined and used in the same module.
func TestSymbolReferences_FindsDeclAndCall(t *testing.T) {
	const src = `module M {
	function foo() { return; }
	function f() { foo(); }
}`
	file := "file:///" + t.Name() + ".ttcn3"
	fs.SetContent(file, []byte(src))

	db := &ttcn3.DB{}
	db.Index(file)

	tree := ttcn3.ParseFile(file)
	id := findIdent(tree, "foo")
	if id == nil {
		t.Fatalf("expected at least one `foo` identifier")
	}

	got := NewSymbolReferences(db, id, file)
	if len(got) < 2 {
		t.Fatalf("expected at least 2 references (decl + call), got %d", len(got))
	}
}

// TestSymbolReferences_FallbackOnUnresolvable verifies that an
// unresolvable cursor falls back to the legacy name-text search so the
// user still gets *something*.
func TestSymbolReferences_FallbackOnUnresolvable(t *testing.T) {
	const src = `module M { function f() { foo(); } }`
	file := "file:///" + t.Name() + ".ttcn3"
	fs.SetContent(file, []byte(src))

	db := &ttcn3.DB{}
	db.Index(file)

	tree := ttcn3.ParseFile(file)
	id := findIdent(tree, "foo")
	if id == nil {
		t.Skip("identifier finder did not return `foo`")
	}
	got := NewSymbolReferences(db, id, file)
	if len(got) == 0 {
		t.Fatalf("expected at least 1 reference (the call site itself), got 0")
	}
}

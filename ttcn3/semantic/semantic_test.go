package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func parse(t *testing.T, src string) *ttcn3.Tree {
	t.Helper()
	tree := ttcn3.Parse(src)
	if tree.Err != nil {
		t.Fatalf("parse error: %v\nsource:\n%s", tree.Err, src)
	}
	return tree
}

func TestAnalyze_UnknownImport(t *testing.T) {
	tree := parse(t, `module M { import from Nope all; }`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if len(diags) == 0 {
		t.Fatalf("expected an unknown-import diagnostic, got none")
	}
	if diags[0].Code != "unknown-import" {
		t.Fatalf("got code %q, want unknown-import", diags[0].Code)
	}
}

func TestAnalyze_DuplicateDefinition(t *testing.T) {
	tree := parse(t, `module M {
		function f() { return; }
		function f() { return; }
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if len(diags) == 0 {
		t.Fatalf("expected a duplicate-definition diagnostic, got none")
	}
	if diags[0].Code != "duplicate-definition" {
		t.Fatalf("got code %q, want duplicate-definition", diags[0].Code)
	}
}

func TestAnalyze_NoSelfImportFalsePositive(t *testing.T) {
	tree := parse(t, `module M { import from M all; }`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if len(diags) != 0 {
		t.Fatalf("self-imports should be silent, got %v", diags)
	}
}

func TestIsPredefinedType(t *testing.T) {
	if !IsPredefinedType("integer") {
		t.Fatal("integer must be predefined")
	}
	if IsPredefinedType("MyType") {
		t.Fatal("MyType must not be predefined")
	}
}

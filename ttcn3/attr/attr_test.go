package attr_test

import (
	"reflect"
	"testing"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/attr"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// withSpecOf parses src as a TTCN-3 module and returns the first WithSpec
// the parser produced. Tests use this so they can keep their fixtures as
// real TTCN-3 source rather than hand-built AST nodes.
func withSpecOf(t *testing.T, src string) *syntax.WithSpec {
	t.Helper()
	tree := ttcn3.Parse(src)
	if tree == nil || tree.Root == nil {
		t.Fatalf("parse returned nil tree")
	}
	if err := tree.Err; err != nil {
		t.Fatalf("parse error: %v", err)
	}

	var found *syntax.WithSpec
	tree.Root.Inspect(func(n syntax.Node) bool {
		if w, ok := n.(*syntax.WithSpec); ok && found == nil {
			found = w
			return false
		}
		return true
	})
	if found == nil {
		t.Fatalf("no WithSpec found in %q", src)
	}
	return found
}

func TestParse_NilWithSpec(t *testing.T) {
	set, diags := attr.Parse(nil)
	if set == nil {
		t.Fatal("expected non-nil AttributeSet")
	}
	if len(set.Attributes) != 0 {
		t.Errorf("expected 0 attributes, got %d", len(set.Attributes))
	}
	if len(diags) != 0 {
		t.Errorf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestParse_EncodeAndVariant(t *testing.T) {
	src := `module M {
		type record R {
			integer i
		} with {
			encode "RAW";
			variant "FIELDORDER(msb)";
			variant (i) "FIELDLENGTH(8)";
		}
	}`
	set, diags := attr.Parse(withSpecOf(t, src))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	if got, want := len(set.Attributes), 3; got != want {
		t.Fatalf("got %d attributes, want %d", got, want)
	}
	if got, want := set.EncodingName(), "RAW"; got != want {
		t.Errorf("EncodingName: got %q, want %q", got, want)
	}
	variants := set.Variants()
	if len(variants) != 2 {
		t.Fatalf("expected 2 variants, got %d", len(variants))
	}
	if variants[0].Value != "FIELDORDER(msb)" {
		t.Errorf("variant[0].Value = %q", variants[0].Value)
	}
	if !reflect.DeepEqual(variants[1].Selectors, []string{"i"}) {
		t.Errorf("variant[1].Selectors = %v, want [i]", variants[1].Selectors)
	}
}

func TestParse_Override(t *testing.T) {
	src := `module M {
		type integer T with { encode override "JSON" }
	}`
	set, diags := attr.Parse(withSpecOf(t, src))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	if len(set.Attributes) != 1 {
		t.Fatalf("expected 1 attribute, got %d", len(set.Attributes))
	}
	a := set.Attributes[0]
	if !a.Override {
		t.Error("expected Override=true")
	}
	if a.Kind != attr.Encode {
		t.Errorf("Kind = %v, want Encode", a.Kind)
	}
	if a.Value != "JSON" {
		t.Errorf("Value = %q, want \"JSON\"", a.Value)
	}
}

func TestParse_Extension(t *testing.T) {
	src := `module M {
		type record R { integer i } with { extension "prototype(convert)" }
	}`
	set, _ := attr.Parse(withSpecOf(t, src))
	ext := set.Extensions()
	if len(ext) != 1 {
		t.Fatalf("expected 1 extension, got %d", len(ext))
	}
	if ext[0].Value != "prototype(convert)" {
		t.Errorf("Value = %q", ext[0].Value)
	}
}

func TestParse_OptionalImplicitOmit(t *testing.T) {
	src := `module M {
		type record R { integer i } with { optional "implicit omit" }
	}`
	set, _ := attr.Parse(withSpecOf(t, src))
	if !set.HasOptionalImplicitOmit() {
		t.Error("HasOptionalImplicitOmit = false, want true")
	}
}

func TestParse_QuotedQuotesAreCollapsed(t *testing.T) {
	src := `module M {
		type integer T with { variant "name(""x"")" }
	}`
	set, _ := attr.Parse(withSpecOf(t, src))
	if len(set.Attributes) != 1 {
		t.Fatalf("expected 1 attribute, got %d", len(set.Attributes))
	}
	if got, want := set.Attributes[0].Value, `name("x")`; got != want {
		t.Errorf("Value = %q, want %q", got, want)
	}
}

func TestKind_String(t *testing.T) {
	cases := []struct {
		k    attr.Kind
		want string
	}{
		{attr.Encode, "encode"},
		{attr.Variant, "variant"},
		{attr.Extension, "extension"},
		{attr.Display, "display"},
		{attr.Optional, "optional"},
		{attr.Unknown, "unknown"},
	}
	for _, tc := range cases {
		if got := tc.k.String(); got != tc.want {
			t.Errorf("Kind(%d).String() = %q, want %q", tc.k, got, tc.want)
		}
	}
}

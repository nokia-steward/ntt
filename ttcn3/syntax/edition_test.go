package syntax

import "testing"

func TestModuleEdition(t *testing.T) {
	defer func(l string) { DefaultLanguage = l }(DefaultLanguage)
	tests := []struct {
		src      string
		def      string
		expected Edition
	}{
		{`module M {}`, "", V4},
		{`module M {}`, V5Language, V5},
		{`module M language "TTCN-3:5" {}`, "", V5},
		{`module M language "TTCN-3:2026" {}`, V5Language, V4},
		{`module M language "TTCN-3:5 Object-Oriented" {}`, "", V5},
		{`module M language "TTCN-3:50" {}`, V5Language, V4},
		{`module M language "TTCN-3:5.1" {}`, V5Language, V4},
		{`module M language "TTCN-3:2018 Object-Oriented" {}`, V5Language, V4},
		{`module M language "XSD" {}`, V5Language, V5},
		{`module M language "XSD" {}`, "", V4},
	}
	for _, tt := range tests {
		DefaultLanguage = tt.def
		root := Parse([]byte(tt.src))
		if root.Err() != nil {
			t.Fatalf("%s: %v", tt.src, root.Err())
		}
		mod, ok := root.Nodes[0].(*Module)
		if !ok {
			t.Fatalf("%s: no module", tt.src)
		}
		if got := mod.Edition(); got != tt.expected {
			t.Errorf("%s with default %q: edition %d, want %d", tt.src, tt.def, got, tt.expected)
		}
	}
}

// A module's own default language wins over DefaultLanguage; empty, it
// makes the module V4 whatever DefaultLanguage says.
func TestModuleDefaultLanguage(t *testing.T) {
	defer func(l string) { DefaultLanguage = l }(DefaultLanguage)
	DefaultLanguage = V5Language
	parse := func() *Module { return Parse([]byte(`module M {}`)).Nodes[0].(*Module) }
	m, other := parse(), parse()
	m.SetDefaultLanguage("")
	if m.Edition() != V4 || other.Edition() != V5 {
		t.Fatalf("editions %d and %d, want V4 and V5", m.Edition(), other.Edition())
	}
	m.SetDefaultLanguage(V5Language)
	if m.Edition() != V5 {
		t.Fatalf("edition %d after setting V5, want V5", m.Edition())
	}
}

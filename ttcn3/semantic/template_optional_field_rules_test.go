package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// Optional fields of templates in assignment notation (15.1, 15.2).
func TestTemplateOptionalFieldRules(t *testing.T) {
	cases := []struct {
		name, defs, want string
	}{
		{"optional field left out", "template R m_r := { f2 := \"a\" };", "template-optional-field-uninitialised"},
		{"optional field of a signature parameter left out", "template S s_t := { p1 := { f2 := \"a\" } };", "template-optional-field-uninitialised"},

		{"optional field omitted", "template R m_r := { f1 := omit, f2 := \"a\" };", ""},
		{"optional field left out under implicit omit", "template R m_r := { f2 := \"a\" } with { optional \"implicit omit\" };", ""},
		{"modified template", "template R m_b := { f1 := 1, f2 := \"a\" }; template R m_r modifies m_b := { f2 := \"b\" };", ""},
		{"value list notation", "template R m_r := { 1, \"a\" };", ""},
	}
	for _, c := range cases {
		tree := parse(t, `module M {
			type record R { integer f1 optional, charstring f2 }
			signature S(in R p1);
			`+c.defs+`
		}`)
		got := codes(NewAnalyzer(&ttcn3.DB{}).Analyze(tree))
		has := slices.Contains(got, "template-optional-field-uninitialised")
		switch {
		case c.want == "" && has:
			t.Errorf("%s: unexpected template-optional-field-uninitialised", c.name)
		case c.want != "" && !has:
			t.Errorf("%s: expected %s, got %v", c.name, c.want, got)
		}
	}
}

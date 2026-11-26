package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// Templates are of data and component types, and of structured types made
// of those (15 a).
func TestTemplateDataTypeRules(t *testing.T) {
	cases := []struct {
		name, defs, want string
	}{
		{"template variable of a record with a default field", "function f() { var template RD v_t := { f1 := 1, f2 := null }; }", "template-non-data-type"},
		{"value parameter of a template", "template integer m_v(RD p_r) := p_r.f1;", "template-non-data-type"},
		{"template parameter of a template", "template integer m_v(template RD p_r) := 1;", "template-non-data-type"},
		{"function returning a template of the default type", "function f() return template default { return null; }", "template-non-data-type"},
		{"union with a nested default field", "type union U { integer c1, RD c2 } template U m_u := { c1 := 5 }", "template-non-data-type"},
		{"record of port", "type record of P RoP; function f(template RoP p_t) {}", "template-non-data-type"},

		{"template of a record", "template R m_r := { f1 := 1 }", ""},
		{"template of a component type", "function f() { var template C v_c := ?; }", ""},
		{"value parameter of a function", "function f(RD p_r) {}", ""},
		{"variable of a record with a default field", "function f() { var RD v_r; }", ""},
		{"recursive type", "type record Rec { integer i, Rec next optional } template Rec m_rec := { i := 1, next := omit }", ""},
	}
	ruleCodes := []string{"template-non-data-type"}
	for _, c := range cases {
		tree := parse(t, `module M {
			type record R { integer f1 }
			type record RD { integer f1, default f2 }
			type port P message { inout integer }
			type component C { port P p }
			`+c.defs+`
		}`)
		got := codes(NewAnalyzer(&ttcn3.DB{}).Analyze(tree))
		var hit []string
		for _, code := range got {
			if slices.Contains(ruleCodes, code) {
				hit = append(hit, code)
			}
		}
		switch {
		case c.want == "" && len(hit) > 0:
			t.Errorf("%s: unexpected %v", c.name, hit)
		case c.want != "" && !slices.Contains(got, c.want):
			t.Errorf("%s: expected %s, got %v", c.name, c.want, got)
		}
	}
}

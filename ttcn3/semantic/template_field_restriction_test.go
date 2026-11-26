package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// A field, alternative or element of a restricted template is restricted
// as table 13c says, when it is assigned and when it is referenced
// (15.8.2); a whole template variable is assigned within its restriction
// too (15.8.1).
func TestTemplateFieldRestriction(t *testing.T) {
	cases := []struct {
		name, stmt, want string
	}{
		{"any value into a mandatory field of a template(value)", "var template(value) R v_t := { a := 1, b := omit }; v_t.a := ?;", "template-restriction-violation"},
		{"any value into an element of a template(omit)", "var template(omit) RoI v_t := { 1, 2 }; v_t[0] := ?;", "template-restriction-violation"},
		{"omit into a template(value)", "var template(value) R v_t; v_t := omit;", "template-restriction-violation"},
		{"value list into an array element", "var template(value) integer v_t[2]; v_t[1] := (1, 2);", "template-restriction-violation"},
		{"field of a template(present) to a template(value) parameter", "var template(present) R v_t := { a := 1, b := omit }; f(v_t.a);", "template-restriction-assignment"},
		{"unrestricted template into a field of a template(value)", "var template integer v_i := ?; var template(value) R v_t; v_t.a := v_i;", "template-restriction-assignment"},
		{"field of an unrestricted template to a template(value) parameter", "var template R v_t := { a := 1, b := * }; f(v_t.a);", "template-restriction-assignment"},
		{"any value or none into a template(present)", "var template(present) integer v_t; v_t := *;", "template-restriction-violation"},

		{"omit into an optional field of a template(value)", "var template(value) R v_t := { a := 1, b := 2 }; v_t.b := omit;", ""},
		{"any value into a field of a template(present)", "var template(present) R v_t := { a := 1, b := omit }; v_t.a := ?;", ""},
		{"any value or none into an optional field of a template(present)", "var template(present) R v_t := { a := 1, b := omit }; v_t.b := *;", ""},
		{"field of a template(value) to a template(value) parameter", "var template(value) R v_t := { a := 1, b := omit }; f(v_t.a);", ""},
		{"value into a field of a template(value)", "var template(value) R v_t; v_t.a := 3;", ""},
		{"any value into an unrestricted template", "var template R v_t; v_t.a := ?;", ""},
		{"field of a value", "var R v_r := { a := 1, b := omit }; f(v_r.a);", ""},
		{"field of an unrestricted template to a template(present) parameter", "var template R v_t := { a := 1, b := * }; g(v_t.a);", ""},
		{"any value or none into a field of an unrestricted template", "var template R v_t; v_t.a := *;", ""},
	}
	ruleCodes := []string{"template-restriction-violation", "template-restriction-assignment", "template-restriction-field"}
	for _, c := range cases {
		tree := parse(t, `module M {
			type record R { integer a, integer b optional }
			type record of integer RoI;
			function f(template(value) integer p_t) {}
			function g(template(present) integer p_t) {}
			type component C {}
			testcase TC() runs on C {
				`+c.stmt+`
			}
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

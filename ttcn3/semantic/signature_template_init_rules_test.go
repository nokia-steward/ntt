package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// A signature template initialises its parameters, not the return value
// (15.2), all of them or those of one direction with inout ones (15.3 e).
func TestSignatureTemplateInitRules(t *testing.T) {
	cases := []struct {
		name, defs, want string
	}{
		{"return value in the parameter list", "template S s_t := { 1, 2, 3, 4 }", "signature-template-return-value"},
		{"inout parameter not used", "template S s_t := { p1 := 1, p2 := 2, p3 := - }", "signature-template-partial"},
		{"in parameter and out parameter only", "template S s_t := { 1, 2, - }", "signature-template-partial"},
		{"local template with an inout parameter missing", "function f() { template S s_t := { p1 := 1, p2 := 2 } }", "signature-template-partial"},

		{"all parameters", "template S s_t := { 1, 2, 3 }", ""},
		{"in and inout parameters", "template S s_t := { p1 := 1, p2 := -, p3 := 3 }", ""},
		{"out and inout parameters", "template S s_t := { p1 := -, p2 := ?, p3 := ? }", ""},
		{"modified template", "template S s_b := { 1, 2, 3 } template S s_t modifies s_b := { p3 := - }", ""},
		{"abstract template", "template @abstract S s_t := { p1 := 1, p2 := 2, p3 := - }", ""},
		{"no parameters", "signature S0(); template S0 s_t := {}", ""},
	}
	ruleCodes := []string{"signature-template-return-value", "signature-template-partial"}
	for _, c := range cases {
		tree := parse(t, `module M {
			signature S(in integer p1, out integer p2, inout integer p3) return integer;
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

package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// countCode is the number of diagnostics of code.
func countCode(diags []Diagnostic, code string) int {
	n := 0
	for _, d := range diags {
		if d.Code == code {
			n++
		}
	}
	return n
}

// TestStaticRulesInControlPart: the rules of the misc pass apply in a
// control part as in a function.
func TestStaticRulesInControlPart(t *testing.T) {
	for src, code := range map[string]string{
		`control { var charstring c := int2char(1, 2); }`: "predefined-arity",
		`control { timer t := infinity; }`:                "timer-infinite-duration",
		`control { var float f := -not_a_number; }`:       "negated-not-a-number",
	} {
		diags := NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, `module M { `+src+` }`))
		if !containsCode(diags, code) {
			t.Errorf("%s: expected %s, got %v", src, code, codes(diags))
		}
	}
}

// TestRuleScopeAndOnce: no false reject on a field or parameter of an
// operation's name, on arrays of lists, and one diagnostic per defect.
func TestRuleScopeAndOnce(t *testing.T) {
	good := map[string]string{
		`type record R { integer clear } control { var R r := {1}; log(r.clear); }`:                                                                               "control-part-port-op",
		`type record of integer RoI; function f() { var RoI arr[2]; for (var RoI x in arr) {} }`:                                                                  "range-loop-element-type",
		`type record of integer RoI; function f(RoI p[2]) { for (var RoI x in p) {} }`:                                                                            "range-loop-element-type",
		`type port P message { inout integer } type component C { port P p } template integer t := *; function f(template integer t) runs on C { p.receive(t); }`: "whole-template-in-receive",
	}
	for src, code := range good {
		diags := NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, `module M { `+src+` }`))
		if containsCode(diags, code) {
			t.Errorf("%s: unexpected %s", src, code)
		}
	}
	once := map[string]string{
		`template charstring t := "a" & '01'B;`:     "concat-operand",
		`control { setverdict(pass); }`:             "control-part-forbidden-op",
		`function f() { var integer i := "abc"; }`:  "type-incompatible-value",
		`function f() { var integer i; i := 0.0; }`: "type-incompatible-assignment",
		`type record A { integer a optional } type record B { integer a } function f() { var B b := { 1 }; var A a := b; }`: "type-incompatible-value",
	}
	for src, code := range once {
		diags := NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, `module M { `+src+` }`))
		if n := countCode(diags, code); n != 1 {
			t.Errorf("%s: %d × %s, want 1: %v", src, n, code, codes(diags))
		}
		for _, d := range diags {
			if d.Severity == SeverityError && d.Code != code {
				t.Errorf("%s: also %s", src, d.Code)
			}
		}
	}
}

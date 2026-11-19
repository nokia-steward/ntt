package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestMiscStaticRules(t *testing.T) {
	bad := map[string]string{
		`function f() { var charstring v := int2char(65, 66); }`:                    "predefined-arity",
		`function f() { var charstring v := testcasename(1); }`:                     "predefined-arity",
		`type union U { integer a optional, boolean b }`:                            "union-optional-alternative",
		`type union U { U a, U b }`:                                                 "union-self-alternative",
		`type integer A[!1 .. 5];`:                                                  "array-dimension-bound",
		`type integer A[1 .. infinity];`:                                            "array-dimension-bound",
		`type default MyDefault (null);`:                                            "subtype-not-allowed",
		`function f() { var float v := -not_a_number; }`:                            "negated-not-a-number",
		`function f() { timer T := infinity; }`:                                     "timer-infinite-duration",
		`template integer t := (infinity .. 0);`:                                    "range-starts-with-infinity",
		`function f(integer x) { select (x) { case else {} case (1) {} } }`:         "select-case-after-else",
		`type component C {} testcase tc() runs on C { all component.start(f()); }`: "all-component-start",
		`type component C {} testcase tc() runs on C { any component.kill; }`:       "any-component-stop-kill",
		`type component C {} testcase tc() runs on C { execute(tc()); }`:            "execute-in-test-behaviour",
		`control { setverdict(pass); }`:                                             "control-part-verdict",
		`control { all port.clear; }`:                                               "control-part-port-op",
		`type port P message { inout integer } type component C { port P p; timer T } testcase tc() runs on C { T.timeout -> @index value v; }`: "index-redirect-without-any-from",
	}
	for _, tmpl := range []string{"*", "omit", "5 ifpresent"} {
		bad[`type port P message { inout integer } type component C { port P p } template integer m := `+tmpl+`; testcase tc() runs on C { p.receive(m); }`] = "whole-template-in-receive"
	}
	for src, code := range bad {
		diags := NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, `module M { `+src+` }`))
		if !containsCode(diags, code) {
			t.Errorf("%s: expected %s, got %v", src, code, codes(diags))
		}
	}
	good := `module M {
		type union MyUnion { MyUnion choice1, integer choice2 }
		type integer A[1 .. 5];
		type component C {}
		altstep a() { [] any timer.timeout { execute(tc()); } }
		testcase tc() runs on C {}
		function f(integer x) { var float v := -infinity; select (x) { case (1) {} case else {} } }
		control { execute(tc()); }
	}`
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, good))
	for _, code := range []string{"union-self-alternative", "array-dimension-bound", "execute-in-test-behaviour", "select-case-after-else", "predefined-arity"} {
		if containsCode(diags, code) {
			t.Errorf("valid module: unexpected %s", code)
		}
	}
}

package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// Configuration operations stay out of control behaviour and component
// type declarations, with what they invoke (21.0); setverdict stays out of
// control behaviour (16.1.5).
func TestCallContextRules(t *testing.T) {
	cases := []struct {
		name, defs, want string
	}{
		{"create in a function invoked from control", `
			function f() { var C v := C.create; }
			control { f(); }`, "configuration-operation-context"},
		{"create two calls away from control", `
			function g() { var C v := C.create; }
			function f() { g(); }
			control { f(); }`, "configuration-operation-context"},
		{"create in a component type declaration", `
			type component D { var C v := C.create; }`, "configuration-operation-context"},
		{"create in a function a component type declaration invokes", `
			function f() return C { return C.create; }
			type component D { var C v := f(); }`, "configuration-operation-context"},
		{"setverdict in a @control function", `
			function @control f() { setverdict(pass); }`, "setverdict-in-control"},
		{"setverdict in a function invoked from control", `
			function f() { setverdict(pass); }
			control { f(); }`, "setverdict-in-control"},

		{"create in a test case executed from control", `
			testcase TC() runs on C { var C v := C.create; }
			control { execute(TC()); }`, ""},
		{"create in a function a test case invokes", `
			function f() runs on C { var C v := C.create; }
			testcase TC() runs on C { f(); }
			control { execute(TC()); }`, ""},
		{"create in a function a PTC starts", `
			function g() runs on C { var C v := C.create; }
			testcase TC() runs on C { var C p := C.create; p.start(g()); }`, ""},
		{"function without operations from control", `
			function f() return integer { return 1; }
			control { var integer v := f(); }`, ""},
	}
	for _, c := range cases {
		tree := parse(t, `module M {
			type component C {}
			`+c.defs+`
		}`)
		got := codes(NewAnalyzer(&ttcn3.DB{}).Analyze(tree))
		ruleCodes := []string{"configuration-operation-context", "setverdict-in-control"}
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

// An external function, without a body, is not followed.
func TestCallContextRulesExternalFunction(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		external function f_ext();
		function f() { f_ext(); }
		control { f(); }
	}`)
	NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
}

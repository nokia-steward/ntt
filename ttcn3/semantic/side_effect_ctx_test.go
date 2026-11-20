package semantic

import "testing"

// TestSideEffectInRestrictedContext: 16.1.4 restricts what functions
// called from specific places may do; what is such a place, and what
// is inherited through a chain of calls, decides valid code.
func TestSideEffectInRestrictedContext(t *testing.T) {
	const code = "restricted-context-side-effect"
	for _, tt := range []struct {
		name, src string
		reject    bool
	}{
		{"fuzzy template not used in a receive (15.0 g)", `module M {
			type component C { var integer vc := 0; }
			function f() runs on C return integer { vc := vc + 1; return vc; }
			testcase tc() runs on C { template @fuzzy integer t := f(); log(match(1, t)); }
		}`, false},
		{"template not used in a receive", `module M {
			type record R { integer num }
			type component C { var integer vc := 0; }
			function f() runs on C return integer { vc := vc + 1; return vc; }
			testcase tc() runs on C { template R b := { num := f() }; template @fuzzy R m modifies b := {}; log(valueof(m)); }
		}`, false},
		{"fuzzy template used in a receive", `module M {
			type port P message { inout integer }
			type component C { port P p; var integer vc := 0; }
			function f() runs on C return template integer { vc := 1; return ?; }
			testcase tc() runs on C { template @fuzzy integer t := f(); p.receive(t); }
		}`, true},
		{"base of a template used in a receive", `module M {
			type record R { integer num }
			type port P message { inout R }
			type component C { port P p; var integer vc := 0; }
			function f() runs on C return integer { vc := 1; return vc; }
			template R b := { num := f() };
			template R m modifies b := {};
			testcase tc() runs on C { p.receive(m); }
		}`, true},
		{"deterministic fuzzy template (15.0 g)", `module M {
			type component C { var integer vc := 0; }
			function f() runs on C return integer { vc := vc + 1; return vc; }
			testcase tc() runs on C { template @fuzzy @deterministic integer t := f(); log(t); }
		}`, true},
		{"fuzzy template calling an out-parameter function (15.0 e)", `module M {
			function f(out integer x) return integer { x := 1; return x; }
			testcase tc() { var integer v; template @fuzzy integer t := f(v); log(t); }
		}`, true},
		{"fuzzy template calling it indirectly (15.0 e)", `module M {
			function f(out integer x) { x := 7; }
			function g() return integer { var integer v; f(v); return v; }
			template @fuzzy integer t := g();
		}`, false},
		{"guard calling an out-parameter function (16.1.4 j)", `module M {
			type component C { timer T := 0.1; }
			function f(out integer x) return boolean { x := 1; return true; }
			testcase tc() runs on C { var integer v; T.start; alt { [f(v)] T.timeout {} } }
		}`, true},
		{"guard calling it indirectly (16.1.4 note 7)", `module M {
			type component C { timer T := 0.1; }
			function f(out integer x) { x := 1; }
			function g() return boolean { var integer v; f(v); return v == 1; }
			testcase tc() runs on C { T.start; alt { [g()] T.timeout {} } }
		}`, false},
		{"guard writing a component variable through an out parameter (16.1.4 g)", `module M {
			type component C { timer T := 0.1; var integer vc; }
			function f(out integer x) { x := 1; }
			function g() runs on C return boolean { f(vc); return true; }
			testcase tc() runs on C { T.start; alt { [g()] T.timeout {} } }
		}`, true},
		{"guard calling testcase.stop (clause 18)", `module M {
			type component C { timer T := 0.1; }
			function g() runs on C return boolean { testcase.stop("x"); return true; }
			testcase tc() runs on C { T.start; alt { [g()] T.timeout {} } }
		}`, false},
		{"guard calling stop", `module M {
			type component C { timer T := 0.1; }
			function g() runs on C return boolean { stop; return true; }
			testcase tc() runs on C { T.start; alt { [g()] T.timeout {} } }
		}`, true},
		{"altstep local definition (16.2 b)", `module M {
			type component C { timer T := 0.1; }
			altstep a() runs on C { var boolean v := T.running; [] T.timeout {} }
		}`, true},
		{"declaration in an altstep branch (16.2.0)", `module M {
			type port P message { inout integer }
			type component C { port P p; }
			altstep d() runs on C { [] p.receive {} }
			altstep a() runs on C { [] p.receive { var default v := activate(d()); } }
		}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := hasCode(analyzeCodes(t, tt.src), code)
			if got != tt.reject {
				t.Errorf("%s reported: %v, want %v", code, got, tt.reject)
			}
		})
	}
}

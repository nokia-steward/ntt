package interpreter_test

import "testing"

// lengthof a template: what a length attribute leaves of the lengths its
// body matches; specific values and patterns count their items (ETSI
// C.2.1).
func TestLengthofTemplate_Lengths(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type record of integer RoI;
		testcase tc() runs on C {
			var template RoI v_l := { 1, 2, * } length(3);
			if (lengthof(charstring:"HELLO") == 5 and lengthof('1??1'B) == 4 and
			    lengthof('1*F'H length(8)) == 8 and lengthof('00*FF'O length(1..2)) == 2 and
			    lengthof(? length(4)) == 4 and lengthof(v_l) == 3 and
			    lengthof(("ab", "cd")) == 2) { setverdict(pass); }
			else { setverdict(fail); }
		}
	}`)
}

func lengthofError(t *testing.T, arg string) {
	t.Helper()
	runErrorVerdict(t, "M.tc", `module M {
		type component C {}
		type record of integer RoI;
		function f(template (present) RoI p) return integer { return lengthof(p); }
		function g(template (present) charstring p) return integer { return lengthof(p); }
		testcase tc() runs on C {
			var integer n := 1;
			setverdict(pass);
			var integer v := `+arg+`;
		}
	}`)
}

// A template matching values of different lengths, a length attribute
// contradicting the body, and a value range have no length (C.2.1).
func TestLengthofTemplate_Errors(t *testing.T) {
	for _, arg := range []string{
		`g(? length(2 .. infinity))`,
		`g(("a", "bc"))`,
		`f({ 1, * })`,
		`f({ 1, 2, 3, * } length(1 .. n + 1))`,
		`g("abc" length(n))`,
		`f({ 1, (2 .. 3) })`,
	} {
		t.Run(arg, func(t *testing.T) { lengthofError(t, arg) })
	}
}

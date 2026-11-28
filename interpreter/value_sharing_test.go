package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// TestSharedValuesStayValues: values are shared where nothing can tell —
// charstrings, which are never changed in place, and the `in` parameters
// of a function that changes nothing but its own — and copied where
// something can: an element assignment to a charstring makes a new one,
// and a function writing the component variable it was passed sees its
// own parameter unchanged (ETSI 5.4.1.1, 6).
func TestSharedValuesStayValues(t *testing.T) {
	src := `module M {
		type record R { integer x }
		type component C { var R cv := { x := 1 }; var charstring cs := "abc" }
		function writesComponent(R p) runs on C return integer { cv.x := 5; return p.x }
		function readsOnly(R p) runs on C return integer { var integer y := p.x; return y }
		function charAt(charstring s, integer i) runs on C return charstring { return s[i] }
		testcase tc() runs on C {
			var charstring s := "abc";
			var charstring s2 := s;
			s2[0] := "z";
			if (s != "abc" or s2 != "zbc") { setverdict(fail, "element assignment showed through: ", s, " ", s2); stop }
			var charstring grow := "";
			for (var integer i := 0; i < 5; i := i + 1) { grow := grow & "ab" }
			var charstring fork := grow & "X";
			var charstring fork2 := grow & "Y";
			if (fork != "ababababab" & "X" or fork2 != "ababababab" & "Y") { setverdict(fail, "appends shared a tail: ", fork, " ", fork2); stop }
			if (writesComponent(cv) != 1 or cv.x != 5) { setverdict(fail, "an in parameter changed with the component variable"); stop }
			if (readsOnly(cv) != 5) { setverdict(fail, "readsOnly"); stop }
			cs[1] := "Z";
			if (charAt(cs, 1) != "Z") { setverdict(fail, "component charstring element"); stop }
			setverdict(pass);
		}
	}`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}

// TestInParametersStayTheirOwn: an `in` parameter keeps its value though
// the component variable it was passed changes during the call — through
// a function the callee calls by a qualified name, a @lazy argument it
// reads, or its catch clause — and an element assignment through a map
// reaches the map.
func TestInParametersStayTheirOwn(t *testing.T) {
	a := parse(t, `module A {
		type record R { integer x }
		type component C { var R cv := { x := 1 } }
		function setcv() runs on C { cv.x := 5 }
		function setcv2() runs on C return integer { cv.x := 5; return 0 }
	}`)
	m := parse(t, `module M {
		import from A all;
		type map from charstring to charstring Ms;
		function qual(R p) runs on C return integer { A.setcv(); return p.x }
		function lazy(R p, @lazy integer q) runs on C return integer { var integer z := q; return p.x }
		function caught(R p) runs on C return integer exception(integer) {
			raise 1;
			return 0;
		} catch (integer e) { cv.x := 5; return p.x }
		testcase tc() runs on C {
			if (qual(cv) != 1) { setverdict(fail, "qualified call"); stop }
			cv.x := 1;
			if (lazy(cv, setcv2()) != 1) { setverdict(fail, "@lazy argument"); stop }
			cv.x := 1;
			if (caught(cv) != 1) { setverdict(fail, "catch clause"); stop }
			var Ms ms;
			ms["k"] := "abc";
			ms["k"][0] := "Z";
			if (ms["k"] != "Zbc") { setverdict(fail, "map element ", ms["k"]); stop }
			setverdict(pass);
		}
	}`)
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{m, a}, "M.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}

package interpreter_test

import "testing"

// A @lazy variable is evaluated at its first use, once; a @fuzzy one at
// each use; each assignment holds the expression with the scope of the
// assignment (ETSI 11.0).
func TestLazyAndFuzzyVariables(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C { var integer vc_count := 0; }
		function f_next() runs on C return integer { vc_count := vc_count + 1; return vc_count; }
		function f_lazy(@lazy integer p) runs on C return boolean {
			var integer a := p, b := p;
			return a == b;
		}
		testcase tc() runs on C {
			var @lazy integer v_lazy := f_next();
			if (vc_count != 0) { setverdict(fail, "lazy evaluated when declared"); stop; }
			var integer x := v_lazy, y := v_lazy;
			if (x != 1 or y != 1 or vc_count != 1) { setverdict(fail, "lazy ", x, y, vc_count); stop; }
			var @fuzzy integer v_fuzzy := f_next();
			x := v_fuzzy; y := v_fuzzy;
			if (x != 2 or y != 3) { setverdict(fail, "fuzzy ", x, y); stop; }
			if (not f_lazy(v_fuzzy)) { setverdict(fail, "fuzzy into a lazy parameter"); stop; }
			var @fuzzy integer v_scope := 1;
			if (true) {
				var integer v_local := 0;
				v_scope := v_local;
				v_local := 10;
			}
			if (v_scope != 10) { setverdict(fail, "scope ", v_scope); stop; }
			var @lazy integer v_self := 1;
			v_self := v_self + 1;
			if (v_self != 2) { setverdict(fail, "self ", v_self); stop; }
			setverdict(pass);
		}
	}`)
}

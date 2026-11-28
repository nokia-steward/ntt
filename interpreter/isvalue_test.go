package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/runtime"
)

// isvalue is true for a specific value only (7.1.8.3): a value list, a
// complement, an ifpresent, a bitstring with a wildcard or a template
// with a matching mechanism in an element or a field is none, ("ABCD")
// and null are.
func TestIsvalue(t *testing.T) {
	v, reason := runSched(t, "M.tc", `module M {
		type component C {}
		type record R { integer a, integer b optional }
		type record of integer RI;
		testcase tc() runs on C {
			template charstring m_char0 := "ABCD";
			template charstring m_char1 := "AB?D";
			template charstring mw_char2 := pattern "ABCD";
			template charstring m_char4 := ("ABCD");
			template charstring mw_char5 := ("ABCD", "EFGH");
			template integer mw_c := complement(1, 2);
			template RI mw_ri := {1, ?};
			template RI m_ri := {1, 2};
			template R mw_r := {a := (1, 2), b := omit};
			template R m_r := {a := 1, b := omit};
			template integer mw_ip := 1 ifpresent;
			template bitstring mw_b := '1?'B;
			var default v_default := null;
			if (not isvalue(m_char0)) { setverdict(fail, "m_char0") }
			if (not isvalue(m_char1)) { setverdict(fail, "m_char1") }
			if (isvalue(mw_char2)) { setverdict(fail, "mw_char2") }
			if (not isvalue(m_char4)) { setverdict(fail, "m_char4") }
			if (isvalue(mw_char5)) { setverdict(fail, "mw_char5") }
			if (isvalue(mw_c)) { setverdict(fail, "mw_c") }
			if (isvalue(mw_ri)) { setverdict(fail, "mw_ri") }
			if (not isvalue(m_ri)) { setverdict(fail, "m_ri") }
			if (isvalue(mw_r)) { setverdict(fail, "mw_r") }
			if (not isvalue(m_r)) { setverdict(fail, "m_r") }
			if (isvalue(mw_ip)) { setverdict(fail, "mw_ip") }
			if (isvalue(mw_b)) { setverdict(fail, "mw_b") }
			if (not isvalue(v_default)) { setverdict(fail, "v_default") }
			setverdict(pass);
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

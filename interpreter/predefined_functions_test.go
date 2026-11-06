package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/runtime"
)

// int2enum assigns its out parameter a new value (ETSI C.1.4): one shared
// with another variable or a constant is left as it was, and an unbound
// variable takes the value of the type it is declared of.
func TestInt2EnumAssignsANewValue(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type enumerated E { A, B, C }
		const E c_e := A;
		testcase tc() runs on C {
			var E a := A;
			var E b := a;
			int2enum(2, b);
			var E k := c_e;
			int2enum(1, k);
			var E u;
			int2enum(1, u);
			if (a == A and b == C and c_e == A and k == B and u == B) { setverdict(pass); }
			else { setverdict(fail, a, b, c_e, k, u); }
		}
	}`)
}

// str2oct of an odd number of digits puts a 0 in front (ETSI C.1.28); the
// empty binary strings convert to the empty charstring (C.1.16, C.1.20,
// C.1.25); hostid is a loopback address of the kind asked for (C.6.3).
func TestConversionsAndHostid(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		testcase tc() runs on C {
			if (str2oct("1D7") == '01D7'O and str2oct("") == ''O and str2oct("0a") == '0A'O and
			    bit2str(''B) == "" and hex2str(''H) == "" and oct2str(''O) == "" and
			    hostid() == "127.0.0.1" and hostid("Ipv4") == "127.0.0.1" and hostid("Ipv6") == "::1" and
			    hostid("bogus") == "") { setverdict(pass); }
			else { setverdict(fail, str2oct("1D7"), bit2str(''B), hostid("Ipv6")); }
		}
	}`)
}

// int2enum converts into the type of its out parameter (C.1.4), one with
// no value yet too: a variable, a field (of a record type's synonym as
// well, or an array field's element), a record of element and an array
// element, the enumerated type named module-qualified or through a
// synonym.
func TestInt2enumIntoAnUnboundTarget(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type enumerated E { e0, e1, e2(5) }
		type E ES;
		type record R { E f, integer i, M.E q, ES s, E arr[2] }
		type record of E RE;
		type R RS;
		testcase tc() runs on C {
			var E v; var R r; var RE l; var E a[2]; var RS rs; var ES vs;
			int2enum(1, v);
			int2enum(5, r.f);
			int2enum(0, l[0]);
			int2enum(5, a[1]);
			int2enum(1, rs.f);
			int2enum(5, r.q);
			int2enum(1, r.s);
			int2enum(5, r.arr[1]);
			int2enum(0, vs);
			if (v == e1 and r.f == e2 and l[0] == e0 and a[1] == e2 and rs.f == e1 and
			    r.q == e2 and r.s == e1 and r.arr[1] == e2 and vs == e0) { setverdict(pass) }
			else { setverdict(fail, v, r, l[0], a[1], rs.f, vs) }
		}
	}`)
}

// substr of a template (C.4.2) indexes it by 15.6.1: a pattern by its
// items, a set expression, a reference and a repetition with its
// expression being one; AnyValue gives count AnyElements of the
// template's type (of a variable, field, parameter, element, inline or
// invoked template), and AnyValueOrNone those with ifpresent; a length
// restriction goes, an ifpresent stays.
func TestSubstrOfTemplates(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type record R { octetstring f optional }
		type record RC { charstring f optional }
		type record of charstring RL;
		template charstring mw_any(integer i) := ?;
		function fp(template charstring p) return template charstring { return substr(p, 0, 2) }
		testcase tc() runs on C {
			var template charstring p := pattern "abc[a-z]+";
			var template charstring a := ?;
			var template octetstring o := 'AB?CD'O;
			var template octetstring ao := ?;
			var template R r := { f := * };
			var template charstring l := ? length(5);
			var template charstring ip := pattern "ab?" ifpresent;
			var charstring fails := "";
			if (not match("cxyz", substr(p, 2, 2))) { fails := fails & "pattern " }
			if (not match("xyz", substr(a, 0, 3)) or match("xy", substr(a, 0, 3))) { fails := fails & "any " }
			if (not match('11CD'O, substr(o, 1, 2))) { fails := fails & "octet " }
			if (not match('010203'O, substr(ao, 0, 3)) or match('0102'O, substr(ao, 0, 3))) { fails := fails & "any-octet " }
			var template R chk := { f := substr(r.f, 0, 2) };
			if (not match({ f := omit }, chk) or not match({ f := '0102'O }, chk)) { fails := fails & "anyornone-field " }
			if (not match("xy", substr(l, 0, 2))) { fails := fails & "length " }
			var template RC ipc := { f := substr(ip, 0, 2) };
			var RC omitted := { f := omit }, ab := { f := "ab" };
			if (not match(omitted, ipc) or not match(ab, ipc)) { fails := fails & "ifpresent " }
			const charstring m_Ref := "abc?def?";
			var template charstring rp := pattern "{m_Ref}\q{0,0,1,113}";
			if (not match(char(0, 0, 1, 113), substr(rp, 1, 1)) or not match("abcxdefy", substr(rp, 0, 1))) { fails := fails & "reference " }
			var template RL rl := { ?, ? };
			if (not match("ab", fp(?)) or not match("ab", substr(rl[0], 0, 2)) or not match("ab", substr(mw_any(1), 0, 2)) or
			    not match("ab", substr(charstring:?, 0, 2))) { fails := fails & "kinds " }
			if (substr(inpar := "abcdef", index := 1, count := 2) != "bc") { fails := fails & "named " }
			var template charstring w := pattern "[a-z]x";
			if (not match("q", substr(w & pattern "y", 0, 1))) { fails := fails & "concat " }
			var template charstring wr := pattern "{m_Ref}x";
			if (not match("abcxdefy", substr(wr & pattern "y", 0, 1))) { fails := fails & "concat-ref " }
			if (not match("abcxdefy", wr[0])) { fails := fails & "element-ref " }
			var template charstring arr[2] := { ?, ? };
			if (not match("ab", substr(arr[0], 0, 2)) or not match("ab", substr((a), 0, 2))) { fails := fails & "array-paren " }
			if (fails == "") { setverdict(pass) } else { setverdict(fail, fails) }
		}
	}`)
}

// substr's error causes (C.4.2, 15.6.1 g): a negative index, index +
// count beyond the length restriction's maximum or the pattern's items.
func TestSubstrOfTemplatesErrors(t *testing.T) {
	for _, call := range []string{
		"substr(a, -1, 2)",
		"substr(l, 3, 4)",
		"substr(p, 2, 3)",
	} {
		v, reason := runSched(t, "M.tc", `module M {
			type component C {}
			testcase tc() runs on C {
				var template charstring a := ?;
				var template charstring l := ? length(5);
				var template charstring p := pattern "a[bc]d";
				var template charstring r := `+call+`;
				setverdict(pass);
			}
		}`)
		if v != runtime.ErrorVerdict {
			t.Errorf("%s: verdict = %s (%s), want error", call, v, reason)
		}
	}
}

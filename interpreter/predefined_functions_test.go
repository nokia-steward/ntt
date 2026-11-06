package interpreter_test

import "testing"

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

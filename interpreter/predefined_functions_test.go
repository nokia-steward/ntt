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

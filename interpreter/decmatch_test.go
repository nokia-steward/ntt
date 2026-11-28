package interpreter_test

import "testing"

// decmatch matches a field that decodes, whole, into a value its target
// matches; one that does not decode does not match (ETSI B.1.2.9).
func TestDecmatch(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type integer Payload (0 .. 255) with { encode "RAW" }
		type record Msg { integer id, bitstring payload }
		template Msg mw_msg := { id := ?, payload := decmatch Payload:(10 .. 20) };
		testcase tc() runs on C {
			var Payload v_in := 15, v_out := 30;
			var Msg v_ok := { id := 1, payload := encvalue(v_in) };
			var Msg v_value := { id := 1, payload := encvalue(v_out) };
			var Msg v_bad := { id := 1, payload := ''B };
			if (match(v_ok, mw_msg) and not match(v_value, mw_msg) and not match(v_bad, mw_msg) and
			    istemplatekind(mw_msg.payload, "decmatch")) { setverdict(pass); }
			else { setverdict(fail); }
		}
	}`)
}

// The encoding format is one of the strings decvalue_unichar takes; any
// other is an error (B.1.2.9 c).
func TestDecmatchUnknownFormatIsAnError(t *testing.T) {
	runErrorVerdict(t, "M.tc", `module M {
		type component C {}
		type integer Payload with { encode "JSON" }
		testcase tc() runs on C {
			var charstring v_format := "UTF-7";
			setverdict(pass);
			var template universal charstring vt := decmatch (v_format) Payload:1;
		}
	}`)
}

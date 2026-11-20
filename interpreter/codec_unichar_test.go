package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// encvalue_unichar is oct2unichar(bit2oct(encvalue(inpar, info, dyn)), ser)
// (ETSI C.5.3).
func TestEncvalueUnichar_IsTheOctetsOfEncvalueReadAsCharacters(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type integer I with { variant "32 bit" };
		const I c_val := 1145258561; // 'ABCD' little-endian
		testcase tc() runs on C {
			var universal charstring v8 := encvalue_unichar(c_val);
			var universal charstring v16 := encvalue_unichar(c_val, "UTF-16BE", "info");
			if (v8 == "ABCD" and
			    v8 == oct2unichar(bit2oct(encvalue(c_val)), "UTF-8") and
			    v16 == oct2unichar(bit2oct(encvalue(c_val, "info")), "UTF-16BE") and
			    lengthof(v16) == 2) { setverdict(pass); }
			else { setverdict(fail, v8, v16); }
		}
	}`)
}

// decvalue_unichar decodes a prefix and leaves the characters of the
// undecoded octets in encoded_value (ETSI C.5.4).
func TestDecvalueUnichar_DecodesAPrefixAndKeepsTheRest(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type integer I with { variant "32 bit" };
		const I c_a := 1145258561;
		const I c_b := 1212630597;
		testcase tc() runs on C {
			var universal charstring v_b := encvalue_unichar(c_b, "UTF-16LE");
			var universal charstring v_enc := encvalue_unichar(c_a, "UTF-16LE") & v_b;
			var I v_dec;
			var integer v_res := decvalue_unichar(v_enc, v_dec, "UTF-16LE");
			if (v_res == 0 and v_dec == c_a and v_enc == v_b) { setverdict(pass); }
			else { setverdict(fail, v_res, v_dec, v_enc); }
		}
	}`)
}

// Too few characters for the declared width: 2, both arguments untouched.
func TestDecvalueUnichar_TooFewCharactersReturnsTwo(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type integer I with { variant "32 bit" };
		testcase tc() runs on C {
			var universal charstring v_enc := "AB";
			var I v_dec;
			if (decvalue_unichar(v_enc, v_dec) == 2 and v_enc == "AB" and not isbound(v_dec)) { setverdict(pass); }
			else { setverdict(fail); }
		}
	}`)
}

// The width of a parameter's type counts, as a variable's does.
func TestEncvalue_IntegerWidthThroughAParameter(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type integer I with { variant "16 bit" };
		function f(in I p) return bitstring { return encvalue(p); }
		testcase tc() runs on C {
			var I v := 1;
			if (f(v) == '0000000100000000'B and encvalue_o(v) == '0100'O) { setverdict(pass); }
			else { setverdict(fail, f(v)); }
		}
	}`)
}

// encvalue_o is encvalue's bits in octets (ETSI C.5.5).
func TestEncvalueO_IsEncvalueInOctets(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		testcase tc() runs on C {
			if (bit2oct(encvalue(10)) == encvalue_o(10) and encvalue_o(10) == '0A000000'O) { setverdict(pass); }
			else { setverdict(fail, encvalue(10)); }
		}
	}`)
}

// The not-used symbol takes the default serialization, UTF-8.
func TestEncvalueUnichar_NotUsedSerializationIsUTF8(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		testcase tc() runs on C {
			if (encvalue_unichar(65, -) == encvalue_unichar(65)) { setverdict(pass); }
			else { setverdict(fail); }
		}
	}`)
}

func runErrorVerdict(t *testing.T, name, src string) {
	t.Helper()
	tree := parse(t, src)
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, name)
	if err != nil {
		t.Fatalf("RunTestcase(%s): %v", name, err)
	}
	if v != runtime.ErrorVerdict {
		t.Fatalf("%s: verdict = %s (%s), want error", name, v, reason)
	}
}

// A serialization none of the seven UCS encoding schemes is an error
// (ETSI C.5.3, C.5.4).
func TestUnicharCodec_UnknownSerializationIsAnError(t *testing.T) {
	runErrorVerdict(t, "M.tc", `module M {
		type component C {}
		function f(charstring p) return universal charstring { return encvalue_unichar(65, p); }
		testcase tc() runs on C {
			setverdict(pass);
			var universal charstring v := f("UTF-7");
		}
	}`)
	runErrorVerdict(t, "M.tc", `module M {
		type component C {}
		testcase tc() runs on C {
			var universal charstring v_enc := "ABCD";
			var integer v_dec;
			var charstring v_ser := "ASCII";
			setverdict(pass);
			var integer v_res := decvalue_unichar(v_enc, v_dec, v_ser);
		}
	}`)
}

// An uninitialised value has no encoding (ETSI C.5, 16.1.2).
func TestEncvalue_UninitialisedValueIsAnError(t *testing.T) {
	runErrorVerdict(t, "M.tc", `module M {
		type component C {}
		testcase tc() runs on C {
			var integer v;
			setverdict(pass);
			var universal charstring v_enc := encvalue_unichar(v);
		}
	}`)
}

package interpreter_test

import "testing"

// `encvalue_o` on a bitstring whose length is not a multiple of 8
// left-aligns the bits and zero-fills the low bits of the last octet,
// behind a 32-bit little-endian bit count (ETSI Annex C.5.5,
// Sem_160102_predefined_functions_110).
func TestEncvalueO_BitstringIsLeftAlignedBehindItsBitCount(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type bitstring MyPDU;
		testcase tc() runs on C {
			var MyPDU v_three := '011'B;
			var octetstring v_enc := encvalue_o(v_three);
			if (v_enc == '0300000060'O) { setverdict(pass); }
			else { setverdict(fail, v_enc); }
		}
	}`)
}

// A whole octet's worth of bits needs no padding, so only the count
// leads it.
func TestEncvalueO_BitstringOfWholeOctetsIsNotPadded(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		testcase tc() runs on C {
			var octetstring v_enc := encvalue_o('1010101100000001'B);
			if (v_enc == '10000000AB01'O) { setverdict(pass); }
			else { setverdict(fail, v_enc); }
		}
	}`)
}

// A record encodes as its fields back to back: a charstring as its
// characters plus a terminating 0x00, an integer as a 32-bit
// little-endian word - the value Sem_160102_predefined_functions_107
// records for `{"testText", 5}`.
func TestEncvalueO_RecordEncodesItsFieldsInOrder(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type record MyPDU { charstring field1 optional, integer field2 }
		testcase tc() runs on C {
			var template MyPDU v_temp := {"testText", 5};
			var octetstring v_enc := encvalue_o(v_temp);
			if (v_enc == '74657374546578740005000000'O) { setverdict(pass); }
			else { setverdict(fail, v_enc); }
		}
	}`)
}

// `decvalue_o` reports 2 - "could not be completed, not enough octets" -
// when the encoded value is shorter than the output slot's declared field
// width, and leaves both slots alone (ETSI Annex C.5.6).
func TestDecvalueO_TooFewOctetsForTheDeclaredWidthReturnsTwo(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		testcase tc() runs on C {
			var integer v_dec with { variant "32 bit" };
			var integer v_res;
			var octetstring v_ref := '65'O;
			v_res := decvalue_o(v_ref, v_dec);
			if (v_res == 2 and v_ref == '65'O) { setverdict(pass); }
			else { setverdict(fail, v_res, v_ref); }
		}
	}`)
}

// With enough octets the same call decodes, so the shortfall check does
// not swallow the ordinary path.
func TestDecvalueO_EnoughOctetsStillDecodes(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		testcase tc() runs on C {
			var integer v_dec with { variant "32 bit" };
			var integer v_res;
			var octetstring v_ref := '0A000000'O;
			v_res := decvalue_o(v_ref, v_dec);
			if (v_res == 0 and v_dec == 10) { setverdict(pass); }
			else { setverdict(fail, v_res, v_dec); }
		}
	}`)
}

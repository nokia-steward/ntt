package interpreter_test

import "testing"

// A @decoded redirect whose encoding format is none of the strings
// decvalue_unichar takes, or whose field cannot be decoded, ends the
// operation with a test case error (ETSI 22.2.2).
func TestDecodedRedirectErrors(t *testing.T) {
	for name, redirect := range map[string]string{
		"format":   `v_inner := @decoded(v_format) payload`,
		"decoding": `v_inner := @decoded raw`,
	} {
		t.Run(name, func(t *testing.T) {
			runErrorVerdict(t, "M.tc", `module M {
				type record Msg { integer id, universal charstring payload, octetstring raw }
				type record Inner { integer x } with { encode "JSON" }
				type port P message { inout Msg }
				type component C { port P p; port P q; timer T := 1.0; }
				testcase tc() runs on C {
					var Inner v_inner;
					var charstring v_format := "UTF-7";
					connect(mtc:p, mtc:q);
					q.send(Msg:{ id := 1, payload := encvalue_unichar(Inner:{ x := 5 }), raw := 'FF'O });
					setverdict(pass);
					T.start;
					alt {
						[] p.receive(Msg:?) -> value (`+redirect+`) { }
						[] T.timeout { setverdict(fail); }
					}
				}
			}`)
		})
	}
}

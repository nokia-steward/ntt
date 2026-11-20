package interpreter_test

import "testing"

// No address of a to or from clause is null at the time of the
// operation (ETSI 22.2.1, 22.2.2, 22.4).
func TestNullAddressInToOrFromIsAnError(t *testing.T) {
	for name, stmt := range map[string]string{
		"send to":      `p.send(1) to v_null;`,
		"receive from": `alt { [] p.receive(integer:1) from v_null { } [] T.timeout { } }`,
		"check from":   `alt { [] p.check(receive(integer:1) from v_null) { } [] T.timeout { } }`,
	} {
		t.Run(name, func(t *testing.T) {
			runErrorVerdict(t, "M.tc", `module M {
				type port P message { inout integer }
				type component C { port P p; port P q; timer T := 1.0; }
				testcase tc() runs on C {
					var C v_null := null;
					connect(mtc:p, mtc:q);
					q.send(1);
					T.start;
					setverdict(pass);
					`+stmt+`
				}
			}`)
		})
	}
}

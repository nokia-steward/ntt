package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/runtime"
)

// A receiving operation matches every message it looks at with one
// template: `any from` the messages of each port of an array (ETSI ES 201
// 873-1 22.2.2, 22.5). A positional record template matches records by
// their field names.
func TestReceiveMatcherAcrossMessages(t *testing.T) {
	for name, body := range map[string]string{
		"any from": `var integer i := -1;
			connect(self:pa[0], self:pa[0]); connect(self:pa[1], self:pa[1]);
			pa[0].send(R:{a := 2, b := "x"}); pa[1].send(R:{a := 3, b := "y"});
			timer t := 1.0; t.start;
			alt {
				[] any from pa.receive(R:{3, ?}) -> @index value i { if (i == 1) { setverdict(pass) } else { setverdict(fail, "index") } }
				[] t.timeout { setverdict(fail, "no match") }
			}`,
	} {
		t.Run(name, func(t *testing.T) {
			assertSameVerdictBothClocks(t, `module M {
				type record R { integer a, charstring b }
				type port P message { inout R, integer }
				type component C { port P p; port P pa[2] }
				testcase tc() runs on C {
					`+body+`
				}
			}`, runtime.PassVerdict)
		})
	}
}

package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/runtime"
)

// A trigger removes the message at the head of its queue, matched or not
// (ETSI ES 201 873-1 22.2.3), and the alt takes a new snapshot with the
// next message at the head: it triggers on the first message that
// matches, however many come before it, as a statement too. One message
// is removed per snapshot, so another alternative a snapshot can take
// still comes first.
func TestTriggerTakesTheNextSnapshot(t *testing.T) {
	for name, c := range map[string]struct {
		body string
		want runtime.Verdict
	}{
		"after an integer and a record": {`p.send(1); p.send(R:{a := 2, b := "x"}); p.send(R:{a := 3, b := "y"});
			timer t := 1.0; t.start;
			alt {
				[] p.trigger(R:{3, "y"}) { setverdict(pass) }
				[] t.timeout { setverdict(fail, "no match") }
			}`, runtime.PassVerdict},
		"as a statement": {`p.send(R:{a := 2, b := "x"}); p.send(R:{a := 3, b := "y"});
			p.trigger(R:{3, "y"}); setverdict(pass);`, runtime.PassVerdict},
		"another alternative first": {`p.send(R:{a := 2, b := "x"}); p.send(R:{a := 3, b := "y"}); q.send(7);
			alt {
				[] p.trigger(R:{3, "y"}) { setverdict(fail, "trigger taken first") }
				[] q.receive(7) { if (p.checkstate("Started")) { setverdict(pass) } }
			}`, runtime.PassVerdict},
	} {
		t.Run(name, func(t *testing.T) {
			assertSameVerdictBothClocks(t, `module M {
				type record R { integer a, charstring b }
				type port P message { inout R, integer }
				type component C { port P p, q }
				testcase tc() runs on C {
					connect(self:p, self:p); connect(self:q, self:q);
					`+c.body+`
				}
			}`, c.want)
		})
	}
}

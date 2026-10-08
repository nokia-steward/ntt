package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// TestComponentAndPortOperations: `running` of a component never started
// and of a component given a behaviour by call, `clear` of one port and of
// all ports, and the bare procedure guards on `any port`.
func TestComponentAndPortOperations(t *testing.T) {
	for name, src := range componentAndPortOps {
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
			if err != nil || v != runtime.PassVerdict {
				t.Errorf("%s, %s clock: %s (%s) %v", name, k.name, v, reason, err)
			}
		}
	}
}

var componentAndPortOps = map[string]string{
	"running": `module M {
		type component C {}
		type component D {}
		function callee() runs on D return integer {
			if (self.running) { setverdict(pass) } else { setverdict(fail, "self.running false under call") }
			return 1;
		}
		testcase tc() runs on C {
			var D d := D.create;
			if (d.running) { setverdict(fail, "never started, yet running") }
			var integer r;
			d.call(callee()) -> value r;
		}
	}`,
	"clear": `module M {
		type port P message { inout integer }
		type component C { port P p }
		testcase tc() runs on C {
			connect(self:p, self:p);
			p.send(1); p.clear; p.send(2);
			alt { [] p.receive(2) { setverdict(pass) } [] p.receive { setverdict(fail, "p.clear") } }
			p.send(3); all port.clear; p.send(4);
			alt { [] p.receive(4) {} [] p.receive { setverdict(fail, "all port.clear") } }
		}
	}`,
	"any port procedure guards": `module M {
		signature S() exception (integer);
		type port PP procedure { inout S }
		type component C { port PP p1, p2 }
		function server() runs on C {
			timer g := 5.0; g.start;
			alt { [] any port.getcall { setverdict(pass, "any port.getcall") } [] g.timeout { setverdict(fail, "any port.getcall") } }
			p2.reply(S:{});
			p2.getcall(S:{});
			p2.raise(S, 7);
		}
		testcase tc() runs on C system C {
			var C s := C.create;
			connect(self:p1, s:p1);
			connect(self:p2, s:p2);
			s.start(server());
			timer g1 := 5.0, g2 := 5.0, g3 := 5.0;
			g1.start;
			p2.call(S:{}, nowait);
			alt { [] any port.getreply { setverdict(pass, "any port.getreply") } [] g1.timeout { setverdict(fail, "any port.getreply") } }
			g2.start;
			p2.call(S:{}, nowait);
			alt { [] any port.catch { setverdict(pass, "any port.catch") } [] g2.timeout { setverdict(fail, "any port.catch") } }
			g3.start;
			alt { [] s.done {} [] g3.timeout { setverdict(fail, "server did not finish") } }
		}
	}`,
}

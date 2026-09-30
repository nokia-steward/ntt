package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// TestComponentAndPortOperations: `running` of a component never started
// and of a component given a behaviour by call, and `clear` of one port
// and of all ports.
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
}

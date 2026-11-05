package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// checkstate answers each of its six states from the port's operational
// state and connections (ETSI 22.5.5): Linked is connected or mapped,
// and any port and all port ask the component's ports, inherited ones
// included, which all port.stop/start/halt change together (22.6).
func TestCheckstate(t *testing.T) {
	v, reason := runSched(t, "M.tc", `module M {
		type port P message { inout integer }
		type component Base { port P p3 }
		type component C extends Base { port P p1, p2 }
		type component S { port P sp }
		function want(boolean b, charstring what) {
			if (not b) { setverdict(fail, what); stop }
		}
		testcase tc() runs on C system S {
			want(p1.checkstate("Started"), "a new port is started");
			want(not p1.checkstate("Stopped") and not p1.checkstate("Halted"), "nor stopped or halted");
			want(not p1.checkstate("Connected") and not p1.checkstate("Mapped") and not p1.checkstate("Linked"), "nor linked");
			want(all port.checkstate("Started"), "all ports start started");

			connect(self:p1, self:p2);
			want(p1.checkstate("Connected") and p1.checkstate("Linked"), "connect links");
			want(not p1.checkstate("Mapped"), "connect doesn't map");
			want(any port.checkstate("Connected") and not all port.checkstate("Connected"), "p3 isn't connected");
			disconnect(self:p1, self:p2);
			want(not p1.checkstate("Linked"), "disconnect unlinks");

			map(self:p3, system:sp);
			want(p3.checkstate("Mapped") and p3.checkstate("Linked"), "map links the inherited port");
			unmap(self:p3, system:sp);
			want(not p3.checkstate("Linked"), "unmap unlinks");

			p2.stop;
			want(p2.checkstate("Stopped") and not p2.checkstate("Started"), "p2.stop");
			want(any port.checkstate("Stopped") and not all port.checkstate("Stopped"), "only p2 stopped");
			all port.halt;
			want(all port.checkstate("Halted"), "all port.halt halts each port");
			all port.start;
			want(all port.checkstate("Started") and not any port.checkstate("Halted"), "all port.start");
			want(p1.checkstate("Start" & "ed"), "the state is any charstring expression");
			setverdict(pass);
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// Any other state, a differently written one included, is an error
// (ETSI 22.5.5 b).
func TestCheckstateInvalidState(t *testing.T) {
	for _, state := range []string{`"started"`, `"Running"`, `"Unconnected"`} {
		v, reason := runSched(t, "M.tc", `module M {
			type port P message { inout integer }
			type component C { port P p1 }
			function f(charstring s) runs on C return boolean { return p1.checkstate(s) }
			testcase tc() runs on C {
				if (f(`+state+`)) { setverdict(pass) } else { setverdict(pass) }
			}
		}`)
		if v != runtime.ErrorVerdict {
			t.Errorf("checkstate(%s): verdict = %s (%s), want error", state, v, reason)
		}
	}
}

// any port and all port reach each element of a port array, named as
// p[i].op names it.
func TestCheckstatePortArrays(t *testing.T) {
	v, reason := runSched(t, "M.tc", `module M {
		type port P message { inout integer }
		type component C { port P pa[2]; port P pr[1..2] }
		function want(boolean b, charstring what) {
			if (not b) { setverdict(fail, what); stop }
		}
		testcase tc() runs on C {
			connect(self:pa[0], self:pr[2]);
			want(any port.checkstate("Connected"), "any port sees pa[0]");
			want(not all port.checkstate("Connected"), "pa[1] and pr[1] aren't connected");
			pr[1].halt;
			want(any port.checkstate("Halted"), "any port sees pr[1] of a ranged array");
			all port.stop;
			want(pa[0].checkstate("Stopped") and pr[2].checkstate("Stopped"), "all port.stop reaches the elements");
			want(all port.checkstate("Stopped"), "all elements stopped");
			setverdict(pass);
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// A component created of another module's type has that type's ports,
// not those of a like-named type of the module creating it.
func TestCheckstateQualifiedComponentType(t *testing.T) {
	b := parse(t, `module B {
		type port P message { inout integer }
		type component Base { port P bp }
		type component C extends Base { port P b1 }
		function f() runs on C {
			b1.stop;
			if (any port.checkstate("Stopped") and not all port.checkstate("Stopped") and any port.checkstate("Started")) {
				setverdict(pass)
			} else {
				setverdict(fail, "the ports of B.C weren't asked")
			}
		}
	}`)
	a := parse(t, `module A {
		import from B all;
		type component C { var integer v_a := 1 }
		type component M {}
		testcase tc() runs on M {
			var B.C v := B.C.create;
			v.start(f());
			v.done;
		}
	}`)
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{a, b}, "A.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}

// A component of a component type synonym (ETSI 6.4) has the ports of the
// type the synonym names, for the MTC and for a PTC.
func TestCheckstateComponentTypeSynonym(t *testing.T) {
	v, reason := runSched(t, "M.tc", `module M {
		type port P message { inout integer }
		type component C { port P p1, p2 }
		type C CS;
		function want(boolean b, charstring what) runs on CS {
			if (not b) { setverdict(fail, what); stop }
		}
		function ptc() runs on CS {
			p1.stop;
			want(any port.checkstate("Stopped") and not all port.checkstate("Stopped"), "the PTC's ports");
		}
		testcase tc() runs on CS {
			connect(self:p1, self:p2);
			p1.send(1);
			all port.clear;
			timer t := 0.1;
			t.start;
			alt {
				[] p2.receive { want(false, "all port.clear left the message") }
				[] t.timeout { }
			}
			all port.stop;
			want(p1.checkstate("Stopped") and all port.checkstate("Stopped"), "all port.stop on the MTC");
			var CS v := CS.create;
			v.start(ptc());
			v.done;
			setverdict(pass);
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// A port array too large to enumerate is asked as one name, not built.
func TestCheckstateHugePortArray(t *testing.T) {
	v, reason := runSched(t, "M.tc", `module M {
		type port P message { inout integer }
		type component C { port P pa[0..9223372036854775807] }
		testcase tc() runs on C {
			if (any port.checkstate("Started")) { setverdict(pass) } else { setverdict(fail) }
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

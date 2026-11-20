package interpreter_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

const connectionTypes = `
	type port P message { inout integer }
	signature S(in integer x);
	type port Q procedure { inout S }
	type component C { port P p, p1, p2, p3; port Q q }
	type component Sys { port P sp, sp2 }
	function f_connect(C c) runs on C { connect(self:p, c:p) }
	function f_idle() runs on C {}
`

// The connections clause 9.1 a and 21.1.1 b, c forbid are test case errors
// when made (9.1 b, 21.1.1 e), and so is a sending operation without a to
// clause on a port connected to several others (22.2.1 d, 22.3.1 e,
// 22.3.3 d, 22.3.5 c).
func TestConnectionRules(t *testing.T) {
	for name, body := range map[string]string{
		"two ports of one component":          `connect(mtc:p1, mtc:p2); connect(mtc:p1, mtc:p3);`,
		"two ports of another component":      `var C c := C.create; connect(mtc:p, c:p1); connect(mtc:p, c:p2);`,
		"a system port to two of a component": `map(mtc:p1, system:sp); map(mtc:p2, system:sp);`,
		"a port to two system ports":          `map(mtc:p, system:sp); map(mtc:p, system:sp2);`,
		"a connected port mapped":             `connect(mtc:p1, mtc:p2); map(mtc:p1, system:sp);`,
		"a mapped port connected":             `map(mtc:p1, system:sp); connect(mtc:p1, mtc:p2);`,
		"a system port connected":             `connect(mtc:p, system:sp);`,
		"the system passed and connected":     `f_connect(system);`,
		"no system port mapped":               `map(mtc:p1, mtc:p2);`,
		"a component not created":             `var C c := null; connect(mtc:p, c:p);`,
		"a reference known only when run":     `var C c := C.create; var C d := c; connect(mtc:p, c:p1); connect(mtc:p, d:p2);`,
		"a port of a killed component":        `var C c := C.create; c.kill; connect(mtc:p, c:p);`,
		"send without to, one to many":        `var C a := C.create, b := C.create; connect(mtc:p, a:p); connect(mtc:p, b:p); p.send(1);`,
		"call without to, one to many":        `var C a := C.create, b := C.create; connect(mtc:q, a:q); connect(mtc:q, b:q); q.call(S:{1}, nowait);`,
	} {
		src := "module M {" + connectionTypes + "testcase tc() runs on C system Sys {\nsetverdict(pass);\n" + body + "\nsetverdict(pass);\n}\n}"
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
			if err != nil || v != runtime.ErrorVerdict || !strings.Contains(reason, "ETSI") {
				t.Errorf("%s, %s clock: %s (%s) %v; want an error", name, k.name, v, reason, err)
			}
		}
	}
}

// What clause 9.1 allows stays allowed: a port connected to itself, a
// connection made twice (F.6), a system port mapped to ports of several
// components, a one-to-many send with a to clause, and a send on a port
// of the MTC of a testcase without a system clause, whose ports are the
// test system interface (16.3).
func TestConnectionRulesAllowed(t *testing.T) {
	for name, c := range map[string]struct{ system, body string }{
		"a port connected to itself":   {"", `connect(self:p, self:p); p.send(1); p.receive(integer:1);`},
		"a connection made twice":      {"", `connect(mtc:p1, mtc:p2); connect(mtc:p1, mtc:p2); connect(mtc:p2, mtc:p1);`},
		"a mapping made twice":         {"system Sys", `map(mtc:p1, system:sp); map(system:sp, mtc:p1);`},
		"a system port to components":  {"system Sys", `var C c := C.create; map(mtc:p1, system:sp); map(c:p1, system:sp);`},
		"a one-to-many send with to":   {"", `var C a := C.create, b := C.create; connect(mtc:p, a:p); connect(mtc:p, b:p); p.send(1) to a;`},
		"reconnected after disconnect": {"", `connect(mtc:p1, mtc:p2); disconnect(mtc:p1, mtc:p2); connect(mtc:p1, mtc:p3);`},
		"after a component ended": {"", `var C a := C.create; connect(mtc:p, a:p); a.start(f_idle()); a.done;
			var C b := C.create; connect(mtc:p, b:p); p.send(1);`},
		"after a component was killed": {"", `var C a := C.create alive; connect(mtc:p, a:p); a.kill;
			var C b := C.create; connect(mtc:p, b:p); p.send(1);`},
		"a map after a component ended": {"system Sys", `var C a := C.create; connect(mtc:p, a:p); a.start(f_idle()); a.done;
			map(mtc:p, system:sp);`},
		"the test system interface": {"", `p.send(1); p.receive(integer:1);`},
	} {
		t.Run(name, func(t *testing.T) {
			assertSameVerdictBothClocks(t, "module M {"+connectionTypes+"testcase tc() runs on C "+c.system+" {\nsetverdict(pass);\n"+c.body+"\n}\n}", runtime.PassVerdict)
		})
	}
}

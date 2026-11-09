package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

const objectKindDecls = `
	type port P message { inout integer }
	type component C { port P p; timer t := 1.0 }
	type record R { integer done, integer start }
	function f() runs on C {}
`

// The object of a component, port or timer operation is of the kind the
// operation takes, and that of a test component operation does not
// resolve to a template (ETSI 21.3.2-21.3.6 a, 22.2.1 g, 22.3.1 n, 22.5.1
// a, 22.5.4 a).
func TestObjectKinds(t *testing.T) {
	for name, c := range map[string]struct{ body, code string }{
		"a value cleared":        {`var integer v := 0; v.clear;`, "object-kind"},
		"a value halted":         {`var integer v := 0; v.halt;`, "object-kind"},
		"a value called":         {`var charstring v := ""; v.call(f, nowait);`, "object-kind"},
		"a timer sent on":        {`t.send(1);`, "object-kind"},
		"a port killed":          {`p.kill;`, "object-kind"},
		"a component timed out":  {`var C c := C.create; c.timeout;`, "object-kind"},
		"a record value started": {`var R r := {1, 2}; r.kill;`, "object-kind"},
		"a template started":     {`var C c := C.create; var template C tc := c; tc.start(f());`, "object-is-template"},
		"a template parameter":   {`g(null);`, "object-is-template"},
	} {
		tree := parse(t, "module M {"+objectKindDecls+
			"function g(template C pc) runs on C { pc.kill }\n"+
			"testcase tc() runs on C {\n"+c.body+"\n}\n}")
		diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
		if !containsCode(diags, c.code) {
			t.Errorf("%s: expected %s, got %v", name, c.code, codes(diags))
		}
	}
}

// What the operations take stays allowed, a field named like an
// operation is a field, and a reference whose declaration is not found
// is let be.
func TestObjectKindsAllowed(t *testing.T) {
	tree := parse(t, `module M {`+objectKindDecls+`
		function h(C pc, timer pt) runs on C { pc.kill; pt.start(1.0); pt.timeout }
		testcase tc() runs on C {
			var C c := C.create;
			c.start(f()); c.done; c.kill; if (c.alive or c.running) {}
			p.send(1); p.clear; p.halt; p.start; p.stop;
			t.start; t.timeout; if (t.running) {} var float x := t.read;
			var R r := {1, 2}; var integer d := r.done + r.start;
			unknown.send(1);
			all component.done; any port.receive;
		}
	}`)
	for _, d := range NewAnalyzer(&ttcn3.DB{}).Analyze(tree) {
		if d.Code == "object-kind" || d.Code == "object-is-template" {
			t.Errorf("unexpected %s: %s", d.Code, d.Message)
		}
	}
}

package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// Functions without a return value are invoked directly (16.1.1 a); all
// component goes with all port, alone (21.1.2); configuration operations
// take one component reference (6.2.11); sending operations have a port
// and a value (22.1.4.1).
func TestOperationShapeRules(t *testing.T) {
	cases := []struct {
		name, stmt, want string
	}{
		{"void function initialises", "var integer v := f_void();", "void-function-in-expression"},
		{"void function assigned", "var integer v; v := f_void();", "void-function-in-expression"},
		{"void function operand", "var integer v := 1 + f_void();", "void-function-in-expression"},
		{"all component without all port", "disconnect(all component:p);", "all-component-without-all-port"},
		{"all component with a second argument", "disconnect(all component:all port, v_ptc:p);", "all-component-not-alone"},
		{"connect on an array", "connect(self:p, v_ptcs:p);", "configuration-on-component-array"},
		{"send without port", "send(integer:1);", "sending-operation-without-port"},
		{"send without value", "p.send();", "sending-operation-without-value"},
		{"call without signature", "q.call();", "sending-operation-without-value"},

		{"void function invoked directly", "f_void();", ""},
		{"value function in an expression", "var integer v := 1 + f_int();", ""},
		{"all component:all port alone", "disconnect(all component:all port);", ""},
		{"disconnect without arguments", "disconnect;", ""},
		{"connect on an element", "connect(self:p, v_ptcs[0]:p);", ""},
		{"send with a value", "p.send(1);", ""},
	}
	for _, c := range cases {
		tree := parse(t, `module M {
			type port P message { inout integer }
			signature S(in integer x);
			type port Q procedure { inout S }
			type component C { port P p; port Q q }
			function f_void() {}
			function f_int() return integer { return 1 }
			testcase TC() runs on C {
				var C v_ptc := C.create;
				var C v_ptcs[2];
				`+c.stmt+`
			}
		}`)
		got := codes(NewAnalyzer(&ttcn3.DB{}).Analyze(tree))
		ruleCodes := []string{"void-function-in-expression", "all-component-without-all-port", "all-component-not-alone",
			"configuration-on-component-array", "sending-operation-without-port", "sending-operation-without-value"}
		var hit []string
		for _, code := range got {
			if slices.Contains(ruleCodes, code) {
				hit = append(hit, code)
			}
		}
		switch {
		case c.want == "" && len(hit) > 0:
			t.Errorf("%s: unexpected %v", c.name, hit)
		case c.want != "" && !slices.Contains(got, c.want):
			t.Errorf("%s: expected %s, got %v", c.name, c.want, got)
		}
	}
}

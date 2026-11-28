package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// Component references (6.2.11), ports of communication operations
// (22.1.4.2, 22.2.1), sender variables (22.4 h) and out template
// parameters (5.4.2).
func TestComponentRefRules(t *testing.T) {
	cases := []struct {
		name, stmt, want string
	}{
		{"reference to a component of an incompatible type", "var CA v_ref := CB.create;", "component-ref-incompatible"},
		{"assigned reference to an incompatible component", "var CA v_ref; v_ref := CB.create;", "component-ref-incompatible"},
		{"integer as a component reference", "var integer v_ref := 0; connect(self:p, v_ref:p);", "port-ref-not-component"},
		{"receive without a port", "receive(integer:?);", "port-op-without-port"},
		{"object sent", "var MyClass v_obj := MyClass.create(); p.send(v_obj);", "send-non-data-type"},
		{"value restricted actual for an out template", "var template(value) integer v_t := 0; f_out(v_t);", "out-template-restriction-stricter"},
		{"sender of a type the from component lacks", "var CA v_src; var CB v_s; p.receive(integer:?) from v_src -> sender v_s;", "sender-type-incompatible"},

		{"reference to a component of a compatible type", "var CA v_ref := CAx.create;", ""},
		{"component reference", "var CA v_ref := CA.create; connect(self:p, v_ref:p);", ""},
		{"sender of a type the from component has", "var CAx v_src; var CA v_s; p.receive(integer:?) from v_src -> sender v_s;", ""},
		{"unrestricted actual for an out template", "var template integer v_t := 0; f_out(v_t);", ""},
		{"value restricted actual for a value restricted out template", "var template(value) integer v_t := 0; f_outv(v_t);", ""},
	}
	ruleCodes := []string{"component-ref-incompatible", "port-ref-not-component", "port-op-without-port", "send-non-data-type", "out-template-restriction-stricter", "sender-type-incompatible"}
	for _, c := range cases {
		tree := parse(t, `module M {
			type port P message { inout integer }
			type class MyClass { var integer v_a := 1; }
			type component CA { var integer vc_a := 1; port P p }
			type component CAx { var integer vc_a := 1; port P p; var float vc_x := 1.0 }
			type component CB { var float vc_b := 1.0 }
			function f_out(out template integer p_t) { p_t := 1 }
			function f_outv(out template(value) integer p_t) { p_t := 1 }
			testcase TC() runs on CA {
				`+c.stmt+`
			}
		}`)
		got := codes(NewAnalyzer(&ttcn3.DB{}).Analyze(tree))
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

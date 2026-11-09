package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

const runsOnDecls = `
	type port P message { inout integer }
	type component Base { var integer v := 0; port P p }
	type component Ext extends Base { timer t := 1.0 }
	type component Same { var integer v := 0; port P p }
	type component Other { var integer v := 1; port P p }
	type port Q message { inout integer }
	type component OtherPort { var integer v := 0; port Q p }
	type component VarNotPort { var integer v := 0; var integer p }
	type component Empty {}
	type component Ports { port P p }
	function f_base() runs on Base {}
	function f_ext() runs on Ext {}
	function f_mtc() mtc Ports {}
	function f_sys() system Ports {}
	function f_g() runs on Base { f_base() }
`

// A function is invoked on a component of a type that has identical
// definitions of all those of its runs on type, and its mtc and system
// clauses' port definitions in the caller's mtc and system types (ETSI
// 6.3.3, 16.1.1 d, f).
func TestRunsOnCompatibility(t *testing.T) {
	for name, c := range map[string]struct{ src, code string }{
		"a component missing a definition": {`testcase tc() runs on Empty { f_base() }`, "runs-on-incompatible"},
		"a port of another type":           {`testcase tc() runs on OtherPort { f_base() }`, "runs-on-incompatible"},
		"a variable where a port is":       {`testcase tc() runs on VarNotPort { f_base() }`, "runs-on-incompatible"},
		"a base calling the extended":      {`testcase tc() runs on Base { f_ext() }`, "runs-on-incompatible"},
		"an mtc without the port":          {`testcase tc() runs on Empty { f_mtc() }`, "mtc-incompatible"},
		"a system without the port":        {`testcase tc() runs on Ports system Empty { f_sys() }`, "system-incompatible"},
	} {
		tree := parse(t, "module M {"+runsOnDecls+c.src+"\n}")
		if diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree); !containsCode(diags, c.code) {
			t.Errorf("%s: expected %s, got %v", name, c.code, codes(diags))
		}
	}
}

// Compatible components stay allowed: the same type, one that extends
// it, one of identical definitions, an mtc and system with the ports;
// and a behaviour started on another component, or an mtc not known in
// a function without an mtc clause, is let be.
func TestRunsOnCompatibilityAllowed(t *testing.T) {
	tree := parse(t, `module M {`+runsOnDecls+`
		testcase t1() runs on Base { f_base() }
		testcase t2() runs on Ext { f_base(); f_ext() }
		testcase t3() runs on Same { f_base() }
		testcase t6() runs on Other { f_base() }
		testcase t4() runs on Ports { f_mtc(); f_sys() }
		testcase t5() runs on Empty { var Base c := Base.create; c.start(f_base()); c.done }
		function f_h() runs on Base { f_mtc() }
	}`)
	for _, d := range NewAnalyzer(&ttcn3.DB{}).Analyze(tree) {
		switch d.Code {
		case "runs-on-incompatible", "mtc-incompatible", "system-incompatible":
			t.Errorf("unexpected %s: %s", d.Code, d.Message)
		}
	}
}

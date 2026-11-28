package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// TestNewerSyntaxRuns: a variadic parameter is the record of the actuals
// from its position on, its default for none or `-`, the record itself in
// assignment notation, and expands `r...`; a variable an if, while or
// select header declares is the statement's own; an embedded field is a
// field named by its type, its value encoded as JSON too.
func TestNewerSyntaxRuns(t *testing.T) {
	runLogged(t, `module M {
		type record of integer RoI;
		type record B { integer b }
		type record A { B, integer x } with { encode "JSON" }
		type component C {}
		function f(in integer i... := {1, 2, 3}) return integer { return lengthof(i); }
		function g(in integer a, in integer r...) return integer {
			var integer s := a;
			for (var integer k := 0; k < lengthof(r); k := k + 1) { s := s + r[k]; }
			return s;
		}
		testcase tc() runs on C {
			var RoI l := {4, 5};
			var integer n := 0;
			if (var integer x := 5; x == 5) { n := n + 1; }
			while (var integer i := 0; i < 0) { }
			select (var integer k := 2; k) { case (2) { n := n + 1; } case else { } }
			var A v := { B := { b := 1 }, x := 2 };
			if (f() == 3 and f(-) == 3 and f(7) == 1 and g(1) == 1 and g(1, 2, 3) == 6 and g(1, l...) == 10 and
				g(a := 1, r := {2, 2}) == 5 and n == 2 and v.B.b == 1 and
				encvalue_unichar(v) == "{""B"":{""b"":1},""x"":2}") {
				setverdict(pass);
			} else {
				setverdict(fail);
			}
		}
	}`, clocks[0].opts)
}

// TestExecuteInAConditionsHeader: a testcase a control part executes in
// an if header's declaration is found with its actual parameters.
func TestExecuteInAConditionsHeader(t *testing.T) {
	src := `module M {
		type component C {}
		testcase tc(integer p) runs on C { if (p == 5) { setverdict(pass); } else { setverdict(fail); } }
		control { if (var verdicttype v := execute(tc(5)); v == pass) { } }
	}`
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", clocks[0].opts)
	if err != nil || v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), %v; want pass", v, reason, err)
	}
}

// TestLoggedDefaultsAndEmptyReceives: an altstep run as a default is
// entered as a default, and a receive that finds its queue empty is logged
// as an event the TCI-TL log leaves out, with its port.
func TestLoggedDefaultsAndEmptyReceives(t *testing.T) {
	rec, _ := runLogged(t, `module M {
		type port P message { inout charstring }
		type component C { port P p; timer t := 0.2; }
		altstep guard() runs on C { [] t.timeout { setverdict(pass); } }
		testcase tc() runs on C {
			var default d := activate(guard());
			t.start;
			alt { [] p.receive("hello") { setverdict(fail); } }
			deactivate(d);
		}
	}`, clocks[0].opts)
	defaults, empty := 0, 0
	for _, e := range rec.Events {
		if e.Op == "tliSEnter" && e.Default {
			defaults++
		}
		if e.Op == "tliMReceiveEmpty" {
			empty++
			if !e.Aside {
				t.Errorf("empty receive not aside")
			}
		}
	}
	if defaults == 0 || empty == 0 {
		t.Fatalf("default entries %d, empty receives %d", defaults, empty)
	}
}

// TestLoggedEmptyReceivesOfAnyPort: `any port` that finds all its queues
// empty is logged once, as of any port, not for each; a template of a
// predefined type names it.
func TestLoggedEmptyReceivesOfAnyPort(t *testing.T) {
	rec, _ := runLogged(t, `module M {
		type port P message { inout charstring }
		type component C { port P p1; port P p2; timer t := 0.1; }
		testcase tc() runs on C {
			t.start;
			alt { [] any port.receive(charstring:?) { setverdict(fail); } [] t.timeout { setverdict(pass); } }
		}
	}`, clocks[0].opts)
	var anyPort, perPort int
	for _, e := range rec.Events {
		if e.Op != "tliMReceiveEmpty" {
			continue
		}
		if e.Path != "(any)" {
			perPort++
			continue
		}
		anyPort++
		if len(e.Parts) != 1 || e.Parts[0].Type != "charstring" {
			t.Errorf("type of the template: %v", e.Parts)
		}
	}
	if anyPort == 0 || perPort != 0 {
		t.Fatalf("any port %d, per port %d", anyPort, perPort)
	}
}

// TestLoggingAnEmptyReceiveChangesNothing: the template of a receive whose
// queue is empty is evaluated for the log only when that does nothing a
// test could tell: a function it calls does not run.
func TestLoggingAnEmptyReceiveChangesNothing(t *testing.T) {
	rec, _ := runLogged(t, `module M {
		type port P message { inout charstring }
		type component C { port P p; var integer n := 0; timer t := 0.1; }
		function g() runs on C return charstring { n := n + 1; return "x"; }
		testcase tc() runs on C {
			t.start;
			alt {
				[] p.receive(g()) { setverdict(fail); }
				[] t.timeout { if (n == 0) { setverdict(pass); } else { setverdict(fail, "logging ran g"); } }
			}
		}
	}`, clocks[0].opts)
	found := false
	for _, e := range rec.Events {
		found = found || e.Op == "tliMReceiveEmpty"
	}
	if !found {
		t.Fatal("no empty receive logged")
	}
}

// TestLoggedInterleavesAndAnyPortRedirects: an interleave is entered and
// left, events TCI-TL has none for; `any port` with a redirect is logged as
// of any port when its queues are empty.
func TestLoggedInterleavesAndAnyPortRedirects(t *testing.T) {
	rec, _ := runLogged(t, `module M {
		type port P message { inout charstring }
		type component C { port P p1; port P p2; timer t := 0.1; }
		altstep dflt() runs on C { [] t.timeout { setverdict(pass); } }
		testcase tc() runs on C {
			var charstring v;
			t.start;
			alt { [] any port.receive(charstring:?) -> value v { setverdict(fail); } [] t.timeout {} }
			var default d := activate(dflt());
			t.start;
			interleave { [] p1.receive("never") {} }
		}
	}`, clocks[0].opts)
	got := map[string]int{}
	for _, e := range rec.Events {
		switch {
		case e.Op == "tliIEnter" || e.Op == "tliILeave":
			if !e.Aside {
				t.Errorf("%s not aside", e.Op)
			}
			got[e.Op]++
		case e.Op == "tliMReceiveEmpty" && e.Path == "(any)":
			got["any"]++
		}
	}
	if got["tliIEnter"] != 1 || got["tliILeave"] != 1 || got["any"] == 0 {
		t.Fatalf("logged %v", got)
	}
}

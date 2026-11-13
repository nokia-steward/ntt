package interpreter_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

const unboundTypes = `
	type record R { integer a, integer b }
	type record R2 { integer c, integer d }
	type record O { integer a, integer b optional }
	type record N { integer f1, O f2 optional }
	type record of integer RI
	type union U { integer c1, charstring c2 }
	type record W { U u, RI l }
	type component C {}
	function f_noret(integer p) return integer { var integer x := p; }
	function f_unbound() return integer { var integer x; return x }
	function f_ret(integer p) return integer { return p + 1 }
	function f_branch(integer p) return integer { if (p == 0) { return 1 } else { return 2 } }
	function f_stop() return integer { stop }
`

// Using an uninitialised value where a value is needed is an error: an
// operand of an operator, which is completely initialised (ETSI ES 201
// 873-1 7.1.0 a, 7.1.3); the right-hand side of an assignment or an
// initialisation (19.1.1 c, 11.1 i, 10 d), a function's result too
// (16.1.1 g); a field of an omitted or uninitialised field of a value
// (6.2.1.1). A function with a return clause that ends without a return
// statement is an error (16.1.0).
func TestUnboundValueErrors(t *testing.T) {
	for name, c := range map[string]struct{ body, clause string }{
		"an operand":                     {`var integer a; var integer b := a + 1;`, "7.1.0"},
		"a relational operand":           {`var integer a; if (a < 1) {}`, "7.1.0"},
		"a logical operand":              {`var boolean a; if (a and true) {}`, "7.1.0"},
		"a partial record compared":      {`var R v; v.a := 1; if (v == {1, 2}) {}`, "7.1.3"},
		"a record of with a hole":        {`var RI v := {0, -, 2}; if (v == {0, 1, 2}) {}`, "7.1.3"},
		"an unbound right-hand side":     {`var R s; var R d := {1, 2}; d := s;`, "19.1.1"},
		"an unbound initialiser":         {`var integer a; var integer b := a;`, "19.1.1"},
		"a constant from an unbound one": {`var integer a; const integer c := a;`, "19.1.1"},
		"a function's unbound result":    {`var integer b := f_unbound();`, "19.1.1"},
		"a field of an omitted field":    {`var N n := {f1 := 1, f2 := omit}; var integer i := n.f2.a;`, "6.2.1.1"},
		"a field of an unbound field":    {`var N n; n.f1 := 1; var integer i := n.f2.a;`, "6.2.1.1"},
		"a unary operand":                {`var integer a; var integer b := -a;`, "7.1.0"},
		"a negated operand":              {`var boolean a; var boolean b := not a;`, "7.1.0"},
		"an omitted field in arithmetic": {`var O o := {a := 1, b := omit}; var integer i := o.b + 1;`, "7.1.0"},
		"an unbound condition":           {`var boolean b; if (b) {}`, "11.1"},
		"a function without a return":    {`var integer r := f_noret(3);`, "16.1.0"},
	} {
		src := "module M {" + unboundTypes + "testcase tc() runs on C {\nsetverdict(pass);\n" + c.body + "\nsetverdict(pass);\n}\n}"
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
			if err != nil || v != runtime.ErrorVerdict || !strings.Contains(reason, c.clause) {
				t.Errorf("%s, %s clock: %s (%s) %v; want the %s error", name, k.name, v, reason, err, c.clause)
			}
		}
	}
}

// What the standard allows stays allowed: an omitted optional field and
// a union's one alternative are initialised; a list concatenation takes
// partially initialised operands (7.1.2 a); an unbound value may be
// passed, returned and invoked as a statement (11.1 d, 16.1.1 g); a
// function may end in stop.
func TestUnboundValueAllowed(t *testing.T) {
	for name, body := range map[string]string{
		"omitted fields":            `var O o := {a := 1, b := omit}; if (not (o == {1, omit}) or o.b != omit) { setverdict(fail) }`,
		"union values":              `var U u := {c1 := 1}; var U w; w.c2 := "x"; if (u != {c1 := 1} or w == u) { setverdict(fail) }`,
		"nested values":             `var W w := {u := {c1 := 1}, l := {1, 2}}; if (w != w) { setverdict(fail) }`,
		"partial concatenated":      `var RI l := {1, -}; var RI m := l & {3}; if (lengthof(m) != 3) { setverdict(fail) }`,
		"an unbound returned":       `f_unbound();`,
		"returns":                   `if (f_ret(3) != 4 or f_branch(0) != 1) { setverdict(fail) }`,
		"compatible types":          `var R2 r := {c := 1, d := 2}; var R v := r; if (v != v) { setverdict(fail) }`,
		"an omitted field compared": `var O o := {a := 1, b := omit}; if (o.b != omit) { setverdict(fail) }`,
		"presence checks":           `var N n := {f1 := 1, f2 := omit}; if (isbound(n.f2.a) or ispresent(n.f2.a)) { setverdict(fail) }`,
	} {
		t.Run(name, func(t *testing.T) {
			assertSameVerdictBothClocks(t, "module M {"+unboundTypes+
				"testcase tc() runs on C {\nsetverdict(pass);\n"+body+"\n}\n}", runtime.PassVerdict)
		})
	}
	v, reason := runSched(t, "M.tc", "module M {"+unboundTypes+"testcase tc() runs on C { setverdict(pass); var integer r := f_stop(); }\n}")
	if v != runtime.PassVerdict {
		t.Errorf("a function ending in stop: %s (%s), want pass", v, reason)
	}
}

// A function with a return clause that waits in an alt when its
// component is stopped does not end without a return (16.1.0, 21.3.3).
func TestStopWhileAReturningFunctionWaits(t *testing.T) {
	assertSameVerdictBothClocks(t, `module M {
		type port P message { inout integer }
		type component C { port P p }
		function f_wait() runs on C return integer {
			alt { [] p.receive(integer:?) { return 1 } }
		}
		function f_stopper() runs on C { setverdict(pass); mtc.stop }
		testcase tc() runs on C {
			var C c := C.create;
			c.start(f_stopper());
			var integer r := f_wait();
		}
	}`, runtime.PassVerdict)
}

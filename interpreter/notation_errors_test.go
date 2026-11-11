package interpreter_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

const notationTypes = `
	type record of integer RI
	type integer A[3]
	type map from charstring to integer TM
	type record O { integer a, integer b optional }
	type union U { integer i, float f }
	type component C {}
`

// An index is given once in an assignment or mixed notation, by position
// or assigned, and every index of a map's evaluates to a different value
// (ETSI ES 201 873-1 6.2.0, 6.2.15.2 a). The value matched by match is
// initialised (15.9 b): an alternative of a union not chosen is not.
func TestNotationErrors(t *testing.T) {
	for name, c := range map[string]struct{ body, clause string }{
		"a map key twice":            {`var TM m := {["a"] := 1, ["b"] := 2, ["a"] := 3};`, "6.2.15.2"},
		"a map key evaluated twice":  {`var charstring k := "a"; var TM m := {["a"] := 1, [k] := 2};`, "6.2.15.2"},
		"an index twice":             {`var RI l := {[0] := 1, [0] := 2};`, "6.2.0"},
		"an index given by position": {`var RI l := {1, 2, [1] := 3};`, "6.2.0"},
		"an array index twice":       {`var A a := {[2] := 1, [2] := 2};`, "6.2.0"},
		"an alternative not chosen":  {`var U u; u.f := 1.0; u.i := 2; if (match(u.f, 1.0)) {}`, "15.9"},
		"an unbound element matched": {`var RI l := {1, -}; if (match(l[1], ?)) {}`, "15.9"},
		"an alternative in parens":   {`var U u; u.i := 1; if (match((u.f), 1.0)) {}`, "15.9"},
	} {
		src := "module M {" + notationTypes + "testcase tc() runs on C {\nsetverdict(pass);\n" + c.body + "\nsetverdict(pass);\n}\n}"
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
			if err != nil || v != runtime.ErrorVerdict || !strings.Contains(reason, c.clause) {
				t.Errorf("%s, %s clock: %s (%s) %v; want the %s error", name, k.name, v, reason, err, c.clause)
			}
		}
	}
}

// What the standard allows stays allowed: distinct indices in any order,
// positional values then assigned ones, an omitted optional field
// matched, the alternative chosen matched.
func TestNotationAllowed(t *testing.T) {
	for name, body := range map[string]string{
		"distinct keys":          `var TM m := {["a"] := 1, ["b"] := 2}; if (m["b"] != 2) { setverdict(fail) }`,
		"distinct indices":       `var A a := {[2] := 1, [0] := 0, [1] := 5}; if (a[1] != 5) { setverdict(fail) }`,
		"mixed notation":         `var RI l := {1, [2] := 3, [1] := 2}; if (l != {1, 2, 3}) { setverdict(fail) }`,
		"an omitted field":       `var O o := {a := 1, b := omit}; if (not match(o.b, *)) { setverdict(fail) }`,
		"the alternative chosen": `var U u; u.f := 1.0; u.i := 2; if (not match(u.i, 2)) { setverdict(fail) }`,
	} {
		t.Run(name, func(t *testing.T) {
			assertSameVerdictBothClocks(t, "module M {"+notationTypes+
				"testcase tc() runs on C {\nsetverdict(pass);\n"+body+"\n}\n}", runtime.PassVerdict)
		})
	}
}

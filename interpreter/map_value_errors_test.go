package interpreter_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

const mapValueTypes = `
	type map from charstring to integer TM
	type map from charstring to charstring TS
	type record R { TS m }
	type component C {}
`

// The map unmap removes from is an initialised variable (ETSI ES 201
// 873-1 6.2.15.3 a), and a map value associates no key with omit
// (6.2.15.9): not by its initialiser, an assignment or an element
// assignment, nor as a field of another value.
func TestMapValueErrors(t *testing.T) {
	for name, c := range map[string]struct{ body, clause string }{
		"unmap of an unbound map":    {`var TM m; unmap(m, "a");`, "6.2.15.3"},
		"an omit initialiser":        {`var TS m := {["a"] := "x", ["b"] := omit};`, "6.2.15.9"},
		"an omit constant":           {`const TS c := {["b"] := omit};`, "6.2.15.9"},
		"an omit assigned":           {`var TS m; m := {["b"] := omit};`, "6.2.15.9"},
		"an omit element assignment": {`var TS m := {["a"] := "x"}; m["b"] := omit;`, "6.2.15.9"},
		"an omit in a field":         {`var R r := {m := {["b"] := omit}};`, "6.2.15.9"},
	} {
		src := "module M {" + mapValueTypes + "testcase tc() runs on C {\nsetverdict(pass);\n" + c.body + "\nsetverdict(pass);\n}\n}"
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
			if err != nil || v != runtime.ErrorVerdict || !strings.Contains(reason, c.clause) {
				t.Errorf("%s, %s clock: %s (%s) %v; want the %s error", name, k.name, v, reason, err, c.clause)
			}
		}
	}
}

// What the standard allows stays allowed: unmap of a key a map does not
// have, and omit in map templates of any restriction.
func TestMapValueAllowed(t *testing.T) {
	for name, body := range map[string]string{
		"unmap of a key not mapped": `var TM m := {["a"] := 1}; unmap(m, "zz"); unmap(m, "a"); if (lengthof(m) != 0) { setverdict(fail) }`,
		"omit in a template":        `var template TS m := {["a"] := "x", ["b"] := omit};`,
		"omit in a restricted one":  `var template(omit) TS m := {["b"] := omit}; m["c"] := omit;`,
		"omit in a definition":      `template(value) TS t := {["b"] := omit};`,
	} {
		t.Run(name, func(t *testing.T) {
			assertSameVerdictBothClocks(t, "module M {"+mapValueTypes+
				"testcase tc() runs on C {\nsetverdict(pass);\n"+body+"\n}\n}", runtime.PassVerdict)
		})
	}
}

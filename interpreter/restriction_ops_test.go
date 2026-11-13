package interpreter_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

const restrictionTypes = `
	type record E { integer a, boolean b optional }
	type component C {}
`

// The omit operation yields a template that fulfils the omit restriction,
// omit or a value, and the present operation one that does not as a whole
// match omit; any other situation is an error (ETSI ES 201 873-1 15.12,
// 15.13, 15.8), also where a template is initialised (15.3).
func TestRestrictionOperationErrors(t *testing.T) {
	for name, c := range map[string]struct{ body, clause string }{
		"omit of AnyValue":             {`var template integer t := ?; var template integer u := omit(t);`, "15.12"},
		"omit of a field matcher":      {`var template E t := {a := ?, b := omit}; var template E u := omit(t);`, "15.12"},
		"omit of an unbound template":  {`var template integer t; var template integer u := omit(t);`, "15.12"},
		"present of AnyValueOrNone":    {`var template E t := *; var template E u := present(t);`, "15.13"},
		"present of omit":              {`var template E t := omit; var template E u := present(t);`, "15.13"},
		"present of ifpresent":         {`var template integer t := 1 ifpresent; var template integer u := present(t);`, "15.13"},
		"present of a list with omit":  {`var template integer t := (1, omit); var template integer u := present(t);`, "15.13"},
		"present in a local template":  {`var template E t := *; template(present) E u := present(t);`, "15.13"},
		"omit in a local template":     {`var template E t := ?; template(omit) E u := omit(t);`, "15.12"},
		"present in a later statement": {`var template E t := *; var template E u; u := present(t);`, "15.13"},
	} {
		src := "module M {" + restrictionTypes + "testcase tc() runs on C {\nsetverdict(pass);\n" + c.body + "\nsetverdict(pass);\n}\n}"
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
			if err != nil || v != runtime.ErrorVerdict || !strings.Contains(reason, c.clause) {
				t.Errorf("%s, %s clock: %s (%s) %v; want the %s error", name, k.name, v, reason, err, c.clause)
			}
		}
	}
}

// What the restrictions allow stays allowed: omit of omit and of values,
// present of values and of matching mechanisms that do not match omit,
// fields included; the operations yield their operand.
func TestRestrictionOperationAllowed(t *testing.T) {
	for name, body := range map[string]string{
		"omit of omit":               `var template E t := omit; template(omit) E u := omit(t); if (ispresent(u)) { setverdict(fail) }`,
		"omit of a value":            `var template E t := {a := 1, b := omit}; var template E u := omit(t); if (not match({1, omit}, u)) { setverdict(fail) }`,
		"present of AnyValue":        `var template E t := ?; template(present) E u := present(t); if (not match({1, true}, u)) { setverdict(fail) }`,
		"present of field wildcards": `var template E t := {a := ?, b := *}; var template E u := present(t); if (not match({1, omit}, u)) { setverdict(fail) }`,
		"present of a value list":    `var template integer t := (1, 2); var template integer u := present(t); if (not match(2, u)) { setverdict(fail) }`,
	} {
		t.Run(name, func(t *testing.T) {
			assertSameVerdictBothClocks(t, "module M {"+restrictionTypes+
				"testcase tc() runs on C {\nsetverdict(pass);\n"+body+"\n}\n}", runtime.PassVerdict)
		})
	}
}

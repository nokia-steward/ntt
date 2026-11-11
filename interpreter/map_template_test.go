package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/runtime"
)

const mapTemplateTypes = `
	type map from charstring to integer TM
	type map from integer to charstring TI
	type component C {}
`

// Maps follow ETSI ES 201 873-1 V4.17.1: they are compared and templates
// of a map type match them (6.2.15.1, whose restrictions are void); a map
// template's `*` matches the elements whose keys it does not have
// (B.1.3.2), and a value that matches omit lets its key be absent
// (B.1.2.4); `m[*]` reads and sets that `*`, and `unmap(m, *)` removes it
// (6.2.15.3, 6.2.15.4); a map's keys may be integers (6.2.15).
func TestMapTemplates(t *testing.T) {
	for name, body := range map[string]string{
		"equal maps": `var TM a := {["x"] := 1, ["y"] := 2}, b := {["y"] := 2, ["x"] := 1};
			if (a != b) { setverdict(fail, "a != b") }`,
		"a value matched": `template TM t := {["x"] := ?}; var TM a := {["x"] := 7};
			if (not match(a, t)) { setverdict(fail, "no match") }`,
		"a key not in the template": `template TM t := {["x"] := 1}; var TM a := {["x"] := 1, ["y"] := 2};
			if (match(a, t)) { setverdict(fail, "matched") }`,
		"AnyElementsOrNone": `template TM t := {*, ["x"] := 1}; var TM a := {["x"] := 1, ["y"] := 2};
			if (not match(a, t)) { setverdict(fail, "no match") }`,
		"a key absent": `template TM t := {["x"] := 1, ["y"] := *}; var TM a := {["x"] := 1};
			if (not match(a, t)) { setverdict(fail, "no match") }`,
		"a key required": `template TM t := {["x"] := 1, ["y"] := ?}; var TM a := {["x"] := 1};
			if (match(a, t)) { setverdict(fail, "matched") }`,
		"m[*] read and set": `var template TM t := {["x"] := 1}; var TM a := {["x"] := 1, ["y"] := 2};
			if (t[*] or isbound(t[*])) { setverdict(fail, "has *") }
			t[*] := true;
			if (not t[*] or not ispresent(t[*]) or not match(a, t)) { setverdict(fail, "no *") }`,
		"unmap of *": `var template TM t := {*, ["x"] := 1}; var TM a := {["x"] := 1, ["y"] := 2};
			unmap(t, *);
			if (match(a, t)) { setverdict(fail, "matched") }`,
		"integer keys": `var TI m := {[5] := "five", [1] := "one"};
			if (lengthof(m) != 2 or m[5] != "five") { setverdict(fail, "keys") }
			var TI n; n[100] := "x";
			if (lengthof(n) != 1 or n[100] != "x") { setverdict(fail, "element") }`,
		"keys and values": `var TI m := {[1] := "a", [2] := "a"}; var TI.from k := m.from;
			if (lengthof(k) != 2 or not match(m.to, {"a", "a"})) { setverdict(fail, "from/to") }`,
	} {
		t.Run(name, func(t *testing.T) {
			assertSameVerdictBothClocks(t, "module M {"+mapTemplateTypes+
				"testcase tc() runs on C {\nsetverdict(pass);\n"+body+"\n}\n}", runtime.PassVerdict)
		})
	}
}

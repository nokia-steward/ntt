package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// Subtypes and the values of them (6.2.13).
func TestSubtypeRules(t *testing.T) {
	cases := []struct {
		name, defs, want string
	}{
		{"element longer than the innermost length", `type record of charstring S23 length(2 .. 3); const S23 c := { "aa", "bbbb" };`, "subtype-length-violation"},
		{"inner list shorter than the innermost length", `type record of charstring S23 length(2 .. 3); type record of S23 S45 length(4 .. 5); const S45 c := { { "aa", "bbb", "cc" } };`, "subtype-length-violation"},
		{"iterated type outside the innermost type", "type integer Small (0 .. 10); type set of Small SmallSet (20 .. 30);", "subtype-not-a-subset"},
		{"enumerated value outside the parent", "type enumerated E { e1, e2, e3 } type E E1 (e1, e2); type E1 E2 (e1, e3);", "subtype-value-not-in-parent"},
		{"record value outside the parent", `type record R { integer f1 optional, charstring f2 } type R R1 ({ f1 := omit, f2 := "a" }); type R1 R2 ({ f1 := 1, f2 := "a" });`, "subtype-value-not-in-parent"},
		{"partial template in a record of list", "type record of integer RoI; type RoI RoI2 ({ 1, 2 }, { 3, - });", "subtype-list-partial-template"},
		{"partial template in value list notation", `type record R3 { integer a, charstring b } type R3 R3s ({ 1, "a" }, { 2, - });`, "subtype-list-partial-template"},
		{"list and length mixed", "type record of integer RoI; type RoI RoI2 ({ 1, 2 }, { 3, 4, 5 }) length(2);", "subtype-mixed-mechanisms"},
		{"value outside the list of a parent", "type record of integer RoI; type RoI RoIL ({ 1, 2 }, { 3, 4, 5 }); type RoIL RoIN length(2 .. 3); const RoIN c := { 7, 8 };", "subtype-value-not-in-list"},

		{"elements within the innermost length", `type record of charstring S23 length(2 .. 3); const S23 c := { "aa", "bbb", "cc", "dd" };`, ""},
		{"iterated type within the innermost type", "type integer Small (0 .. 10); type set of Small SmallSet (2 .. 3);", ""},
		{"enumerated values of the parent", "type enumerated E { e1, e2, e3 } type E E1 (e1, e2); type E1 E2 (e2);", ""},
		{"partial template in field assignment notation", `type record R3 { integer a, charstring b } type R3 R3s ({ a := 1 });`, ""},
		{"value in the list of a parent", "type record of integer RoI; type RoI RoIL ({ 1, 2 }, { 3, 4, 5 }); type RoIL RoIN length(2 .. 3); const RoIN c := { 3, 4, 5 };", ""},
		{"value through a constant", "type record of integer RoI; type RoI RoIL ({ 1, 2 }); const integer c_one := 1; const RoIL c := { c_one, 2 };", ""},
		{"list and length on a string", `type charstring Str ("a", "bb") length(1 .. 2);`, ""},
		{"set of value in another order", "type set of integer SoI; type SoI SoIL ({ 1 }, { 1, 2 }); const SoIL c := { 2, 1 };", ""},
		{"set of subtype in another order", "type set of integer SoI; type SoI SoIL ({ 1, 2 }); type SoIL SoIL2 ({ 2, 1 });", ""},
		{"record of value in another order", "type record of integer RoI; type RoI RoIL ({ 1, 2 }); const RoIL c := { 2, 1 };", "subtype-value-not-in-list"},
		{"parent value in another notation", `type record R { integer f1, charstring f2 } type R R1 ({ 1, "a" }); type R1 R2 ({ f1 := 1, f2 := "a" });`, ""},
	}
	ruleCodes := []string{"subtype-length-violation", "subtype-not-a-subset", "subtype-value-not-in-parent",
		"subtype-list-partial-template", "subtype-mixed-mechanisms", "subtype-value-not-in-list"}
	for _, c := range cases {
		tree := parse(t, "module M {\n"+c.defs+"\n}")
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

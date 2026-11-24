package semantic

import (
	"slices"
	"strings"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// Template lists and complemented lists hold values of their type, take
// all from a record of or set of template of a compatible member type that
// is not a matching mechanism as a whole, and reference templates that obey
// the present restriction (B.1.2.1, B.1.2.2).
func TestTemplateListRules(t *testing.T) {
	cases := []struct {
		name, decls, want string
	}{
		{"element of another type", `template R t := { f1 := (1, "two", 3) }`, "template-list-element-type"},
		{"complemented element of another type", `template R t := { f1 := complement(1, 2.0) }`, "template-list-element-type"},
		{"all from a single template", `template integer s := 5; template integer t := (all from s, 100);`, "all-from-not-a-list"},
		{"all from of an incompatible member type", `template RoI s := { 1, 2 }; template RoI t := (all from s);`, "all-from-member-type-incompatible"},
		{"complemented all from of an incompatible member type", `template RoI s := { 1, 2 }; template RoI t := complement(all from s);`, "all-from-member-type-incompatible"},
		{"all from a matching mechanism", `template RoI s := ?; template integer t := (0, all from s);`, "all-from-matching-mechanism"},
		{"template not obeying present", `template integer s := *; template O t := { f1 := (1, s) };`, "template-list-not-present"},

		{"elements of the type", `template R t := { f1 := (1, 2, 3) }`, ""},
		{"all from a list of the member type", `template RoI s := { 1, 2 }; template integer t := (0, all from s);`, ""},
		{"all from a combined template", `template RoI s := { 1, ? }; template integer t := complement(all from s);`, ""},
		{"omit in an optional field's list", `template O t := { f1 := (1, omit) }`, ""},
		{"charstring in a universal charstring list", `template universal charstring t := ("a", "b");`, ""},
		{"present template in the list", `template integer s := ?; template O t := { f1 := (1, s) };`, ""},
	}
	for _, c := range cases {
		tree := parse(t, `module M {
			type record R { integer f1 }
			type record O { integer f1 optional }
			type record of integer RoI;
			`+c.decls+`
		}`)
		got := codes(NewAnalyzer(&ttcn3.DB{}).Analyze(tree))
		var list []string
		for _, code := range got {
			if strings.HasPrefix(code, "template-list-") || strings.HasPrefix(code, "all-from-") {
				list = append(list, code)
			}
		}
		switch {
		case c.want == "" && len(list) > 0:
			t.Errorf("%s: unexpected %v", c.name, list)
		case c.want != "" && !slices.Contains(got, c.want):
			t.Errorf("%s: expected %s, got %v", c.name, c.want, got)
		}
	}
}

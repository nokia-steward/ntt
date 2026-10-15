package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/runtime"
)

// TestNestedTypeOfADeclaration: a declaration whose type is a type
// definition of its own (ES 201 873-1 11.2, TypeOrNestedTypeDef) has
// that type, as one of a named type would: a record written
// positionally has its fields, an enumerated type's values are known.
// None of these parsed.
func TestNestedTypeOfADeclaration(t *testing.T) {
	assertSameVerdictBothClocks(t, `module M {
		const record of integer c := {1, 2};
		modulepar record of charstring mp := {"a"};
		type component C { var enumerated { red, green } col := green }
		function sum(record of integer p) return integer {
			var integer s := 0;
			for (var integer x in p) { s := s + x }
			return s
		}
		function pair(integer x) return record of integer { return {x, x} }
		testcase tc() runs on C {
			var record of integer a := {1, 2, 3};
			var record { integer f, charstring g } r := {1, "x"};
			var enumerated { e1, e2 } e := e2;
			var map from charstring to integer m := { ["k"] := 5 };
			var union { integer i, charstring s } u := {s := "y"};
			var template record of integer tl := {1, ?};
			var template record { integer f } t := {f := ?};
			var record of record of integer aa := {{1}, {2, 3}};
			if (a[2] != 3 or r.f != 1 or r.g != "x" or e != e2 or m["k"] != 5 or u.s != "y") {
				setverdict(fail, "local: ", a, r, e, m, u)
			} else if (not match({1, 9}, tl) or aa[1][1] != 3 or c[1] != 2 or mp[0] != "a") {
				setverdict(fail, "lists: ", tl, aa, c, mp)
			} else if (not match({f := 3}, t) or col != green or sum({1, 2}) != 3 or pair(4)[1] != 4) {
				setverdict(fail, "module: ", col)
			} else {
				setverdict(pass)
			}
		}
	}`, runtime.PassVerdict)
}

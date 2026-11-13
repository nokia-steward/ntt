package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// runShapeCases runs each body as the testcase of module M, with decls
// beside it, on both clocks, and requires pass.
func runShapeCases(t *testing.T, decls string, cases map[string]string) {
	t.Helper()
	for name, body := range cases {
		src := `module M {
			` + decls + `
			type component C {}
			function chk(boolean ok, charstring what) { if (not ok) { setverdict(fail, what) } }
			testcase tc() runs on C {
				` + body + `
				setverdict(pass);
			}
		}`
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
			if err != nil || v != runtime.PassVerdict {
				t.Errorf("%s, %s clock: %s (%s) %v", name, k.name, v, reason, err)
			}
		}
	}
}

// TestValuesOfADeclaredTypeTakeItsShape: a value written positionally is
// a value of the type it is declared of where it is made — a function's
// or method's result, `T:v`, a class member however it is set, an
// anytype's field — and
// compares by that type's field names with a value written with them,
// in `==` and in a select case alike.
func TestValuesOfADeclaredTypeTakeItsShape(t *testing.T) {
	runShapeCases(t, `
		type record R { integer b, integer a }
		type record R2 { integer y, integer x }
		function named() return R { var R r; r.a := 2; r.b := 1; return r }
		function pos() return R { return {1, 2} }
		function byName() return R { return {a := 2, b := 1} }
		type class K {
			var R m := {1, 2};
			public function get() return R { return m }
			public function lit() return R { return {1, 2} }
			public function put() { m := {3, 4} }
			public function putThis() { this.m := {5, 6} }
			public function ma() return integer { return m.a }
		}
		type class P {
			var R m;
			public function ma() return integer { return m.a }
			public function shadow(anytype m) return integer { m := {R := {1, 2}}; return m.R.a }
		}
		function asR(R2 v) return R { return v }`,
		map[string]string{
			"function result": `chk(named() == {1, 2}, "named");
				chk({1, 2} == byName(), "byName");
				chk(named() != {2, 1}, "differs");
				chk(pos() == named() and pos().a == 2, "pos")`,
			"valueof": `chk(valueof(named()) == {1, 2}, "valueof")`,
			"type-prefixed value": `var R r := named();
				chk(r == (R:{1, 2}), "T:v");
				chk((R:{1, 2}).a == 2, "field")`,
			"select case": `var R r := named();
				var integer n := 0;
				select (r) { case ({1, 2}) { n := 1 } case else { n := 2 } }
				select (named()) { case ({2, 1}) { n := 3 } case ({1, 2}) { n := n + 10 } }
				chk(n == 11, "select")`,
			"class member and method": `var K k := K.create();
				chk(k.get() == named(), "member");
				chk(k.lit() == named(), "method result");
				chk(k.get().a == 2, "member field");
				k.put();
				chk(k.ma() == 4, "assigned in a method");
				k.putThis();
				chk(k.ma() == 6, "assigned through this");
				var P p := P.create({7, 8});
				chk(p.ma() == 8, "implicit constructor")`,
			"anytype field": `var anytype at;
				at.R := named();
				chk(at.R == {1, 2}, "field");
				var anytype at2 := {R := {1, 2}};
				chk(at2.R == named() and at2.R.a == 2, "declared");
				at2 := {R := {3, 4}};
				chk(at2.R.b == 3, "assigned")`,
			"compatible type": `var R2 r2 := {x := 2, y := 1};
				template R t := {1, 2};
				chk(match(r2, t), "match");
				chk(r2 == named(), "equal");
				chk(match(R:r2, {1, 2}) and (R:r2).a == 2, "T:v");
				chk(match(asR(r2), {1, 2}) and asR(r2).a == 2, "result")`,
			"parameter named as a field": `var P p := P.create();
				var anytype x;
				chk(p.shadow(x) == 2, "shadow")`,
		})
}

// TestSetOfValuesAreUnordered: a value of a `set of` type equals and
// matches another whatever the order of their elements (ETSI 6.2.3.2,
// 7.1.3) however it was made — element by element, a function's result,
// a field — and a value literal compared with it is of its type; a
// record of stays ordered, and a set of counts its elements.
func TestSetOfValuesAreUnordered(t *testing.T) {
	runShapeCases(t, `
		type set of integer SI;
		type record R { integer b, integer a }
		type set of R SR;
		type record W { SI s }
		function mk() return SI { var SI s; s[0] := 2; s[1] := 1; return s }
		function lit() return SI { return {2, 1} }`,
		map[string]string{
			"element by element": `var SI a; a[0] := 1; a[1] := 2;
				var SI b; b[0] := 2; b[1] := 1;
				chk(a == b, "a == b");
				chk(b == {1, 2} and {1, 2} == b, "literal")`,
			"function result": `chk(mk() == {1, 2} and lit() == {1, 2} and mk() == lit(), "result")`,
			"field": `var W w1; w1.s[0] := 1; w1.s[1] := 2;
				var W w2 := {s := {2, 1}};
				var W w3; w3.s := {2, 1};
				chk(w1 == w2 and w2 == w3 and w1 == w3, "records");
				chk(w1.s == {2, 1}, "field")`,
			"records in a set of": `var SR s1 := {{1, 2}, {3, 4}};
				var SR s2; s2[0] := {b := 3, a := 4}; s2[1] := {a := 2, b := 1};
				chk(s1 == s2, "s1 == s2");
				chk(s2 == {{1, 2}, {3, 4}}, "literal")`,
			"match": `var SI b; b[0] := 2; b[1] := 1;
				chk(match(b, SI:{1, 2}) and match(mk(), {1, 2}), "match")`,
			"counts": `var SI a := {1, 2, 2};
				var SI b := {1, 1, 2};
				chk(a != b and a != {1, 2}, "counts")`,
			"record of is ordered": `var record of integer a := {1, 2};
				chk(a != {2, 1}, "ordered")`,
		})
}

// TestValuesOfAnotherModulesTypeTakeItsShape: a constant, module
// parameter or template of a type another module declares takes its
// shape whichever of the modules comes first.
func TestValuesOfAnotherModulesTypeTakeItsShape(t *testing.T) {
	user := `module U {
		import from T all;
		const R c := {1, 2};
		modulepar R mp := {1, 2};
		template R tp := {1, ?};
	}`
	types := `module T { type record R { integer b, integer a } }`
	main := `module M {
		import from T all;
		import from U all;
		type component C {}
		testcase tc() runs on C {
			var R r := {a := 2, b := 1};
			if (c == r and mp == r and c.a == 2 and mp.b == 1 and match(r, tp)) { setverdict(pass) } else { setverdict(fail) }
		}
	}`
	for name, order := range map[string][]string{
		"user first":  {user, types, main},
		"types first": {types, user, main},
		"main first":  {main, user, types},
	} {
		for _, k := range clocks {
			var trees []*ttcn3.Tree
			for _, src := range order {
				trees = append(trees, parse(t, src))
			}
			v, reason, err := interpreter.RunTestcaseWith(trees, "M.tc", k.opts)
			if err != nil || v != runtime.PassVerdict {
				t.Errorf("%s, %s clock: %s (%s) %v", name, k.name, v, reason, err)
			}
		}
	}
}

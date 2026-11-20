package interpreter_test

import (
	"testing"
	"time"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// TestSharedValuesAreValuesOfTheirOwn: a value assigned, declared,
// passed, sent, received or started with is shared by both holders until
// one changes it (runtime.Share), and then that one changes a copy of its
// own: in every way of writing into a value, the other holder keeps what
// it had (ETSI 6).
func TestSharedValuesAreValuesOfTheirOwn(t *testing.T) {
	for name, body := range map[string]string{
		"nested field": `
			var R a := { n := 1, s := { x := 1, xs := {1, 2} } };
			var R b := a;
			b.s.x := 5; b.s.xs[0] := 7;
			check(a.s.x == 1 and a.s.xs[0] == 1 and b.s.x == 5 and b.s.xs[0] == 7, a, b);`,
		"the original changed": `
			var R a := { n := 1, s := { x := 1, xs := {1, 2} } };
			var R b := a;
			a.s.xs[1] := 9; a.n := 2;
			check(b.s.xs[1] == 2 and b.n == 1 and a.s.xs[1] == 9, a, b);`,
		"a member taken out": `
			var R a := { n := 1, s := { x := 1, xs := {1, 2} } };
			var S s := a.s;
			a.s.x := 3; s.xs[0] := 4;
			check(s.x == 1 and a.s.xs[0] == 1 and a.s.x == 3 and s.xs[0] == 4, a, s);`,
		"a member put in twice": `
			var S s := { x := 1, xs := {1} };
			var SL l := { s, s };
			l[0].x := 2;
			check(l[1].x == 1 and s.x == 1, l, s);`,
		"growing a record of": `
			var IL a := {1, 2};
			var IL b := a;
			b[3] := 4;
			check(lengthof(a) == 2 and lengthof(b) == 4, a, b);`,
		"several indices": `
			var ILL a := { {1, 2}, {3, 4} };
			var ILL b := a;
			b[1][0] := 9;
			var IL ix := {0, 1};
			b[ix] := 8;
			check(a[1][0] == 3 and a[0][1] == 2 and b[1][0] == 9 and b[0][1] == 8, a, b);`,
		"a map element": `
			var IM a; a["k"] := 1; a["j"] := 2;
			var IM b := a;
			b["k"] := 3; unmap(b, "j");
			check(a["k"] == 1 and ispresent(a["j"]) and b["k"] == 3 and not ispresent(b["j"]), a, b);`,
		"a union's alternative": `
			var U a := { i := 1 };
			var U b := a;
			b.c := "x";
			check(ischosen(a.i) and a.i == 1 and ischosen(b.c), a, b);`,
		"an inout parameter": `
			var R a := { n := 1, s := { x := 1, xs := {1} } };
			var R b := a;
			bump(b);
			check(a.s.x == 1 and b.s.x == 2, a, b);`,
		"an in parameter": `
			var R a := { n := 1, s := { x := 1, xs := {1} } };
			check(writeIn(a) == 2 and a.s.x == 1, a);`,
		"a component variable returned": `
			cv := { x := 1, xs := {1} };
			var S s := get();
			s.x := 2; cv.xs[0] := 3;
			check(cv.x == 1 and s.xs[0] == 1, cv, s);`,
		"a constant": `
			var S s := c_s;
			s.xs[0] := 5;
			var S t := c_s;
			check(t.xs[0] == 1 and s.xs[0] == 5, t, s);`,
		"a range loop": `
			var IL a := {1, 2, 3};
			var integer sum := 0;
			for (var integer e in a) { a[2] := 10; sum := sum + e }
			check(sum == 6 and a[2] == 10, a, sum);`,
		"sent, then changed": `
			var S s := { x := 1, xs := {1} };
			p.send(s);
			s.x := 2; s.xs[0] := 2;
			var S r;
			alt { [] p.receive(S:?) -> value r {} [] g.timeout { setverdict(fail, "no message"); stop } }
			r.xs[0] := 3;
			check(r.x == 1 and s.xs[0] == 2 and r.xs[0] == 3, s, r);`,
		"received into a field": `
			var S s := { x := 1, xs := {1} };
			p.send(s);
			var R r := { n := 0, s := s };
			var R r2 := r;
			alt { [] p.receive(S:?) -> value r.s {} [] g.timeout { setverdict(fail, "no message"); stop } }
			r.s.x := 4;
			check(r2.s.x == 1 and s.x == 1 and r.s.x == 4, r, r2, s);`,
		"started with": `
			var S s := { x := 1, xs := {1} };
			var C c := C.create;
			connect(self:p, c:p);
			c.start(echoChanged(s));
			s.x := 5;
			var S r;
			alt { [] p.receive(S:?) -> value r {} [] g.timeout { setverdict(fail, "no message"); stop } }
			check(r.x == 2 and s.x == 5, r, s);`,
	} {
		src := `module M {
			type record of integer IL;
			type record of IL ILL;
			type record S { integer x, IL xs }
			type record of S SL;
			type record R { integer n, S s }
			type union U { integer i, charstring c }
			type map from charstring to integer IM;
			type port P message { inout S }
			type component C { port P p; var S cv; timer g := 2.0 }
			const S c_s := { x := 1, xs := {1, 2} };
			function check(boolean ok, template anytype a := omit, template anytype b := omit, template anytype c := omit) runs on C {
				if (ok) { setverdict(pass) } else { setverdict(fail, a, b, c) }
			}
			function bump(inout R r) { r.s.x := r.s.x + 1 }
			function writeIn(R r) return integer { r.s.x := 2; return r.s.x }
			function get() runs on C return S { return cv }
			function echoChanged(S s) runs on C { s.x := 2; p.send(s) }
			testcase tc() runs on C system C {
				connect(self:p, self:p);
				g.start;` + body + `
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

// TestNestedClassWritesTheOuterObject: a method of a nested class writes
// into the enclosing object's field, not into a copy of it, though the
// field's value is shared with a variable.
func TestNestedClassWritesTheOuterObject(t *testing.T) {
	src := `module M language "TTCN-3:2018 Object-Oriented" {
		type record R { integer a, integer b }
		type component C {}
		type class Outer {
			var R r;
			function getA() return integer { return r.a }
			function keep() { var R x := r }
			type class Inner {
				public function bump() { r.a := 42 }
			}
		}
		testcase tc() runs on C {
			var Outer o := Outer.create({a := 1, b := 2});
			var Outer.Inner i := o.Inner.create();
			o.keep();
			i.bump();
			if (o.getA() == 42) { setverdict(pass) } else { setverdict(fail, o.getA()) }
		}
	}`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}

// TestPositionalRecordsHaveTheirFields: a record written positionally,
// `{1, 2}`, has its type's field names wherever it is bound — an element
// of a record of, of an array or of a map, a field, a union's
// alternative, a component variable, a parameter, an out parameter, a
// value redirect — so its fields read and write.
func TestPositionalRecordsHaveTheirFields(t *testing.T) {
	for name, body := range map[string]string{
		"record of":      `var RR rr := {{1,2},{3,4}}; rr[0].a := 5; check(rr[0].a == 5 and rr[1].b == 4, rr)`,
		"constant":       `var RR rr := c_rr; rr[0].a := 5; check(rr[0].a == 5 and c_rr[0].a == 1, rr)`,
		"assigned":       `var RR rr; rr := {{1,2}}; rr[0].a := 5; check(rr[0].a == 5, rr)`,
		"element":        `var RR rr := {}; rr[0] := {1,2}; rr[0].a := 5; check(rr[0].a == 5, rr)`,
		"array":          `var RA a := {{1,2},{3,4}}; a[1].a := 5; check(a[1].a == 5, a)`,
		"declared array": `var R a[2] := {{1,2},{3,4}}; a[1].a := 5; check(a[1].a == 5, a)`,
		"field":          `var W w := { l := {{1,2}} }; w.l[0].a := 5; check(w.l[0].a == 5, w)`,
		"map":            `var MR m := {["x"] := {1,2}}; m["x"].a := 5; m := {["z"] := {7,8}}; check(m["x"].a == 5 and m["z"].a == 7, m)`,
		"union":          `var U u := {r := {1,2}}; u.r.a := 3; check(u.r.a == 3 and u.r.b == 2, u)`,
		"component":      `cv.a := 7; cl[0].a := 4; check(cv.a == 7 and cl[0].a == 4, cv)`,
		"parameter":      `check(fin({1, 2}) == 7, "in")`,
		"inout":          `var RR rr := {{1,2},{3,4}}; fio(rr[1]); check(rr[1].a == 9, rr)`,
		"out":            `var R r := {1, 2}; fout(r); check(r.a == 6, r)`,
		"declarator array assigned": `var R d[2]; d := {{1,2},{3,4}}; var RA b := d; d[0] := {7,8};
			check(lengthof(b) == 2 and b[1].b == 4 and d[0].a == 7 and d[1].b == 4, d)`,
		"two dimensions": `var R22 t := {{{1,2},{3,4}},{{5,6},{7,8}}}; t[1][1].a := 70; check(t[1][1].a == 70 and t[0][1].b == 4, t)`,
		"array field":    `var A w := { one := {{1,2},{3,4}} }; w.one[1].a := 5; check(w.one[1].a == 5, w)`,
		"union assigned": `var U u; u := {r := {1,2}}; check(u.r.a == 1, u)`,
		"index notation": `var RR rr := {{1,2}}; rr := {[1] := {9,9}}; check(rr[1].a == 9 and rr[0].a == 1, rr)`,
		"redirect": `connect(self:p, self:p); var W w := { l := {} }; p.send(RR:{{1,2}});
			alt { [] p.receive(RR:?) -> value w.l {} [] g.timeout { setverdict(fail, "no message"); stop } }
			w.l[0].a := 5; check(w.l[0].a == 5, w)`,
	} {
		src := `module M {
			type record R { integer a, integer b optional }
			type record of R RR;
			type R RA[2];
			type map from charstring to R MR;
			type record W { RR l }
			type R R22[2][2];
			type record A { R one[2] }
			type union U { integer i, R r }
			type port P message { inout RR }
			type component C { port P p; var R cv := {1, 2}; var RR cl := {{1,2}}; timer g := 2.0 }
			const RR c_rr := {{1,2},{3,4}};
			function check(boolean ok, template anytype x) { if (ok) { setverdict(pass) } else { setverdict(fail, x) } }
			function fio(inout R x) { x.a := 9 }
			function fout(out R x) { x := {5, omit}; x.a := 6 }
			function fin(R x) return integer { x.a := 7; return x.a }
			testcase tc() runs on C system C {
				g.start;
				` + body + `
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

// TestShapedValuesAreNotWalkedAgain: a record of records assigned again
// and again is given its field names once, not on every assignment —
// which walked all its elements each time.
func TestShapedValuesAreNotWalkedAgain(t *testing.T) {
	src := `module M {
		type record R { integer a, integer b }
		type record of R RR;
		type record W { record of record { integer a, integer b } l }
		type component C {}
		function g(RR x) return RR { return x }
		testcase tc() runs on C {
			var RR rr := {};
			for (var integer i := 0; i < 10000; i := i + 1) { rr[i] := {i, i} }
			var RR r2;
			for (var integer i := 0; i < 2000; i := i + 1) { r2 := rr; r2 := g(rr); var RR r3 := rr }
			var W w := { l := {} };
			for (var integer i := 0; i < 10000; i := i + 1) { w.l[i] := {i, i} }
			var W w2;
			for (var integer i := 0; i < 2000; i := i + 1) { w2 := w }
			if (r2[9999].a == 9999 and w2.l[9999].b == 9999) { setverdict(pass) } else { setverdict(fail, r2[9999]) }
		}
	}`
	start := time.Now()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", clocks[0].opts)
	if err != nil || v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), err %v, want pass", v, reason, err)
	}
	limit := 5 * time.Second
	if raceEnabled {
		limit = 30 * time.Second
	}
	if d := time.Since(start); d > limit {
		t.Fatalf("took %v", d)
	}
}

// TestShapeOfTheNameInScope: a value takes the shape of the declaration
// its name stands for there — an inner block's variable, a parameter —
// not of a like-named outer array or component variable it hides.
func TestShapeOfTheNameInScope(t *testing.T) {
	for name, body := range map[string]string{
		"inner block": `var R a[2] := {{1,2},{3,4}};
			{ var S a := {y := 9, x := 10}; a := {11, 12}; check(a.y == 11 and a.x == 12, a) }
			a := {{5,6},{7,8}}; check(a[1].y == 8, a)`,
		"inner record of": `var R a[2] := {{1,2},{3,4}};
			{ var SL a := {}; a[0] := {1, 2}; check(a[0].y == 1, a) }`,
		"loop body": `var R a[2] := {{1,2},{3,4}};
			for (var integer i := 0; i < 1; i := i + 1) { var SL a := {{5,6}}; a[1] := {1, 2}; check(a[1].y == 1, a) }`,
		"parameter":      `check(g({1, 2}) == 15, "g")`,
		"parameter list": `check(h({}) == 1, "h")`,
	} {
		src := `module M {
			type record R { integer x, integer y }
			type record S { integer y, integer x }
			type record of S SL;
			type component C { var R ca[2] }
			function check(boolean ok, template anytype x) { if (ok) { setverdict(pass) } else { setverdict(fail, x) } }
			function g(S ca) runs on C return integer { ca := {15, 16}; return ca.y }
			function h(SL ca) runs on C return integer { ca[0] := {1, 2}; return ca[0].y }
			testcase tc() runs on C {
				` + body + `
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

// TestPositionalRecordsCompareByName: a record written positionally is
// the record of its fields, by name: it equals and matches the same
// record written with them, in either order, in a record of too, and a
// value literal compared with a variable is of the variable's type.
func TestPositionalRecordsCompareByName(t *testing.T) {
	assertSameVerdictBothClocks(t, `module M {
		type record R { integer a, integer b optional }
		type record of R RR;
		type set S { integer a, integer b }
		type record of S SS;
		type component C {}
		template R t := {a := 1, b := 2};
		function chk(boolean ok, charstring what) { if (not ok) { setverdict(fail, what) } }
		testcase tc() runs on C {
			var RR p := {{1,2},{3,omit}};
			var RR n := {{a := 1, b := 2},{a := 3, b := omit}};
			chk(p == n, "pos==named");
			chk(n == {{1,2},{3,omit}}, "named==lit");
			chk(p == {{a := 1, b := 2},{a := 3, b := omit}}, "pos==namedlit");
			var SS s1 := {{1,2}};
			var SS s2 := {{b := 2, a := 1}};
			chk(s1 == s2, "set");
			var RR d := {{b := 2, a := 1}, {a := 3, b := omit}};
			chk(d == p, "reordered");
			chk(match(p[0], t), "match");
			chk(not match(p[1], t), "no match");
			chk(p != {{1,3},{3,omit}}, "differs");
			setverdict(pass);
		}
	}`, runtime.PassVerdict)
}

// TestCompatibleAndImplicitOmitRecordsCompare: values of compatible
// record types, whose fields have other names, are equal when their
// fields are, in order (ETSI 6.3.2.2, 7.1.3); a field a value with
// implicit omit leaves out equals omit; one union alternative is not
// another of the same value.
func TestCompatibleAndImplicitOmitRecordsCompare(t *testing.T) {
	assertSameVerdictBothClocks(t, `module M {
		type record R { integer a, integer b }
		type record T { integer x, integer y }
		type record O { integer a, integer b optional, integer c optional } with { optional "implicit omit" }
		type union U { integer i, integer j }
		type component C {}
		function chk(boolean ok, charstring what) { if (not ok) { setverdict(fail, what) } }
		testcase tc() runs on C {
			var R r := {1, 2};
			var T t := {x := 1, y := 2};
			var R rn := {a := 1, b := 2};
			var T tp := {1, 2};
			chk(r == t and t == r and rn == tp and rn == t, "compatible");
			chk(r != {x := 2, y := 1}, "compatible differs");
			var O y := {a := 1};
			var O w := {1, omit, omit};
			chk(y == w and w == y and y == {1, omit, omit}, "implicit omit");
			var U u1 := {i := 1};
			var U u2 := {j := 1};
			chk(u1 != u2, "union alternatives");
			setverdict(pass);
		}
	}`, runtime.PassVerdict)
}

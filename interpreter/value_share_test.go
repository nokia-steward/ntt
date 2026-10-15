package interpreter_test

import (
	"testing"

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

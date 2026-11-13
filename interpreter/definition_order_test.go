package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// TestDefinitionsMayComeInAnyOrder: a module definition is evaluated
// before the first reference to it, whatever the order of definitions
// and modules (ETSI ES 201 873-1 8.2.0, 5.3): a constant, module
// parameter or template naming one declared after it, in its module or
// in one it imports from, whichever module comes first in the run, and
// modules that import from each other; and one named like an enumerated
// value it names.
func TestDefinitionsMayComeInAnyOrder(t *testing.T) {
	a := `module A {
		import from B all;
		const integer a1 := b1 + 1;
		const integer a2 := a3 * 2;
		const integer a3 := g();
		modulepar integer mpa := a2 + 1;
		template integer ta := (a1 .. b2);
		function g() return integer { return a4 }
		const integer a4 := 5;
		const integer a5 := mpg + 1;
		modulepar { integer mpg := 3 }
		const Rec green := { c := green };
		const integer a6 := enum2int(green.c);
	}`
	b := `module B {
		import from A all;
		type enumerated Color { red, green }
		type record Rec { Color c }
		const integer b1 := 10;
		const integer b2 := a4 + B.b1 + 100;
	}`
	// What a module names without importing it is bound once that module
	// is evaluated: an early evaluation that does not find it is not the
	// definition's last.
	u := `module U { const integer u1 := 7; }`
	v := `module V {
		import from M all;
		const integer v1 := m1;
	}`
	main := `module M {
		import from A all;
		import from B all;
		type component C {}
		const integer m1 := u1 * 2;
		testcase tc() runs on C {
			if (m1 == 14 and a1 == 11 and a2 == 10 and a3 == 5 and mpa == 11 and b2 == 115 and a5 == 4 and a6 == 1 and match(50, ta)) {
				setverdict(pass)
			} else {
				setverdict(fail)
			}
		}
	}`
	for name, order := range map[string][]string{
		"importer first": {a, b, v, main, u},
		"imported first": {b, a, u, main, v},
		"main first":     {main, a, b, v, u},
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

// TestDefinitionsNamingThemselvesFail: a constant that names itself,
// through another or a function, is an error, not a value — not another
// module's like-named one either, nor an enumerated value of a type it
// does not import.
func TestDefinitionsNamingThemselvesFail(t *testing.T) {
	other := `module S { const integer x := 100; }`
	enum := `module E { type enumerated En { x, y } }`
	for name, src := range map[string]string{
		"through another": `module M {
			type component C {}
			const integer x := y + 1;
			const integer y := x + 1;
			testcase tc() runs on C { setverdict(pass) }
		}`,
		"through a function": `module M {
			type component C {}
			const integer x := f();
			function f() return integer { return x + 1 }
			testcase tc() runs on C { setverdict(pass) }
		}`,
	} {
		v, _, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, other), parse(t, enum), parse(t, src)}, "M.tc", clocks[0].opts)
		if err != nil || v != runtime.ErrorVerdict {
			t.Errorf("%s: verdict %s, err %v; want error", name, v, err)
		}
	}
}

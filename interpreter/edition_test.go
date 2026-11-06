package interpreter_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// v4v5 runs a testcase once in a module of each edition: in V4 its body
// must pass with v4, in V5 with v5.
func v4v5(t *testing.T, decls, v4, v5 string) {
	t.Helper()
	src := func(lang, cond string) string {
		return strings.Join([]string{"module M", lang, `{
			type component C {}
			`, decls, `
			testcase tc() runs on C {
				if (`, cond, `) { setverdict(pass); } else { setverdict(fail); }
			}
		}`}, " ")
	}
	t.Run("V4", func(t *testing.T) { runPass(t, "M.tc", src("", v4)) })
	t.Run("V5", func(t *testing.T) { runPass(t, "M.tc", src(`language "TTCN-3:5"`, v5)) })
}

// The size of a V5 list is the number of its elements, uninitialised ones
// included (the draft's annex B); V4's deprecated sizeof is its length.
func TestEdition_Sizeof(t *testing.T) {
	v4v5(t, `type record of integer RoI; const RoI c_v := { 1, -, 3, - };`,
		`sizeof(c_v) == 3 and lengthof(c_v) == 3`,
		`sizeof(c_v) == 4 and lengthof(c_v) == 3`)
}

// A V5 default reference starts null, a verdict none (the draft's clause
// 11); in V4 both are uninitialised.
func TestEdition_InitialValues(t *testing.T) {
	v4v5(t, `function f_d() return boolean { var default d; return isbound(d); }
		function f_v() return boolean { var verdicttype v; return isbound(v); }
		function f_null() return boolean { var default d; var verdicttype v; return d == null and v == none; }`,
		`not f_d() and not f_v()`,
		`f_null()`)
}

// Enumerators of V5 take integer expressions, constants included, and
// ranges open to infinity; one without integers gets the lowest unused.
func TestEdition_EnumeratorIntegers(t *testing.T) {
	runPass(t, "M.tc", `module M language "TTCN-3:5" {
		type component C {}
		const integer K := 5;
		type enumerated E { A, B(1..9, 10+1), D, F(-10..0) }
		type enumerated E2 { P(K), Q(K+1..K+3), R, S(100..infinity) }
		testcase tc() runs on C {
			if (enum2int(A) == 10 and enum2int(D) == 12 and enum2int(F(-3)) == -3 and
			    enum2int(P) == 5 and enum2int(Q(7)) == 7 and enum2int(R) == 0 and enum2int(S(1000)) == 1000 and
			    match(B(4), B(?)) and not match(B(11), B(1..9))) { setverdict(pass); }
			else { setverdict(fail); }
		}
	}`)
}

// Implicit omit sets the optional fields a template gives no value to omit,
// at any depth (ETSI 27.7). A modified template keeps what it has of its
// base; in V4 that includes the fields its modification does not mention
// (15.5 b), and the attribute changes nothing of it; in V5 those are set
// to omit, but for one it gives `-`.
func TestImplicitOmit_Templates(t *testing.T) {
	v4v5(t, `type record S { integer a optional, boolean c }
		type record R { integer i, integer j optional, S s optional }
		template R t0 := { i := 1, s := { c := true } } with { optional "implicit omit" }
		template R t1 := { i := 1, j := 2, s := { a := 3, c := true } };
		template R t2 modifies t1 := { i := 2 } with { optional "implicit omit" }
		template R t3 modifies t1 := { i := 2, j := - } with { optional "implicit omit" }`,
		`match({ 1, omit, { omit, true } }, t0) and match({ 2, 2, { 3, true } }, t2)`,
		`match({ 1, omit, { omit, true } }, t0) and match({ 2, omit, omit }, t2) and match({ 2, 2, omit }, t3)`)
}

// A component's variables take V5's initial values as a behaviour's do.
func TestEdition_ComponentVariablesInitialValues(t *testing.T) {
	runPass(t, "M.tc", `module M language "TTCN-3:5" {
		type component C { var verdicttype cv; var default cd; }
		testcase tc() runs on C {
			if (cv == none and cd == null) { setverdict(pass); } else { setverdict(fail); }
		}
	}`)
}

// `Label(n)` naming no integer of the label is an error, not an
// uninitialised value (ETSI 6.2.4).
func TestEnumInitialiserNamingNoIntegerOfItsLabel(t *testing.T) {
	v, reason := runStrict(t, "M.tc", `module M {
		type component C {}
		type enumerated E { A, B(1..10) }
		testcase tc() runs on C {
			var E e := A(5);
			setverdict(pass);
		}
	}`)
	if v != "error" {
		t.Fatalf("verdict = %s (%s), want error", v, reason)
	}
}

// A V5 loop whose iteration variable is declared outside it iterates a
// range of initialised elements (Berlin, 2026); V4 iterates an
// uninitialised one too, and the variable is unbound in that iteration.
// A variable the header declares takes one in either edition.
func TestEdition_RangeLoopIntoOutsideVariable(t *testing.T) {
	decls := `type record of charstring RoC;
		function f_out(RoC r) return integer {
			var integer n := 0; var charstring e;
			for (e in r) { if (isbound(e)) { n := n + 1 } }
			return n;
		}
		function f_header(RoC r) return integer {
			var integer n := 0;
			for (var charstring e in r) { if (isbound(e)) { n := n + 1 } }
			return n;
		}`
	v4v5(t, decls,
		`f_out({"a", -, "c"}) == 2 and f_header({"a", -, "c"}) == 2`,
		`f_out({"a", "b", "c"}) == 3 and f_header({"a", -, "c"}) == 2`)

	loop := func(lang string) string {
		return `module M ` + lang + ` {
			type component C {}
			type record of charstring RoC;
			testcase tc() runs on C {
				var RoC r := {"a", -, "c"}; var charstring e;
				for (e in r) {}
				setverdict(pass);
			}
		}`
	}
	runPass(t, "M.tc", loop(""))
	v, reason := runStrict(t, "M.tc", loop(`language "TTCN-3:5"`))
	if v != "error" || !strings.Contains(reason, "declared outside it is uninitialised") {
		t.Fatalf("verdict = %s (%s), want error for the uninitialised element", v, reason)
	}
}

// The rule is the edition of the module the loop is written in, not the
// testcase's: a V5 function's loop fails called from a V4 testcase, and a
// V4 function's loop runs called from a V5 one.
func TestEdition_RangeLoopFollowsItsModule(t *testing.T) {
	lib := func(lang string) string {
		return `module L ` + lang + ` {
			type record of charstring RoC;
			function f(RoC r) return integer {
				var integer n := 0; var charstring e;
				for (e in r) { n := n + 1 }
				return n;
			}
		}`
	}
	main := func(lang string) string {
		return `module M ` + lang + ` {
			import from L all;
			type component C {}
			testcase tc() runs on C {
				if (f({"a", -, "c"}) == 3) { setverdict(pass) } else { setverdict(fail) }
			}
		}`
	}
	run := func(libLang, mainLang string) (runtime.Verdict, string) {
		v, reason, err := interpreter.RunTestcaseWith(
			[]*ttcn3.Tree{parse(t, lib(libLang)), parse(t, main(mainLang))}, "M.tc", interpreter.TestcaseOptions{})
		if err != nil {
			t.Fatalf("RunTestcaseWith: %v", err)
		}
		return v, reason
	}
	if v, reason := run(`language "TTCN-3:5"`, ""); v != "error" || !strings.Contains(reason, "declared outside it is uninitialised") {
		t.Errorf("V5 function, V4 testcase: verdict = %s (%s), want error", v, reason)
	}
	if v, reason := run("", `language "TTCN-3:5"`); v != runtime.PassVerdict {
		t.Errorf("V4 function, V5 testcase: verdict = %s (%s), want pass", v, reason)
	}
}

// A V5 enumerator of a negated constant has the negated integer, however
// often the type's enumerators are evaluated.
func TestEdition_EnumeratorOfNegatedConstant(t *testing.T) {
	runPass(t, "M.tc", `module M language "TTCN-3:5" {
		type component C {}
		const integer K := 2;
		type enumerated E { a(K), b, c(-K) }
		testcase tc() runs on C {
			if (enum2int(a) == 2 and enum2int(c) == -2 and K == 2) { setverdict(pass); }
			else { setverdict(fail, enum2int(a), enum2int(c), K); }
		}
	}`)
}

// Implicit omit does not reach into a template a field takes whose own
// optional attribute says otherwise, unless given with override; one
// without an attribute of its own it does reach (ETSI 27.7, the examples
// mw_myTemplate22, 24 and 25). An outer override wins over an inner
// attribute (27.1.2).
func TestImplicitOmit_LowerScopeAndOverride(t *testing.T) {
	v, reason := runStrict(t, "M.tc", `module M {
		type component C {}
		type record MyRecord1 { integer a, boolean b optional }
		type record MyRecord2 { MyRecord1 m }
		template MyRecord1 t1a := { a := ? }
		template MyRecord1 t1b := { a := ? } with { optional "explicit omit" }
		template MyRecord2 t22 := { m := t1a } with { optional "implicit omit" }
		template MyRecord2 t24 := { m := t1b } with { optional "implicit omit" }
		template MyRecord2 t25 := { m := t1b } with { optional override "implicit omit" }
		group g {
			template MyRecord1 tg := { a := 1 } with { optional "implicit omit" }
		} with { optional override "explicit omit" }
		testcase tc() runs on C {
			setverdict(pass, t22, t24, t25, tg);
		}
	}`)
	want := `{m := {a := ?, b := omit}} {m := {a := ?}} {m := {a := ?, b := omit}} {a := 1}`
	if v != runtime.PassVerdict || reason != want {
		t.Fatalf("verdict = %s (%s), want pass (%s)", v, reason, want)
	}
}

// A field's template is the module's own when it defines one of that
// name, whatever a module it imports defines: an imported one with an
// optional attribute of its own does not keep the field from implicit
// omit.
func TestImplicitOmit_OwnTemplateBeforeImported(t *testing.T) {
	lib := `module L {
		type record MyRecord1 { integer a, boolean b optional }
		template MyRecord1 t_x := { a := 2 } with { optional "explicit omit" }
	}`
	main := `module M {
		import from L all;
		type component C {}
		type record MyRecord2 { MyRecord1 m }
		template MyRecord1 t_x := { a := 1 }
		template MyRecord2 t := { m := t_x } with { optional "implicit omit" }
		template MyRecord2 tq := { m := L.t_x } with { optional "implicit omit" }
		testcase tc() runs on C { setverdict(pass, t, tq); }
	}`
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, lib), parse(t, main)}, "M.tc", interpreter.TestcaseOptions{})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if want := `{m := {a := 1, b := omit}} {m := {a := 2}}`; v != runtime.PassVerdict || reason != want {
		t.Fatalf("verdict = %s (%s), want pass (%s)", v, reason, want)
	}
}

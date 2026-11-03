package interpreter_test

import (
	"strings"
	"testing"
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
	t.Run("V5", func(t *testing.T) { runPass(t, "M.tc", src(`language "TTCN-3:2026"`, v5)) })
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
	runPass(t, "M.tc", `module M language "TTCN-3:2026" {
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
// (15.5 b), in V5 those are set to omit, but for one it gives `-`.
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
	runPass(t, "M.tc", `module M language "TTCN-3:2026" {
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

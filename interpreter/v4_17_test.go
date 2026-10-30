package interpreter_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// runBodiesPass runs M.tc of each source on both clocks and wants pass.
func runBodiesPass(t *testing.T, cases map[string]string) {
	t.Helper()
	for name, body := range cases {
		src := `module M {
			type component C {}
			type record R { integer a, boolean b optional }
			type record of integer RoI;
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

// TestValueExtractionAndPresentness: t.value is the string of a pattern
// or the value of a template holding one (ETSI ES 201 873-1 15.15), and
// t.present turns AnyValueOrNone into AnyValue and drops ifpresent
// (15.14).
func TestValueExtractionAndPresentness(t *testing.T) {
	runBodiesPass(t, map[string]string{
		"pattern": `var template charstring v := pattern "ab\?#(1,1)";
			if (v.value != "ab\?#(1,1)") { setverdict(fail, v.value) }`,
		"ifpresent": `var template charstring v := "abc" ifpresent;
			if (v.value != "abc") { setverdict(fail, v.value) }`,
		"value": `var template integer v := 5;
			if (v.value != 5) { setverdict(fail, v.value) }`,
		"AnyValueOrNone": `template R t := { 1, * };
			var template(present) boolean v := t.b.present;
			if (not istemplatekind(v, "?")) { setverdict(fail, v) }`,
		"ifpresent dropped": `var template integer v := 1 ifpresent;
			if (istemplatekind(v.present, "ifpresent")) { setverdict(fail, v.present) }`,
	})
	for name, body := range map[string]string{
		"AnyValue has no value": `var template charstring v := ?; var charstring s := v.value;`,
		"omit has no presence":  `template boolean t := omit; var template(present) boolean v := t.present;`,
	} {
		src := `module M { type component C {} testcase tc() runs on C { ` + body + ` setverdict(pass); } }`
		for _, k := range clocks {
			v, reason, _ := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
			if v != runtime.ErrorVerdict {
				t.Errorf("%s, %s clock: %s (%s), want error", name, k.name, v, reason)
			}
		}
	}
}

// TestSubstrOfABinaryStringTemplate: each matching mechanism inside a
// binary string template's value is one item for substr (ETSI ES 201
// 873-1 15.6.1, C.4.2).
func TestSubstrOfABinaryStringTemplate(t *testing.T) {
	runBodiesPass(t, map[string]string{
		"bits": `var template bitstring t := '00101*'B;
			if (valueof(substr(t, 1, 2)) != '01'B) { setverdict(fail, substr(t, 1, 2)) }
			if (not match('1'B, substr(t, 5, 1))) { setverdict(fail, "* lost") }`,
		"octets": `var template octetstring t := 'AB?CD'O;
			if (valueof(substr(t, 2, 1)) != 'CD'O) { setverdict(fail, substr(t, 2, 1)) }
			if (not match('77'O, substr(t, 1, 1))) { setverdict(fail, "? lost") }`,
		"hex": `var template hexstring t := 'A*B'H;
			if (valueof(substr(t, 2, 1)) != 'B'H) { setverdict(fail, substr(t, 2, 1)) }`,
	})
}

// TestEncvalueAndEncvalueO: encvalue_o of a value is what encvalue gives,
// left-aligned into octets, the low bits of the last one zero (ETSI ES
// 201 873-1 C.5.5).
func TestEncvalueAndEncvalueO(t *testing.T) {
	runBodiesPass(t, map[string]string{
		"bitstring": `var bitstring v := '011'B;
			var bitstring b := encvalue(v);
			while (lengthof(b) mod 8 > 0) { b[lengthof(b)] := '0'B }
			if (encvalue_o(v) != bit2oct(b)) { setverdict(fail, encvalue_o(v), b) }`,
		"record": `var R v := { 7, omit };
			if (encvalue_o(v) != bit2oct(encvalue(v))) { setverdict(fail, encvalue_o(v), encvalue(v)) }`,
	})
}

// TestIsTemplateKind: istemplatekind tells the kind of matching mechanism
// a template is, holds inside its value on the first level, or has
// attached as an attribute (ETSI ES 201 873-1 C.3.5).
func TestIsTemplateKind(t *testing.T) {
	cases := map[string]bool{
		`1, "value"`: true, `?, "value"`: false, `(1, 2), "list"`: true, `(1, 2), "value"`: false,
		`complement(1), "complement"`: true, `?, "AnyValue"`: true, `?, "?"`: true, `*, "AnyValue"`: false,
		`*, "AnyValueOrNone"`: true, `*, "*"`: true, `(1 .. 3), "range"`: true, `omit, "omit"`: true,
		`? length(2), "length"`: true, `? length(2), "AnyValue"`: true, `(0, 1) ifpresent, "ifpresent"`: true,
		`(0, 1) ifpresent, "list"`: true, `(0, 1) ifpresent, "complement"`: false,
		`pattern "a?b", "pattern"`: true, `"a?b", "pattern"`: false,
		`RoI:{ permutation(1, 2), ? }, "permutation"`: true, `RoI:{ permutation(1, 2), ? }, "AnyElement"`: true,
		`RoI:{ 1, * }, "AnyElementsOrNone"`: true, `RoI:{ 1, 2 }, "AnyElement"`: false,
		`'1?0'B, "AnyElement"`: true, `'1*0'B, "AnyElementsOrNone"`: true, `'10'B, "AnyElement"`: false,
		`RoI:superset(1), "superset"`: true, `RoI:subset(1), "subset"`: true,
	}
	body := ""
	for args, want := range cases {
		op := "not "
		if !want {
			op = ""
		}
		body += `if (` + op + `istemplatekind(` + args + `)) { setverdict(fail, "istemplatekind(` + strings.ReplaceAll(args, `"`, `'`) + `)") }
			`
	}
	runBodiesPass(t, map[string]string{"kinds": body})
}

// TestModifiedAnyValueAndMapElementsOfAnyValue: a base AnyValue is
// expanded into its fields - AnyValue where mandatory, AnyValueOrNone
// where optional - before a modification (ETSI ES 201 873-1 15.5 c,
// 15.6.2), and an element of a map template holding AnyValue is
// AnyValueOrNone (15.6.6 c).
func TestModifiedAnyValueAndMapElementsOfAnyValue(t *testing.T) {
	src := `module M {
		type component C {}
		type record R { integer f1, charstring f2, boolean f3 optional }
		type map from charstring to R TMap;
		type record T { integer a, TMap b optional }
		template R t_mod modifies ? := { f2 := "x" };
		template R t_par(charstring p) modifies ? := { f2 := p };
		testcase tc() runs on C {
			template R t_local modifies ? := { f2 := "x" };
			var template T v := { a := 1, b := ? };
			var template TMap m := ?;
			if (not istemplatekind(t_mod.f1, "?") or not istemplatekind(t_mod.f3, "*") or valueof(t_mod.f2) != "x") { setverdict(fail, t_mod) }
			if (not istemplatekind(t_par("y").f1, "?") or valueof(t_par("y").f2) != "y") { setverdict(fail, t_par("y")) }
			if (not istemplatekind(t_local.f3, "*")) { setverdict(fail, t_local) }
			if (not istemplatekind(v.b["k"], "*")) { setverdict(fail, "field element") }
			if (not istemplatekind(m["k"], "*")) { setverdict(fail, "element") }
			setverdict(pass);
		}
	}`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}

// TestValueAndPresentnessEdgeCases: an empty binary string is a value;
// the presentness conversion of a template list drops its omit
// alternatives and converts the others, and converts under an ifpresent.
func TestValueAndPresentnessEdgeCases(t *testing.T) {
	runBodiesPass(t, map[string]string{
		"empty": `var template bitstring tb := ''B; var bitstring vb := tb.value;
			if (vb != ''B) { setverdict(fail, vb) }`,
		"list": `template integer t := (*, 5);
			if (not istemplatekind(t.present, "list") or match(omit, t.present) == true) { setverdict(fail, t.present) }
			var template integer u := t.present;
			if (not match(7, u) or not match(5, u)) { setverdict(fail, u) }`,
		"omit alternative": `template integer t := (1, omit);
			if (not istemplatekind(t.present, "value") or valueof(t.present) != 1) { setverdict(fail, t.present) }`,
		"ifpresent": `var template integer t := * ifpresent;
			if (not istemplatekind(t.present, "AnyValue")) { setverdict(fail, t.present) }`,
	})
}

// TestTemplateListWithOmitAndBehaviourTypes: a template list holding omit
// matches an omitted field (ETSI ES 201 873-1 B.1.2.1); refers(f) is a
// value of a behaviour type and v.apply(...) calls it (ES 202 785).
func TestTemplateListWithOmitAndBehaviourTypes(t *testing.T) {
	src := `module M {
		type component C { var integer c_n := 10 }
		type record R { integer a, integer b optional }
		type function FT(integer p) runs on C mtc C return integer;
		function f(integer p) runs on C mtc C return integer { return p + c_n }
		testcase tc() runs on C {
			var R v := { a := 1, b := omit };
			if (not match(v, R:{ a := 1, b := (1, omit) })) { setverdict(fail, "omit alternative") }
			if (match(v, R:{ a := 1, b := (1, 2) })) { setverdict(fail, "no omit alternative") }
			if (not match(R:{ a := 1, b := 2 }, R:{ a := 1, b := (2, omit) })) { setverdict(fail, "value alternative") }
			var FT g := refers(f);
			if (g.apply(5) != 15) { setverdict(fail, "apply") }
			setverdict(pass);
		}
	}`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}

// TestAltstepThroughABehaviourReference: an altstep a behaviour-type value
// refers to is activated, and is an alt branch, as the altstep named
// would be (ETSI ES 202 785); apply of an unbound one is an error.
func TestAltstepThroughABehaviourReference(t *testing.T) {
	src := `module M {
		type port P message { inout integer }
		type component C { port P p; timer t; var integer cnt := 0; }
		type altstep AS() runs on C;
		type function FT() return integer;
		altstep as_t() runs on C { [] t.timeout { cnt := cnt + 10; } }
		testcase tc_activate() runs on C {
			var AS a := refers(as_t);
			t.start(0.01);
			var default d := activate(a.apply());
			alt { [] p.receive { } }
			if (cnt == 10) { setverdict(pass) } else { setverdict(fail, cnt) }
		}
		testcase tc_branch() runs on C {
			var AS a := refers(as_t);
			t.start(0.01);
			alt { [] a.apply() { } }
			if (cnt == 10) { setverdict(pass) } else { setverdict(fail, cnt) }
		}
		testcase tc_unbound() runs on C {
			var FT f;
			var integer i := f.apply();
			setverdict(pass);
		}
	}`
	for tc, want := range map[string]runtime.Verdict{"M.tc_activate": runtime.PassVerdict, "M.tc_branch": runtime.PassVerdict, "M.tc_unbound": runtime.ErrorVerdict} {
		for _, k := range clocks {
			v, reason, _ := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, tc, k.opts)
			if v != want {
				t.Errorf("%s, %s clock: %s (%s), want %s", tc, k.name, v, reason, want)
			}
		}
	}
}

// TestLengthRestrictsOnlyAPresentValue: `* length(1)` matches an omitted
// field, and a length-restricted template list matches it if one of its
// alternatives does (ETSI ES 201 873-1 B.1.4.1, B.1.2.1).
func TestLengthRestrictsOnlyAPresentValue(t *testing.T) {
	runBodiesPass(t, map[string]string{
		"length": `var R v := { a := 1, b := omit };
			if (not match(v, R:{ a := 1, b := * length(1) })) { setverdict(fail, "* length") }
			if (match(v, R:{ a := 1, b := ? length(1) })) { setverdict(fail, "? length") }`,
	})
}

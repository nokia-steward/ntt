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

// TestReferencingStringTemplateItems: an element of a pattern is one of
// its items, of a binary string template one of its symbols; assigning
// one to a template turns it into a pattern, past the end after `?`s, a
// pattern of several items as one group (ETSI ES 201 873-1 15.6.1).
func TestReferencingStringTemplateItems(t *testing.T) {
	runBodiesPass(t, map[string]string{
		"read": `var template charstring p := pattern "a\d[x-z]#(2)(bc)+{p}?";
			if (p[1].value != "\d" or p[2].value != "[x-z]#(2)" or p[3].value != "(bc)+" or p[5].value != "?") { setverdict(fail, p[1], p[2], p[3], p[5]) }
			var template octetstring o := '01??AB*'O;
			if (not match('EF'O, o[2]) or valueof(o[3]) != 'AB'O) { setverdict(fail, o[2], o[3]) }`,
		"write": `var template charstring p := pattern "abc?[a-z]";
			p[6] := pattern "x+";
			if (p.value != "abc?[a-z]?x+") { setverdict(fail, p) }
			var template charstring v := "a?c";
			v[1] := pattern "[0-9][0-9]";
			if (v.value != "a([0-9][0-9])c" or match("a?c", v) or not match("a12c", v)) { setverdict(fail, v) }
			var charstring w := "abc";
			w[1] := "X";
			if (w != "aXc") { setverdict(fail, w) }`,
	})
}

// TestReferencingRecordOfTemplateElements: the examples of ETSI ES 201
// 873-1 15.6.3: `?` and `*` expand on the left hand side, a length
// attribute is kept, a permutation shelters its indexes, and an element
// of a template list or of `*` is an error.
func TestReferencingRecordOfTemplateElements(t *testing.T) {
	src := `module M {
		type component C {}
		type record of integer RoI;
		type record of RoI RoRoI;
		type record R { RoI field1 optional }
		testcase tc() runs on C {
			var template RoRoI w := ?;
			w[2] := {0,0};
			if (not match({{1},{},{0,0}}, w) or not match({{1},{},{0,0},{5}}, w) or match({{1},{0,0}}, w)) { setverdict(fail, w) }
			w[4] := {1,1};
			if (not match({{1},{},{0,0},{},{1,1}}, w) or match({{1},{},{0,0},{1,1}}, w)) { setverdict(fail, w) }
			var template RoI l := ? length(2..5);
			l[1] := 1;
			l[3] := ?;
			if (not istemplatekind(l, "length") or not match({0,1,2,3}, l) or match({0,2,2,3}, l)) { setverdict(fail, l) }
			var template R m := { field1 := * };
			m.field1[2] := 2;
			if (not match({ field1 := {7,8,2} }, m)) { setverdict(fail, m) }
			var template RoI p := {permutation(0,1,3,?),2,?};
			if (not istemplatekind(p[5], "?")) { setverdict(fail, p[5]) }
			setverdict(pass);
		}
		testcase tc_sheltered() runs on C { var template RoI p := {permutation(0,1,3,?),2,?}; var template integer i := p[2]; setverdict(pass) }
		testcase tc_list() runs on C { var template RoI v := ({1},{2}); var template integer i := v[0]; setverdict(pass) }
		testcase tc_anyornone() runs on C { var template R m := { field1 := * }; var template integer i := m.field1[0]; setverdict(pass) }
		testcase tc_example6() runs on C {
			var template RoI v := {1, 2, * length(2), 5};
			if (valueof(v[1]) != 2 or not istemplatekind(v[2], "?") or valueof(v[4]) != 5) { setverdict(fail, v) }
			setverdict(pass);
		}
		testcase tc_example7() runs on C { var template RoI v := {1, 2, *, 5}; var template integer i := v[3]; setverdict(pass) }
		testcase tc_example7b() runs on C { var template RoI v := {1, 2, *}; var template integer i := v[2]; setverdict(pass) }
		testcase tc_star_past() runs on C { var template RoI v := {permutation(0,1,3,?),2,*}; var template integer i := v[5]; setverdict(pass) }
		testcase tc_example8() runs on C {
			var template RoI v := {1, 2, * length(2), 5};
			v[1] := 10;
			if (not match({1, 10, 7, 8, 5}, v) or match({1, 10, 7, 5}, v)) { setverdict(fail, v) }
			v[2] := 3;
			if (not match({1, 10, 3, 8, 5}, v) or match({1, 10, 4, 8, 5}, v)) { setverdict(fail, v) }
			setverdict(pass);
		}
		testcase tc_example9() runs on C { var template RoI v := {1, 2, *, 5}; v[3] := 4; setverdict(pass) }
		testcase tc_example10() runs on C {
			var template RoI v := {1, 2, *};
			v[4] := 5;
			if (not match({1, 2, 0, 0, 5}, v) or not match({1, 2, 0, 0, 5, 6}, v) or match({1, 2, 0, 5}, v)) { setverdict(fail, v) }
			v := {1, * length(1..5)};
			v[2] := 3;
			if (not match({1, 0, 3}, v) or not match({1, 0, 3, 4, 4, 4}, v) or match({1, 0, 3, 4, 4, 4, 4}, v)) { setverdict(fail, v) }
			setverdict(pass);
		}
		testcase tc_length() runs on C {
			var template RoI v := ? length(2..5);
			if (not istemplatekind(v[3], "?")) { setverdict(fail, v[3]) }
			setverdict(pass);
		}
		testcase tc_length_past() runs on C { var template RoI v := ? length(2..5); var template integer i := v[5]; setverdict(pass) }
		testcase tc_example2_length() runs on C {
			var template RoI v := ? length(2..5);
			v[1] := 1;
			v[3] := ?;
			if (not istemplatekind(v, "length") or not match({0, 1, 2, 3}, v) or match({0, 2, 2, 3}, v)) { setverdict(fail, v) }
			v[5] := 5;
			if (not match({0, 1, 2, 3, 4, 5}, v) and not istemplatekind(v, "length")) { setverdict(fail, v) }
			var template RoI w := {1, 2} length(2..5);
			w[1] := 9;
			if (not match({1, 9}, w) or match({1, 2}, w)) { setverdict(fail, w) }
			setverdict(pass);
		}
		testcase tc_permutation_assign() runs on C {
			var template RoI p := {permutation(1, 2), 3, *};
			p[2] := 9;
			if (not istemplatekind(p, "permutation") or not match({2, 1, 9}, p) or not match({1, 2, 9, 4}, p) or match({1, 2, 3}, p)) { setverdict(fail, p) }
			setverdict(pass);
		}
		testcase tc_permutation_sheltered() runs on C { var template RoI p := {permutation(1, 2), 3, *}; p[0] := 9; setverdict(pass) }
	}`
	for tc, want := range map[string]runtime.Verdict{"M.tc": runtime.PassVerdict, "M.tc_sheltered": runtime.ErrorVerdict,
		"M.tc_list": runtime.ErrorVerdict, "M.tc_anyornone": runtime.ErrorVerdict,
		"M.tc_example6": runtime.PassVerdict, "M.tc_example7": runtime.ErrorVerdict, "M.tc_example7b": runtime.ErrorVerdict,
		"M.tc_star_past": runtime.ErrorVerdict, "M.tc_example8": runtime.PassVerdict, "M.tc_example9": runtime.ErrorVerdict,
		"M.tc_example10": runtime.PassVerdict, "M.tc_length": runtime.PassVerdict, "M.tc_length_past": runtime.ErrorVerdict,
		"M.tc_example2_length": runtime.PassVerdict, "M.tc_permutation_assign": runtime.PassVerdict,
		"M.tc_permutation_sheltered": runtime.ErrorVerdict} {
		for _, k := range clocks {
			v, reason, _ := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, tc, k.opts)
			if v != want {
				t.Errorf("%s, %s clock: %s (%s), want %s", tc, k.name, v, reason, want)
			}
		}
	}
}

// TestStringElements: one element is assigned to a string, at most just
// past its end, and read within it (ETSI ES 201 873-1 6.1.1.1); an item of
// a pattern takes one indexable item (15.6.1).
func TestStringElements(t *testing.T) {
	runBodiesPass(t, map[string]string{
		"append": `var charstring s := "ab"; s[2] := "c"; if (s != "abc") { setverdict(fail, s) }
			var octetstring o := '01'O; o[1] := '02'O; if (o != '0102'O) { setverdict(fail, o) }`,
	})
	for name, body := range map[string]string{
		"past the end":     `var charstring s := "ab"; s[3] := "c";`,
		"two characters":   `var charstring s := "ab"; s[0] := "xy";`,
		"read past":        `var charstring s := "ab"; var charstring c := s[2];`,
		"read past binary": `var bitstring b := '01'B; var bitstring c := b[2];`,
		"pattern item":     `var template charstring p := pattern "abc"; p[1] := "xy";`,
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

// TestPatternItemsEdgeCases: an unclosed `{` or `(` is a literal
// character (ETSI ES 201 873-1 B.1.5), and a group's sets are skipped
// over when its end is found.
func TestPatternItemsEdgeCases(t *testing.T) {
	runBodiesPass(t, map[string]string{
		"literal brace": `var template charstring p := pattern "a{b";
			if (p[1].value != "{" or p[2].value != "b") { setverdict(fail, p[1], p[2]) }`,
		"set in group": `var template charstring p := pattern "([)]x)y";
			if (p[0].value != "([)]x)" or p[1].value != "y") { setverdict(fail, p[0], p[1]) }`,
		"sets": `var template charstring p := pattern "[]]x[^]a]\[";
			if (p[0].value != "[]]" or p[1].value != "x" or p[2].value != "[^]a]" or p[3].value != "\[") { setverdict(fail, p[0], p[2], p[3]) }`,
	})
}

// TestElementsUnderAttributes: an element of a template with a length or
// ifpresent attribute is the template's, the attribute kept; one of a
// record of with ifpresent, or of a template list, is an error (ETSI ES
// 201 873-1 15.6.1 k, l; 15.6.3 b, c, f). A `* length(n)` before the
// element assigned stays as it is.
func TestElementsUnderAttributes(t *testing.T) {
	runBodiesPass(t, map[string]string{
		"string": `var template charstring s := "abc" length(3); s[1] := "x";
			if (not istemplatekind(s, "length") or not match("axc", s)) { setverdict(fail, s) }
			var template charstring u := "abc" ifpresent; u[1] := "y";
			if (not istemplatekind(u, "ifpresent") or not match("ayc", u)) { setverdict(fail, u) }`,
		"untouched length": `var template RoI v := {permutation(1, 2), * length(2), 3, *}; v[4] := 8;
			if (not istemplatekind(v, "permutation") or not match({2, 1, 0, 0, 8}, v) or match({2, 1, 0, 8}, v)) { setverdict(fail, v) }`,
	})
	for name, body := range map[string]string{
		"value list":     `var template RoI v := ({1}, {2}) length(1); v[0] := 3;`,
		"ifpresent list": `var template RoI v := {1, 2} length(2) ifpresent; v[0] := 3;`,
	} {
		src := `module M { type component C {} type record of integer RoI; testcase tc() runs on C { ` + body + ` setverdict(pass); } }`
		for _, k := range clocks {
			v, reason, _ := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
			if v != runtime.ErrorVerdict {
				t.Errorf("%s, %s clock: %s (%s), want error", name, k.name, v, reason)
			}
		}
	}
}

// TestUnionAlternativeChosenThroughAField: assigning a field of a union's
// alternative chooses it, and drops the alternative the union held (ETSI
// ES 201 873-1 6.2.5).
func TestUnionAlternativeChosenThroughAField(t *testing.T) {
	src := `module M {
		type component C {}
		type union SubU { integer s1, charstring s2 }
		type union U { integer o1, SubU o2 }
		testcase tc() runs on C {
			var U u;
			u.o1 := 1;
			u.o2.s2 := "t";
			if (ischosen(u.o1) or not ischosen(u.o2) or u.o2.s2 != "t") { setverdict(fail, u) }
			u.o1 := 5;
			if (ischosen(u.o2) or u.o1 != 5) { setverdict(fail, u) }
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

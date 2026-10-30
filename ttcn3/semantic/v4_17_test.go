package semantic

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// expectCodes analyses each source and checks that code is reported for
// the ones mapped to true and absent from the others.
func expectCodes(t *testing.T, code string, cases map[string]bool) {
	t.Helper()
	for src, want := range cases {
		diags := NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, src))
		if got := containsCode(diags, code); got != want {
			t.Errorf("%s reported %v, want %v; got %v in\n%s", code, got, want, codes(diags), src)
		}
	}
}

// TestTemplateRestrictionTables: what may be assigned or passed to a
// restricted template follows table 13 of ETSI ES 201 873-1 15.8, and the
// fields of a template(value) or template(omit) are restricted (table
// 13c). A value always fits; t.present converts an unrestricted template
// to a present one (15.14).
func TestTemplateRestrictionTables(t *testing.T) {
	decls := `type record R { integer a, boolean b optional }
		type component C {}
		template R t_any := { 1, * };
		template(omit) R t_omit := { 1, omit };
		template(value) R t_value := { 1, true };
		template(present) R t_present := { 1, ? };
		const R c_r := { 1, true };
		function f_present(template(present) R p) {}
		function f_value(template(value) integer p) {}
		`
	tc := func(body string) string {
		return "module M {\n" + decls + "testcase tc() runs on C {\n" + body + "\n} }"
	}
	expectCodes(t, "template-restriction-assignment", map[string]bool{
		tc(`var template(present) R v := t_omit;`):                    true,
		tc(`var template(present) R v := t_any;`):                     true,
		tc(`var template(present) R v := t_value;`):                   false,
		tc(`var template(present) R v := t_present;`):                 false,
		tc(`var template(present) R v := c_r;`):                       false,
		tc(`var template(omit) R v := t_value;`):                      false,
		tc(`var template(omit) R v := t_present;`):                    true,
		tc(`var template(value) R v := t_omit;`):                      true,
		tc(`var template(present) boolean v := t_any.b.present;`):     false,
		tc(`var template(present) R v := t_any.present;`):             false,
		tc(`var template(value) R v := t_any.present;`):               true,
		tc(`var template(present) R v; v := t_omit;`):                 true,
		tc(`var template R v := t_omit; v := t_any;`):                 false,
		tc(`f_present(t_omit);`):                                      true,
		tc(`f_present(p := t_any);`):                                  true,
		tc(`f_present(t_value);`):                                     false,
		tc(`var template integer v := ?; f_value(v);`):                true,
		tc(`var integer v := 1; f_value(v);`):                         false,
		tc(`var template(value) integer v := 1; f_value(v);`):         false,
		tc(`var template(omit) R v := t_omit; f_present(v.present);`): false,
	})
	expectCodes(t, "template-restriction-field", map[string]bool{
		"module M {\n" + decls + "template(value) R t(template integer p := 1) := { a := p, b := true }\n}":                       true,
		"module M {\n" + decls + "template(value) R t(template(value) integer p := 1) := { a := p, b := true }\n}":                false,
		"module M {\n" + decls + "template(omit) R t(template(present) integer p := 1) := { a := p, b := omit }\n}":               true,
		"module M {\n" + decls + "template(present) R t(template integer p := 1) := { a := p, b := true }\n}":                     true,
		"module M {\n" + decls + "template(present) R t(template boolean p := true) := { a := 1, b := p }\n}":                     false,
		"module M {\n" + decls + "template(present) R t(template(present) integer p := ?) := { a := p, b := * }\n}":               false,
		"module M {\n" + decls + "template(value) R t(template(omit) boolean p := omit) := { a := 1, b := p }\n}":                 false,
		"module M {\n" + decls + "template(value) R t(template(omit) integer p := 1) := { a := p, b := omit }\n}":                 true,
		"module M {\n" + decls + "template(value) R t(template(omit) integer p := 1) := { p, omit }\n}":                           true,
		"module M {\n" + decls + "type union U { integer i } template(value) U t(template(omit) integer p := 1) := { i := p }\n}": true,
		"module M {\n" + decls + "template(value) R t(integer p := 1) := { a := p, b := true }\n}":                                false,
		"module M {\n" + decls + "type record of R L; template(value) L t(template R p := t_any) := { t_value, p }\n}":            true,
	})
}

// TestModifiedTemplateRestrictions: a modified template may change its
// base's restriction as table 13b allows, and its formal template
// parameters' as table 13a does (ETSI ES 201 873-1 15.8 c, d).
func TestModifiedTemplateRestrictions(t *testing.T) {
	mod := func(base, modified string) string {
		return `module M {
			type record R { integer a, boolean b optional }
			template` + base + ` R t_base := { 1, true };
			template` + modified + ` R t_mod modifies t_base := { a := 2 };
		}`
	}
	allowed := map[[2]string]bool{
		{"", ""}: true, {"", "(omit)"}: false, {"", "(value)"}: false, {"", "(present)"}: false,
		{"(omit)", ""}: true, {"(omit)", "(omit)"}: true, {"(omit)", "(value)"}: false, {"(omit)", "(present)"}: false,
		{"(value)", ""}: true, {"(value)", "(omit)"}: true, {"(value)", "(value)"}: true, {"(value)", "(present)"}: true,
		{"(present)", ""}: true, {"(present)", "(omit)"}: false, {"(present)", "(value)"}: false, {"(present)", "(present)"}: true,
	}
	cases := map[string]bool{}
	for k, ok := range allowed {
		cases[mod(k[0], k[1])] = !ok
	}
	expectCodes(t, "modified-template-restriction", cases)

	par := func(base, modified string) string {
		return `module M {
			type record R { integer a, boolean b optional }
			template R t_base(template` + base + ` integer p) := { p, true };
			template R t_mod(template` + modified + ` integer p) modifies t_base := { b := false };
		}`
	}
	allowedPar := map[[2]string]bool{
		{"", ""}: true, {"", "(omit)"}: true, {"", "(value)"}: true, {"", "(present)"}: true,
		{"(omit)", ""}: false, {"(omit)", "(omit)"}: true, {"(omit)", "(value)"}: true, {"(omit)", "(present)"}: false,
		{"(value)", ""}: false, {"(value)", "(omit)"}: false, {"(value)", "(value)"}: true, {"(value)", "(present)"}: false,
		{"(present)", ""}: false, {"(present)", "(omit)"}: false, {"(present)", "(value)"}: true, {"(present)", "(present)"}: true,
	}
	cases = map[string]bool{}
	for k, ok := range allowedPar {
		cases[par(k[0], k[1])] = !ok
	}
	expectCodes(t, "modified-template-param-restriction", cases)
}

// TestModifiedLocalTemplateRestrictions: tables 13a and 13b apply to a
// base declared locally too.
func TestModifiedLocalTemplateRestrictions(t *testing.T) {
	src := func(base, modified string) string {
		return `module M {
			type record R { integer a, boolean b optional }
			type component C {}
			testcase tc() runs on C {
				template` + base + ` R t1 := { 1, true };
				template` + modified + ` R t2 modifies t1 := { a := 2 };
			}
		}`
	}
	expectCodes(t, "modified-template-restriction", map[string]bool{
		src("", "(present)"):        true,
		src("(value)", "(present)"): false,
		src("(present)", ""):        false,
	})
}

// TestModulePrefixOnlyForModuleDefinitions: a module's own name may prefix
// its module-level definitions only (ETSI ES 201 873-1 8.2.3.1).
func TestModulePrefixOnlyForModuleDefinitions(t *testing.T) {
	expectCodes(t, "module-prefix-not-global", map[string]bool{
		`module M { const integer c := 1; type component C {}
			testcase tc() runs on C { log(M.c) } }`: false,
		`module M { group g { const integer c := 1; } type component C {}
			testcase tc() runs on C { log(M.c) } }`: false,
		`module M { modulepar { integer mp := 1 } type component C {}
			testcase tc() runs on C { log(M.mp) } }`: false,
		`module M { type component C {}
			testcase tc() runs on C { var integer c := 1; log(M.c) } }`: true,
		`module M { type component C { var integer c := 1 }
			testcase tc() runs on C { log(M.c) } }`: true,
		`module M { import from N all; type component C {}
			testcase tc() runs on C { log(M.n) } }`: true,
	})
}

// TestSignatureTypesAreDataTypes: signature parameters and the return
// type shall be of a data type (ETSI ES 201 873-1 14 b).
func TestSignatureTypesAreDataTypes(t *testing.T) {
	expectCodes(t, "signature-non-data-type", map[string]bool{
		`module M { signature S(in integer p) return integer; }`:                       false,
		`module M { signature S(in integer p) return default; }`:                       true,
		`module M { signature S(in default p); }`:                                      true,
		`module M { type component C {} signature S(in C p); }`:                        true,
		`module M { type port P message { inout integer } signature S() return P; }`:   true,
		`module M { type default D; signature S(inout D p); }`:                         true,
		`module M { type record R { integer a } signature S(out R p) exception (R); }`: false,
	})
}

// TestConnectionsCarrySomething: connect needs a port that can send, and
// map a component port that sends or a system port that delivers (ETSI
// ES 201 873-1 21.1.1 b.3, c.3).
func TestConnectionsCarrySomething(t *testing.T) {
	src := func(p1, p2, op string) string {
		return `module M {
			type port P1 message { ` + p1 + ` }
			type port P2 message { ` + p2 + ` }
			type component C { port P1 p }
			type component S { port P2 q }
			testcase tc() runs on C system S {
				var C c := C.create;
				` + op + `
			}
		}`
	}
	expectCodes(t, "connect-nothing-to-send", map[string]bool{
		src("in integer", "in integer", "connect(c:p, mtc:p);"):    true,
		src("inout integer", "in integer", "connect(c:p, mtc:p);"): false,
		src("in integer", "in integer", "disconnect(c:p, mtc:p);"): false,
	})
	expectCodes(t, "map-nothing-to-carry", map[string]bool{
		src("in integer", "out integer", "map(mtc:p, system:q);"):   true,
		src("in integer", "out integer", "map(system:q, mtc:p);"):   true,
		src("out integer", "out integer", "map(mtc:p, system:q);"):  false,
		src("in integer", "in integer", "map(mtc:p, system:q);"):    false,
		src("in integer", "out integer", "unmap(mtc:p, system:q);"): false,
	})
}

// TestGotoOutOfInterleave: a goto may jump out of an interleave, under a
// condition or not; one to a label inside a branch that receives may not
// (ETSI ES 201 873-1 20.4 d).
func TestGotoOutOfInterleave(t *testing.T) {
	src := func(branch string) string {
		return `module M {
			type port P message { inout integer }
			type component C { port P p }
			testcase tc() runs on C {
				var boolean b := true;
				interleave {
					[] p.receive(1) { ` + branch + ` }
					[] p.receive(2) {}
				}
				label L1;
			}
		}`
	}
	cases := map[string]bool{
		src(`if (b) { goto L1; }`):                         false,
		src(`goto L1;`):                                    false,
		src(`goto L2; label L2; p.receive(3);`):            true,
		src(`if (b) { goto L2; } label L2; p.receive(3);`): true,
	}
	for s, want := range cases {
		diags := NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, s))
		got := false
		for _, d := range diags {
			got = got || (d.Severity == SeverityError && strings.HasPrefix(d.Code, "interleave-"))
		}
		if got != want {
			t.Errorf("interleave goto error %v, want %v: %v in\n%s", got, want, codes(diags), s)
		}
	}
}

// TestSubstrOfTemplates: substr accepts a binary string template with
// matching mechanisms inside its value, each counting as one item (ETSI
// ES 201 873-1 15.6.1, C.4.2), but not AnyElementsOrNone as an element of
// a sequence.
func TestSubstrOfTemplates(t *testing.T) {
	expectCodes(t, "substr-forbidden-matching-mechanism", map[string]bool{
		`module M { type component C {} testcase tc() runs on C {
			var template bitstring t := '00101*'B; var template bitstring v := substr(t, 1, 2) } }`: false,
		`module M { type record of integer L; type component C {} testcase tc() runs on C {
			var template L t := { 1, *, 3 }; var template L v := substr(t, 0, 1) } }`: true,
	})
}

// TestRestrictionsSeenFromTheComponentRunOn: a component variable is
// seen only by what runs on its component type or one extending it.
func TestRestrictionsSeenFromTheComponentRunOn(t *testing.T) {
	expectCodes(t, "template-restriction-assignment", map[string]bool{
		`module M { type component C { var template integer v := ? } type component D { var template(value) integer v := 1 }
			function f() runs on D { var template(value) integer x := v; } }`: false,
		`module M { type component C { var template integer v := ? } type component D { var template(value) integer v := 1 }
			function f() runs on C { var template(value) integer x := v; } }`: true,
		`module M { type component C { var template integer v := ? } type component E extends C {}
			function f() runs on E { var template(value) integer x := v; } }`: true,
		`module M { import from L all; type component C { var template integer v := ? }
			function f() runs on CI { var template(value) integer x := v; } }`: false,
	})
}

// TestMtcTypeIsKnownOnlyWhereDeclared: in a function without an mtc
// clause the MTC's type is not the runs-on component's, so its ports are
// not judged by that type's.
func TestMtcTypeIsKnownOnlyWhereDeclared(t *testing.T) {
	src := func(fn string) string {
		return `module M {
			type port InP message { in integer }
			type port OutP message { out integer }
			type component MTC { port OutP p }
			type component PTC { port InP p }
			` + fn + `
		}`
	}
	expectCodes(t, "connect-nothing-to-send", map[string]bool{
		src(`function f() runs on PTC { var PTC o := PTC.create; connect(mtc:p, o:p); }`):         false,
		src(`function f() runs on PTC mtc PTC { var PTC o := PTC.create; connect(mtc:p, o:p); }`): true,
		src(`testcase tc() runs on PTC { var PTC o := PTC.create; connect(mtc:p, o:p); }`):        true,
		src(`testcase tc() runs on MTC { var PTC o := PTC.create; connect(mtc:p, o:p); }`):        false,
	})
}

// TestGotoWithinAReceptionFreeBlockOfAnInterleave: a branch that receives
// may still jump within one of its blocks that does not (ETSI ES 201
// 873-1 20.4 d).
func TestGotoWithinAReceptionFreeBlockOfAnInterleave(t *testing.T) {
	src := `module M {
		type port P message { inout integer }
		type component C { port P p }
		testcase tc() runs on C {
			var integer i := 0;
			interleave {
				[] p.receive(1) {
					if (i == 0) { label L1; i := i + 1; if (i < 3) { goto L1; } }
					p.receive(2);
				}
				[] p.receive(3) {}
			}
		}
	}`
	for _, d := range NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, src)) {
		if strings.HasPrefix(d.Code, "interleave-") {
			t.Errorf("unexpected %s: %s", d.Code, d.Message)
		}
	}
}

// TestMtcAndSystemClausesOfFunctions: a function may have an mtc and a
// system clause, but none that the control part calls, directly or
// through others (ETSI ES 201 873-1 16.1 restriction c).
func TestMtcAndSystemClausesOfFunctions(t *testing.T) {
	src := func(control string) string {
		return `module M {
			type component C {}
			function f_mtc() mtc C return integer { return 1 }
			function f_sys() system C {}
			function f_via() { f_sys() }
			function f_plain() {}
			testcase tc() runs on C { log(f_mtc()); f_sys() }
			control { ` + control + ` }
		}`
	}
	expectCodes(t, "control-calls-function-with-mtc-or-system", map[string]bool{
		src(`execute(tc());`):            false,
		src(`f_plain(); execute(tc());`): false,
		src(`var integer i := f_mtc();`): true,
		src(`f_sys();`):                  true,
		src(`f_via();`):                  true,
	})
	expectCodes(t, "function-mtc-on-non-testcase", map[string]bool{src(`execute(tc());`): false})
}

// TestQualifiedCallHasItsParentheses: `M.f()` calls f with parentheses
// (ETSI ES 201 873-1 5.4.2, 8.2.3.1).
func TestQualifiedCallHasItsParentheses(t *testing.T) {
	expectCodes(t, "missing-call-parens", map[string]bool{
		`module M { function f() return integer { return 1 } type component C {}
			testcase tc() runs on C { var integer x := M.f(); } }`: false,
		`module M { function f() return integer { return 1 } type component C {}
			testcase tc() runs on C { var integer x := f; } }`: true,
		`module M { type function FT() return integer; function f() return integer { return 1 } type component C {}
			testcase tc() runs on C { var FT g := refers(f); } }`: false,
	})
}

// TestControlCallsFollowGroupsQualifiedCallsAndCycles: the functions the
// control part reaches are found in groups, through calls qualified by
// the module's name, and through calls that form a cycle.
func TestControlCallsFollowGroupsQualifiedCallsAndCycles(t *testing.T) {
	src := func(control string) string {
		return `module M {
			type component C {}
			group g { function f_mtc() mtc C {} }
			function f_a() { f_b() }
			function f_b() { f_a(); f_mtc() }
			control { ` + control + ` }
		}`
	}
	expectCodes(t, "control-calls-function-with-mtc-or-system", map[string]bool{
		src(`f_mtc();`):   true,
		src(`M.f_mtc();`): true,
		src(`f_a();`):     true,
		src(``):           false,
	})
}

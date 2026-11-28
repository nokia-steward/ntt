package interpreter_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

const fieldExpansionTypes = `
	type record R2 { integer g1, R2 g2 optional }
	type record R1 { integer f1 optional, R2 f2 optional }
	type record P { integer x, integer y }
	type union U { integer c1, P c2 }
	type record W { integer g1, U g2 optional }
	type record of integer RoI
	type record R3 { integer h1, RoI lst }
	type record R4 { integer k, R3 r optional }
	type component C { var template R1 ct := {f1 := 5, f2 := omit} }
	function f_out(out template integer p) { p := 7 }
	template @fuzzy R1 t_fz := {f1 := 5, f2 := omit};
	function f_ct() runs on C { ct.f2.g1 := 4 }
	function want(boolean b, charstring what) {
		if (not b) { setverdict(fail, what); stop }
	}
	function f_par(inout template R1 t) { t.f2.g1 := 7 }
`

// Assigning a field of a template field holding AnyValue, AnyValueOrNone
// or omit expands it: AnyValue for each mandatory field and AnyValueOrNone
// for each optional one, down to the field assigned (ETSI ES 201 873-1
// 15.6.2 a, b); a union's alternative referenced becomes the chosen one,
// expanded by the same rules (15.6.5 a); a signature template of AnyValue
// modified expands to AnyValue for each parameter (15.6.4 b).
func TestTemplateFieldExpansion(t *testing.T) {
	for name, body := range map[string]string{
		"AnyValue": `var template R1 t := {f1 := 0, f2 := ?};
			t.f2.g2.g1 := 1;
			want(match({0, {5, {1, omit}}}, t), "expanded");
			want(not match({0, {5, omit}}, t), "g2 is present");
			want(not match({0, {5, {2, omit}}}, t), "g2.g1 is 1");`,
		"AnyValueOrNone": `var template R1 t := {f1 := 5, f2 := *};
			t.f2.g2.g1 := 3;
			want(match({5, {0, {3, omit}}}, t) and match({5, {7, {3, {1, omit}}}}, t), "expanded");
			want(not match({5, omit}, t), "f2 is present");`,
		"omit": `var template R1 t := {f1 := 5, f2 := omit};
			t.f2.g2.g1 := 3;
			want(match({5, {-9, {3, omit}}}, t), "expanded");`,
		"one level": `var template R1 t := {f1 := 5, f2 := omit};
			t.f2.g1 := 3;
			want(match({5, {3, omit}}, t) and match({5, {3, {1, omit}}}, t), "g2 is *");`,
		"a template parameter": `var template R1 t := {f1 := 5, f2 := omit};
			f_par(t);
			want(match({5, {7, omit}}, t), "expanded through the parameter");`,
		"union omit": `var template W t := {g1 := 5, g2 := omit};
			t.g2.c2.x := 1;
			want(match({5, {c2 := {1, 99}}}, t), "c2 chosen, y is ?");
			want(not match({5, {c2 := {2, 99}}}, t), "x is 1");`,
		"union AnyValue": `var template U t := ?;
			t.c2.x := 1;
			want(match({c2 := {1, 99}}, t) and not match({c1 := 1}, t), "c2 chosen");`,
		"union alternative": `var template W t := {g1 := 5, g2 := *};
			t.g2.c1 := 4;
			want(match({5, {c1 := 4}}, t), "c1 chosen");`,
		"reading AnyValue": `var template R1 t := {f1 := 0, f2 := ?};
			var template R2 u := t.f2.g2;
			want(match({1, omit}, u) and match({1, {2, omit}}, u), "{?, *}");
			var template integer i := t.f2.g1;
			want(match(3, i), "?");`,
		"a value's omit": `var R1 v := {f1 := 5, f2 := omit};
			v.f2.g1 := 3;
			want(not isbound(v.f2.g2), "a value is not expanded");`,
		"a template restricted to omit or value is a value": `var omit R1 t := omit;
			t.f1 := 0;
			want(not isbound(t.f2), "template(omit) is not expanded");
			var template(value) R1 u := {f1 := 1, f2 := omit};
			u.f2.g1 := 0;
			want(not isbound(u.f2.g2), "template(value) is not expanded");
			var template(present) R1 w := {f1 := 1, f2 := omit};
			w.f2.g1 := 0;
			want(match({1, {0, {3, omit}}}, w), "template(present) is expanded");`,
		"a component's template variable": `ct.f2.g1 := 3;
			want(match({5, {3, omit}}, ct) and match({5, {3, {1, omit}}}, ct), "expanded");`,
		"a component's template variable in a function": `f_ct();
			want(match({5, {4, {1, omit}}}, ct), "expanded");`,
		"an element of a field expanded": `var template R4 t := {k := 1, r := ?};
			t.r.lst[0] := 7;
			want(match({1, {99, {7}}}, t) and match({1, {99, {7, 8}}}, t), "h1 is ?");`,
		"an out parameter": `var template R1 t := {f1 := 5, f2 := ?};
			f_out(t.f2.g1);
			want(match({5, {7, omit}}, t) and not match({5, {8, omit}}, t), "written through ?");
			var template R1 u := {f1 := 5, f2 := omit};
			f_out(u.f2.g1);
			want(match({5, {7, omit}}, u), "written through omit");`,
		"a value shadowing a template": `var template R1 t := {f1 := 5, f2 := ?};
			if (true) {
				var R1 t := {f1 := 5, f2 := omit};
				t.f2.g1 := 3;
				want(not isbound(t.f2.g2), "the inner t is a value");
			}`,
	} {
		t.Run(name, func(t *testing.T) {
			assertSameVerdictBothClocks(t, "module M {"+fieldExpansionTypes+
				"testcase tc() runs on C {\n"+body+"\nsetverdict(pass);\n}\n}", runtime.PassVerdict)
		})
	}
	assertSameVerdictBothClocks(t, `module M {
		signature S(in integer par1, in integer par2);
		type port Q procedure { inout S }
		type component C { port Q p }
		template S s_any := ?;
		template S s_mod modifies s_any := { par1 := 3 };
		testcase tc() runs on C {
			connect(self:p, self:p);
			p.call(S:{par1 := 3, par2 := 5}, nowait);
			timer w := 1.0; w.start;
			alt {
				[] p.getcall(s_mod) { setverdict(pass) }
				[] p.getcall { setverdict(fail, "par2 is not ?") }
				[] w.timeout { setverdict(fail, "no call") }
			}
		}
	}`, runtime.PassVerdict)
}

// Reading a field of a template field holding AnyValueOrNone, omit, a
// template list or a complemented list, or with ifpresent, and assigning
// one of a template or complemented list, are errors (ETSI ES 201 873-1
// 15.6.2 a, c; 15.6.5 a).
func TestTemplateFieldExpansionErrors(t *testing.T) {
	for name, body := range map[string]string{
		"reading through omit":                                 `var template R1 t := {f1 := 5, f2 := omit}; setverdict(pass); var template integer i := t.f2.g1;`,
		"reading through *":                                    `var template R1 t := {f1 := 5, f2 := *}; setverdict(pass); var template R2 u := t.f2.g2;`,
		"reading through a list":                               `var template R1 t := ({f1 := omit, f2 := {0, omit}}, {f1 := 5, f2 := {1, omit}}); setverdict(pass); var template R2 u := t.f2;`,
		"reading through a complement":                         `var template R1 t := complement({f1 := omit, f2 := {0, omit}}); setverdict(pass); var template R2 u := t.f2;`,
		"reading through ifpresent":                            `var template R1 t := {f1 := 5, f2 := {1, omit} ifpresent}; setverdict(pass); var template integer i := t.f2.g1;`,
		"reading a component's template variable through omit": `setverdict(pass); var template integer i := ct.f2.g1;`,
		"reading a dynamic template through omit":              `setverdict(pass); var template integer i := t_fz.f2.g1;`,
		"reading a union through omit":                         `var template W t := {g1 := 5, g2 := omit}; setverdict(pass); var template integer i := t.g2.c1;`,
		"assigning through a list":                             `var template R1 t := {f1 := 5, f2 := ({0, omit}, {1, omit})}; setverdict(pass); t.f2.g1 := 3;`,
		"assigning through a complement":                       `var template R1 t := {f1 := 5, f2 := complement({0, omit})}; setverdict(pass); t.f2.g1 := 3;`,
	} {
		src := "module M {" + fieldExpansionTypes + "testcase tc() runs on C {\n" + body + "\n}\n}"
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
			if err != nil || v != runtime.ErrorVerdict || !strings.Contains(reason, "15.6") {
				t.Errorf("%s, %s clock: %s (%s) %v; want the 15.6 error", name, k.name, v, reason, err)
			}
		}
	}
}

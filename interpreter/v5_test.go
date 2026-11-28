package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// TestTemplatesEvaluatedAtEachUse: a global template without parameters is
// evaluated once, where it is declared, unless it is @fuzzy (ETSI ES 201
// 873-1 15.3) or its module's template attribute is "dynamic" (TTCN-3 V5,
// ETSI TR 104 081 6.13); one with a parameter list, empty or not, is
// evaluated at each use, and may be used without `()`. A static template
// computed from a dynamic one evaluates it once.
func TestTemplatesEvaluatedAtEachUse(t *testing.T) {
	decls := `type component C {}
		type record R { integer a, float b }
		template float t_static := rnd();
		template float t_dynamic() := rnd();
		template float t_static2 := t_dynamic();
		template @fuzzy float t_fuzzy := rnd();
		template @fuzzy R t_fuzzyRec := { 1, rnd() };`
	same := func(x string) string {
		return `if (valueof(` + x + `) != valueof(` + x + `)) { setverdict(fail, "` + x + ` evaluated anew") }`
	}
	differ := func(x string) string {
		return `if (valueof(` + x + `) == valueof(` + x + `)) { setverdict(fail, "` + x + ` evaluated once") }`
	}
	for name, c := range map[string]struct{ with, body string }{
		"static module": {"", same("t_static") + same("t_static2") + differ("t_dynamic()") + differ("t_dynamic") +
			differ("t_fuzzy") + `if (valueof(t_fuzzyRec).b == valueof(t_fuzzyRec).b) { setverdict(fail, "t_fuzzyRec evaluated once") }`},
		"dynamic module":   {`with { template "dynamic" }`, differ("t_static") + differ("t_static2") + differ("t_dynamic")},
		"static attribute": {`with { template "static" }`, same("t_static") + differ("t_dynamic")},
	} {
		src := "module M {\n" + decls + "\ntestcase tc() runs on C {\n" + c.body + "\nsetverdict(pass)\n}\n} " + c.with
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
			if err != nil || v != runtime.PassVerdict {
				t.Errorf("%s, %s clock: %s (%s) %v", name, k.name, v, reason, err)
			}
		}
	}
}

// TestModifiedTemplates: a modified template has its base's fields where
// it does not change them (ETSI ES 201 873-1 15.5): a base written
// positionally, a `-` in a modification written positionally, a base with
// parameters - the derived template's own, of the same names - and a
// chain of them; in a module of dynamic templates, too.
func TestModifiedTemplates(t *testing.T) {
	decls := `type component C { port P p }
		type port P message { inout R }
		type record R { integer a, float b }
		template R t_pos := { 1, 1.0 };
		template R t_mod modifies t_pos := { a := 2 };
		template R t_dash modifies t_pos := { 2, - };
		template R t1(integer p) := { a := p, b := 1.0 };
		template R t2(integer p) modifies t1 := { b := 2.0 };
		template R t3(integer p, float q) modifies t2 := { b := q };`
	body := `connect(self:p, self:p);
		if (not match(valueof(t_mod), R:{ 2, 1.0 })) { setverdict(fail, "t_mod ", t_mod) }
		if (not match(valueof(t_dash), R:{ 2, 1.0 })) { setverdict(fail, "t_dash ", t_dash) }
		if (not match(valueof(t2(5)), R:{ 5, 2.0 })) { setverdict(fail, "t2 ", t2(5)) }
		if (not match(valueof(t3(6, 3.0)), R:{ 6, 3.0 })) { setverdict(fail, "t3 ", t3(6, 3.0)) }
		p.send(t_mod);
		alt { [] p.receive(R:{ 2, 1.0 }) { setverdict(pass) } [] p.receive { setverdict(fail, "t_mod sent wrong") } }`
	for _, with := range []string{"", `with { template "dynamic" }`} {
		src := "module M {\n" + decls + "\ntestcase tc() runs on C {\n" + body + "\n}\n} " + with
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
			if err != nil || v != runtime.PassVerdict {
				t.Errorf("%q, %s clock: %s (%s) %v", with, k.name, v, reason, err)
			}
		}
	}
}

// TestTTCN3V5Additions: what TTCN-3 V5 adds without changing what a V4
// module means (ETSI TR 104 081 8.1): a port type declared without its
// kind is a message port type; conversions to and from the universal
// charstring beside those of the charstring, whose names a module may
// still give its own definitions.
func TestTTCN3V5Additions(t *testing.T) {
	for name, src := range map[string]string{
		"port type without kind": `module M {
			type port P { inout integer }
			type component C { port P p }
			testcase tc() runs on C { connect(self:p, self:p); p.send(1);
				alt { [] p.receive(1) { setverdict(pass) } [] p.receive { setverdict(fail) } } }
		}`,
		"universal charstring conversions": `module M {
			type component C {}
			testcase tc() runs on C {
				var universal charstring u := int2unistr(42);
				if (u == "42" and bit2unistr('0101'B) == "0101" and hex2unistr('1F'H) == "1F" and
				    unistr2int("17") == 17 and unistr2hex("AB") == 'AB'H and unistr2oct("0A1B") == '0A1B'O and
				    unistr2float("1.5") == 1.5) { setverdict(pass) } else { setverdict(fail) }
			}
		}`,
		"own definition of a conversion's name": `module M {
			type component C {}
			function int2unistr(integer i) return universal charstring { return "mine" }
			testcase tc() runs on C { if (int2unistr(1) == "mine") { setverdict(pass) } else { setverdict(fail) } }
		}`,
	} {
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
			if err != nil || v != runtime.PassVerdict {
				t.Errorf("%s, %s clock: %s (%s) %v", name, k.name, v, reason, err)
			}
		}
	}
}

// TestDynamicTemplateIsATemplateOfItsType: a dynamic template is used as a
// static one is, its declared type and all: named with its module, encoded
// by its type's encode attribute, its field read.
func TestDynamicTemplateIsATemplateOfItsType(t *testing.T) {
	a := `module A {
		type record R { integer a, float b optional } with { encode "JSON" }
		template @fuzzy float tz := rnd();
		template @fuzzy R tr := { a := 1, b := omit };
		template R ts := { a := 1, b := omit };
	}`
	b := `module B {
		import from A all;
		type component C {}
		testcase tc() runs on C {
			if (valueof(A.tz) == valueof(A.tz)) { setverdict(fail, "A.tz evaluated once") }
			if (valueof(A.tr.a) != 1 or not match(R:{ 1, omit }, A.tr)) { setverdict(fail, "A.tr ", A.tr) }
			if (encvalue(tr) != encvalue(ts)) { setverdict(fail, "encvalue ", encvalue(tr), " ", encvalue(ts)) }
			setverdict(pass)
		}
	}`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, a), parse(t, b)}, "B.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}

// TestModificationReplacesWhatItDoesNotMerge: a modification of a record
// of or union template replaces its base's value (ETSI ES 201 873-1
// 15.5), for a template with parameters and a dynamic one as for a
// static one; a base's parameter takes the derived template's actual, or
// the derived template's default, over its own default.
func TestModificationReplacesWhatItDoesNotMerge(t *testing.T) {
	src := `module M {
		type component C {}
		type record of integer RoI;
		type union U { integer i, charstring s }
		type record R { integer a, float b }
		template RoI b := { 1, 2, 3 };
		template RoI p1(integer x) modifies b := { 7, x };
		template @fuzzy RoI f1 modifies b := { 7, 8 };
		template U ub := { i := 1 };
		template U up(charstring x) modifies ub := { s := x };
		template U us modifies ub := { s := "s" };
		template U ubp(charstring x) := { s := x };
		template U upp(charstring x) modifies ubp := { i := 3 };
		template R t1(integer p := 3) := { a := p, b := 1.0 };
		template R t3(integer p := 7) modifies t1 := { b := 3.0 };
		testcase tc() runs on C {
			if (not match({7, 8}, p1(8)) or match({1, 2, 3}, p1(8))) { setverdict(fail, "p1 ", p1(8)) }
			if (not match({7, 8}, f1)) { setverdict(fail, "f1 ", f1) }
			if (not ischosen(up("x").s) or ischosen(up("x").i)) { setverdict(fail, "up ", up("x")) }
			if (not ischosen(us.s) or ischosen(us.i)) { setverdict(fail, "us ", us) }
			if (not ischosen(upp("z").i) or ischosen(upp("z").s)) { setverdict(fail, "upp ", upp("z")) }
			if (valueof(t3().a) != 7 or valueof(t3(8).a) != 8) { setverdict(fail, "t3 ", t3(), t3(8)) }
			setverdict(pass)
		}
	}`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}

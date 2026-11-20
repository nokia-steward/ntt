package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// TestPredefinedFunctionNamesAreReserved: nothing may be declared by the
// name of a predefined function (ETSI ES 201 873-1 A.1.5); a name merely
// containing one, or a call of one, is fine.
func TestPredefinedFunctionNamesAreReserved(t *testing.T) {
	for name, src := range map[string]string{
		"function":         `function rnd() return float { return 0.5 }`,
		"constant":         `const integer hostid := 1;`,
		"field":            `type record R { integer replace }`,
		"enumerated value": `type enumerated E { e_a, substr }`,
		"parameter":        `function f(integer sizeof) {}`,
		"component member": `type component C { var integer lengthof }`,
		"type":             `type integer int2str;`,
		"template":         `template integer str2int := 1;`,
	} {
		diags := NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, "module M {\n"+src+"\n}"))
		if !containsCode(diags, "predefined-function-name-reserved") {
			t.Errorf("%s: expected predefined-function-name-reserved, got %v", name, codes(diags))
		}
	}
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, `module M {
		type record R { integer replaced, charstring my_substr }
		function f_rnd() return float { return rnd() }
	}`))
	if containsCode(diags, "predefined-function-name-reserved") {
		t.Errorf("names that only contain a predefined function's: got %v", codes(diags))
	}
}

// TestOutArgumentBindsAVariable: a variable passed to an out parameter,
// a function's or decvalue's decoded value (C.5.2), is bound afterwards,
// so passing it on to an in parameter is fine; one never bound, or only
// passed to an inout parameter (which needs it bound, 5.4.2) or sent, is
// not.
func TestOutArgumentBindsAVariable(t *testing.T) {
	decls := `type record R { integer a }
		function f(R r) {}
		function g(out R r) { r := { a := 1 } }
		function k(inout R r) {}
		type port P message { inout R }
		type component C { port P p }`
	for name, body := range map[string]string{
		"decvalue_unichar": `var R j; var universal charstring u := "{}"; if (decvalue_unichar(u, j) == 0) { f(j) }`,
		"decvalue":         `var R j; var bitstring b := '0'B; if (decvalue(b, j) == 0) { f(j) }`,
		"a function's out": `var R j; g(j); f(j);`,
	} {
		diags := NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, "module M {\n"+decls+"\nfunction h() {\n"+body+"\n}\n}"))
		if containsCode(diags, "uninitialised-arg-to-in-inout") {
			t.Errorf("%s: got %v", name, codes(diags))
		}
	}
	for name, body := range map[string]string{
		"never bound":     `var R j; f(j);`,
		"passed to inout": `var R j; k(j); f(j);`,
		"sent":            `var R j; p.send(j); f(j);`,
	} {
		diags := NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, "module M {\n"+decls+"\nfunction h() runs on C {\n"+body+"\n}\n}"))
		if !containsCode(diags, "uninitialised-arg-to-in-inout") {
			t.Errorf("%s: expected uninitialised-arg-to-in-inout, got %v", name, codes(diags))
		}
	}
}

// TestOutArgumentOfAnUnresolvedCall: a function the module does not
// declare - imported, named with its module, a method - may have an out
// parameter, so a variable passed to it is taken as bound; int2enum's
// result is written to its out parameter (C.1.4). A label is a
// declaration too, and may not take a predefined function's name.
func TestOutArgumentOfAnUnresolvedCall(t *testing.T) {
	decls := `type record R { integer a }
		type enumerated E { e0, e1 }
		function f(R r) {}
		function fe(E e) {}`
	for name, body := range map[string]string{
		"imported":  `var R j; g_imported(j); f(j);`,
		"qualified": `var R j; Other.g(j); f(j);`,
		"int2enum":  `var E e; int2enum(0, e); fe(e);`,
	} {
		diags := NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, "module M {\n"+decls+"\nfunction h() {\n"+body+"\n}\n}"))
		if containsCode(diags, "uninitialised-arg-to-in-inout") {
			t.Errorf("%s: got %v", name, codes(diags))
		}
	}
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, "module M {\nfunction h() {\nlabel lengthof;\n}\n}"))
	if !containsCode(diags, "predefined-function-name-reserved") {
		t.Errorf("label: expected predefined-function-name-reserved, got %v", codes(diags))
	}
}

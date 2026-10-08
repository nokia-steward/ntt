package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestIdentifierShadowsComponentMember(t *testing.T) {
	tree := parse(t, `module M {
		type component C { const integer cl_int := 0 }
		testcase tc() runs on C {
			const integer cl_int := 0;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "identifier-shadows-component-member") {
		t.Fatalf("expected identifier-shadows-component-member, got %v", codes(diags))
	}
}

func TestIdentifierShadowsModuleDef(t *testing.T) {
	tree := parse(t, `module M {
		const integer c_int := 0;
		type component C {}
		function f() {
			const integer c_int := 0;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "identifier-shadows-module-def") {
		t.Fatalf("expected identifier-shadows-module-def, got %v", codes(diags))
	}
}

func TestIdentifierShadowsModuleName(t *testing.T) {
	tree := parse(t, `module Mname {
		type component C {}
		function f() {
			var boolean Mname := true;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "identifier-shadows-module-def") {
		t.Fatalf("expected identifier-shadows-module-def for module name, got %v", codes(diags))
	}
}

func TestDuplicateIdentifierInBody(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		function f() {
			var integer x := 1;
			var integer x := 2;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "duplicate-identifier-in-scope") {
		t.Fatalf("expected duplicate-identifier-in-scope, got %v", codes(diags))
	}
}

func TestFormalParamShadowedByLocal(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		function f(boolean x) {
			const integer x := 0;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "duplicate-identifier-in-scope") {
		t.Fatalf("expected duplicate-identifier-in-scope for param shadow, got %v", codes(diags))
	}
}

func TestPerScopeLoopsDoNotConflict(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		const integer cN := 3;
		function f() {
			for (var integer i := 0; i < cN; i := i + 1) {}
			for (var integer i := 0; i < cN; i := i + 1) {}
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "duplicate-identifier-in-scope") {
		t.Fatalf("unexpected duplicate-identifier-in-scope for sibling for-loops, got %v", codes(diags))
	}
}

func TestCallNonComponentReceiver(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		function f() runs on C {}
		testcase tc() runs on C system C {
			timer t;
			t.call(f());
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "call-on-non-component") {
		t.Fatalf("expected call-on-non-component, got %v", codes(diags))
	}
}

func TestCallForbiddenNestedParamType(t *testing.T) {
	tree := parse(t, `module M {
		type port P message { inout integer }
		type record R { P field1 }
		type component C {}
		function f(R p_par) runs on C {}
		testcase tc() runs on C system C {
			var C v_ptc := C.create;
			v_ptc.call(f({ field1 := null }));
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "call-forbidden-param-type") {
		t.Fatalf("expected call-forbidden-param-type via nested field, got %v", codes(diags))
	}
}

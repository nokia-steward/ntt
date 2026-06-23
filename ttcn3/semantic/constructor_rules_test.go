package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestConstructorOutParamRejected(t *testing.T) {
	tree := parse(t, `module M language "TTCN-3:2018 Object-Oriented" {
		type class C {
			var integer v_i;
			create(out integer v_i) { this.v_i := v_i; }
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "constructor-out-inout-param") {
		t.Fatalf("expected constructor-out-inout-param, got %v", codes(diags))
	}
}

func TestConstructorInParamAccepted(t *testing.T) {
	tree := parse(t, `module M language "TTCN-3:2018 Object-Oriented" {
		type class C {
			var integer v_i;
			create(in integer v_i) { this.v_i := v_i; }
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "constructor-out-inout-param") {
		t.Fatalf("unexpected diag, got %v", codes(diags))
	}
}

func TestClassFieldSelfInitRejected(t *testing.T) {
	tree := parse(t, `module M language "TTCN-3:2018 Object-Oriented" {
		type class C {
			var integer v_i := v_i + 1;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "class-field-self-init") {
		t.Fatalf("expected class-field-self-init, got %v", codes(diags))
	}
}

func TestClassFieldUninitRefRejected(t *testing.T) {
	tree := parse(t, `module M language "TTCN-3:2018 Object-Oriented" {
		type class C {
			var integer v_u;
			var integer v_i := v_u;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "class-field-uninit-ref") {
		t.Fatalf("expected class-field-uninit-ref, got %v", codes(diags))
	}
}

func TestClassFieldInitRefSiblingAccepted(t *testing.T) {
	tree := parse(t, `module M language "TTCN-3:2018 Object-Oriented" {
		type class C {
			var integer v_a := 1;
			var integer v_b := v_a + 1;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "class-field-uninit-ref") {
		t.Fatalf("unexpected diag, got %v", codes(diags))
	}
}

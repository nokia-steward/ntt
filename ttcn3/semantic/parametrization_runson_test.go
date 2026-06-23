package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestDefaultValueReferencesComponentMember_2016Strict(t *testing.T) {
	tree := parse(t, `module M language "TTCN-3:2016" {
		type component C { var integer vc_i := 0; }
		function f(in integer p := vc_i) runs on C { }
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "default-value-references-runs-on-member") {
		t.Fatalf("expected default-value-references-runs-on-member, got %v", codes(diags))
	}
}

func TestDefaultValueReferencesComponentMember_2017Allowed(t *testing.T) {
	// Same module without the explicit 2016 declarator: the
	// default language profile is 2017+, where this is allowed.
	tree := parse(t, `module M {
		type component C { var integer vc_i := 0; }
		function f(in integer p := vc_i) runs on C { }
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "default-value-references-runs-on-member") {
		t.Fatalf("expected no diagnostic on 2017+ module, got %v", codes(diags))
	}
}

func TestDefaultValueInvokesRunsOnFunction_2016Strict(t *testing.T) {
	tree := parse(t, `module M language "TTCN-3:2016" {
		type component C {}
		function fx() runs on C return integer { return 1; }
		function f(in integer p := fx()) runs on C { }
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "default-value-invokes-runs-on-function") {
		t.Fatalf("expected default-value-invokes-runs-on-function, got %v", codes(diags))
	}
}

func TestDefaultValueInvokesRunsOnFunction_2017Allowed(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		function fx() runs on C return integer { return 1; }
		function f(in integer p := fx()) runs on C { }
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "default-value-invokes-runs-on-function") {
		t.Fatalf("expected no diagnostic on 2017+ module, got %v", codes(diags))
	}
}

func TestDefaultValueOk_LiteralOnPre2017(t *testing.T) {
	// Default value that doesn't touch any component member or
	// runs-on function: should pass even under 2016 strict mode.
	tree := parse(t, `module M language "TTCN-3:2016" {
		type component C {}
		function f(in integer p := 42) runs on C { }
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	for _, d := range diags {
		switch d.Code {
		case "default-value-references-runs-on-member",
			"default-value-invokes-runs-on-function":
			t.Fatalf("unexpected diagnostic on literal default: %s", d.Message)
		}
	}
}

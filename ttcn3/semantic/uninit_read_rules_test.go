package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestDirectAssignmentFromUninitialisedVarRejected(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		testcase tc() runs on C {
			var integer i;
			var integer j;
			j := i;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "uninit-var-read") {
		t.Fatalf("expected uninit-var-read, got %v", codes(diags))
	}
}

func TestDirectAssignmentFromInitialisedVarAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		testcase tc() runs on C {
			var integer i := 1;
			var integer j;
			j := i;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "uninit-var-read") {
		t.Fatalf("unexpected uninit-var-read, got %v", codes(diags))
	}
}

func TestDirectMapAssignmentFromUninitialisedVarAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type map from integer to integer IntMap;
		type component C {}
		testcase tc() runs on C {
			var IntMap src;
			var IntMap dst := src;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "uninit-var-read") {
		t.Fatalf("unexpected uninit-var-read, got %v", codes(diags))
	}
}

func TestDirectTemplateAssignmentFromUninitialisedVarAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		testcase tc() runs on C {
			var template integer src;
			var template integer dst := src;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "uninit-var-read") {
		t.Fatalf("unexpected uninit-var-read, got %v", codes(diags))
	}
}

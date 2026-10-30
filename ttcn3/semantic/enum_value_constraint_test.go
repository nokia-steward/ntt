package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestEnumBooleanValueListRejected(t *testing.T) {
	// NegSem_06010202_ListOfTypes_008: boolean subtype only
	// accepts `false`; assigning `true` must be rejected.
	tree := parse(t, `module M {
		type boolean MyBoolean1 (false);
		type component C {}
		testcase tc() runs on C {
			var MyBoolean1 v;
			v := true;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "enum-value-out-of-list") {
		t.Fatalf("expected enum-value-out-of-list, got %v", codes(diags))
	}
}

func TestEnumBooleanValueListInitAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type boolean MyBoolean1 (false);
		type component C {}
		testcase tc() runs on C {
			var MyBoolean1 v := false;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "enum-value-out-of-list") {
		t.Fatalf("did not expect enum-value-out-of-list, got %v", codes(diags))
	}
}

func TestEnumVerdictListOfTypesRejected(t *testing.T) {
	// NegSem_06010202_ListOfTypes_009: verdict subtype that
	// unions (pass, error) and (inconc, none); `fail` is not in
	// either and must be rejected.
	tree := parse(t, `module M {
		type verdicttype Myverdict1 (pass, error);
		type verdicttype Myverdict2 (inconc, none);
		type verdicttype Myverdict_1_2 (Myverdict1, Myverdict2);
		type component C {}
		testcase tc() runs on C {
			var Myverdict_1_2 v;
			v := fail;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "enum-value-out-of-list") {
		t.Fatalf("expected enum-value-out-of-list, got %v", codes(diags))
	}
}

func TestEnumVerdictListOfTypesAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type verdicttype Myverdict1 (pass, error);
		type verdicttype Myverdict2 (inconc, none);
		type verdicttype Myverdict_1_2 (Myverdict1, Myverdict2);
		type component C {}
		testcase tc() runs on C {
			var Myverdict_1_2 v := error;
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "enum-value-out-of-list") {
		t.Fatalf("did not expect enum-value-out-of-list on union member, got %v", codes(diags))
	}
}

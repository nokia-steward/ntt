package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestAllFromWithAnyElementsInPermutationAccepted(t *testing.T) {
	// B.1.3.3 c: inside a permutation the all from source may be a
	// combined template holding `*`.
	tree := parse(t, `module M {
		type record of integer RoI;
		template RoI mw_roi1 := {1, 2, *};
		template RoI mw_roi2 := {permutation(0, all from mw_roi1), 4, 5};
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "all-from-source-has-matching-mechanism") {
		t.Fatalf("did not expect all-from-source-has-matching-mechanism, got %v", codes(diags))
	}
}

func TestAllFromSourceRestrictionsStillRejected(t *testing.T) {
	for _, tc := range []struct {
		name, src, code string
	}{
		{
			// NegSem_B010201_value_list_002: `*` outside a permutation.
			"value list", `module M {
				type record of integer RoI;
				template RoI t_source := {1, *};
				template integer t_int := (all from t_source);
			}`, "all-from-source-has-matching-mechanism",
		},
		{
			// NegSem_B010303_permutation_006: the source contains a
			// permutation.
			"nested permutation", `module M {
				type record of integer RoI;
				template RoI t_source := {1, permutation(2, 3)};
				template RoI t_perm1 := {permutation(all from t_source, *)};
			}`, "all-from-source-has-matching-mechanism",
		},
		{
			// NegSem_B010303_permutation_004: float members in an
			// integer permutation.
			"member type", `module M {
				type record of float RoF;
				type record of integer RoI;
				template RoF t_source := {?, *};
				template RoI t_perm1 := {permutation(3, all from t_source)};
			}`, "all-from-member-type-incompatible",
		},
	} {
		tree := parse(t, tc.src)
		diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
		if !containsCode(diags, tc.code) {
			t.Errorf("%s: expected %s, got %v", tc.name, tc.code, codes(diags))
		}
	}
}

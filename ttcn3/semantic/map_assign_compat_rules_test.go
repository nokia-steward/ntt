package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// charstring and universal charstring from and to types are compatible
// both ways (6.3.1, 6.3.8).
func TestMapAssignCharstringCompatible(t *testing.T) {
	tree := parse(t, `module M {
		type map from charstring to universal charstring Map1;
		type map from universal charstring to charstring Map2;
		type component C {}
		testcase TC() runs on C {
			var Map2 v_m2 := { ["key"] := "value" };
			var Map1 v_m1 := v_m2;
		}
	}`)
	if diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree); containsCode(diags, "map-assign-incompatible-map") {
		t.Fatalf("unexpected map-assign-incompatible-map: %v", codes(diags))
	}
}

// A key with a character charstring does not have indexes no charstring
// keyed map (6.3.1, 6.3.8).
func TestMapKeyCharRules(t *testing.T) {
	cases := []struct {
		name, decls string
		want        bool
	}{
		{"char quadruple constant", `const universal charstring c_key := char(0, 0, 0, 228);
			testcase TC() runs on C { var Map1 v_m1; var universal charstring v_x := v_m1[c_key]; }`, true},
		{"char quadruple", `testcase TC() runs on C { var Map1 v_m1; var universal charstring v_x := v_m1[char(0, 0, 1, 0)]; }`, true},
		{"ASCII constant", `const universal charstring c_key := "key";
			testcase TC() runs on C { var Map1 v_m1; var universal charstring v_x := v_m1[c_key]; }`, false},
		{"universal charstring keyed map", `const universal charstring c_key := char(0, 0, 0, 228);
			testcase TC() runs on C { var Map2 v_m2; var charstring v_x := v_m2[c_key]; }`, false},
	}
	for _, c := range cases {
		tree := parse(t, `module M {
			type map from charstring to universal charstring Map1;
			type map from universal charstring to charstring Map2;
			type component C {}
			`+c.decls+`
		}`)
		diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
		if has := containsCode(diags, "map-key-not-charstring"); has != c.want {
			t.Errorf("%s: map-key-not-charstring %v, want %v (%v)", c.name, has, c.want, codes(diags))
		}
	}
}

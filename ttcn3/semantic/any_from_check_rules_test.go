package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// any from on a single port is an error with an operation too (22.4).
func TestAnyFromCheckOnSinglePortRejected(t *testing.T) {
	tree := parse(t, `module M {
		type port MP message { inout integer }
		type component C { port MP p2 }
		function f() runs on C {
			alt { [] any from p2.check(receive(5)) { setverdict(pass); } }
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "any-from-non-port-array") {
		t.Fatalf("expected any-from-non-port-array, got %v", codes(diags))
	}
}

// The index target of the operation a check checks follows the rules of
// the operation's own (22.4 e, f): its type holds the highest index, and a
// multi-dimensional array's has as many elements as it has dimensions.
func TestAnyFromCheckIndexTarget(t *testing.T) {
	cases := []struct {
		name, decls, want string
	}{
		{"range too small", "type integer SmallIndex (0..2); type component C { port MP pa[5] }", "index-redirect-type-range"},
		{"multi-dim range too small", "type integer SmallIndex (0..1); type component C { port MP pa[2][4] }", "index-redirect-type-range"},
	}
	for _, c := range cases {
		tree := parse(t, `module M {
			type port MP message { inout integer }
			`+c.decls+`
			function f() runs on C {
				var SmallIndex v_idx;
				alt { [] any from pa.check(receive(5) -> @index value v_idx) { setverdict(pass); } }
			}
		}`)
		diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
		if !containsCode(diags, c.want) {
			t.Errorf("%s: expected %s, got %v", c.name, c.want, codes(diags))
		}
	}
	tree := parse(t, `module M {
		type port MP message { inout integer }
		type component C { port MP pa[2][2] }
		function f() runs on C {
			var integer v_idx[3];
			alt { [] any from pa.check(receive(5) -> @index value v_idx) { setverdict(pass); } }
		}
	}`)
	if diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree); len(diags) == 0 {
		t.Errorf("3 elements for a 2-dimensional port array: no diagnostic")
	}
	tree = parse(t, `module M {
		type port MP message { inout integer }
		type integer Index (0..4);
		type component C { port MP pa[5] }
		function f() runs on C {
			var Index v_idx;
			alt { [] any from pa.check(receive(5) -> @index value v_idx) { setverdict(pass); } }
		}
	}`)
	if diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree); containsCode(diags, "index-redirect-type-range") {
		t.Errorf("a type holding every index: %v", codes(diags))
	}
}

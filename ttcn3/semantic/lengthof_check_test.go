package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestLengthofUndeterminedLength(t *testing.T) {
	tests := []struct {
		arg  string
		want bool
	}{
		// C.2.1 EXAMPLE 2
		{`'1??1'B`, false},
		{`'1*F'H length(8)`, false},
		{`'00*FF'O length(1..2)`, false},
		{`'1*F'H`, true},
		// NegSem_160102_predefined_functions_008
		{`'1*0'B length(3..6)`, true},
		{`'1*0'B length(3..infinity)`, true},
		{`'1'B length(3..6)`, true},
	}
	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			tree := parse(t, `module M {
				type component C {}
				testcase TC() runs on C {
					var integer v_i := lengthof(`+tt.arg+`);
				}
			}`)
			diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
			if got := containsCode(diags, "lengthof-undetermined-length"); got != tt.want {
				t.Fatalf("lengthof-undetermined-length = %v, want %v; got %v", got, tt.want, codes(diags))
			}
		})
	}
}

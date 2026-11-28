package semantic

import "testing"

// TestStrayValueLiteralStmt: a literal alone at statement position is a
// syntax residue, but the not-implemented function `???` is a valid
// stand-alone statement (16.1.6).
func TestStrayValueLiteralStmt(t *testing.T) {
	const code = "stray-value-literal-stmt"
	for _, tt := range []struct {
		name, src string
		reject    bool
	}{
		{"stray literal", `module M {
			type component C { timer t; }
			function f() runs on C { t.start; 1.0; }
		}`, true},
		{"not-implemented statement", `module M {
			function f() { ???; ???("todo"); }
		}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := hasCode(analyzeCodes(t, tt.src), code)
			if got != tt.reject {
				t.Errorf("%s reported: %v, want %v", code, got, tt.reject)
			}
		})
	}
}

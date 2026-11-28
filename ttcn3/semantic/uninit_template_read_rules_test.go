package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestUninitTemplateRead(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		reject     bool
	}{
		{"template variable", `var template integer u; var template integer x := u;`, true},
		{"local template definition", `var template integer u; template integer m := u;`, true},
		{"initialised first", `var template integer u := ?; template integer m := u;`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tree := parse(t, "module M { type component C {} testcase TC() runs on C { "+tt.body+" } }")
			got := containsCode(NewAnalyzer(&ttcn3.DB{}).Analyze(tree), "uninit-template-read")
			if got != tt.reject {
				t.Errorf("uninit-template-read reported: %v, want %v", got, tt.reject)
			}
		})
	}
}

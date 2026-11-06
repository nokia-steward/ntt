package semantic

import (
	"fmt"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// TestPortTypeWithoutKindIsAMessagePort: in TTCN-3 V5 the kind of a port
// type may be left out, and such a port is a message port (ETSI TR 104
// 081 6.5.4): checked as one declared `message`.
func TestPortTypeWithoutKindIsAMessagePort(t *testing.T) {
	var got [2][]string
	for i, kind := range []string{"message ", ""} {
		tree := parse(t, `module M {
			signature S();
			type port P `+kind+`{ inout integer }
			type component C { port P p }
			function f() runs on C { p.send(1); p.call(S:{}, nowait) }
		}`)
		got[i] = codes(NewAnalyzer(&ttcn3.DB{}).Analyze(tree))
	}
	if fmt.Sprint(got[0]) != fmt.Sprint(got[1]) || !contains(got[1], "port-op-wrong-port-kind") {
		t.Errorf("declared message: %v; without its kind: %v", got[0], got[1])
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// TestTemplateAttribute: only a module has a template attribute, "static"
// or "dynamic" (ETSI TR 104 081 6.13).
func TestTemplateAttribute(t *testing.T) {
	for src, want := range map[string]string{
		`module M {} with { template "dynamic" }`:                              "",
		`module M {} with { template "static" }`:                               "",
		`module M {} with { template "lazy" }`:                                 "template-attribute-value",
		`module M { type record R { integer a } with { template "dynamic" } }`: "template-attribute-not-on-module",
		`module M { template integer t := 1 with { template "dynamic" } }`:     "template-attribute-not-on-module",
	} {
		diags := NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, src))
		for _, code := range []string{"template-attribute-value", "template-attribute-not-on-module"} {
			if got := containsCode(diags, code); got != (code == want) {
				t.Errorf("%s: %s reported %v, got %v", src, code, got, codes(diags))
			}
		}
	}
}

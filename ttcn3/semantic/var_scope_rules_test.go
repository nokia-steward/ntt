package semantic

import "testing"

// scopeModule is a module whose function declares line in the block of an
// if, after body.
func scopeModule(body string) string {
	return `module M {
		type record R { charstring line }
		function g(charstring line) return R { return { line := line } }
		type port P message { inout R }
		type component C { port P p }
		function f(boolean b) runs on C {
			var R r := g("a");
			` + body + `
			if (b) { var charstring line := "b"; log(line) }
		}
	}`
}

// A field named after a variable declared in an inner block is not that
// variable: neither the field selected after a dot, nor the field or the
// parameter named on the left of := in a value list or an argument list
// (ETSI ES 201 873-1 5.2, 6.2.1, 5.4.2), nor the field a redirect names
// (22.2.2).
func TestVarScopeFieldNames(t *testing.T) {
	for name, body := range map[string]string{
		"a field selected":           `log(r.line); if (lengthof(r.line) > 0) { log(r.line) }`,
		"a field of a value list":    `r := { line := "c" };`,
		"a parameter of a call":      `r := g(line := "c");`,
		"a field of a nested record": `log(g("c").line);`,
		"a field redirected":         `var charstring v; p.receive(R:?) -> value (v := line);`,
	} {
		if codes := analyzeCodes(t, scopeModule(body)); hasCode(codes, "var-out-of-scope") {
			t.Errorf("%s: unexpected var-out-of-scope", name)
		}
	}
}

// The variable itself, read after its block closed, is still reported.
func TestVarScopeOutside(t *testing.T) {
	for name, body := range map[string]string{
		"a read":                 `if (b) { var charstring s := "c" } log(s);`,
		"a read in a value list": `if (b) { var charstring s := "c" } r := { line := s };`,
		"a read in a call":       `if (b) { var charstring s := "c" } r := g(line := s);`,
		"a read selected from":   `if (b) { var R s := r } log(s.line);`,
		"a redirect into it":     `if (b) { var charstring s := "c" } p.receive(R:?) -> value (s := line);`,
	} {
		if codes := analyzeCodes(t, scopeModule(body)); !hasCode(codes, "var-out-of-scope") {
			t.Errorf("%s: want var-out-of-scope, got %v", name, codes)
		}
	}
}

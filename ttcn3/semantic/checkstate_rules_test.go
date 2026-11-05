package semantic

import "testing"

// checkstate takes one of six charstrings (ETSI 22.5.5 a), on a port
// (22.5.5 c). A literal argument is checked statically; an expression
// waits for execution.
func TestCheckstateRules(t *testing.T) {
	for _, tt := range []struct {
		call, code string
	}{
		{`p1.checkstate("Started")`, ""},
		{`p1.checkstate("Linked")`, ""},
		{`all port.checkstate("Halted")`, ""},
		{`any port.checkstate("Start" & "ed")`, ""},
		{`p1.checkstate(v_s)`, ""},
		{`p1.checkstate("Running")`, "checkstate-invalid-state"},
		{`p1.checkstate("started")`, "checkstate-invalid-state"},
		{`p1.checkstate(1)`, "checkstate-invalid-state"},
		{`v_i.checkstate("Started")`, "port-op-non-port-receiver"},
	} {
		codes := analyzeCodes(t, `module M {
			type port P message { inout integer }
			type component C { port P p1 }
			testcase tc() runs on C {
				var charstring v_s := "Mapped";
				var integer v_i := 1;
				var boolean b := `+tt.call+`;
			}
		}`)
		if tt.code == "" {
			if hasCode(codes, "checkstate-invalid-state") || hasCode(codes, "port-op-non-port-receiver") {
				t.Errorf("%s: unexpected %v", tt.call, codes)
			}
			continue
		}
		if !hasCode(codes, tt.code) {
			t.Errorf("%s: want %s, got %v", tt.call, tt.code, codes)
		}
	}
}

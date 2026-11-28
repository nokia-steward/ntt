package semantic

import "testing"

// The top alternatives of an interleave altstep follow the restrictions
// of the interleave statement (ETSI 16.2.0 g, 20.4); a normal altstep's
// don't.
func TestInterleaveAltstepRestrictions(t *testing.T) {
	body := `{
		[] p.receive(integer:1) { repeat }
		[] p.receive(integer:2) { }
	}`
	src := func(kind string) string {
		return `module M {
			type port P message { inout integer }
			type component C { port P p }
			altstep ` + kind + ` a() runs on C ` + body + `
		}`
	}
	if codes := analyzeCodes(t, src("interleave")); !hasCode(codes, "interleave-forbidden-branch") {
		t.Errorf("interleave altstep: want interleave-forbidden-branch, got %v", codes)
	}
	if codes := analyzeCodes(t, src("")); hasCode(codes, "interleave-forbidden-branch") {
		t.Errorf("normal altstep: unexpected interleave-forbidden-branch in %v", codes)
	}
	for code, alt := range map[string]string{
		"interleave-guard-not-empty": "[x > 0] p.receive(integer:1) { }",
		"interleave-else-forbidden":  "[else] { }",
	} {
		codes := analyzeCodes(t, `module M {
			type port P message { inout integer }
			type component C { port P p }
			altstep interleave a(integer x) runs on C {
				`+alt+`
				[] p.receive(integer:2) { }
			}
		}`)
		if !hasCode(codes, code) {
			t.Errorf("%s: want %s, got %v", alt, code, codes)
		}
	}
}

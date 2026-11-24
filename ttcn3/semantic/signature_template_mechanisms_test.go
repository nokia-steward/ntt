package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// TestSignatureTemplateMechanisms: value lists, ranges and other
// matching mechanisms, directly or through a template, in a slot of a
// call or reply that needs a specific value (15.2, 22.3).
func TestSignatureTemplateMechanisms(t *testing.T) {
	const decls = `signature S(in integer p1, out integer p2, inout integer p3) return integer;
		type port P procedure { inout S } type component C { port P p }
		template integer mw_range := (1 .. 5); template integer mw_any := ?;`
	bad := []string{
		`p.call(S:{ p1 := (1, 2), p2 := -, p3 := 3 }, nowait);`,
		`p.call(S:{ p1 := mw_range, p2 := -, p3 := 3 }, nowait);`,
		`p.call(S:{ p1 := complement(1), p2 := -, p3 := 3 }, nowait);`,
		`p.reply(S:{ p1 := -, p2 := ?, p3 := 30 } value 1);`,
		`p.reply(S:{ p1 := -, p2 := 2, p3 := mw_any } value 1);`,
		`p.reply(S:{ p1 := -, p2 := 2, p3 := 3 } value ?);`,
	}
	for _, body := range bad {
		src := `module M { ` + decls + ` testcase tc() runs on C { ` + body + ` } }`
		if diags := NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, src)); !containsCode(diags, "signature-template-matching-not-allowed") {
			t.Errorf("%s: expected signature-template-matching-not-allowed, got %v", body, codes(diags))
		}
	}
	good := `module M { ` + decls + ` testcase tc() runs on C {
		p.call(S:{ p1 := 1, p2 := (1 .. 5), p3 := 3 }, nowait);
		p.reply(S:{ p1 := (1, 2), p2 := 2, p3 := 3 } value 1);
		p.getreply(S:{ p1 := -, p2 := ?, p3 := (1 .. 2) } value ?);
	} }`
	if diags := NewAnalyzer(&ttcn3.DB{}).Analyze(parse(t, good)); containsCode(diags, "signature-template-matching-not-allowed") {
		t.Errorf("valid module: unexpected %v", codes(diags))
	}
}

// omit of an optional field inside an in parameter's value is a value,
// not a matching mechanism (15.2 c).
func TestSignatureTemplateOmitInField(t *testing.T) {
	tree := parse(t, `module M {
		type record Rec { integer field1 optional, charstring field2 }
		signature RecProc(in Rec Par1);
		type port P procedure { inout RecProc }
		type component C { port P p }
		template RecProc s_explicit := { Par1 := { field1 := omit, field2 := "a" } }
		function f() runs on C { p.call(s_explicit, nowait); }
	}`)
	for _, d := range NewAnalyzer(&ttcn3.DB{}).Analyze(tree) {
		if d.Code == "signature-template-matching-not-allowed" {
			t.Errorf("unexpected %s: %s", d.Code, d.Message)
		}
	}
}

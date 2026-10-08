package semantic

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func codes(diags []Diagnostic) []string {
	out := make([]string, 0, len(diags))
	for _, d := range diags {
		out = append(out, d.Code)
	}
	return out
}

func containsCode(diags []Diagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

func TestAnalyze_UnknownComponent(t *testing.T) {
	tree := parse(t, `module M {
		function f() runs on MissingComp { }
	}`)
	diags := NewAnalyzer(&ttcn3.DB{Modules: map[string]map[string]bool{}}).Analyze(tree)
	if !containsCode(diags, "unknown-component") {
		t.Fatalf("expected unknown-component diagnostic, got %v", codes(diags))
	}
}

func TestAnalyze_KnownComponent(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		function f() runs on C { }
	}`)
	diags := NewAnalyzer(&ttcn3.DB{Modules: map[string]map[string]bool{}}).Analyze(tree)
	if containsCode(diags, "unknown-component") {
		t.Fatalf("did not expect unknown-component, got %v", codes(diags))
	}
}

func TestAnalyze_UnknownEncodingWarning(t *testing.T) {
	tree := parse(t, `module M {
		type integer T with { encode "ZIM" }
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	var found *Diagnostic
	for i := range diags {
		if diags[i].Code == "attr.unknown-encoding" {
			found = &diags[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected attr.unknown-encoding diagnostic, got %v", codes(diags))
	}
	if found.Severity != SeverityWarn {
		t.Errorf("severity = %d, want SeverityWarn(%d)", found.Severity, SeverityWarn)
	}
	if !strings.Contains(found.Message, "ZIM") {
		t.Errorf("message %q does not mention the encoding", found.Message)
	}
}

func TestAnalyze_EmptyEncoding(t *testing.T) {
	tree := parse(t, `module M {
		type integer T with { encode "" }
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "attr.empty-encoding") {
		t.Fatalf("expected attr.empty-encoding, got %v", codes(diags))
	}
}

func TestAnalyze_KnownEncodingsSilent(t *testing.T) {
	for _, enc := range []string{"RAW", "JSON", "XML", "BER", "OER", "PER"} {
		tree := parse(t, `module M { type integer T with { encode "`+enc+`" } }`)
		diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
		if containsCode(diags, "attr.unknown-encoding") {
			t.Errorf("encoding %q raised unknown-encoding", enc)
		}
	}
}

func TestAnalyze_ReturnMissingValue(t *testing.T) {
	tree := parse(t, `module M {
		function f() return integer { return; }
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "return.missing-value") {
		t.Fatalf("expected return.missing-value, got %v", codes(diags))
	}
}

func TestAnalyze_ReturnUnexpectedValue(t *testing.T) {
	tree := parse(t, `module M {
		function f() { return 7; }
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "return.unexpected-value") {
		t.Fatalf("expected return.unexpected-value, got %v", codes(diags))
	}
}

func TestAnalyze_ReturnConsistent(t *testing.T) {
	tree := parse(t, `module M {
		function f() return integer { return 7; }
		function g() { return; }
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	for _, d := range diags {
		if strings.HasPrefix(d.Code, "return.") {
			t.Errorf("unexpected diagnostic %+v", d)
		}
	}
}

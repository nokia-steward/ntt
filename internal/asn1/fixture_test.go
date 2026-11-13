package asn1

import (
	"os"
	"strings"
	"testing"

	"github.com/nokia/ntt/internal/asn1/ast"
)

// TestFixture_SampleASN1 exercises the parser end-to-end against a
// non-trivial ASN.1 module that hits the features the new frontend
// should support. The intent is to catch regressions; the
// per-production tests in parser_test.go and parser_class_test.go are
// the source of truth for any single feature.
func TestFixture_SampleASN1(t *testing.T) {
	src, err := os.ReadFile("testdata/sample.asn")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	m := ParseModule(src)
	if m == nil || m.Identifier.Name != "RRC-Sample" {
		t.Fatalf("module name: %+v", m.Identifier)
	}
	if m.Tagging != ast.TagsAutomatic {
		t.Errorf("tagging: %v want AUTOMATIC", m.Tagging)
	}
	if m.Extensible {
		t.Errorf("module should not be extensibility implied")
	}
	if len(m.Imports) != 1 || m.Imports[0].From != "Common" {
		t.Errorf("imports: %+v", m.Imports)
	}
	if m.Exports == nil || len(m.Exports.Symbols) != 1 || m.Exports.Symbols[0] != "Status" {
		t.Errorf("exports: %+v", m.Exports)
	}

	want := []string{
		"Status", "Counter", "Person", "Names", "Tagged",
		"Pair", "IntPair", "Reply", "maxRetries",
	}
	got := map[string]bool{}
	for _, a := range m.Assignments {
		got[ast.AssignmentName(a)] = true
	}
	for _, n := range want {
		if !got[n] {
			t.Errorf("missing assignment %q in %+v", n, got)
		}
	}

	// No diagnostics from such a clean fixture.
	for _, d := range m.Diagnostics {
		t.Errorf("unexpected diag: %s", d.Message)
	}

	if !strings.Contains(string(src), "EXPORTS") {
		t.Fatal("fixture lost EXPORTS clause unexpectedly")
	}
}

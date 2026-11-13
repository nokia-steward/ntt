package asn1

import "testing"

func TestParse_HeaderAndImports(t *testing.T) {
	const src = `RRC-PDU-Definitions {
    itu-t (0) identified-organization (4) etsi (0) mobileDomain (0)
    umts-Access (20) modules (3) rrc (1) version-22 (22)
} DEFINITIONS AUTOMATIC TAGS ::=

BEGIN

IMPORTS
    NR-RRC-Defs ,
    SetupRelease
FROM Common ;

MyEnum ::= ENUMERATED { red, green, blue }
myValue MyEnum ::= red
END`

	m := Parse([]byte(src))
	if m.Name != "RRC-PDU-Definitions" {
		t.Errorf("got module name %q, want RRC-PDU-Definitions", m.Name)
	}
	if m.TaggingDefault != "AUTOMATIC" {
		t.Errorf("got tagging %q, want AUTOMATIC", m.TaggingDefault)
	}
	if len(m.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(m.Imports))
	}
	imp := m.Imports[0]
	if imp.From != "Common" {
		t.Errorf("got import source %q, want Common", imp.From)
	}
	wantSyms := map[string]bool{"NR-RRC-Defs": true, "SetupRelease": true}
	for _, s := range imp.Symbols {
		if !wantSyms[s] {
			t.Errorf("unexpected imported symbol %q", s)
		}
	}
	if len(m.Assignments) < 2 {
		t.Fatalf("expected at least 2 assignments, got %d: %v", len(m.Assignments), m.Assignments)
	}
}

func TestHasAssignment_WhenNoExports(t *testing.T) {
	m := Parse([]byte(`M DEFINITIONS ::= BEGIN
		Foo ::= INTEGER
	END`))
	if !m.HasAssignment("Foo") {
		t.Fatalf("expected Foo to be exported without EXPORTS clause")
	}
	if m.HasAssignment("Bar") {
		t.Fatalf("Bar should not be reported as exported")
	}
}

func TestParse_TolerantOfGarbage(t *testing.T) {
	m := Parse([]byte("this is not valid ASN.1"))
	if m == nil {
		t.Fatal("Parse must always return a non-nil module")
	}
	if len(m.Diagnostics) == 0 {
		t.Fatal("expected at least one diagnostic for invalid input")
	}
}

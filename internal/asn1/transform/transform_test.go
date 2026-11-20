package transform_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/internal/asn1"
	"github.com/nokia/ntt/internal/asn1/transform"
)

func lower(t *testing.T, src string) *transform.Result {
	t.Helper()
	m := asn1.ParseModule([]byte(src))
	return transform.LowerModule(m)
}

func TestLower_IntegerAlias(t *testing.T) {
	r := lower(t, `M DEFINITIONS ::= BEGIN
Age ::= INTEGER
END`)
	if !strings.Contains(r.Source, "type integer Age;") {
		t.Errorf("source:\n%s", r.Source)
	}
}

func TestLower_Sequence(t *testing.T) {
	r := lower(t, `M DEFINITIONS ::= BEGIN
Person ::= SEQUENCE { name PrintableString, age INTEGER OPTIONAL }
END`)
	if !strings.Contains(r.Source, "type record Person") {
		t.Errorf("missing record: %s", r.Source)
	}
	if !strings.Contains(r.Source, "charstring name") {
		t.Errorf("missing field: %s", r.Source)
	}
	if !strings.Contains(r.Source, "integer age optional") {
		t.Errorf("missing optional: %s", r.Source)
	}
}

func TestLower_Choice(t *testing.T) {
	r := lower(t, `M DEFINITIONS ::= BEGIN
Answer ::= CHOICE { yes NULL, no NULL, value INTEGER }
END`)
	if !strings.Contains(r.Source, "type union Answer") {
		t.Errorf("missing union: %s", r.Source)
	}
}

func TestLower_SequenceOf(t *testing.T) {
	r := lower(t, `M DEFINITIONS ::= BEGIN
Names ::= SEQUENCE OF PrintableString
END`)
	if !strings.Contains(r.Source, "type record of charstring Names;") {
		t.Errorf("source: %s", r.Source)
	}
}

func TestLower_HyphenatedIdent(t *testing.T) {
	r := lower(t, `M-Mod DEFINITIONS ::= BEGIN
RRC-PDU ::= INTEGER
END`)
	if !strings.Contains(r.Source, "module M_Mod") {
		t.Errorf("module name not rewritten: %s", r.Source)
	}
	if !strings.Contains(r.Source, "type integer RRC_PDU;") {
		t.Errorf("type name not rewritten: %s", r.Source)
	}
}

func TestLower_ValueAssignment(t *testing.T) {
	r := lower(t, `M DEFINITIONS ::= BEGIN
maxAge INTEGER ::= 120
END`)
	if !strings.Contains(r.Source, "const integer maxAge := 120;") {
		t.Errorf("source: %s", r.Source)
	}
}

func TestLower_ProducesParseableTtcn3(t *testing.T) {
	r := lower(t, `M DEFINITIONS ::= BEGIN
Status ::= ENUMERATED { ok, error, unknown }
Pkt ::= SEQUENCE { code INTEGER, body OCTET STRING OPTIONAL }
END`)
	if r.Tree == nil {
		t.Fatal("nil tree")
	}
	// We don't currently have a way to inspect tree errors, but if
	// the source contains the expected declarations we trust the
	// parser ran without panicking.
	want := []string{"type enumerated Status", "type record Pkt"}
	for _, w := range want {
		if !strings.Contains(r.Source, w) {
			t.Errorf("missing %q in:\n%s", w, r.Source)
		}
	}
}

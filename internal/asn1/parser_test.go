package asn1

import (
	"testing"

	"github.com/nokia/ntt/internal/asn1/ast"
)

func mustParse(t *testing.T, src string) *ast.Module {
	t.Helper()
	m := ParseModule([]byte(src))
	if m == nil {
		t.Fatal("ParseModule returned nil")
	}
	for _, d := range m.Diagnostics {
		t.Logf("parse diag: %s", d.Message)
	}
	return m
}

func TestParser_Module_Empty(t *testing.T) {
	m := mustParse(t, `Empty DEFINITIONS ::= BEGIN END`)
	if m.Identifier.Name != "Empty" {
		t.Errorf("got module name %q, want Empty", m.Identifier.Name)
	}
	if len(m.Assignments) != 0 {
		t.Errorf("expected no assignments, got %d", len(m.Assignments))
	}
}

func TestParser_TypeAssignment_Integer(t *testing.T) {
	m := mustParse(t, `M DEFINITIONS ::= BEGIN Age ::= INTEGER END`)
	if len(m.Assignments) != 1 {
		t.Fatalf("got %d assignments", len(m.Assignments))
	}
	ta, ok := m.Assignments[0].(*ast.TypeAssignment)
	if !ok {
		t.Fatalf("got %T, want *TypeAssignment", m.Assignments[0])
	}
	if ta.Name != "Age" {
		t.Errorf("name: got %q want Age", ta.Name)
	}
	bt, ok := ta.Type.(*ast.IntegerType)
	if !ok {
		t.Fatalf("type: got %T want *IntegerType", ta.Type)
	}
	if len(bt.NamedNumbers) != 0 {
		t.Errorf("got %d named numbers", len(bt.NamedNumbers))
	}
}

func TestParser_NamedNumbers(t *testing.T) {
	m := mustParse(t, `M DEFINITIONS ::= BEGIN
Status ::= INTEGER { ok(0), error(-1), unknown(255) }
END`)
	ta := m.Assignments[0].(*ast.TypeAssignment)
	it := ta.Type.(*ast.IntegerType)
	if len(it.NamedNumbers) != 3 {
		t.Fatalf("got %d named numbers", len(it.NamedNumbers))
	}
	if it.NamedNumbers[0].Name != "ok" || it.NamedNumbers[2].Name != "unknown" {
		t.Errorf("names: %+v", it.NamedNumbers)
	}
}

func TestParser_Enumerated(t *testing.T) {
	m := mustParse(t, `M DEFINITIONS ::= BEGIN
Color ::= ENUMERATED { red, green(2), blue, ..., yellow }
END`)
	ta := m.Assignments[0].(*ast.TypeAssignment)
	et := ta.Type.(*ast.EnumeratedType)
	if len(et.Items) != 3 || !et.Extensible || len(et.Extensions) != 1 {
		t.Fatalf("items=%d ext=%v exts=%d", len(et.Items), et.Extensible, len(et.Extensions))
	}
	if et.Items[1].Name != "green" {
		t.Errorf("got %q want green", et.Items[1].Name)
	}
	if et.Extensions[0].Name != "yellow" {
		t.Errorf("got %q want yellow", et.Extensions[0].Name)
	}
}

func TestParser_Sequence(t *testing.T) {
	m := mustParse(t, `M DEFINITIONS ::= BEGIN
Person ::= SEQUENCE {
    name    PrintableString,
    age     INTEGER OPTIONAL,
    weight  INTEGER DEFAULT 0,
    ...
}
END`)
	ta := m.Assignments[0].(*ast.TypeAssignment)
	st := ta.Type.(*ast.SequenceType)
	if !st.Extensible {
		t.Errorf("expected extensible")
	}
	if len(st.Components) != 3 {
		t.Fatalf("got %d components", len(st.Components))
	}
	if !st.Components[1].Optional {
		t.Error("age should be optional")
	}
	if st.Components[2].Default == nil {
		t.Error("weight should have default")
	}
}

func TestParser_SequenceOf(t *testing.T) {
	m := mustParse(t, `M DEFINITIONS ::= BEGIN
Names ::= SEQUENCE OF PrintableString
END`)
	ta := m.Assignments[0].(*ast.TypeAssignment)
	st, ok := ta.Type.(*ast.SequenceOfType)
	if !ok {
		t.Fatalf("got %T want *SequenceOfType", ta.Type)
	}
	if bt, ok := st.Element.(*ast.BuiltinType); !ok || bt.Kind != ast.PrintableString {
		t.Errorf("element: got %v want PrintableString", st.Element)
	}
}

func TestParser_Choice(t *testing.T) {
	m := mustParse(t, `M DEFINITIONS ::= BEGIN
Answer ::= CHOICE { yes NULL, no NULL, maybe INTEGER }
END`)
	ta := m.Assignments[0].(*ast.TypeAssignment)
	ct := ta.Type.(*ast.ChoiceType)
	if len(ct.Alternatives) != 3 {
		t.Fatalf("got %d alternatives", len(ct.Alternatives))
	}
}

func TestParser_TaggedType(t *testing.T) {
	m := mustParse(t, `M DEFINITIONS ::= BEGIN
Foo ::= [APPLICATION 5] IMPLICIT INTEGER
END`)
	ta := m.Assignments[0].(*ast.TypeAssignment)
	tt := ta.Type.(*ast.TaggedType)
	if tt.Tag.Class != ast.ApplicationTag {
		t.Errorf("got class %v", tt.Tag.Class)
	}
	if tt.Tag.Mode != ast.TagModeImplicit {
		t.Errorf("got mode %v", tt.Tag.Mode)
	}
}

func TestParser_ValueAssignment(t *testing.T) {
	m := mustParse(t, `M DEFINITIONS ::= BEGIN
maxAge INTEGER ::= 120
greeting PrintableString ::= "hello"
END`)
	if len(m.Assignments) != 2 {
		t.Fatalf("got %d assignments", len(m.Assignments))
	}
	va := m.Assignments[0].(*ast.ValueAssignment)
	if va.Name != "maxAge" {
		t.Errorf("name: %q", va.Name)
	}
	if _, ok := va.Value.(*ast.IntegerValue); !ok {
		t.Errorf("got %T want *IntegerValue", va.Value)
	}
}

func TestParser_Constraint_SizeRange(t *testing.T) {
	m := mustParse(t, `M DEFINITIONS ::= BEGIN
Octets ::= OCTET STRING (SIZE (1..16))
Small ::= INTEGER (0..255)
END`)
	if len(m.Assignments) != 2 {
		t.Fatalf("got %d", len(m.Assignments))
	}
	oct := m.Assignments[0].(*ast.TypeAssignment).Type.(*ast.ConstrainedType)
	if _, ok := oct.Inner.(*ast.BuiltinType); !ok {
		t.Errorf("inner: %T", oct.Inner)
	}
	if oct.Constraint == nil {
		t.Fatal("missing constraint")
	}
	small := m.Assignments[1].(*ast.TypeAssignment).Type.(*ast.ConstrainedType)
	if small.Constraint == nil || small.Constraint.Set == nil || len(small.Constraint.Set.Root) == 0 {
		t.Fatal("missing constraint set")
	}
	rng, ok := small.Constraint.Set.Root[0][0].(*ast.ValueRangeConstraint)
	if !ok {
		t.Fatalf("got %T want *ValueRangeConstraint", small.Constraint.Set.Root[0][0])
	}
	if rng.Lower == nil || rng.Upper == nil {
		t.Error("range endpoints missing")
	}
}

func TestParser_HeaderAndImports(t *testing.T) {
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
	m := mustParse(t, src)
	if m.Identifier.Name != "RRC-PDU-Definitions" {
		t.Errorf("module: %q", m.Identifier.Name)
	}
	if m.Tagging != ast.TagsAutomatic {
		t.Errorf("tagging: %v", m.Tagging)
	}
	if len(m.Imports) != 1 {
		t.Fatalf("imports: %d", len(m.Imports))
	}
	imp := m.Imports[0]
	if imp.From != "Common" {
		t.Errorf("from: %q", imp.From)
	}
	wantSyms := map[string]bool{"NR-RRC-Defs": true, "SetupRelease": true}
	for _, s := range imp.Symbols {
		if !wantSyms[s] {
			t.Errorf("unexpected symbol %q", s)
		}
	}
	if len(m.Assignments) != 2 {
		t.Fatalf("got %d assignments", len(m.Assignments))
	}
}

func TestParser_TolerantOfGarbage(t *testing.T) {
	m := ParseModule([]byte("this is not valid ASN.1"))
	if m == nil {
		t.Fatal("nil module")
	}
	if len(m.Diagnostics) == 0 {
		t.Fatal("expected diagnostics")
	}
}

func TestParser_RecoversAcrossAssignments(t *testing.T) {
	m := mustParse(t, `M DEFINITIONS ::= BEGIN
Bad ::= !!!nonsense
Good ::= INTEGER
END`)
	// We don't care about Bad; we want Good to still be discovered.
	foundGood := false
	for _, a := range m.Assignments {
		if a, ok := a.(*ast.TypeAssignment); ok && a.Name == "Good" {
			foundGood = true
		}
	}
	if !foundGood {
		t.Errorf("did not recover to Good; assignments=%v", m.Assignments)
	}
}

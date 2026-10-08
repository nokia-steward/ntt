package asn1

import (
	"testing"

	"github.com/nokia/ntt/internal/asn1/ast"
)

func TestParser_ObjectClass_Basic(t *testing.T) {
	m := mustParse(t, `M DEFINITIONS ::= BEGIN
ERROR ::= CLASS {
    &errorCode    INTEGER UNIQUE,
    &Type         OPTIONAL,
    &description  PrintableString OPTIONAL
} WITH SYNTAX {
    CODE &errorCode
    [TYPE &Type]
    [DESCRIPTION &description]
}
END`)
	if len(m.Assignments) != 1 {
		t.Fatalf("got %d assignments", len(m.Assignments))
	}
	oc, ok := m.Assignments[0].(*ast.ObjectClassAssignment)
	if !ok {
		t.Fatalf("got %T want *ObjectClassAssignment", m.Assignments[0])
	}
	if oc.Name != "ERROR" {
		t.Errorf("name: %q", oc.Name)
	}
	if oc.Class == nil {
		t.Fatal("missing class body")
	}
	if got := len(oc.Class.Fields); got != 3 {
		t.Errorf("fields: %d want 3", got)
	}
	if oc.Class.WithSyntax == nil {
		t.Fatal("missing WITH SYNTAX")
	}
	if len(oc.Class.WithSyntax.Tokens) < 3 {
		t.Errorf("WITH SYNTAX tokens: %d", len(oc.Class.WithSyntax.Tokens))
	}
}

func TestParser_TableConstraint(t *testing.T) {
	m := mustParse(t, `M DEFINITIONS ::= BEGIN
Container ::= SEQUENCE {
    code  INTEGER ({MySet}),
    data  OCTET STRING ({MySet}{@code})
}
END`)
	if len(m.Assignments) != 1 {
		t.Fatalf("got %d assignments", len(m.Assignments))
	}
	st := m.Assignments[0].(*ast.TypeAssignment).Type.(*ast.SequenceType)
	if len(st.Components) != 2 {
		t.Fatalf("got %d components", len(st.Components))
	}
	first := st.Components[0].Type.(*ast.ConstrainedType)
	tc, ok := first.Constraint.Set.Root[0][0].(*ast.TableConstraint)
	if !ok {
		t.Fatalf("first component: got %T want *TableConstraint", first.Constraint.Set.Root[0][0])
	}
	if tc.ObjectSet == nil {
		t.Error("missing object set")
	}
	second := st.Components[1].Type.(*ast.ConstrainedType)
	tc2, ok := second.Constraint.Set.Root[0][0].(*ast.TableConstraint)
	if !ok {
		t.Fatalf("second component: got %T want *TableConstraint", second.Constraint.Set.Root[0][0])
	}
	if tc2.AtNotation == nil || len(tc2.AtNotation.Path) != 1 || tc2.AtNotation.Path[0] != "code" {
		t.Errorf("@notation: %+v", tc2.AtNotation)
	}
}

func TestParser_ParameterizedTypeAssignment(t *testing.T) {
	m := mustParse(t, `M DEFINITIONS ::= BEGIN
Pair { ItemType } ::= SEQUENCE { first ItemType, second ItemType }
END`)
	if len(m.Assignments) != 1 {
		t.Fatalf("got %d", len(m.Assignments))
	}
	ta := m.Assignments[0].(*ast.TypeAssignment)
	if ta.Params == nil || len(ta.Params.Params) != 1 {
		t.Fatalf("params: %+v", ta.Params)
	}
	if ta.Params.Params[0].Reference != "ItemType" {
		t.Errorf("param name: %q", ta.Params.Params[0].Reference)
	}
}

func TestParser_AmpRefFieldAccess(t *testing.T) {
	m := mustParse(t, `M DEFINITIONS ::= BEGIN
Foo ::= SEQUENCE { code ERROR.&errorCode }
END`)
	st := m.Assignments[0].(*ast.TypeAssignment).Type.(*ast.SequenceType)
	c := st.Components[0]
	if otf, ok := c.Type.(*ast.OpenTypeFieldType); !ok {
		t.Fatalf("got %T want *OpenTypeFieldType", c.Type)
	} else if otf.Field != "&errorCode" {
		t.Errorf("field: %q", otf.Field)
	}
}

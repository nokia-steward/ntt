package class_test

import (
	"testing"

	"github.com/nokia/ntt/internal/asn1"
	"github.com/nokia/ntt/internal/asn1/ast"
	"github.com/nokia/ntt/internal/asn1/class"
	"github.com/nokia/ntt/internal/asn1/resolver"
)

func parseModule(t *testing.T, src string) (*ast.Module, *resolver.Basket, *resolver.Scope) {
	t.Helper()
	m := asn1.ParseModule([]byte(src))
	b := resolver.NewBasket()
	s := b.Add(m)
	resolver.Resolve(b, m)
	return m, b, s
}

func TestWithSyntaxParser_NoTemplate(t *testing.T) {
	// Object body without a WITH SYNTAX template: settings are
	// returned as-is.
	src := `M DEFINITIONS ::= BEGIN
ERROR ::= CLASS { &code INTEGER, &name PrintableString }
e1 ERROR ::= { &code 42, &name "boom" }
END`
	m, _, _ := parseModule(t, src)
	if len(m.Assignments) < 2 {
		t.Fatalf("need two assignments, got %d", len(m.Assignments))
	}
	// The second assignment is currently classified as a value
	// assignment because the parser doesn't yet distinguish object
	// vs value assignments (Phase 9 will). For now use the object's
	// settings directly via a synthesised Object.
	obj := &ast.Object{Settings: []ast.ObjectSetting{
		{FieldRef: "&code", Value: &ast.IntegerValue{Text: "42"}},
		{FieldRef: "&name", Value: &ast.StringValue{Kind: ast.StringCString, Text: `"boom"`}},
	}}
	oc := m.Assignments[0].(*ast.ObjectClassAssignment).Class
	p := class.NewWithSyntaxParser(oc)
	settings, diags := p.Parse(obj)
	if len(diags) > 0 {
		t.Fatalf("diags: %+v", diags)
	}
	if len(settings) != 2 {
		t.Fatalf("got %d settings", len(settings))
	}
	if settings[0].Field != "&code" || settings[1].Field != "&name" {
		t.Errorf("settings: %+v", settings)
	}
}

func TestWithSyntaxParser_WithSyntaxTemplate(t *testing.T) {
	src := `M DEFINITIONS ::= BEGIN
ERROR ::= CLASS {
    &code INTEGER,
    &name PrintableString
} WITH SYNTAX {
    CODE &code NAME &name
}
END`
	m, _, _ := parseModule(t, src)
	oc := m.Assignments[0].(*ast.ObjectClassAssignment).Class
	// Source object body: CODE 42 NAME "boom" mapped to chunks.
	obj := &ast.Object{Settings: []ast.ObjectSetting{
		{FieldRef: "CODE"},
		{Value: &ast.IntegerValue{Text: "42"}},
		{FieldRef: "NAME"},
		{Value: &ast.StringValue{Kind: ast.StringCString, Text: `"boom"`}},
	}}
	p := class.NewWithSyntaxParser(oc)
	settings, diags := p.Parse(obj)
	if len(diags) > 0 {
		t.Fatalf("diags: %+v", diags)
	}
	if len(settings) != 2 {
		t.Fatalf("got %d settings", len(settings))
	}
	if settings[0].Field != "&code" || settings[0].Value == nil {
		t.Errorf("expected &code = value, got %+v", settings[0])
	}
	if settings[1].Field != "&name" || settings[1].Value == nil {
		t.Errorf("expected &name = value, got %+v", settings[1])
	}
}

func TestWithSyntaxParser_OptionalGroupSkipped(t *testing.T) {
	src := `M DEFINITIONS ::= BEGIN
ERROR ::= CLASS {
    &code INTEGER,
    &desc PrintableString OPTIONAL
} WITH SYNTAX { CODE &code [DESC &desc] }
END`
	m, _, _ := parseModule(t, src)
	oc := m.Assignments[0].(*ast.ObjectClassAssignment).Class
	// Source body omits the [DESC &desc] optional group entirely.
	obj := &ast.Object{Settings: []ast.ObjectSetting{
		{FieldRef: "CODE"},
		{Value: &ast.IntegerValue{Text: "1"}},
	}}
	p := class.NewWithSyntaxParser(oc)
	settings, diags := p.Parse(obj)
	if len(diags) > 0 {
		t.Fatalf("diags: %+v", diags)
	}
	if len(settings) != 1 || settings[0].Field != "&code" {
		t.Errorf("settings: %+v", settings)
	}
}

func TestObjectSetResolver_ExpandsLiterals(t *testing.T) {
	src := `M DEFINITIONS ::= BEGIN
ERROR ::= CLASS { &code INTEGER, &name PrintableString }
END`
	m, b, scope := parseModule(t, src)
	oc := m.Assignments[0].(*ast.ObjectClassAssignment).Class

	set := &ast.ObjectSet{Root: ast.UnionElements{
		&ast.ObjectLiteralElement{Object: &ast.Object{Settings: []ast.ObjectSetting{
			{FieldRef: "&code", Value: &ast.IntegerValue{Text: "1"}},
			{FieldRef: "&name", Value: &ast.StringValue{Kind: ast.StringCString, Text: `"a"`}},
		}}},
		&ast.ObjectLiteralElement{Object: &ast.Object{Settings: []ast.ObjectSetting{
			{FieldRef: "&code", Value: &ast.IntegerValue{Text: "2"}},
			{FieldRef: "&name", Value: &ast.StringValue{Kind: ast.StringCString, Text: `"b"`}},
		}}},
	}}

	r := class.NewObjectSetResolver(b, oc)
	out, diags := r.Resolve(scope, set)
	if len(diags) > 0 {
		t.Fatalf("diags: %+v", diags)
	}
	if len(out) != 2 {
		t.Fatalf("got %d resolved objects", len(out))
	}
	if got := out[0].Settings[0].Value.(*ast.IntegerValue).Text; got != "1" {
		t.Errorf("first object code: %q", got)
	}
}

func TestSolve_ComponentRelation(t *testing.T) {
	objs := []class.ResolvedObject{
		{Settings: []class.Setting{
			{Field: "&code", Value: &ast.IntegerValue{Text: "1"}},
			{Field: "&Type", Type: &ast.IntegerType{}},
		}},
		{Settings: []class.Setting{
			{Field: "&code", Value: &ast.IntegerValue{Text: "2"}},
			{Field: "&Type", Type: &ast.BuiltinType{Kind: ast.PrintableString, Name: "PrintableString"}},
		}},
	}
	alts := class.Solve(objs, "&Type", "&code")
	if len(alts) != 2 {
		t.Fatalf("got %d alternatives", len(alts))
	}
	if _, ok := alts[0].Type.(*ast.IntegerType); !ok {
		t.Errorf("alt 0 type: %T", alts[0].Type)
	}
	if bt, ok := alts[1].Type.(*ast.BuiltinType); !ok || bt.Kind != ast.PrintableString {
		t.Errorf("alt 1 type: %v", alts[1].Type)
	}
}

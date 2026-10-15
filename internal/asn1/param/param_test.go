package param_test

import (
	"testing"

	"github.com/nokia/ntt/internal/asn1"
	"github.com/nokia/ntt/internal/asn1/ast"
	"github.com/nokia/ntt/internal/asn1/param"
	"github.com/nokia/ntt/internal/asn1/resolver"
)

func TestInstantiate_SimpleTypeParam(t *testing.T) {
	src := `M DEFINITIONS ::= BEGIN
Pair { ItemType } ::= SEQUENCE { first ItemType, second ItemType }
END`
	m := asn1.ParseModule([]byte(src))
	b := resolver.NewBasket()
	scope := b.Add(m)
	resolver.Resolve(b, m)

	// Build an actual parameter list: { INTEGER }
	actuals := &ast.ActualParameterList{
		Params: []ast.ActualParameter{
			{Type: &ast.IntegerType{}},
		},
	}
	ref := &ast.TypeRef{Name: "Pair"}

	in := param.NewInstantiator(b)
	out, diags := in.Instantiate(scope, ref, actuals)
	if len(diags) > 0 {
		t.Fatalf("unexpected diags: %+v", diags)
	}
	seq, ok := out.(*ast.SequenceType)
	if !ok {
		t.Fatalf("expected SequenceType, got %T", out)
	}
	if len(seq.Components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(seq.Components))
	}
	for i, c := range seq.Components {
		if _, ok := c.Type.(*ast.IntegerType); !ok {
			t.Errorf("component %d: %T want *IntegerType", i, c.Type)
		}
	}
}

func TestInstantiate_ArityMismatch(t *testing.T) {
	src := `M DEFINITIONS ::= BEGIN
Pair { ItemType } ::= SEQUENCE { a ItemType }
END`
	m := asn1.ParseModule([]byte(src))
	b := resolver.NewBasket()
	scope := b.Add(m)
	resolver.Resolve(b, m)

	actuals := &ast.ActualParameterList{} // empty
	in := param.NewInstantiator(b)
	_, diags := in.Instantiate(scope, &ast.TypeRef{Name: "Pair"}, actuals)
	if len(diags) == 0 || diags[0].Code != "param.arity-mismatch" {
		t.Errorf("expected arity mismatch, got %+v", diags)
	}
}

func TestInstantiate_UnresolvedReference(t *testing.T) {
	m := asn1.ParseModule([]byte(`M DEFINITIONS ::= BEGIN END`))
	b := resolver.NewBasket()
	scope := b.Add(m)

	actuals := &ast.ActualParameterList{}
	in := param.NewInstantiator(b)
	_, diags := in.Instantiate(scope, &ast.TypeRef{Name: "Unknown"}, actuals)
	if len(diags) == 0 || diags[0].Code != "param.unresolved" {
		t.Errorf("expected param.unresolved, got %+v", diags)
	}
}

func TestInstantiate_CrossModuleChain(t *testing.T) {
	a := asn1.ParseModule([]byte(`A DEFINITIONS ::= BEGIN
Box { T } ::= SEQUENCE { value T }
END`))
	b := asn1.ParseModule([]byte(`B DEFINITIONS ::= BEGIN
IMPORTS Box FROM A ;
IntBox ::= Box { INTEGER }
END`))
	basket := resolver.NewBasket()
	basket.Add(a)
	scopeB := basket.Add(b)
	resolver.Resolve(basket, a)
	resolver.Resolve(basket, b)

	rt := b.Assignments[0].(*ast.TypeAssignment).Type.(*ast.ReferencedType)
	in := param.NewInstantiator(basket)
	out, diags := in.Instantiate(scopeB, rt.Ref, rt.Actuals)
	if len(diags) > 0 {
		t.Fatalf("diags: %+v", diags)
	}
	seq, ok := out.(*ast.SequenceType)
	if !ok {
		t.Fatalf("got %T", out)
	}
	if len(seq.Components) != 1 {
		t.Fatalf("got %d comps", len(seq.Components))
	}
	if _, ok := seq.Components[0].Type.(*ast.IntegerType); !ok {
		t.Errorf("got %T want *IntegerType", seq.Components[0].Type)
	}
}

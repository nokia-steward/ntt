package asn1_test

import (
	"os"
	"strings"
	"testing"

	"github.com/nokia/ntt/internal/asn1"
	"github.com/nokia/ntt/internal/asn1/ast"
	"github.com/nokia/ntt/internal/asn1/param"
	"github.com/nokia/ntt/internal/asn1/resolver"
	"github.com/nokia/ntt/internal/asn1/transform"
)

// TestEndToEnd_ParseResolveLower drives the full pipeline:
//
//	parser   -> AST
//	AST      -> Basket / Resolver  (semantic check)
//	AST      -> Transform          (lower to TTCN-3 source)
//	source   -> ttcn3.Parse        (re-parsed by existing parser)
//
// It catches regressions where one layer produces output the next
// can't handle.
func TestEndToEnd_ParseResolveLower(t *testing.T) {
	src, err := os.ReadFile("testdata/sample.asn")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	m := asn1.ParseModule(src)
	if m == nil {
		t.Fatal("nil module")
	}

	b := resolver.NewBasket()
	b.Add(m)
	resolver.Resolve(b, m)

	r := transform.LowerModule(m)
	if r == nil || r.Source == "" {
		t.Fatal("transform produced empty source")
	}
	if r.Tree == nil {
		t.Fatal("re-parsed tree was nil")
	}

	for _, want := range []string{
		"module RRC_Sample",
		"type enumerated Status",
		"type record Person",
		"type record of charstring Names",
		"type union Reply",
	} {
		if !strings.Contains(r.Source, want) {
			t.Errorf("lowered source missing %q\n----\n%s", want, r.Source)
		}
	}
}

// TestEndToEnd_ParameterisedInstantiation exercises the parameter
// engine against the fixture's `Pair { ItemType }` template via the
// `IntPair ::= Pair { INTEGER }` use site.
func TestEndToEnd_ParameterisedInstantiation(t *testing.T) {
	src, _ := os.ReadFile("testdata/sample.asn")
	m := asn1.ParseModule(src)
	b := resolver.NewBasket()
	scope := b.Add(m)
	resolver.Resolve(b, m)

	var intPair *ast.TypeAssignment
	for _, a := range m.Assignments {
		if ta, ok := a.(*ast.TypeAssignment); ok && ta.Name == "IntPair" {
			intPair = ta
			break
		}
	}
	if intPair == nil {
		t.Fatal("IntPair assignment not found")
	}
	rt, ok := intPair.Type.(*ast.ReferencedType)
	if !ok {
		t.Fatalf("IntPair body: %T want *ReferencedType", intPair.Type)
	}
	if rt.Actuals == nil {
		t.Fatal("IntPair actuals are nil")
	}

	in := param.NewInstantiator(b)
	out, diags := in.Instantiate(scope, rt.Ref, rt.Actuals)
	if len(diags) > 0 {
		t.Fatalf("diags: %+v", diags)
	}
	seq, ok := out.(*ast.SequenceType)
	if !ok {
		t.Fatalf("instantiation produced %T want *SequenceType", out)
	}
	if len(seq.Components) != 2 {
		t.Fatalf("components: %d want 2", len(seq.Components))
	}
	for i, c := range seq.Components {
		if _, ok := c.Type.(*ast.IntegerType); !ok {
			t.Errorf("component %d: %T want *IntegerType", i, c.Type)
		}
	}
}

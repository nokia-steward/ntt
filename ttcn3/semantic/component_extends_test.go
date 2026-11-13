package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestComponentExtendsCycleRejected(t *testing.T) {
	// NegSyn_060210_ReuseofComponentTypes_001:
	// A -> GeneralComp -> B -> A   (cycle via B)
	tree := parse(t, `module M {
		type port P message { inout integer; }
		type component MyCompA extends GeneralComp { port P pa; }
		type component MyCompB extends MyCompA { var integer x; }
		type component GeneralComp extends MyCompB { port P pb; }
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "component-extends-cycle") {
		t.Fatalf("expected component-extends-cycle, got %v", codes(diags))
	}
}

func TestComponentExtendsParentClashRejected(t *testing.T) {
	// NegSyn_060210_ReuseofComponentTypes_002:
	// Extending two parents that both declare `MyInt`.
	tree := parse(t, `module M {
		type port P message { inout integer; }
		type component MyCompA { port P pa; var integer MyInt; }
		type component MyCompB { var integer MyInt; }
		type component GeneralComp extends MyCompA, MyCompB { port P pb; }
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "component-extends-name-clash") {
		t.Fatalf("expected component-extends-name-clash on parents, got %v", codes(diags))
	}
}

func TestComponentExtendsChildClashRejected(t *testing.T) {
	// NegSyn_060210_ReuseofComponentTypes_003:
	// Child redeclares a member inherited from a parent.
	tree := parse(t, `module M {
		type port P message { inout integer; }
		type component MyCompA { port P pa; var integer MyInt; }
		type component GeneralComp extends MyCompA { port P pb; var integer MyInt; }
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "component-extends-name-clash") {
		t.Fatalf("expected component-extends-name-clash on child redecl, got %v", codes(diags))
	}
}

func TestComponentExtendsCleanAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type port P message { inout integer; }
		type component A { port P pa; var integer x; }
		type component B extends A { var integer y; }
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "component-extends-cycle") {
		t.Fatalf("did not expect cycle, got %v", codes(diags))
	}
	if containsCode(diags, "component-extends-name-clash") {
		t.Fatalf("did not expect name-clash, got %v", codes(diags))
	}
}

func TestComponentExtendsDiamondAccepted(t *testing.T) {
	// 6.2.10.2 NOTE 1: CB's members reach CG through CE and CF (and
	// directly); they are one definition, not a clash.
	tree := parse(t, `module M {
		type component CB { var integer vc_t; timer tc_t; }
		type component CE extends CB { var integer vc_e; }
		type component CF extends CB { var float vc_f; }
		type component CG extends CB, CE, CF {}
		type component CH extends CE, CF {}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "component-extends-name-clash") {
		t.Fatalf("did not expect name-clash for a diamond, got %v", codes(diags))
	}
}

func TestComponentExtendsDiamondRedeclarationRejected(t *testing.T) {
	// A diamond does not hide a real clash: CE and CF both declare
	// vc_e, so they contribute different definitions.
	tree := parse(t, `module M {
		type component CB { var integer vc_t; }
		type component CE extends CB { var integer vc_e; }
		type component CF extends CB { var integer vc_e; }
		type component CG extends CE, CF {}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "component-extends-name-clash") {
		t.Fatalf("expected component-extends-name-clash, got %v", codes(diags))
	}
}

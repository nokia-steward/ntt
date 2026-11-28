package resolver_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/internal/asn1"
	"github.com/nokia/ntt/internal/asn1/ast"
	"github.com/nokia/ntt/internal/asn1/resolver"
)

func parse(t *testing.T, src string) *asn1.Parser {
	t.Helper()
	return asn1.NewParser([]byte(src))
}

func TestResolve_DuplicateDefinition(t *testing.T) {
	m := asn1.ParseModule([]byte(`M DEFINITIONS ::= BEGIN
Foo ::= INTEGER
Foo ::= BOOLEAN
END`))
	b := resolver.NewBasket()
	resolver.Resolve(b, m)
	if !containsDiag(m.Diagnostics, "duplicate-definition") {
		t.Errorf("expected duplicate-definition diag, got %+v", m.Diagnostics)
	}
}

func TestResolve_UnknownReference(t *testing.T) {
	m := asn1.ParseModule([]byte(`M DEFINITIONS ::= BEGIN
Foo ::= Bar
END`))
	b := resolver.NewBasket()
	resolver.Resolve(b, m)
	if !containsDiagWith(m.Diagnostics, "unknown type reference") {
		t.Errorf("expected unknown reference diag, got %+v", m.Diagnostics)
	}
}

func TestResolve_LocalReferenceResolves(t *testing.T) {
	m := asn1.ParseModule([]byte(`M DEFINITIONS ::= BEGIN
Age ::= INTEGER
Person ::= SEQUENCE { age Age }
END`))
	b := resolver.NewBasket()
	resolver.Resolve(b, m)
	if containsDiagWith(m.Diagnostics, "unknown type reference") {
		t.Errorf("unexpected diags: %+v", m.Diagnostics)
	}
}

func TestResolve_ImportFromUnknownModule(t *testing.T) {
	m := asn1.ParseModule([]byte(`M DEFINITIONS ::= BEGIN
IMPORTS Foo FROM N ;
Bar ::= Foo
END`))
	b := resolver.NewBasket()
	resolver.Resolve(b, m)
	if !containsDiag(m.Diagnostics, "import.unknown-module") {
		t.Errorf("expected import.unknown-module, got %+v", m.Diagnostics)
	}
}

func TestResolve_ImportFromKnownModule(t *testing.T) {
	n := asn1.ParseModule([]byte(`N DEFINITIONS ::= BEGIN
Foo ::= INTEGER
END`))
	m := asn1.ParseModule([]byte(`M DEFINITIONS ::= BEGIN
IMPORTS Foo FROM N ;
Bar ::= Foo
END`))
	b := resolver.NewBasket()
	b.Add(n)
	resolver.Resolve(b, n)
	resolver.Resolve(b, m)
	if containsDiag(m.Diagnostics, "import.unknown-module") {
		t.Errorf("unexpected import.unknown-module diag: %+v", m.Diagnostics)
	}
	if containsDiag(m.Diagnostics, "import.unknown-symbol") {
		t.Errorf("unexpected import.unknown-symbol diag: %+v", m.Diagnostics)
	}
}

func TestResolve_ImportUnknownSymbol(t *testing.T) {
	n := asn1.ParseModule([]byte(`N DEFINITIONS ::= BEGIN
Other ::= INTEGER
END`))
	m := asn1.ParseModule([]byte(`M DEFINITIONS ::= BEGIN
IMPORTS Foo FROM N ;
Bar ::= Foo
END`))
	b := resolver.NewBasket()
	b.Add(n)
	resolver.Resolve(b, m)
	if !containsDiag(m.Diagnostics, "import.unknown-symbol") {
		t.Errorf("expected import.unknown-symbol diag, got %+v", m.Diagnostics)
	}
}

func TestResolve_RespectsExportsList(t *testing.T) {
	n := asn1.ParseModule([]byte(`N DEFINITIONS ::= BEGIN
EXPORTS Other ;
Other ::= INTEGER
Hidden ::= BOOLEAN
END`))
	m := asn1.ParseModule([]byte(`M DEFINITIONS ::= BEGIN
IMPORTS Hidden FROM N ;
Bar ::= Hidden
END`))
	b := resolver.NewBasket()
	b.Add(n)
	resolver.Resolve(b, m)
	if !containsDiag(m.Diagnostics, "import.not-exported") {
		t.Errorf("expected import.not-exported diag, got %+v", m.Diagnostics)
	}
}

func TestResolve_ParameterReferenceResolves(t *testing.T) {
	m := asn1.ParseModule([]byte(`M DEFINITIONS ::= BEGIN
Pair { ItemType } ::= SEQUENCE { first ItemType, second ItemType }
END`))
	b := resolver.NewBasket()
	resolver.Resolve(b, m)
	if containsDiagWith(m.Diagnostics, "unknown type reference") {
		t.Errorf("unexpected diag: %+v", m.Diagnostics)
	}
}

func TestBasket_CrossBasketResolution(t *testing.T) {
	a := asn1.ParseModule([]byte(`A DEFINITIONS ::= BEGIN
T ::= INTEGER
END`))
	b := asn1.ParseModule([]byte(`B DEFINITIONS ::= BEGIN
IMPORTS T FROM A ;
U ::= T
END`))
	primary := resolver.NewBasket()
	external := resolver.NewBasket()
	external.Add(a)
	primary.AddReference(external)
	primary.Add(b)
	resolver.Resolve(primary, b)
	if containsDiag(b.Diagnostics, "import.unknown-module") {
		t.Errorf("unexpected diag: %+v", b.Diagnostics)
	}
	_ = parse(t, "")
}

func containsDiag(diags []ast.Diagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

func containsDiagWith(diags []ast.Diagnostic, fragment string) bool {
	for _, d := range diags {
		if strings.Contains(d.Message, fragment) {
			return true
		}
	}
	return false
}

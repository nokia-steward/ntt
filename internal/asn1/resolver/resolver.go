// Package resolver performs name binding and semantic validation on
// ASN.1 modules produced by the parser. It builds per-module symbol
// tables partitioned by namespace, resolves references (in-module
// first, then via IMPORTS), and emits diagnostics with byte ranges.
//
// Cross-module resolution is mediated by a Basket - a registry of
// resolved modules keyed by module name. Editors and the compiler use
// the same Basket so the same name resolves consistently everywhere.
//
// The Basket abstraction owns one parsed module per name and
// chases cross-module references lazily so callers only pay for
// what they actually resolve.
package resolver

import (
	"fmt"

	"github.com/nokia/ntt/internal/asn1/ast"
)

// Namespace partitions the symbol table; ASN.1 lets a type, value, and
// information object class share the same surface name.
type Namespace int

const (
	NsType Namespace = iota
	NsValue
	NsClass
	NsObject
	NsObjectSet
	NsParameter
)

// Symbol is a resolved binding. Definition is the AST node that
// declared the name; Module is the owning module name.
type Symbol struct {
	Namespace Namespace
	Module    string
	Name      string
	Definition ast.Node
}

// Scope is a per-module symbol table. Lookups are O(map) and scoped to
// a single namespace.
type Scope struct {
	module *ast.Module
	syms   map[symKey]*Symbol
	// importedFrom maps an imported symbol to its source module.
	// Resolution falls back through here when the local table misses.
	importedFrom map[string]string
}

type symKey struct {
	ns   Namespace
	name string
}

// NewScope constructs a Scope for m, indexing every top-level
// assignment under its appropriate namespace.
func NewScope(m *ast.Module) *Scope {
	s := &Scope{
		module:       m,
		syms:         make(map[symKey]*Symbol),
		importedFrom: make(map[string]string),
	}
	for _, a := range m.Assignments {
		switch a := a.(type) {
		case *ast.TypeAssignment:
			s.add(NsType, a.Name, a)
		case *ast.ValueAssignment:
			s.add(NsValue, a.Name, a)
		case *ast.ValueSetTypeAssignment:
			s.add(NsValue, a.Name, a)
		case *ast.ObjectClassAssignment:
			s.add(NsClass, a.Name, a)
		case *ast.ObjectAssignment:
			s.add(NsObject, a.Name, a)
		case *ast.ObjectSetAssignment:
			s.add(NsObjectSet, a.Name, a)
		}
	}
	for _, imp := range m.Imports {
		for _, sym := range imp.Symbols {
			s.importedFrom[sym] = imp.From
		}
	}
	return s
}

func (s *Scope) add(ns Namespace, name string, n ast.Node) {
	s.syms[symKey{ns, name}] = &Symbol{
		Namespace:  ns,
		Module:     s.module.Identifier.Name,
		Name:       name,
		Definition: n,
	}
}

// Module returns the parsed module the scope was built from.
func (s *Scope) Module() *ast.Module { return s.module }

// Lookup returns a symbol with the given namespace and name, or nil if
// not present in this scope. It does not follow imports.
func (s *Scope) Lookup(ns Namespace, name string) *Symbol {
	return s.syms[symKey{ns, name}]
}

// Names returns every symbol name in a given namespace. Useful for
// diagnostics and "did you mean?" hints.
func (s *Scope) Names(ns Namespace) []string {
	var out []string
	for k := range s.syms {
		if k.ns == ns {
			out = append(out, k.name)
		}
	}
	return out
}

// ImportedFrom returns the source module name for an imported symbol,
// or empty if name was not imported.
func (s *Scope) ImportedFrom(name string) string { return s.importedFrom[name] }

// Basket is a registry of resolved modules keyed by module name. It
// also tracks cross-basket references so that imports across separately
// configured suites still resolve.
type Basket struct {
	scopes     map[string]*Scope
	references []*Basket
}

// NewBasket constructs an empty Basket.
func NewBasket() *Basket { return &Basket{scopes: make(map[string]*Scope)} }

// Add registers a module in the basket. Re-adding the same name
// replaces the previous scope.
func (b *Basket) Add(m *ast.Module) *Scope {
	s := NewScope(m)
	b.scopes[m.Identifier.Name] = s
	return s
}

// Get returns the scope for a module name, searching this basket and
// any referenced baskets in registration order.
func (b *Basket) Get(name string) *Scope {
	if s := b.scopes[name]; s != nil {
		return s
	}
	for _, ref := range b.references {
		if s := ref.Get(name); s != nil {
			return s
		}
	}
	return nil
}

// AddReference declares that this basket may resolve names through
// other into baskets when its own modules don't contain them.
func (b *Basket) AddReference(other *Basket) { b.references = append(b.references, other) }

// Modules returns every module name known to this basket (not
// including referenced baskets). Useful for diagnostics.
func (b *Basket) Modules() []string {
	out := make([]string, 0, len(b.scopes))
	for n := range b.scopes {
		out = append(out, n)
	}
	return out
}

// ---------------------------------------------------------------------------
// Resolution
// ---------------------------------------------------------------------------

// Resolve runs all validation passes on a single module, appending
// diagnostics to m.Diagnostics. It is safe to call multiple times.
func Resolve(b *Basket, m *ast.Module) {
	scope := b.scopes[m.Identifier.Name]
	if scope == nil {
		scope = b.Add(m)
	}
	r := &resolver{basket: b, scope: scope, mod: m}
	r.checkImports()
	r.checkDuplicates()
	r.walkAssignments()
	m.Diagnostics = append(m.Diagnostics, r.diags...)
}

type resolver struct {
	basket *Basket
	scope  *Scope
	mod    *ast.Module
	diags  []ast.Diagnostic
}

func (r *resolver) report(n ast.Node, sev ast.Severity, code, format string, args ...interface{}) {
	r.diags = append(r.diags, ast.Diagnostic{
		Pos:      n.Pos(),
		End:      n.End(),
		Severity: sev,
		Code:     code,
		Message:  fmt.Sprintf(format, args...),
	})
}

// checkImports verifies each imported module exists in the basket and
// that the symbols listed are actually exported by it.
func (r *resolver) checkImports() {
	for _, imp := range r.mod.Imports {
		src := r.basket.Get(imp.From)
		if src == nil {
			r.report(imp, ast.SeverityWarning, "import.unknown-module",
				"unknown imported module %q", imp.From)
			continue
		}
		exports := exportedNames(src.module)
		for _, sym := range imp.Symbols {
			if exports != nil {
				if _, ok := exports[sym]; !ok {
					r.report(imp, ast.SeverityError, "import.not-exported",
						"module %q does not export %q", imp.From, sym)
					continue
				}
			}
			// Make sure the symbol actually exists in the target.
			if !hasAnyAssignment(src.module, sym) {
				r.report(imp, ast.SeverityError, "import.unknown-symbol",
					"module %q has no assignment named %q", imp.From, sym)
			}
		}
	}
}

func exportedNames(m *ast.Module) map[string]struct{} {
	if m.Exports == nil || m.Exports.All {
		return nil // nil means "all assignments exported"
	}
	out := make(map[string]struct{}, len(m.Exports.Symbols))
	for _, s := range m.Exports.Symbols {
		out[s] = struct{}{}
	}
	return out
}

func hasAnyAssignment(m *ast.Module, name string) bool {
	for _, a := range m.Assignments {
		if ast.AssignmentName(a) == name {
			return true
		}
	}
	return false
}

// checkDuplicates reports two assignments with the same name in the
// same namespace.
func (r *resolver) checkDuplicates() {
	seen := make(map[symKey]ast.Assignment)
	for _, a := range r.mod.Assignments {
		ns := assignmentNamespace(a)
		k := symKey{ns, ast.AssignmentName(a)}
		if prev, dup := seen[k]; dup {
			r.report(a, ast.SeverityError, "duplicate-definition",
				"duplicate %s assignment %q (previous declaration at offset %d)",
				namespaceLabel(ns), ast.AssignmentName(a), prev.Pos())
			continue
		}
		seen[k] = a
	}
}

func assignmentNamespace(a ast.Assignment) Namespace {
	switch a.(type) {
	case *ast.TypeAssignment:
		return NsType
	case *ast.ValueAssignment, *ast.ValueSetTypeAssignment:
		return NsValue
	case *ast.ObjectClassAssignment:
		return NsClass
	case *ast.ObjectAssignment:
		return NsObject
	case *ast.ObjectSetAssignment:
		return NsObjectSet
	}
	return NsType
}

func namespaceLabel(ns Namespace) string {
	switch ns {
	case NsValue:
		return "value"
	case NsClass:
		return "class"
	case NsObject:
		return "object"
	case NsObjectSet:
		return "object set"
	case NsParameter:
		return "parameter"
	}
	return "type"
}

// walkAssignments validates each assignment's type and value
// references, surfacing unresolved-name diagnostics.
func (r *resolver) walkAssignments() {
	for _, a := range r.mod.Assignments {
		switch a := a.(type) {
		case *ast.TypeAssignment:
			r.checkType(a.Type, paramSet(a.Params))
		case *ast.ValueAssignment:
			r.checkType(a.Type, nil)
			r.checkValue(a.Value)
		case *ast.ValueSetTypeAssignment:
			r.checkType(a.Type, nil)
		}
	}
}

// paramSet returns the set of parameter reference names that are
// in-scope for a parametrised assignment body.
func paramSet(p *ast.ParameterList) map[string]struct{} {
	if p == nil {
		return nil
	}
	out := make(map[string]struct{}, len(p.Params))
	for _, prm := range p.Params {
		if prm.Reference != "" {
			out[prm.Reference] = struct{}{}
		}
	}
	return out
}

func (r *resolver) checkType(t ast.Type, params map[string]struct{}) {
	if t == nil {
		return
	}
	switch t := t.(type) {
	case *ast.BuiltinType, *ast.IntegerType, *ast.BitStringType, *ast.EnumeratedType:
		return
	case *ast.SequenceType:
		for _, c := range t.Components {
			r.checkType(c.Type, params)
		}
		for _, e := range t.Extensions {
			for _, c := range e.Components {
				r.checkType(c.Type, params)
			}
		}
	case *ast.SetType:
		for _, c := range t.Components {
			r.checkType(c.Type, params)
		}
		for _, e := range t.Extensions {
			for _, c := range e.Components {
				r.checkType(c.Type, params)
			}
		}
	case *ast.ChoiceType:
		for _, a := range t.Alternatives {
			r.checkType(a.Type, params)
		}
		for _, e := range t.Extensions {
			for _, c := range e.Components {
				r.checkType(c.Type, params)
			}
		}
	case *ast.SequenceOfType:
		r.checkType(t.Element, params)
	case *ast.SetOfType:
		r.checkType(t.Element, params)
	case *ast.TaggedType:
		r.checkType(t.Underlying, params)
	case *ast.ConstrainedType:
		r.checkType(t.Inner, params)
	case *ast.ReferencedType:
		r.checkReferencedType(t, params)
	case *ast.OpenTypeFieldType:
		// Look up the class reference; full field-existence check
		// belongs to Phase 7.
		if t.ClassRef != nil {
			r.checkRef(t, t.ClassRef, NsClass, params)
		}
	}
}

func (r *resolver) checkReferencedType(t *ast.ReferencedType, params map[string]struct{}) {
	if t.Ref == nil {
		return
	}
	r.checkRef(t, t.Ref, NsType, params)
}

func (r *resolver) checkRef(host ast.Node, ref *ast.TypeRef, ns Namespace, params map[string]struct{}) {
	if ref == nil {
		return
	}
	if ref.Module != "" {
		// Module-qualified: target module must exist.
		scope := r.basket.Get(ref.Module)
		if scope == nil {
			r.report(host, ast.SeverityError, "ref.unknown-module",
				"reference %q targets unknown module %q", ref.Name, ref.Module)
			return
		}
		if scope.Lookup(ns, ref.Name) == nil {
			r.report(host, ast.SeverityError, "ref.unknown-symbol",
				"module %q has no %s named %q", ref.Module, namespaceLabel(ns), ref.Name)
		}
		return
	}
	// Parameter?
	if params != nil {
		if _, ok := params[ref.Name]; ok {
			return
		}
	}
	// Local scope?
	if r.scope.Lookup(ns, ref.Name) != nil {
		return
	}
	// Imported?
	if from := r.scope.ImportedFrom(ref.Name); from != "" {
		if scope := r.basket.Get(from); scope != nil {
			if scope.Lookup(ns, ref.Name) != nil {
				return
			}
		}
		// Module known but symbol missing - reported by checkImports already.
		return
	}
	// Some builtin types (BMPString, etc.) are tokenised as KEYWORD
	// and never reach this path; bare TYPEREFERENCEs we can't resolve
	// are real errors.
	r.report(host, ast.SeverityError, "ref.unknown",
		"unknown %s reference %q", namespaceLabel(ns), ref.Name)
}

func (r *resolver) checkValue(v ast.Value) {
	switch v := v.(type) {
	case *ast.ReferenceValue:
		if v.Module != "" {
			if scope := r.basket.Get(v.Module); scope == nil {
				r.report(v, ast.SeverityError, "value-ref.unknown-module",
					"value reference targets unknown module %q", v.Module)
			} else if scope.Lookup(NsValue, v.Name) == nil {
				r.report(v, ast.SeverityError, "value-ref.unknown-symbol",
					"module %q has no value named %q", v.Module, v.Name)
			}
			return
		}
		// Builtin pseudo-references (MIN/MAX/PLUS-INFINITY/NULL/...) get
		// a free pass; they're not in any scope.
		switch v.Name {
		case "MIN", "MAX", "PLUS-INFINITY", "MINUS-INFINITY", "NOT-A-NUMBER":
			return
		}
		if r.scope.Lookup(NsValue, v.Name) != nil {
			return
		}
		// CHOICE alternative tags and enum identifiers look like
		// value references in their declaration context; don't
		// report those - we need richer per-context info to do that
		// safely. So this is intentionally conservative.
	case *ast.ChoiceValue:
		r.checkValue(v.Value)
	case *ast.SequenceValue:
		for _, f := range v.Fields {
			r.checkValue(f.Value)
		}
	case *ast.SequenceOfValue:
		for _, e := range v.Elements {
			r.checkValue(e)
		}
	}
}

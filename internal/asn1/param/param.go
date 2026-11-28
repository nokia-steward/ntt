// Package param implements ASN.1 X.683 parameterised type and value
// instantiation. Given a `Pair { ItemType }` parametric template and a
// `Pair { INTEGER }` use site, it returns a fully-substituted concrete
// `SEQUENCE { first INTEGER, second INTEGER }` type expression.
//
// Cross-module chained substitution is handled by routing reference
// lookups through a Basket (see internal/asn1/resolver). Instantiation
// results are cached by (template, hash(actuals)) so editor-driven
// re-walks don't recompute the same expansion.
package param

import (
	"fmt"
	"hash/fnv"
	"sync"

	"github.com/nokia/ntt/internal/asn1/ast"
	"github.com/nokia/ntt/internal/asn1/resolver"
)

// Context binds formal parameters to actual ones. We model it as a
// linked list so nested instantiations can layer bindings without
// mutating an outer context.
type Context struct {
	bindings map[string]ast.ActualParameter
	parent   *Context
}

// NewContext constructs an empty binding context.
func NewContext() *Context { return &Context{bindings: make(map[string]ast.ActualParameter)} }

// Child returns a context with bindings layered on top of this one.
func (c *Context) Child(bindings map[string]ast.ActualParameter) *Context {
	return &Context{bindings: bindings, parent: c}
}

// Lookup walks the context chain looking for a binding with the given
// formal-parameter name.
func (c *Context) Lookup(name string) (ast.ActualParameter, bool) {
	for it := c; it != nil; it = it.parent {
		if v, ok := it.bindings[name]; ok {
			return v, true
		}
	}
	return ast.ActualParameter{}, false
}

// ---------------------------------------------------------------------------
// Instantiator
// ---------------------------------------------------------------------------

// Instantiator walks parametrised template bodies, substituting formal
// parameters with their actuals and recursively expanding nested
// instantiations.
type Instantiator struct {
	basket *resolver.Basket
	cache  sync.Map // key string -> ast.Type
}

// NewInstantiator returns an Instantiator that resolves cross-module
// references through b.
func NewInstantiator(b *resolver.Basket) *Instantiator {
	return &Instantiator{basket: b}
}

// Instantiate returns the type produced by substituting actuals into
// the body of the parametrised assignment named by ref, looking it up
// in scope. If ref doesn't name a parametrised type or actuals is nil,
// returns the unmodified referenced type.
func (in *Instantiator) Instantiate(scope *resolver.Scope, ref *ast.TypeRef, actuals *ast.ActualParameterList) (ast.Type, []ast.Diagnostic) {
	target := in.resolveTarget(scope, ref)
	if target == nil {
		return nil, []ast.Diagnostic{{
			Pos: ref.Pos(), End: ref.End(),
			Severity: ast.SeverityError, Code: "param.unresolved",
			Message: fmt.Sprintf("cannot resolve parametric type %q", ref.Name),
		}}
	}
	ta, ok := target.(*ast.TypeAssignment)
	if !ok || ta.Params == nil {
		// Not parametric. The reference resolves to the assignment's
		// type as-is.
		return assignmentType(target), nil
	}
	if actuals == nil {
		return nil, []ast.Diagnostic{{
			Pos: ref.Pos(), End: ref.End(),
			Severity: ast.SeverityError, Code: "param.missing-actuals",
			Message: fmt.Sprintf("parametric type %q used without actual parameters", ta.Name),
		}}
	}
	if got, want := len(actuals.Params), len(ta.Params.Params); got != want {
		return nil, []ast.Diagnostic{{
			Pos: ref.Pos(), End: ref.End(),
			Severity: ast.SeverityError, Code: "param.arity-mismatch",
			Message: fmt.Sprintf("parametric type %q expects %d arguments, got %d", ta.Name, want, got),
		}}
	}

	key := instantiationKey(ta, actuals)
	if cached, ok := in.cache.Load(key); ok {
		return cached.(ast.Type), nil
	}

	bindings := make(map[string]ast.ActualParameter, len(ta.Params.Params))
	for i, p := range ta.Params.Params {
		bindings[p.Reference] = actuals.Params[i]
	}
	ctx := NewContext().Child(bindings)
	out := in.substType(scope, ctx, ta.Type)
	in.cache.Store(key, out)
	return out, nil
}

func (in *Instantiator) resolveTarget(scope *resolver.Scope, ref *ast.TypeRef) ast.Assignment {
	if ref == nil {
		return nil
	}
	if ref.Module != "" {
		if target := in.basket.Get(ref.Module); target != nil {
			if sym := target.Lookup(resolver.NsType, ref.Name); sym != nil {
				if a, ok := sym.Definition.(ast.Assignment); ok {
					return a
				}
			}
		}
		return nil
	}
	if sym := scope.Lookup(resolver.NsType, ref.Name); sym != nil {
		if a, ok := sym.Definition.(ast.Assignment); ok {
			return a
		}
	}
	// Follow IMPORTS - if the name was imported, look it up there.
	if from := scope.ImportedFrom(ref.Name); from != "" {
		if target := in.basket.Get(from); target != nil {
			if sym := target.Lookup(resolver.NsType, ref.Name); sym != nil {
				if a, ok := sym.Definition.(ast.Assignment); ok {
					return a
				}
			}
		}
	}
	return nil
}

func assignmentType(a ast.Assignment) ast.Type {
	if ta, ok := a.(*ast.TypeAssignment); ok {
		return ta.Type
	}
	return nil
}

// substType returns a copy of t with all formal-parameter references
// replaced by the actuals bound in ctx. References to non-parametric
// types are returned as-is.
func (in *Instantiator) substType(scope *resolver.Scope, ctx *Context, t ast.Type) ast.Type {
	if t == nil {
		return nil
	}
	switch t := t.(type) {
	case *ast.ReferencedType:
		if t.Ref != nil {
			if ap, ok := ctx.Lookup(t.Ref.Name); ok && ap.Type != nil {
				return ap.Type
			}
		}
		if t.Actuals != nil {
			// Nested parametric instantiation.
			child, diags := in.Instantiate(scope, t.Ref, in.substActuals(scope, ctx, t.Actuals))
			_ = diags // chained diagnostics should be surfaced by caller via Instantiate again
			if child != nil {
				return child
			}
		}
		return t
	case *ast.SequenceType:
		copy := *t
		copy.Components = in.substComponents(scope, ctx, t.Components)
		copy.Extensions = in.substExtensions(scope, ctx, t.Extensions)
		return &copy
	case *ast.SetType:
		copy := *t
		copy.Components = in.substComponents(scope, ctx, t.Components)
		copy.Extensions = in.substExtensions(scope, ctx, t.Extensions)
		return &copy
	case *ast.ChoiceType:
		copy := *t
		copy.Alternatives = in.substComponents(scope, ctx, t.Alternatives)
		copy.Extensions = in.substExtensions(scope, ctx, t.Extensions)
		return &copy
	case *ast.SequenceOfType:
		copy := *t
		copy.Element = in.substType(scope, ctx, t.Element)
		return &copy
	case *ast.SetOfType:
		copy := *t
		copy.Element = in.substType(scope, ctx, t.Element)
		return &copy
	case *ast.TaggedType:
		copy := *t
		copy.Underlying = in.substType(scope, ctx, t.Underlying)
		return &copy
	case *ast.ConstrainedType:
		copy := *t
		copy.Inner = in.substType(scope, ctx, t.Inner)
		return &copy
	}
	return t
}

func (in *Instantiator) substComponents(scope *resolver.Scope, ctx *Context, comps []ast.Component) []ast.Component {
	if len(comps) == 0 {
		return comps
	}
	out := make([]ast.Component, len(comps))
	for i, c := range comps {
		out[i] = c
		out[i].Type = in.substType(scope, ctx, c.Type)
	}
	return out
}

func (in *Instantiator) substExtensions(scope *resolver.Scope, ctx *Context, exts []ast.ExtensionAddition) []ast.ExtensionAddition {
	if len(exts) == 0 {
		return exts
	}
	out := make([]ast.ExtensionAddition, len(exts))
	for i, e := range exts {
		out[i] = e
		out[i].Components = in.substComponents(scope, ctx, e.Components)
	}
	return out
}

func (in *Instantiator) substActuals(scope *resolver.Scope, ctx *Context, al *ast.ActualParameterList) *ast.ActualParameterList {
	if al == nil {
		return nil
	}
	out := &ast.ActualParameterList{Span: al.Span, Params: make([]ast.ActualParameter, len(al.Params))}
	for i, p := range al.Params {
		out.Params[i] = ast.ActualParameter{Span: p.Span}
		if p.Type != nil {
			out.Params[i].Type = in.substType(scope, ctx, p.Type)
		} else {
			out.Params[i].Value = p.Value
		}
	}
	return out
}

// instantiationKey produces a stable cache key for (template, actuals).
// Identical actuals across different use sites collide here; that's
// intentional - we want to return the same expansion both times.
func instantiationKey(ta *ast.TypeAssignment, actuals *ast.ActualParameterList) string {
	h := fnv.New64a()
	fmt.Fprintf(h, "%p|%d", ta, len(actuals.Params))
	for _, a := range actuals.Params {
		if a.Type != nil {
			fmt.Fprintf(h, "|T:%d-%d", a.Type.Pos(), a.Type.End())
		} else if a.Value != nil {
			fmt.Fprintf(h, "|V:%d-%d", a.Value.Pos(), a.Value.End())
		}
	}
	return fmt.Sprintf("%x", h.Sum64())
}

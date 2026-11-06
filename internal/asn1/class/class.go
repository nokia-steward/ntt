// Package class implements the X.681 information object class
// machinery used by the rest of the ASN.1 frontend:
//
//   - WithSyntaxParser walks an object literal body against the
//     declaring class's WITH SYNTAX template, returning (fieldRef ->
//     value/type) settings. This is the Go equivalent of asn1c's
//     `asn1fix_cws.c` driver.
//
//   - ObjectSetResolver flattens an ObjectSet expression (literals,
//     references, unions, ALL EXCEPT) into a sequence of resolved
//     objects.
//
//   - ComponentRelationSolver narrows an open-type field given a
//     `({Set}{@discriminator})` table constraint, producing the
//     concrete CHOICE alternatives that the lowering pass turns into
//     a TTCN-3 union.
package class

import (
	"fmt"
	"strings"

	"github.com/nokia/ntt/internal/asn1/ast"
	"github.com/nokia/ntt/internal/asn1/resolver"
)

// Setting is one resolved (field, value-or-type) pair from an object
// literal. Type is non-nil for type fields; Value is non-nil for value
// fields. Mutually exclusive.
type Setting struct {
	Field string // includes the leading "&"
	Type  ast.Type
	Value ast.Value
}

// WithSyntaxParser converts an object body into Settings, driven by a
// class's WITH SYNTAX template.
type WithSyntaxParser struct {
	class *ast.ObjectClass
}

// NewWithSyntaxParser returns a parser bound to a class definition.
func NewWithSyntaxParser(class *ast.ObjectClass) *WithSyntaxParser {
	return &WithSyntaxParser{class: class}
}

// Parse walks obj against the class's WITH SYNTAX template. If the
// template is nil, the object's settings are returned as-is.
func (p *WithSyntaxParser) Parse(obj *ast.Object) ([]Setting, []ast.Diagnostic) {
	if obj == nil {
		return nil, nil
	}
	if p.class == nil || p.class.WithSyntax == nil {
		return objectAsSettings(obj), nil
	}
	// We replay the WITH SYNTAX template token-by-token; each WORD
	// must match the corresponding source token, each &FieldRef
	// captures the next syntactic chunk as the field's value/type,
	// and OptionalGroups are skipped if their first WORD doesn't
	// match the next source token.
	var settings []Setting
	var diags []ast.Diagnostic
	src := flattenObjectBody(obj)
	pos := 0
	_, _ = p.matchTemplate(p.class.WithSyntax.Tokens, src, &pos, &settings, &diags, true)
	if pos < len(src) {
		diags = append(diags, ast.Diagnostic{
			Pos: obj.Pos(), End: obj.End(),
			Severity: ast.SeverityError, Code: "class.trailing-tokens",
			Message: fmt.Sprintf("WITH SYNTAX did not consume all tokens (leftover: %s)", joinChunks(src[pos:])),
		})
	}
	return settings, diags
}

// matchTemplate advances *pos through src as it consumes template
// tokens. If required is false (we're inside an OptionalGroup), the
// match returns success only if every required template token had a
// corresponding source chunk; otherwise it rolls back *pos.
func (p *WithSyntaxParser) matchTemplate(tmpl []ast.WithSyntaxToken, src chunkSlice, pos *int, out *[]Setting, diags *[]ast.Diagnostic, required bool) (bool, []Setting) {
	startPos := *pos
	local := append([]Setting(nil), (*out)...)
	for _, t := range tmpl {
		switch t.Kind {
		case ast.WSKWord:
			if *pos >= len(src) || !src[*pos].isWord() || !strings.EqualFold(src[*pos].word, t.Text) {
				if !required {
					*pos = startPos
					return false, nil
				}
				*diags = append(*diags, ast.Diagnostic{
					Pos: t.Pos(), End: t.End(),
					Severity: ast.SeverityError, Code: "class.word-mismatch",
					Message: fmt.Sprintf("expected literal word %q, got %s", t.Text, src.describe(*pos)),
				})
				return false, nil
			}
			*pos++
		case ast.WSKFieldRef:
			if *pos >= len(src) {
				if !required {
					*pos = startPos
					return false, nil
				}
				*diags = append(*diags, ast.Diagnostic{
					Pos: t.Pos(), End: t.End(),
					Severity: ast.SeverityError, Code: "class.missing-field",
					Message: fmt.Sprintf("expected value for field %q, ran off end of object body", t.Text),
				})
				return false, nil
			}
			ch := src[*pos]
			*pos++
			s := Setting{Field: t.Text}
			if ch.tp != nil {
				s.Type = ch.tp
			} else {
				s.Value = ch.value
			}
			local = append(local, s)
		case ast.WSKOptionalGroup:
			ok, group := p.matchTemplate(t.Group, src, pos, &local, diags, false)
			if ok {
				local = group
			}
		}
	}
	*out = local
	return true, local
}

// chunk is one syntactic atom from the source object body: either a
// bare word, an embedded type, or an embedded value.
type chunk struct {
	word  string
	tp    ast.Type
	value ast.Value
}

type chunkSlice []chunk

func (c chunk) isWord() bool { return c.tp == nil && c.value == nil }

func (s chunkSlice) describe(i int) string {
	if i >= len(s) {
		return "end of object"
	}
	c := s[i]
	if c.isWord() {
		return fmt.Sprintf("word %q", c.word)
	}
	if c.tp != nil {
		return fmt.Sprintf("type at %d..%d", c.tp.Pos(), c.tp.End())
	}
	return fmt.Sprintf("value at %d..%d", c.value.Pos(), c.value.End())
}

func joinChunks(s chunkSlice) string {
	var b strings.Builder
	for i, c := range s {
		if i > 0 {
			b.WriteString(" ")
		}
		if c.isWord() {
			b.WriteString(c.word)
		} else if c.tp != nil {
			b.WriteString("<type>")
		} else {
			b.WriteString("<value>")
		}
	}
	return b.String()
}

// flattenObjectBody turns an object literal's settings into the linear
// token stream the template matcher consumes. Word-only entries get
// promoted from the field text (Phase 4 parser already captured them
// as Settings with empty field), values map to value chunks, types to
// type chunks.
func flattenObjectBody(obj *ast.Object) chunkSlice {
	out := make(chunkSlice, 0, len(obj.Settings))
	for _, s := range obj.Settings {
		// If the parser successfully attached a &Field to a value or
		// type, that's a real "field+value" setting. We treat the
		// field reference itself as a word so the template's
		// matching WSKFieldRef sees the value/type as the next chunk.
		switch {
		case s.Type != nil:
			out = append(out, chunk{tp: s.Type})
		case s.Value != nil:
			out = append(out, chunk{value: s.Value})
		case s.FieldRef != "":
			out = append(out, chunk{word: s.FieldRef})
		}
	}
	return out
}

// objectAsSettings returns the raw settings when the class declares no
// WITH SYNTAX template (i.e. the object body is already field-tagged).
func objectAsSettings(obj *ast.Object) []Setting {
	out := make([]Setting, 0, len(obj.Settings))
	for _, s := range obj.Settings {
		out = append(out, Setting{Field: s.FieldRef, Type: s.Type, Value: s.Value})
	}
	return out
}

// ---------------------------------------------------------------------------
// ObjectSetResolver
// ---------------------------------------------------------------------------

// ObjectSetResolver expands an ObjectSet expression into a flat list
// of Settings (one per resolved object). It chases object references
// and object-set references via the basket; cross-module references
// work the same way as for types.
type ObjectSetResolver struct {
	basket *resolver.Basket
	class  *ast.ObjectClass
}

// NewObjectSetResolver returns a resolver bound to a class.
func NewObjectSetResolver(basket *resolver.Basket, class *ast.ObjectClass) *ObjectSetResolver {
	return &ObjectSetResolver{basket: basket, class: class}
}

// ResolvedObject is one object expanded from a set. SourceObject is
// the AST node it originated from (for go-to-definition).
type ResolvedObject struct {
	Source   ast.Node
	Settings []Setting
}

// Resolve walks set, returning every concrete object it transitively
// references. Recursive references are detected and reported once.
func (r *ObjectSetResolver) Resolve(scope *resolver.Scope, set *ast.ObjectSet) ([]ResolvedObject, []ast.Diagnostic) {
	if set == nil {
		return nil, nil
	}
	seen := make(map[string]bool)
	out, diags := r.expand(scope, set.Root, seen)
	if set.Extensible {
		moreOut, moreDiags := r.expand(scope, set.Extension, seen)
		out = append(out, moreOut...)
		diags = append(diags, moreDiags...)
	}
	return out, diags
}

func (r *ObjectSetResolver) expand(scope *resolver.Scope, elements ast.UnionElements, seen map[string]bool) ([]ResolvedObject, []ast.Diagnostic) {
	var out []ResolvedObject
	var diags []ast.Diagnostic
	for _, el := range elements {
		switch el := el.(type) {
		case *ast.ObjectLiteralElement:
			settings, d := NewWithSyntaxParser(r.class).Parse(el.Object)
			diags = append(diags, d...)
			out = append(out, ResolvedObject{Source: el, Settings: settings})
		case *ast.ObjectReferenceElement:
			obj, d := r.lookupObject(scope, el.Ref, seen)
			diags = append(diags, d...)
			if obj != nil {
				out = append(out, *obj)
			}
		case *ast.ObjectSetReferenceElement:
			more, d := r.lookupObjectSet(scope, el.Ref, seen)
			diags = append(diags, d...)
			out = append(out, more...)
		}
	}
	return out, diags
}

func (r *ObjectSetResolver) lookupObject(scope *resolver.Scope, ref *ast.TypeRef, seen map[string]bool) (*ResolvedObject, []ast.Diagnostic) {
	if ref == nil {
		return nil, nil
	}
	target := scope
	if ref.Module != "" {
		target = r.basket.Get(ref.Module)
		if target == nil {
			return nil, []ast.Diagnostic{{
				Pos: ref.Pos(), End: ref.End(),
				Severity: ast.SeverityError, Code: "class.unknown-module",
				Message: fmt.Sprintf("unknown module %q in object reference", ref.Module),
			}}
		}
	}
	sym := target.Lookup(resolver.NsObject, ref.Name)
	if sym == nil {
		return nil, []ast.Diagnostic{{
			Pos: ref.Pos(), End: ref.End(),
			Severity: ast.SeverityError, Code: "class.unknown-object",
			Message: fmt.Sprintf("unknown object %q", ref.Name),
		}}
	}
	oa, ok := sym.Definition.(*ast.ObjectAssignment)
	if !ok {
		return nil, nil
	}
	key := target.Module().Identifier.Name + "." + ref.Name
	if seen[key] {
		return nil, []ast.Diagnostic{{
			Pos: ref.Pos(), End: ref.End(),
			Severity: ast.SeverityError, Code: "class.cycle",
			Message: fmt.Sprintf("recursive object reference %q", ref.Name),
		}}
	}
	seen[key] = true
	settings, diags := NewWithSyntaxParser(r.class).Parse(oa.Object)
	return &ResolvedObject{Source: oa, Settings: settings}, diags
}

func (r *ObjectSetResolver) lookupObjectSet(scope *resolver.Scope, ref *ast.TypeRef, seen map[string]bool) ([]ResolvedObject, []ast.Diagnostic) {
	if ref == nil {
		return nil, nil
	}
	target := scope
	if ref.Module != "" {
		target = r.basket.Get(ref.Module)
		if target == nil {
			return nil, []ast.Diagnostic{{
				Pos: ref.Pos(), End: ref.End(),
				Severity: ast.SeverityError, Code: "class.unknown-module",
				Message: fmt.Sprintf("unknown module %q in object set reference", ref.Module),
			}}
		}
	}
	sym := target.Lookup(resolver.NsObjectSet, ref.Name)
	if sym == nil {
		return nil, []ast.Diagnostic{{
			Pos: ref.Pos(), End: ref.End(),
			Severity: ast.SeverityError, Code: "class.unknown-object-set",
			Message: fmt.Sprintf("unknown object set %q", ref.Name),
		}}
	}
	osa, ok := sym.Definition.(*ast.ObjectSetAssignment)
	if !ok || osa.Set == nil {
		return nil, nil
	}
	key := target.Module().Identifier.Name + ".set." + ref.Name
	if seen[key] {
		return nil, []ast.Diagnostic{{
			Pos: ref.Pos(), End: ref.End(),
			Severity: ast.SeverityError, Code: "class.cycle",
			Message: fmt.Sprintf("recursive object set reference %q", ref.Name),
		}}
	}
	seen[key] = true
	out, diags := r.expand(target, osa.Set.Root, seen)
	if osa.Set.Extensible {
		more, moreD := r.expand(target, osa.Set.Extension, seen)
		out = append(out, more...)
		diags = append(diags, moreD...)
	}
	return out, diags
}

// ---------------------------------------------------------------------------
// Component-relation solver
// ---------------------------------------------------------------------------

// Alternative is one resolved CHOICE alternative produced by expanding
// an open-type field via a component-relation constraint.
type Alternative struct {
	Discriminator ast.Value // the value of @field for this object
	Type          ast.Type  // the resolved open-type field's actual type
}

// Solve looks at the constraint `({Set}{@discriminator})` attached to
// an open-type field and produces one Alternative per resolved object,
// using the field name openField to pull the type out of each object.
//
// objects is the result of an earlier ObjectSetResolver.Resolve call;
// passing them in keeps this function pure.
func Solve(objects []ResolvedObject, openField, discriminator string) []Alternative {
	out := make([]Alternative, 0, len(objects))
	for _, o := range objects {
		var disc ast.Value
		var openT ast.Type
		for _, s := range o.Settings {
			switch s.Field {
			case discriminator:
				disc = s.Value
			case openField:
				openT = s.Type
			}
		}
		if openT != nil {
			out = append(out, Alternative{Discriminator: disc, Type: openT})
		}
	}
	return out
}

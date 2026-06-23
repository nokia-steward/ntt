// subset_superset.go enforces ETSI ES 201 873-1 clauses B.1.2.6
// (SubSet) and B.1.2.7 (SuperSet) on the *applicability* of those
// template matching mechanisms: both are only valid when the
// matched target is a `set of` type.
//
//   - subset-on-non-setof: emitted when `subset(...)` is used on a
//     target whose declared type is anything other than `set of T`.
//   - superset-on-non-setof: same rule for `superset(...)`.
//
// The check is purely syntactic: we collect every named type's
// "category" (set-of / record-of / array / record / set / union /
// other) from the module's top-level declarations, then walk every
// template decl looking for `subset(...)` / `superset(...)` call
// sites. The target type is taken either from the template's
// `Type` declaration (direct assignment) or, when the call appears
// as a `field := subset(...)` entry inside a CompositeLiteral, from
// the matching field of the template's owning struct type.
//
// Cross-module / parameterised / type-of-type-of references fall
// through silently to keep the false-positive rate at zero.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// typeCategory classifies a named type by what value notation /
// matching mechanisms it accepts.
type typeCategory int

const (
	typeCategoryOther    typeCategory = iota
	typeCategoryRecord                // RECORD (not record-of)
	typeCategorySet                   // SET    (not set-of)
	typeCategoryUnion                 // UNION
	typeCategoryRecordOf              // record of T
	typeCategorySetOf                 // set of T
	typeCategoryArray                 // T[N]
)

func (c typeCategory) String() string {
	switch c {
	case typeCategoryRecord:
		return "record"
	case typeCategorySet:
		return "set"
	case typeCategoryUnion:
		return "union"
	case typeCategoryRecordOf:
		return "record of"
	case typeCategorySetOf:
		return "set of"
	case typeCategoryArray:
		return "array"
	}
	return "other"
}

// fieldSpec captures both the declared *type name* of a struct
// field (when it references one) and the field's direct
// typeCategory (used for inline ListSpec / StructSpec fields that
// don't have a name). At least one of the two is populated.
type fieldSpec struct {
	typeName string       // declared type identifier, "" for anonymous
	category typeCategory // direct category for inline specs
}

// fieldsOfStruct records the declared field name -> fieldSpec
// for every named struct type. Used to resolve
// `field := subset(...)` entries inside a composite literal.
type fieldsOfStruct = map[string]fieldSpec

// checkSubsetSupersetRules is wired into Analyze; emits the two
// applicability diagnostics described at the top of this file.
//
// Template declarations occur both at module level and inside
// function / testcase / altstep bodies (TTCN-3 allows local
// templates). We use syntax.Inspect to reach both.
func (a *Analyzer) checkSubsetSupersetRules(mod *syntax.Module) []Diagnostic {
	typeCats := collectTypeCategories(mod)
	if len(typeCats) == 0 {
		return nil
	}
	structFieldTypes := collectStructFieldTypes(mod)
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		td, ok := n.(*syntax.TemplateDecl)
		if !ok || td == nil || td.Type == nil || td.Value == nil {
			return true
		}
		typeName := identName(td.Type)
		if typeName == "" {
			return true
		}
		// Direct-assignment shape:
		// `template T name := subset(...) [length(...)];`.
		// The Value is a CallExpr (possibly wrapped) for
		// subset / superset.
		diags = append(diags, checkSubsetCall(
			td.Value, typeName, "", typeCats)...)

		// Composite-literal shape:
		// `template T name := { field := subset(...) }`.
		// We walk the entries and resolve each field's type
		// against the template's owning struct.
		if cl, ok := td.Value.(*syntax.CompositeLiteral); ok {
			fields := structFieldTypes[typeName]
			for _, item := range cl.List {
				be, ok := item.(*syntax.BinaryExpr)
				if !ok || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
					continue
				}
				fieldName := identName(be.X)
				if fieldName == "" {
					continue
				}
				spec, has := fields[fieldName]
				if !has {
					continue
				}
				diags = append(diags, checkSubsetCallWithSpec(
					be.Y, spec, fieldName, typeCats)...)
			}
		}
		return true
	})
	return diags
}

// checkSubsetCallWithSpec is the field-context variant of
// checkSubsetCall; it accepts a pre-resolved fieldSpec instead of
// a type-name string so inline ListSpec / StructSpec fields work
// without a name being declared.
func checkSubsetCallWithSpec(
	val syntax.Expr,
	spec fieldSpec,
	field string,
	typeCats map[string]typeCategory,
) []Diagnostic {
	ce, ok := stripTemplateAttrs(val).(*syntax.CallExpr)
	if !ok {
		return nil
	}
	callee := identName(ce.Fun)
	if callee != "subset" && callee != "superset" {
		return nil
	}
	cat := spec.category
	if cat == typeCategoryOther && spec.typeName != "" {
		if c, ok := typeCats[spec.typeName]; ok {
			cat = c
		}
	}
	if cat == typeCategoryOther {
		return nil
	}
	if cat == typeCategorySetOf {
		return nil
	}
	return []Diagnostic{{
		Code:     callee + "-on-non-setof",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"%s(...) requires a `set of` target; field %q has category %s",
			callee, field, cat.String()),
		Node: ce,
		Span: syntax.SpanOf(ce),
	}}
}

// checkSubsetCall returns the diagnostics produced when `val` is a
// `subset(...)` / `superset(...)` call on a target whose type
// (`targetType`) is not `set of T`. `field` is the optional field
// name to include in the message when the call lives in a
// composite literal entry.
func checkSubsetCall(
	val syntax.Expr,
	targetType string,
	field string,
	typeCats map[string]typeCategory,
) []Diagnostic {
	ce, ok := stripTemplateAttrs(val).(*syntax.CallExpr)
	if !ok {
		return nil
	}
	callee := identName(ce.Fun)
	if callee != "subset" && callee != "superset" {
		return nil
	}
	cat, ok := typeCats[targetType]
	if !ok {
		// Unknown target type; the resolver elsewhere will
		// catch totally-undefined types.
		return nil
	}
	if cat == typeCategorySetOf {
		return nil
	}
	// Synthesise the human-readable locus: "field X of template
	// type Y" or just "template type Y" when used directly.
	where := fmt.Sprintf("template type %q", targetType)
	if field != "" {
		where = fmt.Sprintf("field %q of %s", field, where)
	}
	return []Diagnostic{{
		Code:     callee + "-on-non-setof",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"%s(...) requires a `set of` target; %s has category %s",
			callee, where, cat.String()),
		Node: ce,
		Span: syntax.SpanOf(ce),
	}}
}

// stripTemplateAttrs peels common template-side wrappers
// (LengthExpr, IFPRESENT UnaryExpr) off the value so the inner
// CallExpr is visible to the rule.
func stripTemplateAttrs(e syntax.Expr) syntax.Expr {
	for {
		switch v := e.(type) {
		case *syntax.LengthExpr:
			e = v.X
		case *syntax.UnaryExpr:
			if v.Op != nil && v.Op.Kind() == syntax.IFPRESENT {
				e = v.X
				continue
			}
			return e
		default:
			return e
		}
	}
}

// identName returns the textual name of a bare Ident expression,
// or "" for any other expression shape.
func identName(e syntax.Expr) string {
	if id, ok := e.(*syntax.Ident); ok && id != nil {
		return id.String()
	}
	return ""
}

// collectTypeCategories builds the name -> category table used by
// the subset / superset target-type lookup. Subtypes inherit the
// category of their base type via a fixpoint pass so
// `type SoI MessageType;` (a SubTypeDecl wrapping an Ident) still
// reports as `set of` when SoI does.
func collectTypeCategories(mod *syntax.Module) map[string]typeCategory {
	out := map[string]typeCategory{}
	// alias[name] -> base type identifier for SubTypeDecls
	// whose Field.Type is a bare RefSpec / Ident; resolved in
	// the fixpoint below.
	alias := map[string]string{}

	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		switch x := d.Def.(type) {
		case *syntax.StructTypeDecl:
			if x.Name == nil || x.KindTok == nil {
				continue
			}
			switch x.KindTok.Kind() {
			case syntax.RECORD:
				out[x.Name.String()] = typeCategoryRecord
			case syntax.SET:
				out[x.Name.String()] = typeCategorySet
			case syntax.UNION:
				out[x.Name.String()] = typeCategoryUnion
			}
		case *syntax.SubTypeDecl:
			if x.Field == nil || x.Field.Name == nil {
				continue
			}
			switch t := x.Field.Type.(type) {
			case *syntax.ListSpec:
				if t.KindTok == nil {
					continue
				}
				switch t.KindTok.Kind() {
				case syntax.RECORD:
					out[x.Field.Name.String()] = typeCategoryRecordOf
				case syntax.SET:
					out[x.Field.Name.String()] = typeCategorySetOf
				}
			case *syntax.StructSpec:
				if t.KindTok == nil {
					continue
				}
				switch t.KindTok.Kind() {
				case syntax.RECORD:
					out[x.Field.Name.String()] = typeCategoryRecord
				case syntax.SET:
					out[x.Field.Name.String()] = typeCategorySet
				case syntax.UNION:
					out[x.Field.Name.String()] = typeCategoryUnion
				}
			case *syntax.RefSpec:
				if id, ok := t.X.(*syntax.Ident); ok && id != nil {
					alias[x.Field.Name.String()] = id.String()
				}
			}
			// Field-level array dim makes the named type an
			// array regardless of the underlying spec.
			if x.Field.ArrayDef != nil {
				out[x.Field.Name.String()] = typeCategoryArray
			}
		}
	}

	// Fixpoint: subtypes that alias another named type inherit
	// its category. Cycles fall out when no change happens.
	for changed := true; changed; {
		changed = false
		for name, base := range alias {
			if _, done := out[name]; done {
				continue
			}
			if cat, ok := out[base]; ok {
				out[name] = cat
				delete(alias, name)
				changed = true
			}
		}
	}
	return out
}

// collectStructFieldTypes maps every named struct type's field
// name to the field's declared type info (a fieldSpec). The
// subset / superset walker uses the result to look up
// `field := subset(...)` entries' target type. Inline
// ListSpec / StructSpec fields (anonymous types) are recorded as
// a direct typeCategory so the walker doesn't need a named type
// to flag them.
func collectStructFieldTypes(mod *syntax.Module) map[string]fieldsOfStruct {
	out := map[string]fieldsOfStruct{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		st, ok := d.Def.(*syntax.StructTypeDecl)
		if !ok || st.Name == nil {
			continue
		}
		fields := fieldsOfStruct{}
		for _, f := range st.Fields {
			if f == nil || f.Name == nil {
				continue
			}
			fields[f.Name.String()] = fieldTypeSpec(f.Type)
		}
		out[st.Name.String()] = fields
	}
	return out
}

// fieldTypeSpec classifies a Field's TypeSpec into a fieldSpec:
// either a named type ref (typeName populated) or an inline
// container spec (category populated). Anonymous shapes we don't
// know how to classify return the zero fieldSpec.
func fieldTypeSpec(t syntax.TypeSpec) fieldSpec {
	switch v := t.(type) {
	case *syntax.RefSpec:
		if v != nil {
			return fieldSpec{typeName: identName(v.X)}
		}
	case *syntax.ListSpec:
		if v != nil && v.KindTok != nil {
			switch v.KindTok.Kind() {
			case syntax.RECORD:
				return fieldSpec{category: typeCategoryRecordOf}
			case syntax.SET:
				return fieldSpec{category: typeCategorySetOf}
			}
		}
	case *syntax.StructSpec:
		if v != nil && v.KindTok != nil {
			switch v.KindTok.Kind() {
			case syntax.RECORD:
				return fieldSpec{category: typeCategoryRecord}
			case syntax.SET:
				return fieldSpec{category: typeCategorySet}
			case syntax.UNION:
				return fieldSpec{category: typeCategoryUnion}
			}
		}
	}
	return fieldSpec{}
}

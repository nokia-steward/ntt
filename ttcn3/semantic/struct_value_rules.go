// struct_value_rules.go implements ETSI ES 201 873-1 clause 6.2
// "value notation" and "indexed access" rules for structured types:
//
//   - record / set: the value notation is the named-field form
//     `{ name := value, ... }` or the positional list `{ v1, v2, ... }`
//     in declaration order; indexed access `x[i]` is illegal because
//     records / sets have no element index.
//   - union: the value notation is exactly one named alternative
//     `{ alt := value }`; positional / value-list notation is illegal
//     (there is no "first alternative" the runtime can pick from).
//     Indexed access `x[i]` is illegal for the same reason as
//     records / sets.
//   - arrays, record-of and set-of types are unaffected - they are
//     the indexable categories.
//
// The check is purely syntactic: it identifies named struct types
// in the module, then walks every ValueDecl (constants, module
// params, locals) and every assignment whose LHS is an IndexExpr,
// and reports a diagnostic when the value notation / access form
// is incompatible with the target type's category.
//
// Conservative bias matches the rest of the semantic checks: only
// references to types declared in the same module are inspected;
// cross-module / parameterised / type-of-type-of references fall
// through silently.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// structKind classifies a named struct type by its category.
type structKind int

const (
	structKindNone   structKind = iota
	structKindRecord            // RECORD (not record-of)
	structKindSet               // SET    (not set-of)
	structKindUnion             // UNION
)

func (k structKind) String() string {
	switch k {
	case structKindRecord:
		return "record"
	case structKindSet:
		return "set"
	case structKindUnion:
		return "union"
	}
	return ""
}

// checkStructValueRules is wired into Analyze; it returns the
// diagnostics for the rules listed at the top of this file.
func (a *Analyzer) checkStructValueRules(mod *syntax.Module) []Diagnostic {
	kinds := collectStructKinds(mod)
	if len(kinds) == 0 {
		return nil
	}
	// Pre-compute the alternatives of every union type. The
	// list is used by checkUnionAlternativeRefs below for the
	// ETSI 6.2.5.1 "type U.alt" extended-type-reference check.
	unionAlts := collectUnionAlternatives(mod)
	var diags []Diagnostic
	diags = append(diags, checkUnionAlternativeRefs(mod, unionAlts)...)
	diags = append(diags, checkRecordSetFieldSelfRefs(mod)...)
	syntax.Inspect(mod, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		switch x := n.(type) {
		case *syntax.FuncDecl:
			if x.Body == nil {
				return false
			}
			vars := collectLocalStructVars(x.Body, kinds)
			diags = append(diags, checkStructInBody(x.Body, kinds, vars)...)
			return false
		case *syntax.ValueDecl:
			diags = append(diags, checkStructValueDeclInit(x, kinds)...)
		}
		return true
	})
	return diags
}

// collectUnionAlternatives walks every top-level union
// declaration and records the set of alternative names. Used by
// checkUnionAlternativeRefs to validate `type U.altName T;`
// extended type references per ETSI 6.2.5.1.
func collectUnionAlternatives(mod *syntax.Module) map[string]map[string]bool {
	return collectStructFieldNames(mod, syntax.UNION)
}

// collectStructFieldNames is the shared helper behind both the
// union and the record / set field-name collectors. KindTok must
// be one of RECORD / SET / UNION.
func collectStructFieldNames(mod *syntax.Module, kind syntax.Kind) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		st, ok := d.Def.(*syntax.StructTypeDecl)
		if !ok || st.Name == nil || st.KindTok == nil {
			continue
		}
		if st.KindTok.Kind() != kind {
			continue
		}
		fields := map[string]bool{}
		for _, f := range st.Fields {
			if f == nil || f.Name == nil {
				continue
			}
			fields[f.Name.String()] = true
		}
		out[st.Name.String()] = fields
	}
	return out
}

// checkUnionAlternativeRefs flags `type U.altName T;` declarations
// where altName is not a known alternative of the union U.
// ETSI 6.2.5.1 requires altName to resolve to one of U's declared
// alternatives; an unknown alt is undefined behaviour.
//
// Also flags an alternative whose own declared type is `U.alt2`
// when that resolves back to a name in the same union (direct or
// indirect self-reference); the runtime has no fixed size to
// allocate for such a type.
func checkUnionAlternativeRefs(
	mod *syntax.Module,
	unionAlts map[string]map[string]bool,
) []Diagnostic {
	var diags []Diagnostic
	if len(unionAlts) == 0 {
		return nil
	}

	// 1) `type U.alt T;` extended type-reference check.
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		st, ok := d.Def.(*syntax.SubTypeDecl)
		if !ok || st.Field == nil {
			continue
		}
		refUnion, refAlt, ok := unionAltSelector(st.Field.Type)
		if !ok {
			continue
		}
		alts, known := unionAlts[refUnion]
		if !known {
			continue
		}
		if !alts[refAlt] {
			diags = append(diags, Diagnostic{
				Code:     "union-unknown-alternative",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"%q is not an alternative of union %q",
					refAlt, refUnion),
				Node: st.Field,
				Span: syntax.SpanOf(st.Field),
			})
		}
	}

	// 2) Self-referencing alternative inside a union body.
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		st, ok := d.Def.(*syntax.StructTypeDecl)
		if !ok || st.Name == nil || st.KindTok == nil {
			continue
		}
		if st.KindTok.Kind() != syntax.UNION {
			continue
		}
		owner := st.Name.String()
		for _, f := range st.Fields {
			if f == nil || f.Type == nil {
				continue
			}
			refUnion, refAlt, ok := unionAltSelector(f.Type)
			if !ok {
				continue
			}
			if refUnion != owner {
				continue
			}
			altName := ""
			if f.Name != nil {
				altName = f.Name.String()
			}
			diags = append(diags, Diagnostic{
				Code:     "union-self-referencing-alternative",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"alternative %q of union %q references its own alternative %q (self-cycle has no fixed size)",
					altName, owner, refAlt),
				Node: f,
				Span: syntax.SpanOf(f),
			})
		}
	}
	return diags
}

// checkRecordSetFieldSelfRefs flags record / set bodies whose
// own field is declared with a type of `T.otherField` referring
// back into the same enclosing struct. ETSI 6.2.1.1 (record) and
// 6.2.2.1 (set) both prohibit this because the type has no
// fixed size.
func checkRecordSetFieldSelfRefs(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		st, ok := d.Def.(*syntax.StructTypeDecl)
		if !ok || st.Name == nil || st.KindTok == nil {
			continue
		}
		kindName := ""
		switch st.KindTok.Kind() {
		case syntax.RECORD:
			kindName = "record"
		case syntax.SET:
			kindName = "set"
		default:
			continue
		}
		owner := st.Name.String()
		for _, f := range st.Fields {
			if f == nil || f.Type == nil {
				continue
			}
			refStruct, refField, ok := unionAltSelector(f.Type)
			if !ok {
				continue
			}
			if refStruct != owner {
				continue
			}
			fieldName := ""
			if f.Name != nil {
				fieldName = f.Name.String()
			}
			diags = append(diags, Diagnostic{
				Code:     "struct-self-referencing-field",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"field %q of %s %q references its own field %q (self-cycle has no fixed size)",
					fieldName, kindName, owner, refField),
				Node: f,
				Span: syntax.SpanOf(f),
			})
		}
	}
	return diags
}

// unionAltSelector classifies a TypeSpec as a `U.alt` extended
// type reference and returns the union name and the alternative
// name. Returns ok=false for any other shape (plain type
// references, list / map / struct specs, ...).
func unionAltSelector(t syntax.TypeSpec) (string, string, bool) {
	ref, ok := t.(*syntax.RefSpec)
	if !ok || ref.X == nil {
		return "", "", false
	}
	sel, ok := ref.X.(*syntax.SelectorExpr)
	if !ok || sel.X == nil || sel.Sel == nil {
		return "", "", false
	}
	id, ok := sel.X.(*syntax.Ident)
	if !ok {
		return "", "", false
	}
	altId, ok := sel.Sel.(*syntax.Ident)
	if !ok {
		return "", "", false
	}
	return id.String(), altId.String(), true
}

// collectStructKinds walks the module's top-level type declarations
// and records the kind of every named struct type. Both
// `type record R { ... }` (StructTypeDecl) and
// `type R record { ... }` (SubTypeDecl with embedded StructSpec) are
// recognised so the alias-style and the direct-style declarations
// share the same lookup table.
func collectStructKinds(mod *syntax.Module) map[string]structKind {
	out := map[string]structKind{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		switch x := d.Def.(type) {
		case *syntax.StructTypeDecl:
			if x.Name == nil || x.KindTok == nil {
				continue
			}
			if k := kindOf(x.KindTok); k != structKindNone {
				out[x.Name.String()] = k
			}
		case *syntax.SubTypeDecl:
			if x.Field == nil || x.Field.Name == nil {
				continue
			}
			if ss, ok := x.Field.Type.(*syntax.StructSpec); ok && ss.KindTok != nil {
				if k := kindOf(ss.KindTok); k != structKindNone {
					out[x.Field.Name.String()] = k
				}
			}
		}
	}
	return out
}

// kindOf returns the structKind matching a KindTok of RECORD / SET /
// UNION. Returns structKindNone for any other (or absent) token so
// the callers can `if k != structKindNone` filter once.
func kindOf(tok syntax.Token) structKind {
	if tok == nil {
		return structKindNone
	}
	switch tok.Kind() {
	case syntax.RECORD:
		return structKindRecord
	case syntax.SET:
		return structKindSet
	case syntax.UNION:
		return structKindUnion
	}
	return structKindNone
}

// checkStructValueDeclInit verifies a single variable declaration's
// initialiser against the structKind rules. Skips silently when the
// declaration's type is anonymous, parameterised, or refers to a
// type not in our kind table.
func checkStructValueDeclInit(vd *syntax.ValueDecl, kinds map[string]structKind) []Diagnostic {
	if vd == nil {
		return nil
	}
	id, ok := vd.Type.(*syntax.Ident)
	if !ok {
		return nil
	}
	k, ok := kinds[id.String()]
	if !ok {
		return nil
	}
	var diags []Diagnostic
	for _, dec := range vd.Decls {
		if dec == nil || dec.Value == nil {
			continue
		}
		diags = append(diags, checkStructInit(id.String(), k, dec.Value, dec)...)
	}
	return diags
}

// checkStructInit verifies one value expression against the
// structKind of the declared type. For union types only the single-
// alternative `{ alt := value }` form is legal; record / set accept
// either the named-field or the positional form.
func checkStructInit(typeName string, k structKind, val syntax.Expr, locus syntax.Node) []Diagnostic {
	cl, ok := val.(*syntax.CompositeLiteral)
	if !ok {
		return nil
	}
	switch k {
	case structKindUnion:
		// Union rules per 6.2.5: exactly one entry, and that
		// entry must be a named-alternative assignment
		// `name := value`. Positional and multi-entry forms
		// are both illegal - the runtime has no rule for
		// picking which alternative the bare values bind to.
		if !isAllNamedAssign(cl) || len(cl.List) != 1 {
			return []Diagnostic{{
				Code:     "union-value-notation",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"value notation for union type %q must be { name := value } with exactly one alternative",
					typeName),
				Node: locus,
				Span: syntax.SpanOf(locus),
			}}
		}
		// Record / set / array / record-of / set-of value notation
		// is intentionally NOT flagged here: 6.2 explicitly allows
		// the positional, named and mixed forms (see Sem_0602
		// _TopLevel_020). The narrower "same field given twice"
		// rule still needs implementing; it lives in NegSem_0602
		// _TopLevel_005 / _006 and is deferred until field-name
		// position tracking lands.
	}
	return nil
}

// isAllNamedAssign reports whether every entry of the literal is
// a `name := value` BinaryExpr.
func isAllNamedAssign(cl *syntax.CompositeLiteral) bool {
	if cl == nil || len(cl.List) == 0 {
		return false
	}
	for _, e := range cl.List {
		if !isNamedAssign(e) {
			return false
		}
	}
	return true
}

// isNamedAssign matches the `name := value` form a CompositeLiteral
// uses for explicit field initialisers.
func isNamedAssign(e syntax.Expr) bool {
	be, ok := e.(*syntax.BinaryExpr)
	if !ok || be.Op == nil {
		return false
	}
	if be.Op.Kind() != syntax.ASSIGN {
		return false
	}
	_, ok = be.X.(*syntax.Ident)
	return ok
}

// checkStructInBody walks a function body and reports both the value-
// notation rule (re-applied to local var decls) and the index-access
// rule (forbidden on record/set/union LHS / RHS).
//
// Implementation note: walks every AST node, matches BinaryExpr with
// ASSIGN op on the LHS and IndexExpr otherwise. and reports both the value-
// notation rule (re-applied to local var decls) and the index-access
// rule (forbidden on record/set/union LHS / RHS).
func checkStructInBody(
	body *syntax.BlockStmt,
	kinds map[string]structKind,
	vars map[string]string,
) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(body, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		switch x := n.(type) {
		case *syntax.ValueDecl:
			diags = append(diags, checkStructValueDeclInit(x, kinds)...)
			return true
		case *syntax.BinaryExpr:
			// `lhs := rhs` where lhs is `var[i]` and var is
			// a record/set/union variable is illegal per 6.2.
			if x.Op != nil && x.Op.Kind() == syntax.ASSIGN {
				if ie, ok := x.X.(*syntax.IndexExpr); ok {
					if d := checkIndexOnNonIndexable(ie, vars, kinds, "assignment"); d != nil {
						diags = append(diags, *d)
					}
				}
			}
		case *syntax.IndexExpr:
			// Plain RHS read `var[i]` is illegal too; the
			// BinaryExpr branch above will also see it on
			// the LHS but the locus we report there is the
			// assignment, which reads better.
			if d := checkIndexOnNonIndexable(x, vars, kinds, "read"); d != nil {
				diags = append(diags, *d)
			}
		}
		return true
	})
	return diags
}

// checkIndexOnNonIndexable returns a diagnostic when an IndexExpr
// targets a variable whose declared type is a record / set / union.
// Returns nil when the target is indexable (array / record-of /
// set-of / map) or when the target type cannot be resolved from the
// local-variable table.
func checkIndexOnNonIndexable(
	ie *syntax.IndexExpr,
	vars map[string]string,
	kinds map[string]structKind,
	context string,
) *Diagnostic {
	if ie == nil || ie.X == nil {
		return nil
	}
	id, ok := ie.X.(*syntax.Ident)
	if !ok {
		return nil
	}
	typeName, ok := vars[id.String()]
	if !ok {
		return nil
	}
	k, ok := kinds[typeName]
	if !ok {
		return nil
	}
	return &Diagnostic{
		Code:     "struct-index-not-allowed",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"indexed %s on %s type %q (%s) is not allowed; only arrays, record-of and set-of support [i]",
			context, k.String(), typeName, id.String()),
		Node: ie,
		Span: syntax.SpanOf(ie),
	}
}

// collectLocalStructVars walks a function body and records the
// declared type name of every variable whose type matches a known
// struct kind. Used by checkStructInBody to map IndexExpr targets
// back to their type for the indexed-access rule.
func collectLocalStructVars(body syntax.Node, kinds map[string]structKind) map[string]string {
	out := map[string]string{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok {
			return true
		}
		id, ok := vd.Type.(*syntax.Ident)
		if !ok {
			return true
		}
		if _, ok := kinds[id.String()]; !ok {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil {
				continue
			}
			out[dec.Name.String()] = id.String()
		}
		return true
	})
	return out
}

// map_index_rules.go enforces ETSI ES 201 873-1 clause 6.2.15.4
// (Index Notation) restriction a / b on map types.
//
//   - map-index-type-mismatch: the index expression used for index
//     notation must be compatible with the map's `from` type.
//     We catch the easy literal cases (octet/bit/hex strings used
//     where the key is a character string, character strings used
//     where the key is a numeric / binary type, integer-literal
//     used where the key is a charstring, ...).
//
//   - map-value-type-mismatch: when an index expression sits on the
//     left-hand side of an assignment, the assigned RHS must be
//     compatible with the map's `to` type. We catch the literal
//     RHS cases where the literal kind clearly disagrees with the
//     declared `to` type (e.g. float literal -> integer-valued
//     map).
//
// The rule is intentionally conservative: full assignment-
// compatibility resolution is the job of the type checker. We only
// emit diagnostics when the expression on either side is a
// ValueLiteral whose token kind makes the mismatch unambiguous, so
// the false-positive rate is zero on the ETSI fixtures.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// mapKindFromTypeName returns a coarse "literal family" for the
// named TTCN-3 type when it ultimately resolves to a basic
// scalar. Used to compare a ValueLiteral's token kind to the
// map's `from`/`to` type without a full type system.
//
// Values: "charstring" / "ucharstring" / "octetstring" /
// "bitstring" / "hexstring" / "integer" / "float" / "boolean" /
// "verdict" / "" (unknown / not basic).
func mapKindFromTypeName(name string) string {
	switch name {
	case "charstring":
		return "charstring"
	case "universal charstring":
		return "ucharstring"
	case "octetstring":
		return "octetstring"
	case "bitstring":
		return "bitstring"
	case "hexstring":
		return "hexstring"
	case "integer":
		return "integer"
	case "float":
		return "float"
	case "boolean":
		return "boolean"
	case "verdict":
		return "verdict"
	}
	return ""
}

// literalKind classifies a ValueLiteral's token into the same
// family namespace as mapKindFromTypeName. Returns "" for shapes
// we don't know how to classify (e.g. enum identifiers, function
// calls, composite literals, casts).
func literalKind(e syntax.Expr) string {
	lit, ok := e.(*syntax.ValueLiteral)
	if !ok || lit == nil || lit.Tok == nil {
		return ""
	}
	switch lit.Tok.Kind() {
	case syntax.STRING:
		return "charstring"
	case syntax.BSTRING:
		// BSTRING covers bit/hex/octet binary literals; the
		// trailing suffix on the token text disambiguates.
		txt := lit.Tok.String()
		if n := len(txt); n > 0 {
			switch txt[n-1] {
			case 'B', 'b':
				return "bitstring"
			case 'H', 'h':
				return "hexstring"
			case 'O', 'o':
				return "octetstring"
			}
		}
		return ""
	case syntax.INT:
		return "integer"
	case syntax.FLOAT:
		return "float"
	case syntax.TRUE, syntax.FALSE:
		return "boolean"
	case syntax.PASS, syntax.FAIL, syntax.NONE, syntax.INCONC, syntax.ERROR:
		return "verdict"
	}
	return ""
}

// compatibleScalar reports whether a literal of family `lit` can
// be assigned to a target of family `target`. The relation is
// reflexive plus a few well-known widenings (integer literal ->
// float target).
func compatibleScalar(lit, target string) bool {
	if lit == "" || target == "" {
		return true // unknown -> defer to runtime/full type check
	}
	if lit == target {
		return true
	}
	// Integer-literal -> float target is the only widening the
	// spec gives us implicitly; both directions must be valid
	// for the rule below not to false-positive.
	if lit == "integer" && target == "float" {
		return true
	}
	return false
}

// mapSpec records the `from` / `to` type-family pair for a named
// map type. The strings are families (see mapKindFromTypeName);
// "" means "we couldn't reduce the side to a basic family" and the
// rule treats it as opaque.
type mapSpec struct {
	from string
	to   string
}

// checkMapIndexRules collects every named map type in the module
// and walks executable bodies (testcase / function / altstep) to
// flag index notation that violates the family constraints. Map
// declarations local to a function or inline aren't supported (the
// resolver only finds top-level SubTypeDecls); they would need a
// full per-scope walker.
func (a *Analyzer) checkMapIndexRules(mod *syntax.Module) []Diagnostic {
	maps := collectMapSpecs(mod)
	if len(maps) == 0 {
		return nil
	}
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		body, vars := executableBodyAndVars(n, maps)
		if body == nil {
			return true
		}
		syntax.Inspect(body, func(sn syntax.Node) bool {
			if st, ok := sn.(syntax.Stmt); ok {
				diags = append(diags, checkAssignForMapMisuse(st, vars, maps)...)
				diags = append(diags, checkReadForMapMisuse(st, vars, maps)...)
				diags = append(diags, checkDeclInitForMapMisuse(st, vars, maps)...)
			}
			if be, ok := sn.(*syntax.BinaryExpr); ok {
				diags = append(diags, checkMapEqualityMisuse(be, vars, maps)...)
			}
			if ce, ok := sn.(*syntax.CallExpr); ok {
				diags = append(diags, checkUnmapCallMisuse(ce, vars, maps)...)
			}
			return true
		})
		return true
	})
	return diags
}

// checkMapEqualityMisuse flags `==` / `!=` whose either operand
// is a known map-typed local variable. ETSI 6.2.15.1 restriction
// b forbids map values from appearing as operands of equality
// expressions.
func checkMapEqualityMisuse(
	be *syntax.BinaryExpr,
	vars map[string]string,
	maps map[string]mapSpec,
) []Diagnostic {
	if be == nil || be.Op == nil {
		return nil
	}
	if be.Op.Kind() != syntax.EQ && be.Op.Kind() != syntax.NE {
		return nil
	}
	leftMap := mapVarName(be.X, vars)
	rightMap := mapVarName(be.Y, vars)
	if leftMap == "" && rightMap == "" {
		return nil
	}
	hit := leftMap
	if hit == "" {
		hit = rightMap
	}
	return []Diagnostic{{
		Code:     "map-equality-not-allowed",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"map-typed values (here %q) cannot appear as operands of equality (ETSI 6.2.15.1 restriction b)",
			hit),
		Node: be,
		Span: syntax.SpanOf(be),
	}}
}

// mapVarName returns the variable name of e when e is a bare
// Ident that resolves to a map-typed local variable in vars.
func mapVarName(e syntax.Expr, vars map[string]string) string {
	name := identName(e)
	if name == "" {
		return ""
	}
	if _, ok := vars[name]; !ok {
		return ""
	}
	return name
}

// checkUnmapCallMisuse flags `unmap(m, key)` calls whose key
// literal disagrees with the map's `from` type. The runtime
// otherwise discards the call silently and the negative fixture
// passes.
func checkUnmapCallMisuse(
	ce *syntax.CallExpr,
	vars map[string]string,
	maps map[string]mapSpec,
) []Diagnostic {
	if ce == nil || ce.Fun == nil || identName(ce.Fun) != "unmap" {
		return nil
	}
	if ce.Args == nil || len(ce.Args.List) < 2 {
		return nil
	}
	mapName := mapVarName(ce.Args.List[0], vars)
	if mapName == "" {
		return nil
	}
	spec, has := maps[vars[mapName]]
	if !has {
		return nil
	}
	if d := checkKeyMismatch(ce.Args.List[1], spec.from, vars[mapName]); d != nil {
		return []Diagnostic{*d}
	}
	return nil
}

// checkDeclInitForMapMisuse handles two related shapes:
//
//   - `var Map v := { [k] := v, ... };` -- the initializer is a
//     composite literal where each entry is an IndexExpr-keyed
//     assignment. Each key is checked against the map's from-type
//     and each value against the to-type.
//   - `var T v := m[k];` -- the initializer reads from a known
//     map variable; the key is checked against the from-type.
func checkDeclInitForMapMisuse(
	st syntax.Stmt,
	vars map[string]string,
	maps map[string]mapSpec,
) []Diagnostic {
	vd, ok := st.(*syntax.DeclStmt)
	if !ok || vd == nil {
		return nil
	}
	val, ok := vd.Decl.(*syntax.ValueDecl)
	if !ok || val == nil {
		return nil
	}
	typeName := identName(val.Type)
	for _, dec := range val.Decls {
		if dec == nil || dec.Value == nil {
			continue
		}
		if spec, ok := maps[typeName]; ok {
			// `var Map v := { [k] := v, ... };` form.
			if cl, ok := dec.Value.(*syntax.CompositeLiteral); ok {
				for _, item := range cl.List {
					diags := checkIndexedAssignEntry(item, spec, typeName)
					if len(diags) > 0 {
						return diags
					}
				}
				continue
			}
		}
		// `var T v := m[k];` form: check the RHS as a read.
		if ie, ok := dec.Value.(*syntax.IndexExpr); ok {
			varName := identName(ie.X)
			if mapName, isMap := vars[varName]; isMap {
				if d := checkKeyMismatch(ie.Index, maps[mapName].from, mapName); d != nil {
					return []Diagnostic{*d}
				}
			}
		}
	}
	return nil
}

// checkIndexedAssignEntry inspects a single `[key] := value`
// entry of a map composite literal.
func checkIndexedAssignEntry(item syntax.Expr, spec mapSpec, mapName string) []Diagnostic {
	be, ok := item.(*syntax.BinaryExpr)
	if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
		return nil
	}
	ie, ok := be.X.(*syntax.IndexExpr)
	if !ok || ie == nil {
		return nil
	}
	var diags []Diagnostic
	if d := checkKeyMismatch(ie.Index, spec.from, mapName); d != nil {
		diags = append(diags, *d)
	}
	if d := checkValueMismatch(be.Y, spec.to, mapName); d != nil {
		diags = append(diags, *d)
	}
	return diags
}

// collectMapSpecs walks top-level type decls and records every
// named `type map from K to V Name;` as a mapSpec. The parser
// emits MapTypeDecl for the dedicated `type map ...` form; the
// nested SubTypeDecl + MapSpec path covers any other shape we
// might see.
func collectMapSpecs(mod *syntax.Module) map[string]mapSpec {
	out := map[string]mapSpec{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		switch x := d.Def.(type) {
		case *syntax.MapTypeDecl:
			if x == nil || x.Name == nil || x.Spec == nil {
				continue
			}
			out[x.Name.String()] = mapSpec{
				from: mapKindFromTypeName(typeSpecToName(x.Spec.FromType)),
				to:   mapKindFromTypeName(typeSpecToName(x.Spec.ToType)),
			}
		case *syntax.SubTypeDecl:
			if x.Field == nil || x.Field.Name == nil {
				continue
			}
			ms, ok := x.Field.Type.(*syntax.MapSpec)
			if !ok || ms == nil {
				continue
			}
			out[x.Field.Name.String()] = mapSpec{
				from: mapKindFromTypeName(typeSpecToName(ms.FromType)),
				to:   mapKindFromTypeName(typeSpecToName(ms.ToType)),
			}
		}
	}
	return out
}

// typeSpecToName extracts the underlying named-type identifier
// from a TypeSpec when the spec is a bare RefSpec(Ident). Returns
// "" for any compound shape (we leave compound types alone).
func typeSpecToName(t syntax.TypeSpec) string {
	rs, ok := t.(*syntax.RefSpec)
	if !ok || rs == nil {
		// `universal charstring` parses as a TokenNode in the
		// builtin types path. Best-effort: stringify and trim.
		return ""
	}
	return identName(rs.X)
}

// executableBodyAndVars returns the body (BlockStmt) and the
// local variable -> map-type table for a FuncDecl / TestcaseDecl /
// AltstepDecl node. Returns (nil, nil) for any other shape.
func executableBodyAndVars(
	n syntax.Node,
	maps map[string]mapSpec,
) (syntax.Node, map[string]string) {
	switch d := n.(type) {
	case *syntax.FuncDecl:
		if d == nil || d.Body == nil {
			return nil, nil
		}
		return d.Body, collectLocalMapVars(d.Body, maps)
	}
	return nil, nil
}

// collectLocalMapVars returns var name -> map type-name for every
// `var T name ...` declaration in body whose T is a known map
// subtype.
func collectLocalMapVars(body syntax.Node, maps map[string]mapSpec) map[string]string {
	out := map[string]string{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		typeName := identName(vd.Type)
		if typeName == "" {
			return true
		}
		if _, isMap := maps[typeName]; !isMap {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil {
				continue
			}
			out[dec.Name.String()] = typeName
		}
		return true
	})
	return out
}

// checkAssignForMapMisuse handles `m[key] := value` statements:
// validates both the key against m's from-type and value against
// m's to-type.
func checkAssignForMapMisuse(
	st syntax.Stmt,
	vars map[string]string,
	maps map[string]mapSpec,
) []Diagnostic {
	exs, ok := st.(*syntax.ExprStmt)
	if !ok || exs == nil {
		return nil
	}
	be, ok := exs.Expr.(*syntax.BinaryExpr)
	if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
		return nil
	}
	ie, ok := be.X.(*syntax.IndexExpr)
	if !ok || ie == nil {
		return nil
	}
	varName := identName(ie.X)
	if varName == "" {
		return nil
	}
	mapName, isMap := vars[varName]
	if !isMap {
		return nil
	}
	spec, has := maps[mapName]
	if !has {
		return nil
	}
	var diags []Diagnostic
	if d := checkKeyMismatch(ie.Index, spec.from, mapName); d != nil {
		diags = append(diags, *d)
	}
	if d := checkValueMismatch(be.Y, spec.to, mapName); d != nil {
		diags = append(diags, *d)
	}
	return diags
}

// checkReadForMapMisuse flags `... := m[key]` where the key
// literal's family clearly disagrees with the map's from-type.
// We only inspect ExprStmt → BinaryExpr(:=) → RHS IndexExpr; deep
// nested reads (e.g. inside function call args) are deferred to
// the broader type checker.
func checkReadForMapMisuse(
	st syntax.Stmt,
	vars map[string]string,
	maps map[string]mapSpec,
) []Diagnostic {
	exs, ok := st.(*syntax.ExprStmt)
	if !ok || exs == nil {
		return nil
	}
	be, ok := exs.Expr.(*syntax.BinaryExpr)
	if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
		return nil
	}
	ie, ok := be.Y.(*syntax.IndexExpr)
	if !ok || ie == nil {
		return nil
	}
	varName := identName(ie.X)
	if varName == "" {
		return nil
	}
	mapName, isMap := vars[varName]
	if !isMap {
		return nil
	}
	spec, has := maps[mapName]
	if !has {
		return nil
	}
	if d := checkKeyMismatch(ie.Index, spec.from, mapName); d != nil {
		return []Diagnostic{*d}
	}
	return nil
}

// checkKeyMismatch emits a map-index-type-mismatch when the key
// literal's family is incompatible with the map's from-type.
func checkKeyMismatch(idx syntax.Expr, from string, mapName string) *Diagnostic {
	if from == "" {
		return nil
	}
	lk := literalKind(idx)
	if lk == "" {
		return nil
	}
	if compatibleScalar(lk, from) {
		return nil
	}
	return &Diagnostic{
		Code:     "map-index-type-mismatch",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"map %q expects index of type %s, got %s literal",
			mapName, from, lk),
		Node: idx,
		Span: syntax.SpanOf(idx),
	}
}

// checkValueMismatch emits a map-value-type-mismatch when the
// assigned RHS literal's family is incompatible with the map's
// to-type.
func checkValueMismatch(val syntax.Expr, to string, mapName string) *Diagnostic {
	if to == "" {
		return nil
	}
	lk := literalKind(val)
	if lk == "" {
		return nil
	}
	if compatibleScalar(lk, to) {
		return nil
	}
	return &Diagnostic{
		Code:     "map-value-type-mismatch",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"map %q expects values of type %s, got %s literal",
			mapName, to, lk),
		Node: val,
		Span: syntax.SpanOf(val),
	}
}

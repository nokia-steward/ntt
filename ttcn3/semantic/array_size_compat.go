// array_size_compat.go enforces ETSI ES 201 873-1 clause 6.3.1
// "Type compatibility" - the array-size rule:
//
//	A value of array type T[N] can only be assigned to a variable
//	whose declared array type is T[N]; assigning a T[M] value to
//	a T[N] variable when M != N is rejected even if the element
//	types are identical.
//
// We catch the static shape:
//
//	type integer A[1];
//	var integer v_int[2] := { 5, 4 };
//	var A v_a;
//	v_a := v_int;             // <- array-size mismatch
//
// Three pieces of state, all per function body:
//   - subtypeArraySize: module-level subtypes whose declaration
//     pins a single array dimension (collected once per module);
//   - localArraySize: function-local vars whose declaration pins
//     an inline array dimension on the Declarator;
//   - literalArrayLen: function-local vars whose declared
//     initialiser is a composite literal of known length.
//
// The composite-literal initialiser also feeds the direct-assign
// path so `v_a := { 5, 4 }` against `A[1]` is caught.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkArraySizeCompat(mod *syntax.Module) []Diagnostic {
	diags := checkMixedLiteralNotation(mod)
	subtypeSize := collectSubtypeArraySizes(mod)
	if len(subtypeSize) == 0 {
		return diags
	}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		diags = append(diags, checkArraySizeInBody(fn.Body, subtypeSize)...)
	}
	return diags
}

// checkMixedLiteralNotation flags composite literals that mix
// positional values with INDEXED assignments in the same `{ ... }`.
// ETSI 6.2 forbids this combination because the positional
// elements implicitly start at index 0, so any `[i] := v` whose
// index overlaps the positional run double-binds the slot:
//
//	{ 1, [0] := 3 }          // positional + [idx] - illegal
//	{ 1, 2, 3 }              // positional only - ok
//	{ [0] := 3, [1] := 4 }   // indexed only - ok
//	{ 5, field3 := 3.14 }    // positional + named field - ok
//	                           (record-style; the positional run
//	                            covers leading fields, the named
//	                            assignment fills in a later one)
//	{ a := 1, b := 2 }       // named only - ok
//
// We deliberately accept positional + named-field combinations
// because that's the documented "mixed notation" form for records
// per Sem_0602_TopLevel_20 in the conformance suite.
func checkMixedLiteralNotation(mod *syntax.Module) []Diagnostic {
	recordFields := collectRecordFieldOrder(mod)
	varRecordType := collectVarRecordType(mod, recordFields)
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		cl, ok := n.(*syntax.CompositeLiteral)
		if !ok || cl == nil || len(cl.List) < 2 {
			return true
		}
		positional := 0
		var indexedAt []int
		var namedAt []string
		allIndexLiteral := true
		for _, el := range cl.List {
			be, isAssign := el.(*syntax.BinaryExpr)
			if !isAssign || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
				positional++
				continue
			}
			if ix, isIndex := be.X.(*syntax.IndexExpr); isIndex {
				if ix.Index == nil {
					allIndexLiteral = false
					continue
				}
				n, ok := intLiteralValue(ix.Index)
				if !ok {
					allIndexLiteral = false
					continue
				}
				indexedAt = append(indexedAt, n)
				continue
			}
			if id, isField := be.X.(*syntax.Ident); isField {
				namedAt = append(namedAt, id.String())
			}
		}
		if positional == 0 {
			return true
		}
		// Positional run implicitly covers indices [0, positional)
		// for arrays / record-of and the first `positional`
		// fields for records. Any indexed / named assignment
		// referencing a slot inside that range is a double bind.
		if len(indexedAt) > 0 && allIndexLiteral {
			for _, i := range indexedAt {
				if i < positional {
					diags = append(diags, Diagnostic{
						Code:     "mixed-literal-notation",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"composite literal: indexed assignment [%d] := ... overlaps the positional run that already covers indices [0..%d) (ETSI 6.2)",
							i, positional),
						Node: cl,
						Span: syntax.SpanOf(cl),
					})
					break
				}
			}
		}
		if len(namedAt) > 0 {
			fieldOrder := findEnclosingRecordFields(cl, mod, recordFields, varRecordType)
			if fieldOrder != nil {
				covered := map[string]bool{}
				if positional <= len(fieldOrder) {
					for _, f := range fieldOrder[:positional] {
						covered[f] = true
					}
				}
				for _, name := range namedAt {
					if covered[name] {
						diags = append(diags, Diagnostic{
							Code:     "mixed-literal-notation",
							Severity: SeverityError,
							Message: fmt.Sprintf(
								"composite literal: field %q is assigned via named notation but is already covered by the positional run (ETSI 6.2)",
								name),
							Node: cl,
							Span: syntax.SpanOf(cl),
						})
						break
					}
				}
			}
		}
		return true
	})
	return diags
}

// collectRecordFieldOrder maps each `type record T { f1, f2, ... }`
// to the in-source field-name order. record-of and set-of
// declarations are skipped: their elements are unnamed.
func collectRecordFieldOrder(mod *syntax.Module) map[string][]string {
	out := map[string][]string{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		st, ok := d.Def.(*syntax.StructTypeDecl)
		if !ok || st.Name == nil {
			continue
		}
		if st.KindTok == nil {
			continue
		}
		switch st.KindTok.Kind() {
		case syntax.RECORD, syntax.SET:
			// record/set with named fields - good.
		default:
			continue
		}
		var names []string
		for _, f := range st.Fields {
			if f == nil || f.Name == nil {
				continue
			}
			names = append(names, f.Name.String())
		}
		if len(names) > 0 {
			out[st.Name.String()] = names
		}
	}
	return out
}

// collectVarRecordType maps each `var <T> x` declaration whose T
// is a known record type to its field-order list. We use this to
// look up the receiver type of a composite-literal initialiser
// for `var T x := { ... }`.
func collectVarRecordType(mod *syntax.Module, recordFields map[string][]string) map[string][]string {
	out := map[string][]string{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		name := identName(vd.Type)
		fields, ok := recordFields[name]
		if !ok {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil {
				continue
			}
			out[dec.Name.String()] = fields
		}
		return true
	})
	return out
}

// findEnclosingRecordFields tries to recover the field-name list
// of the record type that a composite-literal initialises. Three
// strategies, in order:
//
//  1. The composite literal is the immediate RHS of a `var T x
//     := { ... }` declaration: look T up in recordFields.
//  2. The literal is the immediate RHS of an assignment `x := {
//     ... }`: look x up in varRecordType.
//
// Returns nil when neither shape matches. Future strategies (named
// args to a function call, nested fields) plug in here.
func findEnclosingRecordFields(cl *syntax.CompositeLiteral, mod *syntax.Module, recordFields map[string][]string, varRecordType map[string][]string) []string {
	var found []string
	syntax.Inspect(mod, func(n syntax.Node) bool {
		if found != nil {
			return false
		}
		if vd, ok := n.(*syntax.ValueDecl); ok {
			typeName := identName(vd.Type)
			for _, dec := range vd.Decls {
				if dec != nil && dec.Value == cl {
					if fields, ok := recordFields[typeName]; ok {
						found = fields
						return false
					}
				}
			}
		}
		if be, ok := n.(*syntax.BinaryExpr); ok &&
			be.Op != nil && be.Op.Kind() == syntax.ASSIGN && be.Y == cl {
			if id, ok := be.X.(*syntax.Ident); ok {
				if fields, ok := varRecordType[id.String()]; ok {
					found = fields
				}
			}
		}
		return found == nil
	})
	return found
}

// checkArraySizeInBody walks one function body and flags every
// assignment whose target has a known array size and whose source
// has a different known array size. Variables whose size we
// cannot resolve are skipped silently.
func checkArraySizeInBody(body *syntax.BlockStmt, subtypeSize map[string]int) []Diagnostic {
	localSize := collectLocalArraySizes(body, subtypeSize)
	literalArr := collectArrayLiteralInits(body)
	// Reassignments invalidate the literalArr capture for that var.
	syntax.Inspect(body, func(n syntax.Node) bool {
		be, ok := n.(*syntax.BinaryExpr)
		if !ok || be == nil || be.Op == nil ||
			be.Op.Kind() != syntax.ASSIGN {
			return true
		}
		if id, ok := be.X.(*syntax.Ident); ok {
			delete(literalArr, id.String())
		}
		return true
	})

	var diags []Diagnostic
	// Direct init: `var A v := { 8, 11, ... }`.
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		typeName := identName(vd.Type)
		expected, ok := subtypeSize[typeName]
		if !ok {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Value == nil {
				continue
			}
			if got, ok := compositeLiteralLen(dec.Value); ok && got != expected {
				diags = append(diags, Diagnostic{
					Code:     "array-size-mismatch",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"array literal has %d elements; %q is declared with size %d (ETSI 6.3.1)",
						got, typeName, expected),
					Node: dec,
					Span: syntax.SpanOf(dec),
				})
			}
		}
		return true
	})
	// Plain assign: `v := w` or `v := { ... }`.
	syntax.Inspect(body, func(n syntax.Node) bool {
		be, ok := n.(*syntax.BinaryExpr)
		if !ok || be == nil || be.Op == nil ||
			be.Op.Kind() != syntax.ASSIGN {
			return true
		}
		id, ok := be.X.(*syntax.Ident)
		if !ok {
			return true
		}
		expected, ok := localSize[id.String()]
		if !ok {
			return true
		}
		got, found := sourceArrayLen(be.Y, localSize, literalArr)
		if !found || got == expected {
			return true
		}
		diags = append(diags, Diagnostic{
			Code:     "array-size-mismatch",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"array value of size %d assigned to %q whose declared array size is %d (ETSI 6.3.1)",
				got, id.String(), expected),
			Node: be,
			Span: syntax.SpanOf(be),
		})
		return true
	})
	return diags
}

// compositeLiteralLen returns (len, true) when expr is a literal
// `{e1, e2, ...}` whose elements are positional values. Literals
// using the indexed-assignment notation (`{ [i] := v, [j] := w
// }`) or field-name assignment (`{ f := v }`) return (_, false)
// because the literal size doesn't correspond to the array
// dimension: an array of size 5 with `{ [3] := 1 }` is a valid
// initialiser that leaves four elements unbound.
func compositeLiteralLen(expr syntax.Expr) (int, bool) {
	cl, ok := expr.(*syntax.CompositeLiteral)
	if !ok || cl == nil {
		return 0, false
	}
	for _, el := range cl.List {
		if be, ok := el.(*syntax.BinaryExpr); ok && be.Op != nil && be.Op.Kind() == syntax.ASSIGN {
			return 0, false
		}
	}
	return len(cl.List), true
}

// sourceArrayLen resolves the size of the RHS of an assignment.
// Three shapes are handled: composite literal, bare-ident var
// with a declared array dim, bare-ident var captured in
// literalArr (an array-literal init).
func sourceArrayLen(expr syntax.Expr, localSize map[string]int, literalArr map[string]int) (int, bool) {
	if n, ok := compositeLiteralLen(expr); ok {
		return n, true
	}
	id, ok := expr.(*syntax.Ident)
	if !ok || id == nil {
		return 0, false
	}
	if n, ok := localSize[id.String()]; ok {
		return n, true
	}
	if n, ok := literalArr[id.String()]; ok {
		return n, true
	}
	return 0, false
}

// collectSubtypeArraySizes walks the module and returns a map of
// subtype-name -> declared single-dimension array size. Multi-
// dimensional subtypes are skipped (they need full element-by-
// element compatibility which we don't model yet).
func collectSubtypeArraySizes(mod *syntax.Module) map[string]int {
	out := map[string]int{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		st, ok := d.Def.(*syntax.SubTypeDecl)
		if !ok || st.Field == nil || st.Field.Name == nil {
			continue
		}
		if len(st.Field.ArrayDef) != 1 {
			continue
		}
		n, ok := arrayDimLiteralSize(st.Field.ArrayDef[0])
		if !ok {
			continue
		}
		out[st.Field.Name.String()] = n
	}
	return out
}

// collectLocalArraySizes returns each local var's effective array
// dimension. Two paths feed it: the var's declared subtype (when
// looked up in subtypeSize) and an inline array dim on the
// Declarator (the `var T x[N] := ...` shape).
func collectLocalArraySizes(body syntax.Node, subtypeSize map[string]int) map[string]int {
	out := map[string]int{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		viaType, _ := subtypeSize[identName(vd.Type)]
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil {
				continue
			}
			if len(dec.ArrayDef) == 1 {
				if n, ok := arrayDimLiteralSize(dec.ArrayDef[0]); ok {
					out[dec.Name.String()] = n
					continue
				}
			}
			if viaType > 0 {
				out[dec.Name.String()] = viaType
			}
		}
		return true
	})
	return out
}

// collectArrayLiteralInits records each `var T x := {e1,e2,...}`
// the body declares, keyed by name. Vars that are later
// reassigned are dropped by the caller.
func collectArrayLiteralInits(body syntax.Node) map[string]int {
	out := map[string]int{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil || dec.Value == nil {
				continue
			}
			if n, ok := compositeLiteralLen(dec.Value); ok {
				out[dec.Name.String()] = n
			}
		}
		return true
	})
	return out
}

// arrayDimLiteralSize extracts the constant size out of a single
// array-dim `[<expr>]`. We only support the literal `[N]` and
// `[lo..hi]` shapes; constants, identifiers and arithmetic fall
// through silently to keep the false-positive rate at zero.
func arrayDimLiteralSize(pe *syntax.ParenExpr) (int, bool) {
	if pe == nil || len(pe.List) != 1 {
		return 0, false
	}
	expr := pe.List[0]
	if be, ok := expr.(*syntax.BinaryExpr); ok && be.Op != nil && be.Op.String() == ".." {
		lo, ok1 := intLiteralValue(be.X)
		hi, ok2 := intLiteralValue(be.Y)
		if ok1 && ok2 && hi >= lo {
			return hi - lo + 1, true
		}
		return 0, false
	}
	if n, ok := intLiteralValue(expr); ok && n >= 0 {
		return n, true
	}
	return 0, false
}

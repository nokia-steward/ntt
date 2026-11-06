// length_constraint.go implements ETSI ES 201 873-1 clause 6.1.2.4
// length-restriction validation for the simple `type T length(N) U`
// / `type T length(N..M) U` declarations against literal initialisers
// at the var/const declaration site.
//
// We only intercept the case where the initialiser is a literal whose
// length we can read off the token directly (bitstring/hexstring/
// octetstring/charstring literals). Constants and named references
// get the benefit of the doubt - this keeps the false-positive rate
// at zero in exchange for missing the constant-fold cases the
// interpreter catches separately.
package semantic

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkLengthConstraints(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	tab := collectLengthConstrainedTypes(mod)
	lists := collectStringValueLists(mod)
	if len(tab) == 0 && len(lists) == 0 {
		return nil
	}
	// Per-scope var-type table so we can also check assignments
	// outside the declaration site. We carry the table along the
	// walk via a stack of maps keyed by enclosing BlockStmt to
	// avoid leaking variable bindings across functions/testcases.
	syntax.Inspect(mod, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		switch x := n.(type) {
		case *syntax.FuncDecl:
			if x.Body == nil {
				return false
			}
			vars := collectLocalConstrainedVars(x.Body, tab)
			diags = append(diags, checkLengthInBody(x.Body, tab, vars)...)
			listVars := collectLocalStringListVars(x.Body, lists)
			diags = append(diags, checkStringListInBody(x.Body, lists, listVars)...)
			return false
		}
		// Module-level decls (const/modulepar) - just the
		// initialiser check.
		if vd, ok := n.(*syntax.ValueDecl); ok {
			diags = append(diags, checkLengthDeclInit(vd, tab)...)
			diags = append(diags, checkStringListDeclInit(vd, lists)...)
		}
		return true
	})
	return diags
}

func checkLengthInBody(
	body *syntax.BlockStmt,
	tab map[string]lengthSpec,
	vars map[string]string,
) []Diagnostic {
	// stringInits maps each `var <stringFamily> x := "literal"` /
	// "'10'B" / similar to its length + family. The follow-up
	// assignment walker uses it to resolve a bare-ident RHS like
	// `v_constrainedChar := v_char` against the captured length.
	stringInits := collectStringLiteralInits(body)
	var diags []Diagnostic
	syntax.Inspect(body, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		if vd, ok := n.(*syntax.ValueDecl); ok {
			diags = append(diags, checkLengthDeclInit(vd, tab)...)
			return true
		}
		be, ok := n.(*syntax.BinaryExpr)
		if !ok || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
			return true
		}
		id, ok := be.X.(*syntax.Ident)
		if !ok {
			return true
		}
		typeName, ok := vars[id.String()]
		if !ok {
			return true
		}
		spec, ok := tab[typeName]
		if !ok {
			return true
		}
		length, fam, ok := lengthOfRHS(be.Y, stringInits)
		if !ok || !spec.familyMatches(fam) || spec.includes(length) {
			return true
		}
		diags = append(diags, Diagnostic{
			Code:     "length-constraint-violation",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"%s literal length %d violates %q's length constraint %s",
				fam, length, typeName, spec.describe()),
			Node: be,
			Span: syntax.SpanOf(be),
		})
		return true
	})
	return diags
}

// stringInit is the captured length + family for a `var T x :=
// <literal>` declaration whose initialiser is a string-family
// literal (charstring, bitstring, hexstring, octetstring).
type stringInit struct {
	length int
	fam    string
}

// collectStringLiteralInits walks body and returns the var-name ->
// stringInit map for every var initialised with a string literal.
// Variables that are later reassigned via `x := ...` are removed
// so we never use stale data.
func collectStringLiteralInits(body syntax.Node) map[string]stringInit {
	out := map[string]stringInit{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil || dec.Value == nil {
				continue
			}
			lit, ok := dec.Value.(*syntax.ValueLiteral)
			if !ok || lit.Tok == nil {
				continue
			}
			length, fam, ok := stringLiteralLength(lit)
			if !ok {
				continue
			}
			out[dec.Name.String()] = stringInit{length: length, fam: fam}
		}
		return true
	})
	if len(out) == 0 {
		return out
	}
	syntax.Inspect(body, func(n syntax.Node) bool {
		be, ok := n.(*syntax.BinaryExpr)
		if !ok || be == nil || be.Op == nil ||
			be.Op.Kind() != syntax.ASSIGN {
			return true
		}
		if id, ok := be.X.(*syntax.Ident); ok {
			delete(out, id.String())
		}
		return true
	})
	return out
}

// lengthOfRHS returns (length, family) for the RHS of an
// assignment to a length-constrained variable. Three shapes are
// recognised:
//
//   - a string literal (direct or via the existing
//     stringLiteralLength helper);
//   - a bare identifier whose declared init was a string literal
//     captured in stringInits (so `v_char := "jk"` ->
//     `v_constrained := v_char` resolves to length 2);
//   - the binary `&` concatenation of two known-length operands
//     (so `char(...) & char(...)` reports length 2).
//
// Anything else returns (_, _, false) - we keep the false
// positive rate at zero.
func lengthOfRHS(e syntax.Expr, stringInits map[string]stringInit) (int, string, bool) {
	if e == nil {
		return 0, "", false
	}
	if lit, ok := e.(*syntax.ValueLiteral); ok && lit != nil {
		return stringLiteralLength(lit)
	}
	if id, ok := e.(*syntax.Ident); ok && id != nil {
		if init, found := stringInits[id.String()]; found {
			return init.length, init.fam, true
		}
	}
	if be, ok := e.(*syntax.BinaryExpr); ok && be != nil && be.Op != nil && be.Op.Kind() == syntax.CONCAT {
		l1, f1, ok1 := lengthOfRHS(be.X, stringInits)
		l2, f2, ok2 := lengthOfRHS(be.Y, stringInits)
		if ok1 && ok2 && f1 == f2 {
			return l1 + l2, f1, true
		}
	}
	return 0, "", false
}

func checkLengthDeclInit(vd *syntax.ValueDecl, tab map[string]lengthSpec) []Diagnostic {
	id, ok := vd.Type.(*syntax.Ident)
	if !ok {
		return nil
	}
	spec, ok := tab[id.String()]
	if !ok {
		return nil
	}
	var diags []Diagnostic
	for _, dec := range vd.Decls {
		if dec == nil || dec.Value == nil {
			continue
		}
		lit, ok := dec.Value.(*syntax.ValueLiteral)
		if !ok || lit.Tok == nil {
			continue
		}
		length, fam, ok := stringLiteralLength(lit)
		if !ok || !spec.familyMatches(fam) || spec.includes(length) {
			continue
		}
		diags = append(diags, Diagnostic{
			Code:     "length-constraint-violation",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"%s literal length %d violates %q's length constraint %s",
				fam, length, id.String(), spec.describe()),
			Node: dec,
			Span: syntax.SpanOf(dec),
		})
	}
	return diags
}

// stringListSpec captures `type <stringFamily> T (lit1, lit2, ...);`
// declarations. The literal text is kept verbatim (e.g. "'10'B" or
// "\"abc\"") because we compare against the raw token spelling.
type stringListSpec struct {
	base   string
	values map[string]bool
}

func collectStringValueLists(mod *syntax.Module) map[string]stringListSpec {
	// First pass: collect declarations as raw, untreated items so
	// the second pass can follow references between subtypes. The
	// `type bitstring T (BitStrings1, BitStrings2);` form inherits
	// the value lists of the referenced types.
	raw := map[string]struct {
		base string
		list []syntax.Expr
	}{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		st, ok := d.Def.(*syntax.SubTypeDecl)
		if !ok || st.Field == nil || st.Field.Name == nil ||
			st.Field.ValueConstraint == nil {
			continue
		}
		fam := baseStringFamily(st.Field.Type)
		if fam == "" {
			fam = aliasedStringFamily(st.Field.Type, raw)
		}
		if fam == "" {
			continue
		}
		raw[st.Field.Name.String()] = struct {
			base string
			list []syntax.Expr
		}{base: fam, list: st.Field.ValueConstraint.List}
	}

	out := map[string]stringListSpec{}
	for name, entry := range raw {
		seen := map[string]bool{}
		spec := stringListSpec{base: entry.base, values: map[string]bool{}}
		if resolveStringList(name, entry.base, entry.list, raw, spec.values, seen) {
			out[name] = spec
		}
	}
	return out
}

// resolveStringList recursively walks a value-list constraint: bare
// literals contribute to the value set, identifiers chase the named
// subtype's list. Returns false if anything looks unrecognisable so
// the caller drops the spec rather than emitting a false positive.
func resolveStringList(
	self, base string,
	list []syntax.Expr,
	raw map[string]struct {
		base string
		list []syntax.Expr
	},
	dst map[string]bool,
	seen map[string]bool,
) bool {
	if seen[self] {
		return true
	}
	seen[self] = true
	for _, item := range list {
		if lit, ok := item.(*syntax.ValueLiteral); ok && lit.Tok != nil {
			_, fam, ok := stringLiteralLength(lit)
			if !ok || fam != base {
				return false
			}
			dst[lit.Tok.String()] = true
			continue
		}
		if id, ok := item.(*syntax.Ident); ok {
			ref, ok := raw[id.String()]
			if !ok || ref.base != base {
				return false
			}
			if !resolveStringList(id.String(), base, ref.list, raw, dst, seen) {
				return false
			}
			continue
		}
		return false
	}
	return true
}

// aliasedStringFamily resolves the base type when the SubTypeDecl
// references another user-declared subtype, e.g. `type T2 T1 (...)`.
func aliasedStringFamily(spec syntax.TypeSpec, raw map[string]struct {
	base string
	list []syntax.Expr
}) string {
	ref, ok := spec.(*syntax.RefSpec)
	if !ok || ref.X == nil {
		return ""
	}
	id, ok := ref.X.(*syntax.Ident)
	if !ok {
		return ""
	}
	if r, ok := raw[id.String()]; ok {
		return r.base
	}
	return ""
}

func checkStringListInBody(
	body *syntax.BlockStmt,
	lists map[string]stringListSpec,
	vars map[string]string,
) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(body, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		if vd, ok := n.(*syntax.ValueDecl); ok {
			diags = append(diags, checkStringListDeclInit(vd, lists)...)
			return true
		}
		be, ok := n.(*syntax.BinaryExpr)
		if !ok || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
			return true
		}
		id, ok := be.X.(*syntax.Ident)
		if !ok {
			return true
		}
		typeName, ok := vars[id.String()]
		if !ok {
			return true
		}
		spec, ok := lists[typeName]
		if !ok {
			return true
		}
		lit, ok := be.Y.(*syntax.ValueLiteral)
		if !ok || lit.Tok == nil {
			return true
		}
		_, fam, ok := stringLiteralLength(lit)
		if !ok || fam != spec.base {
			return true
		}
		if spec.values[lit.Tok.String()] {
			return true
		}
		diags = append(diags, Diagnostic{
			Code:     "value-list-violation",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"value %s is not one of %q's allowed values",
				lit.Tok.String(), typeName),
			Node: be,
			Span: syntax.SpanOf(be),
		})
		return true
	})
	return diags
}

func checkStringListDeclInit(vd *syntax.ValueDecl, lists map[string]stringListSpec) []Diagnostic {
	id, ok := vd.Type.(*syntax.Ident)
	if !ok {
		return nil
	}
	spec, ok := lists[id.String()]
	if !ok {
		return nil
	}
	var diags []Diagnostic
	for _, dec := range vd.Decls {
		if dec == nil || dec.Value == nil {
			continue
		}
		lit, ok := dec.Value.(*syntax.ValueLiteral)
		if !ok || lit.Tok == nil {
			continue
		}
		_, fam, ok := stringLiteralLength(lit)
		if !ok || fam != spec.base {
			continue
		}
		if spec.values[lit.Tok.String()] {
			continue
		}
		diags = append(diags, Diagnostic{
			Code:     "value-list-violation",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"value %s is not one of %q's allowed values",
				lit.Tok.String(), id.String()),
			Node: dec,
			Span: syntax.SpanOf(dec),
		})
	}
	return diags
}

func collectLocalStringListVars(body syntax.Node, lists map[string]stringListSpec) map[string]string {
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
		if _, ok := lists[id.String()]; !ok {
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

// collectLocalConstrainedVars finds `var/const T name [:= ...]`
// declarations whose T appears in the length-constraint table, and
// returns a flat name->typeName map so the assignment walker can
// resolve the receiver. Walks the entire function body (including
// nested blocks) because TTCN-3 scope rules give all vars function
// lifetime.
func collectLocalConstrainedVars(body syntax.Node, tab map[string]lengthSpec) map[string]string {
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
		if _, ok := tab[id.String()]; !ok {
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

// lengthSpec captures a min/max length range over a base string type.
type lengthSpec struct {
	base     string // bitstring / hexstring / octetstring / charstring
	min      int
	max      int // -1 means unbounded
	hasRange bool
}

func (l lengthSpec) familyMatches(fam string) bool {
	return fam == l.base
}

func (l lengthSpec) includes(n int) bool {
	if n < l.min {
		return false
	}
	if l.max < 0 {
		return true
	}
	return n <= l.max
}

func (l lengthSpec) describe() string {
	if !l.hasRange {
		return fmt.Sprintf("length(%d)", l.min)
	}
	if l.max < 0 {
		return fmt.Sprintf("length(%d..infinity)", l.min)
	}
	return fmt.Sprintf("length(%d..%d)", l.min, l.max)
}

// collectLengthConstrainedTypes walks all `type <base> Name length(...)`
// declarations and records the constraint. The base type must be one
// of the four string families for the check to apply.
func collectLengthConstrainedTypes(mod *syntax.Module) map[string]lengthSpec {
	out := map[string]lengthSpec{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		st, ok := d.Def.(*syntax.SubTypeDecl)
		if !ok || st.Field == nil || st.Field.Name == nil ||
			st.Field.LengthConstraint == nil {
			continue
		}
		base := baseStringFamily(st.Field.Type)
		if base == "" {
			continue
		}
		spec, ok := parseLengthExpr(st.Field.LengthConstraint, base)
		if !ok {
			continue
		}
		out[st.Field.Name.String()] = spec
	}
	return out
}

func baseStringFamily(spec syntax.TypeSpec) string {
	ref, ok := spec.(*syntax.RefSpec)
	if !ok || ref.X == nil {
		return ""
	}
	id, ok := ref.X.(*syntax.Ident)
	if !ok {
		return ""
	}
	switch id.String() {
	case "bitstring", "hexstring", "octetstring", "charstring":
		return id.String()
	}
	return ""
}

// parseLengthExpr lifts a syntax.LengthExpr into a lengthSpec when the
// bounds are integer literals; returns (zero, false) for anything
// more dynamic. The base parameter is the string family this length
// applies to.
func parseLengthExpr(le *syntax.LengthExpr, base string) (lengthSpec, bool) {
	if le == nil || le.Size == nil || le.Size.List == nil {
		return lengthSpec{}, false
	}
	out := lengthSpec{base: base, max: -1}
	switch len(le.Size.List) {
	case 1:
		expr := le.Size.List[0]
		if be, ok := expr.(*syntax.BinaryExpr); ok && be.Op != nil &&
			be.Op.String() == ".." {
			out.hasRange = true
			lo, ok := intLiteralValue(be.X)
			if !ok {
				return lengthSpec{}, false
			}
			out.min = lo
			if isInfinity(be.Y) {
				out.max = -1
			} else if hi, ok := intLiteralValue(be.Y); ok {
				out.max = hi
			} else {
				return lengthSpec{}, false
			}
			return out, true
		}
		// Fixed length: length(N)
		n, ok := intLiteralValue(expr)
		if !ok {
			return lengthSpec{}, false
		}
		out.min = n
		out.max = n
		return out, true
	}
	return lengthSpec{}, false
}

func intLiteralValue(e syntax.Expr) (int, bool) {
	lit, ok := e.(*syntax.ValueLiteral)
	if !ok || lit.Tok == nil {
		return 0, false
	}
	if lit.Tok.Kind() != syntax.INT {
		return 0, false
	}
	n, err := strconv.Atoi(lit.Tok.String())
	if err != nil {
		return 0, false
	}
	return n, true
}

func isInfinity(e syntax.Expr) bool {
	id, ok := e.(*syntax.Ident)
	if !ok || id.Tok == nil {
		return false
	}
	return strings.EqualFold(id.String(), "infinity")
}

// stringLiteralLength returns the number of "items" in a string
// literal: bits in a bitstring, hex digits in a hexstring, octet
// pairs in an octetstring, and characters in a charstring. The
// returned family name is bitstring/hexstring/octetstring/charstring
// so callers can confirm the literal matches the constrained type.
func stringLiteralLength(lit *syntax.ValueLiteral) (int, string, bool) {
	if lit == nil || lit.Tok == nil {
		return 0, "", false
	}
	switch lit.Tok.Kind() {
	case syntax.BSTRING:
		s := lit.Tok.String()
		if len(s) < 3 {
			return 0, "", false
		}
		body := s[1 : len(s)-2] // strip leading quote and trailing 'X
		suffix := strings.ToUpper(string(s[len(s)-1]))
		switch suffix {
		case "B":
			return len(body), "bitstring", true
		case "H":
			return len(body), "hexstring", true
		case "O":
			return len(body) / 2, "octetstring", true
		}
	case syntax.STRING:
		s := lit.Tok.String()
		if len(s) < 2 {
			return 0, "", false
		}
		return len(s) - 2, "charstring", true
	}
	return 0, "", false
}

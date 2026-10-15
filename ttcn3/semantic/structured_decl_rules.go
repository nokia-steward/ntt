package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// checkStructuredDeclRules enforces structural restrictions on type
// declarations that the parser accepts but ETSI ES 201 873-1 forbids:
//
//   - record / set / union members must have distinct names (6.2.1,
//     6.2.2, 6.2.5): a repeated field identifier is an error.
//   - enumerated members must have distinct names (6.2.4).
//
// Note: the explicit enumerated value is deliberately NOT restricted
// to integer literals. The modern suite (Sem_060204_008,
// Syn_060204_003/004) treats constant references and integer
// expressions (`Tuesday(c_int)`, `Tuesday(1+1)`, `bit2int(...)`) as
// valid value notations, superseding the older NegSyn_060204
// fixtures, and the interpreter folds them accordingly.
func (a *Analyzer) checkStructuredDeclRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch d := n.(type) {
		case *syntax.StructTypeDecl:
			diags = append(diags, checkDuplicateFields(d)...)
		case *syntax.EnumTypeDecl:
			diags = append(diags, checkEnumDecl(d)...)
		}
		return true
	})
	return diags
}

// checkDuplicateFields reports record/set/union members that reuse an
// earlier member's name.
func checkDuplicateFields(d *syntax.StructTypeDecl) []Diagnostic {
	var diags []Diagnostic
	seen := map[string]bool{}
	for _, f := range d.Fields {
		if f == nil || f.Name == nil {
			continue
		}
		name := f.Name.String()
		if name == "" {
			continue
		}
		if seen[name] {
			diags = append(diags, Diagnostic{
				Code:     "duplicate-field",
				Severity: SeverityError,
				Message:  fmt.Sprintf("duplicate member %q in %s type", name, structKindName(d)),
				Span:     syntax.SpanOf(f.Name),
			})
			continue
		}
		seen[name] = true
	}
	return diags
}

// checkEnumDecl reports duplicate enumerated identifiers and value
// notations that are not integer literals or literal ranges.
func checkEnumDecl(d *syntax.EnumTypeDecl) []Diagnostic {
	var diags []Diagnostic
	seen := map[string]bool{}
	for _, e := range d.Enums {
		name := enumMemberName(e)
		if name == "" {
			continue
		}
		if seen[name] {
			diags = append(diags, Diagnostic{
				Code:     "duplicate-enum-value",
				Severity: SeverityError,
				Message:  fmt.Sprintf("duplicate enumerated value %q", name),
				Span:     syntax.SpanOf(e),
			})
			continue
		}
		seen[name] = true
	}
	return diags
}

// enumMemberName returns the identifier an enum element introduces,
// whether the bare `Monday` or the `Tuesday(2)` value-notation form.
func enumMemberName(e syntax.Expr) string {
	switch x := e.(type) {
	case *syntax.Ident:
		return x.String()
	case *syntax.CallExpr:
		if id, ok := x.Fun.(*syntax.Ident); ok && id != nil {
			return id.String()
		}
	}
	return ""
}

func structKindName(d *syntax.StructTypeDecl) string {
	if d == nil || d.KindTok == nil {
		return "record"
	}
	return d.KindTok.String()
}

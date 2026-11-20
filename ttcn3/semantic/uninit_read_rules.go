// uninit_read_rules.go enforces a narrow flavour of ETSI ES 201
// 873-1 clauses 11.1 / 11.2:
//
//	the initializer expression of a variable / template
//	declaration shall evaluate to a (at-least partially)
//	initialised value. Reading a same-block variable that
//	has been declared without an initializer and that has
//	not been assigned to since the declaration shall cause
//	a semantic error.
//
// We only track straight-line declarations inside a single
// BlockStmt: any control-flow split, function call (which
// might assign to the variable via an out/inout parameter),
// nested block or alt branch resets the analysis. This keeps
// the rule strictly additive - false positives would dwarf
// the small set of test cases we want to catch otherwise.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkUninitReadRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		diags = append(diags, checkUninitInBlock(fn.Body)...)
	}
	return diags
}

func checkUninitInBlock(body *syntax.BlockStmt) []Diagnostic {
	if body == nil {
		return nil
	}
	var diags []Diagnostic
	// Per-block scan: track uninitialised same-block vars.
	uninit := map[string]*syntax.Declarator{}
	directReadUninit := map[string]bool{}
	stmtCount := len(body.Stmts)
	for i := 0; i < stmtCount; i++ {
		stmt := body.Stmts[i]
		switch s := stmt.(type) {
		case *syntax.DeclStmt:
			if s == nil {
				continue
			}
			vd, ok := s.Decl.(*syntax.ValueDecl)
			if !ok || vd == nil || vd.KindTok == nil ||
				vd.KindTok.Kind() != syntax.VAR {
				continue
			}
			for _, dc := range vd.Decls {
				if dc == nil || dc.Name == nil {
					continue
				}
				if dc.Value != nil {
					name := readsUninitVar(dc.Value, uninit)
					if name == "" {
						name = directUninitIdent(dc.Value, directReadUninit)
					}
					if name != "" {
						diags = append(diags, Diagnostic{
							Code:     "uninit-var-read",
							Severity: SeverityError,
							Message: fmt.Sprintf(
								"variable %q is read before it is initialised (ETSI 11.1 / 11.2)",
								name),
							Node: dc.Value,
							Span: syntax.SpanOf(dc.Value),
						})
					}
					// new var has an initializer -> initialised
					delete(uninit, dc.Name.String())
					delete(directReadUninit, dc.Name.String())
					continue
				}
				uninit[dc.Name.String()] = dc
				if directBareUninitReadsAreInvalid(vd) {
					directReadUninit[dc.Name.String()] = true
				}
			}
		case *syntax.ExprStmt:
			if s == nil || s.Expr == nil {
				return diags
			}
			be, ok := s.Expr.(*syntax.BinaryExpr)
			if !ok || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
				// any non-assignment statement may reach into
				// uninitialised vars in ways we don't model;
				// stop tracking conservatively.
				return diags
			}
			name := readsUninitVar(be.Y, uninit)
			if name == "" {
				name = directUninitIdent(be.Y, directReadUninit)
			}
			if name != "" {
				diags = append(diags, Diagnostic{
					Code:     "uninit-var-read",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"variable %q is read before it is initialised (ETSI 11.1 / 11.2)",
						name),
					Node: be.Y,
					Span: syntax.SpanOf(be.Y),
				})
			}
			if id, ok := be.X.(*syntax.Ident); ok && id != nil {
				delete(uninit, id.String())
				delete(directReadUninit, id.String())
			}
		default:
			// any other statement: conservatively reset tracking.
			return diags
		}
	}
	return diags
}

// readsUninitVar walks expr and returns the name of the first
// bare identifier that's in the uninit set, but ONLY when the
// identifier is used in a "value-essential" context: as a direct
// operand of an arithmetic / comparison / concat binary
// expression. Identifiers inside CallExpr arguments are skipped
// because the called function might take the operand by `out`
// or `inout` and initialise it, and we don't have signatures.
// Identifiers used as the bare RHS of an assignment / decl
// initializer are skipped to mirror the same uncertainty
// (the read may be a transient alias under a later operation).
func readsUninitVar(expr syntax.Expr, uninit map[string]*syntax.Declarator) string {
	if expr == nil || len(uninit) == 0 {
		return ""
	}
	be, ok := expr.(*syntax.BinaryExpr)
	if !ok || be == nil || be.Op == nil {
		return ""
	}
	if !isValueEssentialOp(be.Op.Kind()) {
		return ""
	}
	for _, side := range []syntax.Expr{be.X, be.Y} {
		if name := bareIdentInUninit(side, uninit); name != "" {
			return name
		}
	}
	return ""
}

func directUninitIdent(expr syntax.Expr, uninit map[string]bool) string {
	if len(uninit) == 0 {
		return ""
	}
	if id, ok := expr.(*syntax.Ident); ok && id != nil {
		if uninit[id.String()] {
			return id.String()
		}
	}
	return ""
}

func directBareUninitReadsAreInvalid(vd *syntax.ValueDecl) bool {
	if vd == nil || vd.TemplateRestriction != nil {
		return false
	}
	name := identName(vd.Type)
	switch name {
	case "integer", "float", "boolean", "bitstring", "hexstring",
		"octetstring", "charstring", "universal charstring",
		"verdicttype":
		return true
	}
	return false
}

// bareIdentInUninit returns the name when expr is a direct
// identifier in the uninit set; recursion descends only through
// further binary / unary expressions so we stay clear of calls.
func bareIdentInUninit(expr syntax.Expr, uninit map[string]*syntax.Declarator) string {
	switch v := expr.(type) {
	case *syntax.Ident:
		if v == nil {
			return ""
		}
		if _, bad := uninit[v.String()]; bad {
			return v.String()
		}
	case *syntax.BinaryExpr:
		if v == nil || v.Op == nil || !isValueEssentialOp(v.Op.Kind()) {
			return ""
		}
		if n := bareIdentInUninit(v.X, uninit); n != "" {
			return n
		}
		return bareIdentInUninit(v.Y, uninit)
	case *syntax.UnaryExpr:
		if v == nil {
			return ""
		}
		return bareIdentInUninit(v.X, uninit)
	case *syntax.ParenExpr:
		if v == nil {
			return ""
		}
		for _, el := range v.List {
			if n := bareIdentInUninit(el, uninit); n != "" {
				return n
			}
		}
	}
	return ""
}

func isValueEssentialOp(k syntax.Kind) bool {
	switch k {
	case syntax.ADD, syntax.SUB, syntax.MUL, syntax.DIV,
		syntax.MOD, syntax.REM,
		syntax.CONCAT,
		syntax.LT, syntax.LE, syntax.GT, syntax.GE,
		syntax.SHL, syntax.SHR, syntax.ROL, syntax.ROR:
		return true
	}
	return false
}

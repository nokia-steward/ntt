// reserved_names.go enforces ETSI ES 201 873-1 Annex A.1.5: "The
// predefined function identifiers defined in table 15 and described in
// annex C shall also be treated as reserved words." A module may not
// declare anything - a definition, a field, a parameter, an enumerated
// value, a label - by one of those names.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// predefinedFunctionNames are the identifiers of table 15 (clause 16.1.2).
var predefinedFunctionNames = map[string]bool{
	"int2char": true, "int2unichar": true, "int2bit": true, "int2enum": true,
	"int2hex": true, "int2oct": true, "int2str": true, "int2float": true,
	"float2int": true, "char2int": true, "char2oct": true, "unichar2oct": true,
	"unichar2int": true, "bit2int": true, "bit2hex": true, "bit2oct": true,
	"bit2str": true, "hex2int": true, "hex2bit": true, "hex2oct": true,
	"hex2str": true, "oct2int": true, "oct2bit": true, "oct2hex": true,
	"oct2str": true, "oct2char": true, "oct2unichar": true, "str2int": true,
	"str2hex": true, "str2oct": true, "str2float": true, "enum2int": true,
	"any2unistr": true, "lengthof": true, "sizeof": true, "istemplatekind": true,
	"regexp": true, "substr": true, "replace": true, "encvalue": true,
	"decvalue": true, "encvalue_unichar": true, "decvalue_unichar": true,
	"encvalue_o": true, "decvalue_o": true, "get_stringencoding": true,
	"remove_bom": true, "rnd": true, "testcasename": true, "hostid": true,
}

func (a *Analyzer) checkPredefinedNamesReserved(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	check := func(id *syntax.Ident) {
		if id == nil || !predefinedFunctionNames[id.String()] {
			return
		}
		diags = append(diags, Diagnostic{
			Code:     "predefined-function-name-reserved",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"%q is the name of a predefined function, a reserved word (ETSI ES 201 873-1 A.1.5)",
				id.String()),
			Node: id,
			Span: syntax.SpanOf(id),
		})
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch d := n.(type) {
		case *syntax.Module:
			check(d.Name)
		case *syntax.GroupDecl:
			check(d.Name)
		case *syntax.Field:
			check(d.Name)
		case *syntax.Declarator:
			check(d.Name)
		case *syntax.BranchStmt:
			if d.Tok != nil && d.Tok.Kind() == syntax.LABEL {
				check(d.Label)
			}
		case *syntax.CatchClause:
			check(d.Var)
		case *syntax.FormalPar:
			check(d.Name)
		case *syntax.TemplateDecl:
			check(d.Name)
		case *syntax.FuncDecl:
			check(d.Name)
		case *syntax.SignatureDecl:
			check(d.Name)
		case *syntax.StructTypeDecl:
			check(d.Name)
		case *syntax.ClassTypeDecl:
			check(d.Name)
		case *syntax.MapTypeDecl:
			check(d.Name)
		case *syntax.BehaviourTypeDecl:
			check(d.Name)
		case *syntax.PortTypeDecl:
			check(d.Name)
		case *syntax.ComponentTypeDecl:
			check(d.Name)
		case *syntax.EnumTypeDecl:
			check(d.Name)
			for _, e := range d.Enums {
				switch e := e.(type) {
				case *syntax.Ident:
					check(e)
				case *syntax.CallExpr:
					if id, ok := e.Fun.(*syntax.Ident); ok {
						check(id)
					}
				}
			}
		}
		return true
	})
	return diags
}

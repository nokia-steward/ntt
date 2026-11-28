// component_call_rules.go enforces ETSI ES 201 873-1 clause
// 21.3.10 "call" test-component operation:
//
//	<componentRef>.call(<funcCall>)
//
// where componentRef must resolve to a component-typed value and
// the callee shall have no port-, timer-, or default-typed
// formal parameter or return type.
//
// The check disambiguates this form from the procedure-port
// `<portRef>.call(<Sig>:<template>)` operation handled in
// port_ops.go: when the single argument is itself a function-
// call expression (no `Type:` qualifier), we treat it as the
// component-call shape.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkComponentCallRules(mod *syntax.Module) []Diagnostic {
	componentTypes := collectComponentTypeNames(mod)
	if len(componentTypes) == 0 {
		return nil
	}
	funcSigs := collectFunctionSignatures(mod)
	forbiddenParamFamily := map[string]string{}
	for tname := range collectPortTypes(mod) {
		forbiddenParamFamily[tname] = "port"
	}
	// Built-in `timer` and `default` types use those literal
	// names; user-named subtypes of them are out of scope until
	// we have a full type resolver.
	structFieldTypes := collectStructFieldTypes(mod)
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		varTypes := collectFuncBodyVarTypes(fn.Body)
		diags = append(diags, checkComponentCallInBody(fn.Body, componentTypes, forbiddenParamFamily, funcSigs, varTypes, structFieldTypes)...)
	}
	return diags
}

// nestedForbiddenFamily recursively walks a type's structure
// looking for a port / timer / default field. Returns the family
// label of the first hit; cycles are blocked via `visited`. The
// search descends through record / set / union named-field types
// only - opaque references (functions, signatures, list elements)
// don't carry a stable path right now.
func nestedForbiddenFamily(typeName string, forbiddenParamFamily map[string]string, structFieldTypes map[string]fieldsOfStruct, visited map[string]bool) (string, bool) {
	if typeName == "" {
		return "", false
	}
	if visited[typeName] {
		return "", false
	}
	visited[typeName] = true
	if fam, bad := forbiddenFamily(typeName, forbiddenParamFamily); bad {
		return fam, true
	}
	for _, fs := range structFieldTypes[typeName] {
		if fam, bad := nestedForbiddenFamily(fs.typeName, forbiddenParamFamily, structFieldTypes, visited); bad {
			return fam, true
		}
	}
	return "", false
}

func checkComponentCallInBody(
	body *syntax.BlockStmt,
	componentTypes map[string]bool,
	forbiddenParamFamily map[string]string,
	funcSigs map[string]*syntax.FuncDecl,
	varTypes map[string]string,
	structFieldTypes map[string]fieldsOfStruct,
) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(body, func(n syntax.Node) bool {
		ce, redirect, ok := componentCallExpr(n)
		if !ok || ce == nil {
			return true
		}
		sel, ok := ce.Fun.(*syntax.SelectorExpr)
		if !ok {
			return true
		}
		op, ok := sel.Sel.(*syntax.Ident)
		if !ok || op.String() != "call" || ce.Args == nil {
			return true
		}
		if len(ce.Args.List) != 1 {
			return true
		}
		inner, ok := ce.Args.List[0].(*syntax.CallExpr)
		if !ok {
			return true
		}
		recvIdent, ok := sel.X.(*syntax.Ident)
		if !ok {
			return true
		}
		recvType, hasType := varTypes[recvIdent.String()]
		if !hasType {
			return true
		}
		// The disambiguation hinges on the inner CallExpr.
		// `<port>.call(Sig:tmpl)` parses the actual as a
		// BinaryExpr (`Sig:tmpl`), NOT a CallExpr; so seeing
		// a CallExpr inside is a strong signal we have the
		// component-call shape.
		if !componentTypes[recvType] {
			diags = append(diags, Diagnostic{
				Code:     "call-on-non-component",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"call on %s: receiver is of type %q, but the call test-component operation requires a component-typed value (ETSI 21.3.10)",
					recvIdent.String(), recvType),
				Node: ce,
				Span: syntax.SpanOf(ce),
			})
			return true
		}
		// Receiver is a component; check the callee's
		// parameters and return type.
		funcName := ""
		if fid, ok := inner.Fun.(*syntax.Ident); ok {
			funcName = fid.String()
		}
		if funcName == "" {
			return true
		}
		fd, ok := funcSigs[funcName]
		if !ok || fd == nil {
			return true
		}
		// Forbidden parameter types (direct or nested through
		// record / set fields).
		if fd.Params != nil {
			for _, p := range fd.Params.List {
				if p == nil || p.Type == nil {
					continue
				}
				name := identName(p.Type)
				family, bad := nestedForbiddenFamily(name, forbiddenParamFamily, structFieldTypes, map[string]bool{})
				if !bad {
					continue
				}
				pname := ""
				if p.Name != nil {
					pname = p.Name.String()
				}
				diags = append(diags, Diagnostic{
					Code:     "call-forbidden-param-type",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"call on component: callee %q has formal parameter %q whose type contains a %s element (port/timer/default not allowed, ETSI 21.3.10)",
						funcName, pname, family),
					Node: ce,
					Span: syntax.SpanOf(ce),
				})
				break
			}
		}
		// Forbidden return type (direct or nested).
		if fd.Return != nil && fd.Return.Type != nil {
			name := identName(fd.Return.Type)
			if family, bad := nestedForbiddenFamily(name, forbiddenParamFamily, structFieldTypes, map[string]bool{}); bad {
				diags = append(diags, Diagnostic{
					Code:     "call-forbidden-return-type",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"call on component: callee %q returns a type containing a %s element (port/timer/default not allowed, ETSI 21.3.10)",
						funcName, family),
					Node: ce,
					Span: syntax.SpanOf(ce),
				})
			}
		}
		// `-> value v` redirect: the variable's type must
		// match the callee's return type. A function with
		// no return cannot supply a value redirect at all.
		if redirect != nil && redirect.ValueTok != nil && len(redirect.Value) > 0 {
			target := ""
			if id, ok := redirect.Value[0].(*syntax.Ident); ok && id != nil {
				target = id.String()
			}
			if target == "" {
				return true
			}
			targetType, known := varTypes[target]
			if !known {
				return true
			}
			if fd.Return == nil || fd.Return.Type == nil {
				diags = append(diags, Diagnostic{
					Code:     "call-value-redirect-no-return",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"call on component: cannot bind `-> value %s` because callee %q has no return type (ETSI 21.3.10)",
						target, funcName),
					Node: redirect,
					Span: syntax.SpanOf(redirect),
				})
				return true
			}
			retType := identName(fd.Return.Type)
			if retType != "" && targetType != "" &&
				isPrimitiveType(retType) && isPrimitiveType(targetType) &&
				retType != targetType {
				diags = append(diags, Diagnostic{
					Code:     "call-value-redirect-type-mismatch",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"call on component: `-> value %s` target type %q is not compatible with callee %q return type %q (ETSI 21.3.10)",
						target, targetType, funcName, retType),
					Node: redirect,
					Span: syntax.SpanOf(redirect),
				})
			}
		}
		return true
	})
	return diags
}

// componentCallExpr unwraps a RedirectExpr around a `<comp>.call`
// CallExpr and returns the inner CallExpr alongside the redirect
// (or nil) so the value-redirect rule can reach the redirect's
// `-> value v` payload. Non-redirect callsites fall through with
// redirect == nil.
func componentCallExpr(n syntax.Node) (*syntax.CallExpr, *syntax.RedirectExpr, bool) {
	switch v := n.(type) {
	case *syntax.CallExpr:
		return v, nil, true
	case *syntax.RedirectExpr:
		if v == nil {
			return nil, nil, false
		}
		ce, ok := v.X.(*syntax.CallExpr)
		if !ok {
			return nil, nil, false
		}
		return ce, v, true
	}
	return nil, nil, false
}

// forbiddenFamily classifies a type name as port/timer/default
// when ETSI 21.3.10 forbids it as a `.call(f())` formal-param
// or return type. forbiddenPortTypes carries user-defined
// port-type names; the builtin timer / default keywords are
// pinned by literal name.
func forbiddenFamily(name string, portTypes map[string]string) (string, bool) {
	switch name {
	case "timer":
		return "timer", true
	case "default":
		return "default", true
	}
	if fam, ok := portTypes[name]; ok {
		return fam, true
	}
	return "", false
}

// collectComponentTypeNames returns the set of all `type
// component T` declarations in the module. Used to classify a
// `<X>.call(...)` receiver as component- vs non-component.
func collectComponentTypeNames(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		ct, ok := d.Def.(*syntax.ComponentTypeDecl)
		if !ok || ct.Name == nil {
			continue
		}
		out[ct.Name.String()] = true
	}
	return out
}

// collectFunctionSignatures returns the per-name FuncDecl table.
// Function-name resolution is module-local for now (no imports).
func collectFunctionSignatures(mod *syntax.Module) map[string]*syntax.FuncDecl {
	out := map[string]*syntax.FuncDecl{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fd, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fd.Name == nil {
			continue
		}
		out[fd.Name.String()] = fd
	}
	return out
}

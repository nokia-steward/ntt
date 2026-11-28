// component_op_receiver.go enforces ETSI ES 201 873-1 clauses 21.3.2
// to 21.3.10: the receiver of a test-component operation (.start /
// .stop / .kill / .done / .killed / .running / .alive) must be of
// component type. The existing component_ops.go check focuses on
// receivers that ARE component-typed and validates the operation's
// arguments; this rule complements it by rejecting receivers that
// are not component-typed at all.
//
// Two reject shapes are caught:
//
//   - Variable receivers whose declared type is not a known
//     component type, e.g. `var Rec v; v.start(f());`. We treat
//     "known component type" as the set of `type component X`
//     declarations in the module - any declared non-component
//     type lands in the reject set.
//
//   - Bare type-name receivers, e.g. `MyRec.done`. The bare-name
//     form is only meaningful for `mtc.x` / `self.x` / `system.x`,
//     for `any component` / `all component`, and for the all/any
//     timer / port qualifiers; everything else that resolves to a
//     non-component type is rejected.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// componentOpsBuiltin lists the receiver-bound operations defined
// by ETSI 21.3.2-21.3.10. We treat them as the trigger set for
// the receiver-kind check.
var componentOpsBuiltin = map[string]bool{
	"start":   true,
	"stop":    true,
	"kill":    true,
	"done":    true,
	"killed":  true,
	"running": true,
	"alive":   true,
}

func (a *Analyzer) checkComponentOpReceivers(mod *syntax.Module) []Diagnostic {
	declKinds := collectDeclaredTypeKinds(mod)
	if len(declKinds) == 0 {
		return nil
	}
	funcReturns := collectFuncReturnTypes(mod)
	arraySubtypes := collectArraySubtypeNames(mod)

	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		varTypes := collectVarDeclaredTypeNames(fn.Body)
		varArrays := collectVarArrayNames(fn.Body)
		paramTypes := collectFormalParamTypeNames(fn.Params)
		diags = append(diags, checkComponentOpReceiverInBody(
			fn.Body, varTypes, varArrays, paramTypes,
			declKinds, funcReturns, arraySubtypes,
		)...)
	}
	return diags
}

// collectFuncReturnTypes maps module-level function/altstep/testcase
// names to the bare ident name of their return type, or "" when the
// function has no return type. Used by the receiver check to resolve
// `f().<op>` calls.
func collectFuncReturnTypes(mod *syntax.Module) map[string]string {
	out := map[string]string{}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}
		if fn.Return == nil || fn.Return.Type == nil {
			out[fn.Name.String()] = ""
			continue
		}
		if id, ok := fn.Return.Type.(*syntax.Ident); ok {
			out[fn.Name.String()] = id.String()
		}
	}
	return out
}

// collectArraySubtypeNames returns the set of declared subtype names
// whose right-hand side is an array (e.g. `type GeneralComp CompArray[2]`).
// Component-op receivers that resolve to one of these are flagged
// because the spec only allows the operation on a single component.
func collectArraySubtypeNames(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		st, ok := d.Def.(*syntax.SubTypeDecl)
		if !ok || st.Field == nil || st.Field.Name == nil {
			continue
		}
		if len(st.Field.ArrayDef) > 0 {
			out[st.Field.Name.String()] = true
		}
	}
	return out
}

// collectVarArrayNames returns the set of variable names whose
// declarator carries an array-dimension suffix, i.e. things declared
// as `var T x[N]` regardless of T. We use it to flag e.g.
// `var GeneralComp v_ptc[2]; v_ptc.stop` where the array shape
// promotes a component-typed variable to a record-of value the
// component op can no longer apply to.
func collectVarArrayNames(body *syntax.BlockStmt) map[string]bool {
	out := map[string]bool{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		ds, ok := n.(*syntax.DeclStmt)
		if !ok {
			return true
		}
		vd, ok := ds.Decl.(*syntax.ValueDecl)
		if !ok || vd.KindTok == nil || vd.KindTok.Kind() != syntax.VAR {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil || len(dec.ArrayDef) == 0 {
				continue
			}
			out[dec.Name.String()] = true
		}
		return true
	})
	return out
}

// typeKind is the high-level category we sort module-level type
// declarations into. We only care about "component" vs "other";
// everything we'd otherwise distinguish (record, set, record-of,
// enum, port, ...) ends up in "other" because the only thing
// .start/.stop/.done/etc. accept is a component.
type typeKind int

const (
	tkOther typeKind = iota
	tkComponent
	tkPort
	tkTimer
)

// collectDeclaredTypeKinds walks the module and tags every named
// type with its kind. Anything we don't recognise stays out of
// the map so the receiver check silently passes on imports we
// haven't resolved.
func collectDeclaredTypeKinds(mod *syntax.Module) map[string]typeKind {
	out := map[string]typeKind{}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		switch x := d.Def.(type) {
		case *syntax.ComponentTypeDecl:
			if x.Name != nil {
				out[x.Name.String()] = tkComponent
			}
		case *syntax.StructTypeDecl:
			if x.Name != nil {
				out[x.Name.String()] = tkOther
			}
		case *syntax.EnumTypeDecl:
			if x.Name != nil {
				out[x.Name.String()] = tkOther
			}
		case *syntax.PortTypeDecl:
			if x.Name != nil {
				out[x.Name.String()] = tkPort
			}
		case *syntax.SubTypeDecl:
			if x.Field != nil && x.Field.Name != nil {
				out[x.Field.Name.String()] = tkOther
			}
		case *syntax.SignatureDecl:
			if x.Name != nil {
				out[x.Name.String()] = tkOther
			}
		case *syntax.ClassTypeDecl:
			// Classes have their own .create() semantics
			// but are not started / stopped / done via the
			// component ops. Mark them as "other".
			if x.Name != nil {
				out[x.Name.String()] = tkOther
			}
		}
	}
	return out
}

// collectVarDeclaredTypeNames returns the per-body map of
// variable name -> declared (bare) type ident name. We capture
// every `var <T> v` declaration, even when T is not a component
// type, so the receiver check can detect the non-component case.
func collectVarDeclaredTypeNames(body *syntax.BlockStmt) map[string]string {
	out := map[string]string{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		ds, ok := n.(*syntax.DeclStmt)
		if !ok {
			return true
		}
		vd, ok := ds.Decl.(*syntax.ValueDecl)
		if !ok || vd.KindTok == nil || vd.KindTok.Kind() != syntax.VAR {
			return true
		}
		id, ok := vd.Type.(*syntax.Ident)
		if !ok || id == nil || id.Tok == nil {
			return true
		}
		typeName := id.String()
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

// collectFormalParamTypeNames mirrors the var collector but for
// the function's formal-parameter list. Without it, helpers that
// take a `Rec p_rec` parameter and immediately call
// `p_rec.start(...)` would slip past the receiver check.
func collectFormalParamTypeNames(pars *syntax.FormalPars) map[string]string {
	out := map[string]string{}
	if pars == nil {
		return out
	}
	for _, fp := range pars.List {
		if fp == nil || fp.Name == nil || fp.Type == nil {
			continue
		}
		id, ok := fp.Type.(*syntax.Ident)
		if !ok || id == nil || id.Tok == nil {
			continue
		}
		out[fp.Name.String()] = id.String()
	}
	return out
}

func checkComponentOpReceiverInBody(
	body *syntax.BlockStmt,
	varTypes map[string]string,
	varArrays map[string]bool,
	paramTypes map[string]string,
	declKinds map[string]typeKind,
	funcReturns map[string]string,
	arraySubtypes map[string]bool,
) []Diagnostic {
	// Pre-collect every selector that's the Fun of a CallExpr,
	// so the main pass skips it and avoids producing two
	// diagnostics for the same `recv.op(...)` call. Also collect
	// selectors that sit inside an `any from <arr>.<op>` /
	// `all from <arr>.<op>` form so the array-receiver check
	// skips them - the FromExpr is the legal iteration shape.
	wrappedSel := map[syntax.Node]bool{}
	insideFromExpr := map[syntax.Node]bool{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		switch v := n.(type) {
		case *syntax.CallExpr:
			if s, ok := v.Fun.(*syntax.SelectorExpr); ok {
				wrappedSel[s] = true
			}
		case *syntax.FromExpr:
			if s, ok := v.X.(*syntax.SelectorExpr); ok {
				insideFromExpr[s] = true
			}
			if c, ok := v.X.(*syntax.CallExpr); ok {
				if s, ok := c.Fun.(*syntax.SelectorExpr); ok {
					insideFromExpr[s] = true
				}
			}
		}
		return true
	})

	var diags []Diagnostic
	syntax.Inspect(body, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		var sel *syntax.SelectorExpr
		switch v := n.(type) {
		case *syntax.SelectorExpr:
			if wrappedSel[v] {
				return true
			}
			sel = v
		case *syntax.CallExpr:
			s, ok := v.Fun.(*syntax.SelectorExpr)
			if !ok {
				return true
			}
			sel = s
		default:
			return true
		}
		opIdent, ok := sel.Sel.(*syntax.Ident)
		if !ok {
			return true
		}
		op := opIdent.String()
		if !componentOpsBuiltin[op] {
			return true
		}
		// Receiver shapes:
		//   - Ident: most common, handled below.
		//   - CallExpr: `f().<op>` - look up f's return type.
		//   - IndexExpr / SelectorExpr: skip (we can't resolve
		//     without a type system).
		insideFrom := insideFromExpr[sel]
		switch recv := sel.X.(type) {
		case *syntax.Ident:
			name := recv.String()
			switch name {
			case "mtc", "self", "system", "all", "any":
				return true
			}
			if typeName, ok := varTypes[name]; ok {
				if varArrays[name] {
					if !insideFrom {
						diags = append(diags, nonCompRecvDiag(
							op, name, typeName+"[]", "array variable", sel))
					}
					return true
				}
				if checkNonComponentRecv(declKinds, typeName) {
					diags = append(diags, nonCompRecvDiag(op, name, typeName, "variable", sel))
				}
				if arraySubtypes[typeName] && !insideFrom {
					diags = append(diags, nonCompRecvDiag(
						op, name, typeName, "array subtype variable", sel))
				}
				return true
			}
			if typeName, ok := paramTypes[name]; ok {
				if checkNonComponentRecv(declKinds, typeName) {
					diags = append(diags, nonCompRecvDiag(op, name, typeName, "parameter", sel))
				}
				if arraySubtypes[typeName] && !insideFrom {
					diags = append(diags, nonCompRecvDiag(
						op, name, typeName, "array subtype parameter", sel))
				}
				return true
			}
			if kind, ok := declKinds[name]; ok && kind != tkComponent {
				diags = append(diags, nonCompRecvDiag(op, name, name, "type name", sel))
			}
		case *syntax.CallExpr:
			fnIdent, ok := recv.Fun.(*syntax.Ident)
			if !ok {
				return true
			}
			fname := fnIdent.String()
			ret, known := funcReturns[fname]
			if !known || ret == "" {
				return true
			}
			if checkNonComponentRecv(declKinds, ret) {
				diags = append(diags, nonCompRecvDiag(
					op, fname+"()", ret, "function return", sel))
				return true
			}
			if arraySubtypes[ret] && !insideFrom {
				diags = append(diags, nonCompRecvDiag(
					op, fname+"()", ret, "function returning array subtype", sel))
			}
		}
		return true
	})
	return diags
}

func checkNonComponentRecv(declKinds map[string]typeKind, typeName string) bool {
	kind, known := declKinds[typeName]
	if !known {
		return false
	}
	switch kind {
	case tkComponent, tkPort, tkTimer:
		// Components are obviously fine; ports (ETSI 22.5) and
		// timers (ETSI 26.2) reuse the start/stop/halt/kill/
		// running/alive names with their own semantics, so we
		// must not flag them here.
		return false
	}
	return true
}

func nonCompRecvDiag(op, recvName, typeName, role string, node syntax.Node) Diagnostic {
	return Diagnostic{
		Code:     "component-op-non-component-receiver",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"%s on %s %q: receiver type %q is not a component (ETSI 21.3)",
			op, role, recvName, typeName),
		Node: node,
		Span: syntax.SpanOf(node),
	}
}

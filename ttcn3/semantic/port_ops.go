// port_ops.go implements static checks on the typed forms of port
// communication operations defined in ETSI ES 201 873-1 clauses
// 22.2 (receive / trigger / check) and 22.3 (call). The check is
// intentionally narrow: it only fires when the source uses the
// `Type:expr` qualifier on a send/receive/call argument, where the
// type to verify is syntactically present and can be matched
// against the port's in / out / inout list without a full type
// inference pass.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// portDirs is the per-port-type set of incoming, outgoing and
// bidirectional types / signatures declared on the port. The
// allInDir / allOutDir / allInoutDir flags model `in all` / `out all`
// / `inout all`, which allow every type and disable the
// type-membership check for that direction.
type portDirs struct {
	in          map[string]bool // receive-allowed
	out         map[string]bool // send / call-allowed
	inout       map[string]bool // both
	allInDir    bool
	allOutDir   bool
	allInoutDir bool
	// kind is the surface category from the port declaration:
	// MESSAGE, PROCEDURE, MIXED, or 0 when unknown. The check
	// uses this to flag message-only operations on procedure
	// ports and vice-versa (ETSI 22.2 / 22.3).
	kind syntax.Kind
}

func (p *portDirs) canReceive(t string) bool {
	if p.allInDir || p.allInoutDir {
		return true
	}
	return p.in[t] || p.inout[t]
}
func (p *portDirs) canSend(t string) bool {
	if p.allOutDir || p.allInoutDir {
		return true
	}
	return p.out[t] || p.inout[t]
}

// receivableCount returns the number of distinct types the port can
// receive (in + inout, with duplicates folded). It does NOT count the
// "all" flag - an `in all` port admits any type so a non-prefixed
// template is fine. Returns >=2 only when the user actually listed
// multiple message types or signatures, in which case the
// template-without-type-prefix matching is ambiguous (ETSI 22.2.2).
func (p *portDirs) receivableCount() int {
	if p.allInDir || p.allInoutDir {
		return 0
	}
	seen := map[string]bool{}
	for k := range p.in {
		seen[k] = true
	}
	for k := range p.inout {
		seen[k] = true
	}
	return len(seen)
}

// sendableCount mirrors receivableCount for the send direction.
func (p *portDirs) sendableCount() int {
	if p.allOutDir || p.allInoutDir {
		return 0
	}
	seen := map[string]bool{}
	for k := range p.out {
		seen[k] = true
	}
	for k := range p.inout {
		seen[k] = true
	}
	return len(seen)
}

func (a *Analyzer) checkPortOps(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic

	portTypes := collectPortTypes(mod)
	if len(portTypes) == 0 {
		return diags
	}
	compPorts := collectComponentPorts(mod)
	if len(compPorts) == 0 {
		return diags
	}
	noBlock := collectNoBlockSignatures(mod)
	tmpls := collectTemplateBodies(mod)

	// Walk every FuncDecl; per-function we know which component
	// it runs on, which gives us the local `port-name -> port-type`
	// table for that function's body.
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok {
			continue
		}
		comp := ""
		if fn.RunsOn != nil && fn.RunsOn.Comp != nil {
			comp = syntax.Name(fn.RunsOn.Comp)
		}
		ports := compPorts[comp]
		if ports == nil {
			continue
		}
		if fn.Body == nil {
			continue
		}
		diags = append(diags, checkPortOpsInBody(fn.Body, ports, portTypes, noBlock, tmpls)...)
	}
	return diags
}

// collectTemplateBodies returns a map from template-name to its
// initializer expression. The send-no-wildcard check follows
// template references through this map so a `port.send(mw_t)` flags
// the same shapes as the inline form `port.send({...})`.
func collectTemplateBodies(mod *syntax.Module) map[string]syntax.Expr {
	out := map[string]syntax.Expr{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		td, ok := n.(*syntax.TemplateDecl)
		if !ok || td == nil || td.Name == nil {
			return true
		}
		out[td.Name.String()] = td.Value
		return true
	})
	return out
}

// templateHasWildcard reports whether the (possibly nested) value of
// a template literal contains a bare `?` (AnyValue) or `*`
// (AnyOrNone) wildcard. Used by the send-op validator: ETSI 22.2.1
// requires the actual to be a specific value, not a matching
// template.
//
// We follow template references through `tmpls` so `port.send(mw_t)`
// catches the same shape as the inline form. Cycles are unlikely in
// well-formed code but we keep a visited set anyway.
func templateHasWildcard(expr syntax.Expr, tmpls map[string]syntax.Expr) bool {
	visited := map[string]bool{}
	var walk func(e syntax.Expr) bool
	walk = func(e syntax.Expr) bool {
		if e == nil {
			return false
		}
		switch v := e.(type) {
		case *syntax.ValueLiteral:
			if v.Tok != nil {
				switch v.Tok.Kind() {
				case syntax.ANY, syntax.MUL:
					return true
				}
			}
		case *syntax.CompositeLiteral:
			for _, c := range v.List {
				if walk(c) {
					return true
				}
			}
		case *syntax.BinaryExpr:
			if walk(v.X) || walk(v.Y) {
				return true
			}
		case *syntax.UnaryExpr:
			if walk(v.X) {
				return true
			}
		case *syntax.ParenExpr:
			for _, c := range v.List {
				if walk(c) {
					return true
				}
			}
		case *syntax.Ident:
			// Follow named-template references.
			name := v.String()
			if visited[name] {
				return false
			}
			visited[name] = true
			if body, ok := tmpls[name]; ok {
				return walk(body)
			}
		}
		return false
	}
	return walk(expr)
}

// collectNoBlockSignatures returns the set of signature names whose
// declaration carries the `noblock` modifier. Used by the call
// validator to enforce the no-timeout rule from ETSI 22.3.1 l).
func collectNoBlockSignatures(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		sd, ok := d.Def.(*syntax.SignatureDecl)
		if !ok || sd.Name == nil {
			continue
		}
		if sd.NoBlock != nil {
			out[sd.Name.String()] = true
		}
	}
	return out
}

// callSignatureName extracts the signature-type name from the first
// actual of a `p.call(<sig>:<args>)` invocation. Returns "" when the
// actual isn't in the typed form (e.g. a bare template reference).
func callSignatureName(expr syntax.Expr) string {
	if pe, ok := expr.(*syntax.ParenExpr); ok && len(pe.List) == 1 {
		return callSignatureName(pe.List[0])
	}
	be, ok := expr.(*syntax.BinaryExpr)
	if !ok || be.Op == nil || be.Op.Kind() != syntax.COLON {
		return ""
	}
	id, ok := be.X.(*syntax.Ident)
	if !ok {
		return ""
	}
	return id.String()
}

// checkPortOpsInBody walks one function-body and reports each
// typed-form port op that names a type missing from the port's
// declared direction list.
func checkPortOpsInBody(body *syntax.BlockStmt, ports map[string]string, portTypes map[string]*portDirs, sigNoBlock map[string]bool, tmpls map[string]syntax.Expr) []Diagnostic {
	var diags []Diagnostic

	syntax.Inspect(body, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		// Bare-selector form: `p.getcall` / `p.receive` /
		// `p.trigger` etc. used as a standalone statement or
		// the head of an alt branch. These do not appear as a
		// CallExpr in the AST, so the typed-form branch below
		// misses them. We catch only the kind violation here
		// (no args means no template / type to validate).
		if es, ok := n.(*syntax.ExprStmt); ok && es.Expr != nil {
			if sel, ok := es.Expr.(*syntax.SelectorExpr); ok {
				if d := checkBarePortOpKind(sel, ports, portTypes); d != nil {
					diags = append(diags, *d)
				}
			}
		}
		ce, ok := n.(*syntax.CallExpr)
		if !ok {
			return true
		}
		// We want `<port>.<op>(<typed-arg>)` only.
		sel, ok := ce.Fun.(*syntax.SelectorExpr)
		if !ok {
			return true
		}
		portIdent, ok := sel.X.(*syntax.Ident)
		if !ok {
			return true
		}
		op, ok := sel.Sel.(*syntax.Ident)
		if !ok {
			return true
		}
		portType, known := ports[portIdent.String()]
		if !known {
			return true
		}
		dirs := portTypes[portType]
		if dirs == nil {
			return true
		}
		// Operation-vs-port-kind compatibility (ETSI 22.2 vs
		// 22.3). Message ports only accept the
		// send/receive/trigger/check family; procedure ports
		// only accept call/getcall/reply/getreply/raise/catch.
		// Mixed ports (MIXED) accept both. We flag the
		// violation before touching args so empty calls
		// (`p.getcall { ... }` without a typed actual) are
		// caught too.
		if dirs.kind != 0 && dirs.kind != syntax.MIXED {
			if msg := portOpKindViolation(op.String(), dirs.kind); msg != "" {
				diags = append(diags, Diagnostic{
					Code:     "port-op-wrong-port-kind",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"%s on port %q: %s",
						op.String(), portIdent.String(), msg),
					Node: ce,
					Span: syntax.SpanOf(ce),
				})
				return true
			}
		}

		// ETSI 22.3.1 l): a non-blocking call has no
		// response/exception handling and shall not be issued
		// with a timeout duration. We catch the simple form
		// `p.call(Sig:..., 1.0)` where Sig is declared
		// `noblock` - more args than the template means a
		// timeout was supplied.
		if op.String() == "call" && ce.Args != nil && len(ce.Args.List) >= 2 {
			if sigName := callSignatureName(ce.Args.List[0]); sigName != "" {
				if sigNoBlock != nil && sigNoBlock[sigName] {
					extra := ce.Args.List[1]
					// nowait keyword is acceptable
					// only on blocking calls (ETSI
					// 22.3.1), and the conformance
					// suite uses `nowait` as the
					// timeout-shaped argument; the
					// second slot is therefore always
					// a violation for a noblock call.
					diags = append(diags, Diagnostic{
						Code:     "call-noblock-extra-arg",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"call on port %q: non-blocking signature %q does not take a timeout / nowait argument",
							portIdent.String(), sigName),
						Node: extra,
						Span: syntax.SpanOf(extra),
					})
				}
			}
		}
		if ce.Args == nil || len(ce.Args.List) == 0 {
			return true
		}
		// The first actual is the value / template; subsequent
		// positions are `to clause` / `from clause` / `value v`
		// / `sender v` redirects we don't validate here.
		first := ce.Args.List[0]
		typeName := typedFormType(first)

		// ETSI 22.2.1: `port.send(...)` requires a specific
		// value. A template with wildcards (`?` / `*`) at any
		// depth is rejected. We follow named template
		// references so `port.send(mw_t)` catches the same
		// shape as the inline form.
		if op.String() == "send" {
			payload := first
			if be, ok := first.(*syntax.BinaryExpr); ok && be.Op != nil &&
				be.Op.Kind() == syntax.COLON {
				payload = be.Y
			}
			if templateHasWildcard(payload, tmpls) {
				diags = append(diags, Diagnostic{
					Code:     "send-template-wildcard",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"send on port %q: argument contains a wildcard; send requires a specific value",
						portIdent.String()),
					Node: first,
					Span: syntax.SpanOf(first),
				})
			}
		}

		// Ambiguous-template check (ETSI 22.2.2): when the
		// inline template lacks a `Type:` prefix and the port
		// supports multiple types in the relevant direction,
		// the matcher cannot tell which type the user meant.
		// We flag this for the receive family (the dominant
		// case in the conformance suite) and the send family
		// alike; the latter is technically already covered by
		// type inference in most implementations but the spec
		// still requires an unambiguous resolution.
		if typeName == "" {
			switch op.String() {
			case "receive", "trigger", "check":
				if dirs.receivableCount() >= 2 && looksAmbiguousTemplate(first) {
					diags = append(diags, Diagnostic{
						Code:     "port-op-ambiguous-template",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"%s on port %q: template type is ambiguous (port admits %d types); use Type:template",
							op.String(), portIdent.String(), dirs.receivableCount()),
						Node: first,
						Span: syntax.SpanOf(first),
					})
				}
			case "send", "raise":
				if dirs.sendableCount() >= 2 && looksAmbiguousTemplate(first) {
					diags = append(diags, Diagnostic{
						Code:     "port-op-ambiguous-template",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"%s on port %q: template type is ambiguous (port admits %d types); use Type:template",
							op.String(), portIdent.String(), dirs.sendableCount()),
						Node: first,
						Span: syntax.SpanOf(first),
					})
				}
			}
			return true
		}

		var diag *Diagnostic
		switch op.String() {
		case "send", "call", "raise":
			if !dirs.canSend(typeName) {
				diag = &Diagnostic{
					Code:     "port-op-type-not-allowed",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"%s on port %q: type %q is not in the port's out/inout list",
						op.String(), portIdent.String(), typeName),
					Node: first,
					Span: syntax.SpanOf(first),
				}
			}
		case "receive", "trigger", "check", "getcall", "getreply", "catch":
			if !dirs.canReceive(typeName) {
				diag = &Diagnostic{
					Code:     "port-op-type-not-allowed",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"%s on port %q: type %q is not in the port's in/inout list",
						op.String(), portIdent.String(), typeName),
					Node: first,
					Span: syntax.SpanOf(first),
				}
			}
		}
		if diag != nil {
			diags = append(diags, *diag)
		}
		return true
	})
	return diags
}

// checkBarePortOpKind returns a port-op-wrong-port-kind diagnostic
// for an argless `<port>.<op>` selector form (e.g. `p.getcall` /
// `m.receive`) used as a statement or alt-branch head. Returns nil
// when the selector isn't a recognisable port op or the port's
// declared kind permits the op.
func checkBarePortOpKind(sel *syntax.SelectorExpr, ports map[string]string, portTypes map[string]*portDirs) *Diagnostic {
	if sel == nil {
		return nil
	}
	portIdent, ok := sel.X.(*syntax.Ident)
	if !ok {
		return nil
	}
	opIdent, ok := sel.Sel.(*syntax.Ident)
	if !ok {
		return nil
	}
	op := opIdent.String()
	if portOpFamily(op) == "" {
		return nil
	}
	portType, known := ports[portIdent.String()]
	if !known {
		return nil
	}
	dirs := portTypes[portType]
	if dirs == nil || dirs.kind == 0 || dirs.kind == syntax.MIXED {
		return nil
	}
	msg := portOpKindViolation(op, dirs.kind)
	if msg == "" {
		return nil
	}
	return &Diagnostic{
		Code:     "port-op-wrong-port-kind",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"%s on port %q: %s",
			op, portIdent.String(), msg),
		Node: sel,
		Span: syntax.SpanOf(sel),
	}
}

// portOpKindViolation returns an empty string when `op` is allowed on a
// port of category `kind`, otherwise a short reason mentioning which
// surface the operation belongs to. Operations that fit neither
// category (such as the synchronisation primitives `clear` / `start`
// / `stop` / `halt` / `checkstate`) are quietly accepted on either
// side - they're always legal regardless of port kind.
func portOpKindViolation(op string, kind syntax.Kind) string {
	const (
		message   = "message"
		procedure = "procedure"
	)
	belongs := portOpFamily(op)
	if belongs == "" {
		return ""
	}
	switch kind {
	case syntax.MESSAGE:
		if belongs == procedure {
			return "this operation requires a procedure-based port"
		}
	case syntax.PROCEDURE:
		if belongs == message {
			return "this operation requires a message-based port"
		}
	}
	return ""
}

// portOpFamily maps a port-op name to the surface family it belongs
// to ("message", "procedure", or ""). Unknown / kind-agnostic ops
// return "" so the kind check stays opt-in.
func portOpFamily(op string) string {
	switch op {
	case "send", "receive", "trigger":
		return "message"
	case "call", "getcall", "reply", "getreply", "raise", "catch":
		return "procedure"
	}
	// `check` is overloaded - `port.check(receive(...))` is
	// message, `port.check(getcall(...))` is procedure, and the
	// bare form is allowed on both. Keep it unclassified so the
	// kind check stays silent.
	return ""
}

// looksAmbiguousTemplate reports whether expr is an inline template
// that could match more than one record / record-of / set type and so
// requires a `Type:template` qualifier on a multi-type port. The
// matcher rules in ETSI 22.2.2 specifically call out three shapes:
//
//   - composite literals (`{a := 1, b := 2}`, `{1, 2}`) - applicable
//     to any record / set / record-of with the matching arity;
//   - bare wildcards (`?`, `*`) - applicable to any type;
//   - referenced templates by bare identifier - the identifier alone
//     does not pin a type when the template was declared as
//     `template T mt := ...` and T is in the port's direction list
//     more than once.
//
// We deliberately do NOT flag charstring / integer / boolean literals
// because those carry an implicit single-type interpretation and
// every implementation accepts them on multi-type ports as long as
// exactly one declared type matches.
func looksAmbiguousTemplate(expr syntax.Expr) bool {
	if expr == nil {
		return false
	}
	if pe, ok := expr.(*syntax.ParenExpr); ok && len(pe.List) == 1 {
		return looksAmbiguousTemplate(pe.List[0])
	}
	if cl, ok := expr.(*syntax.CompositeLiteral); ok && cl != nil {
		// `{...}` literals are inherently shapeless - they
		// match any record / set / record-of of the same
		// arity, so a multi-type port can't pick a winner
		// without a type prefix.
		return true
	}
	if vl, ok := expr.(*syntax.ValueLiteral); ok && vl != nil && vl.Tok != nil {
		switch vl.Tok.Kind() {
		case syntax.ANY, syntax.MUL:
			return true
		}
	}
	return false
}

// typedFormType returns the type name carried by a `Type:body`
// qualifier inside a port operation, or "" when the actual doesn't
// use that form. Three input shapes are recognised:
//
//	Type:body                (BinaryExpr with COLON op)
//	(Type:body)              (ParenExpr wrapping the above)
//	Type:?                   (same shape; Y is the `?` wildcard)
//
// Field-level templates like `{a := 1, b := 2}` are returned as
// "" because there's no syntactic type to compare against.
func typedFormType(expr syntax.Expr) string {
	if expr == nil {
		return ""
	}
	if pe, ok := expr.(*syntax.ParenExpr); ok && len(pe.List) == 1 {
		return typedFormType(pe.List[0])
	}
	be, ok := expr.(*syntax.BinaryExpr)
	if !ok || be.Op == nil || be.Op.Kind() != syntax.COLON {
		return ""
	}
	id, ok := be.X.(*syntax.Ident)
	if !ok || id.Tok == nil {
		return ""
	}
	return id.String()
}

// collectPortTypes walks the module and indexes each `type port`
// declaration with the names of types / signatures listed under
// each direction (`in`, `out`, `inout`).
func collectPortTypes(mod *syntax.Module) map[string]*portDirs {
	out := map[string]*portDirs{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		pt, ok := d.Def.(*syntax.PortTypeDecl)
		if !ok || pt.Name == nil {
			continue
		}
		dirs := &portDirs{
			in:    map[string]bool{},
			out:   map[string]bool{},
			inout: map[string]bool{},
		}
		if pt.KindTok != nil {
			dirs.kind = pt.KindTok.Kind()
		}
		for _, attr := range pt.Attrs {
			pa, ok := attr.(*syntax.PortAttribute)
			if !ok || pa.KindTok == nil {
				continue
			}
			var slot map[string]bool
			var allFlag *bool
			switch pa.KindTok.Kind() {
			case syntax.IN:
				slot = dirs.in
				allFlag = &dirs.allInDir
			case syntax.OUT:
				slot = dirs.out
				allFlag = &dirs.allOutDir
			case syntax.INOUT:
				slot = dirs.inout
				allFlag = &dirs.allInoutDir
			default:
				continue
			}
			for _, t := range pa.Types {
				name := syntax.Name(t)
				if name == "" {
					continue
				}
				// `in all` / `out all` / `inout all`
				// opt out of type-membership checks for
				// the matching direction.
				if name == "all" {
					*allFlag = true
					continue
				}
				slot[name] = true
			}
		}
		out[pt.Name.String()] = dirs
	}
	return out
}

// collectComponentPorts walks the module and returns, per
// component, the map of locally-declared port-instance names to the
// port-type names. Inherited / `extends`-aggregated ports are not
// included; revisit when we have a full component-type inheritance
// model.
func collectComponentPorts(mod *syntax.Module) map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		ct, ok := d.Def.(*syntax.ComponentTypeDecl)
		if !ok || ct.Name == nil || ct.Body == nil {
			continue
		}
		ports := map[string]string{}
		for _, stmt := range ct.Body.Stmts {
			ds, ok := stmt.(*syntax.DeclStmt)
			if !ok {
				continue
			}
			vd, ok := ds.Decl.(*syntax.ValueDecl)
			if !ok || vd.KindTok == nil || vd.KindTok.Kind() != syntax.PORT {
				continue
			}
			id, ok := vd.Type.(*syntax.Ident)
			if !ok {
				continue
			}
			portType := id.String()
			for _, dec := range vd.Decls {
				if dec == nil || dec.Name == nil {
					continue
				}
				ports[dec.Name.String()] = portType
			}
		}
		out[ct.Name.String()] = ports
	}
	return out
}

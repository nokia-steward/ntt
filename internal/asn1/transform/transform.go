// Package transform lowers an ASN.1 module's AST into TTCN-3 source
// that can be re-parsed by ttcn3.Parse. The resulting *ttcn3.Tree
// flows through the existing semantic, formatter, and LSP layers as
// if the user had hand-written a TTCN-3 module.
//
// We emit text rather than constructing ttcn3/syntax nodes directly
// because the ttcn3 syntax tree is not meant to be built piecemeal
// from outside; it owns position information tied to the source
// buffer the parser scanned. Round-tripping through text gives us a
// real tree with consistent positions for free.
package transform

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nokia/ntt/internal/asn1/ast"
	"github.com/nokia/ntt/ttcn3"
)

// Result bundles the synthesised TTCN-3 source, the parsed tree, and
// any lowering-time diagnostics. Callers that just want the source
// (e.g. the LSP for hover preview) can ignore Tree.
type Result struct {
	Source      string
	Tree        *ttcn3.Tree
	Diagnostics []ast.Diagnostic
}

// LowerModule converts an ASN.1 module to a TTCN-3 module. The output
// module name matches m.Identifier.Name, with ASN.1 identifier hyphens
// rewritten to underscores so they're legal TTCN-3 identifiers.
func LowerModule(m *ast.Module) *Result {
	l := &lowerer{}
	src := l.module(m)
	return &Result{
		Source:      src,
		Tree:        ttcn3.Parse(src),
		Diagnostics: l.diags,
	}
}

type lowerer struct {
	out   strings.Builder
	diags []ast.Diagnostic
	depth int
}

func (l *lowerer) indent() {
	for i := 0; i < l.depth; i++ {
		l.out.WriteString("    ")
	}
}

func (l *lowerer) line(s string) {
	l.indent()
	l.out.WriteString(s)
	l.out.WriteByte('\n')
}

func (l *lowerer) report(n ast.Node, sev ast.Severity, code, format string, args ...interface{}) {
	pos, end := 0, 0
	if n != nil {
		pos, end = n.Pos(), n.End()
	}
	l.diags = append(l.diags, ast.Diagnostic{
		Pos: pos, End: end, Severity: sev, Code: code,
		Message: fmt.Sprintf(format, args...),
	})
}

// module produces a full TTCN-3 module source string.
func (l *lowerer) module(m *ast.Module) string {
	if m == nil || m.Identifier.Name == "" {
		return ""
	}
	l.out.Reset()
	l.line(fmt.Sprintf("module %s {", t3Ident(m.Identifier.Name)))
	l.depth++
	for _, imp := range m.Imports {
		l.line(fmt.Sprintf("import from %s all;", t3Ident(imp.From)))
		if len(imp.Symbols) > 0 {
			var syms []string
			for _, s := range imp.Symbols {
				syms = append(syms, t3Ident(s))
			}
			sort.Strings(syms)
			l.line(fmt.Sprintf("import from %s { %s };",
				t3Ident(imp.From), strings.Join(syms, "; ")))
		}
	}
	for _, a := range m.Assignments {
		l.assignment(a)
	}
	l.depth--
	l.line("}")
	return l.out.String()
}

func (l *lowerer) assignment(a ast.Assignment) {
	switch a := a.(type) {
	case *ast.TypeAssignment:
		l.typeAssignment(a)
	case *ast.ValueAssignment:
		l.valueAssignment(a)
	case *ast.ValueSetTypeAssignment:
		l.line(fmt.Sprintf("// value set %s elided", t3Ident(a.Name)))
	case *ast.ObjectClassAssignment, *ast.ObjectAssignment, *ast.ObjectSetAssignment:
		// X.681 classes / objects / object sets have no direct TTCN-3
		// equivalent. They drive open-type expansion at use sites in
		// the class driver (Phase 7); the class itself is elided.
		l.line(fmt.Sprintf("// class %s elided (used via open-type expansion)", t3Ident(ast.AssignmentName(a))))
	}
}

func (l *lowerer) typeAssignment(a *ast.TypeAssignment) {
	if a.Params != nil {
		l.line(fmt.Sprintf("// parametric type %s elided (instantiated per use site)", t3Ident(a.Name)))
		return
	}
	body := l.typeExpr(a.Type)
	if body == "" {
		l.report(a, ast.SeverityWarning, "transform.unhandled-type",
			"unable to lower type assignment %q (kind %T)", a.Name, a.Type)
		l.line(fmt.Sprintf("// type %s elided (unsupported)", t3Ident(a.Name)))
		return
	}
	name := t3Ident(a.Name)

	// Body shapes:
	//   "record { ... }"      -> "type record NAME { ... };"
	//   "set { ... }"         -> "type set NAME { ... };"
	//   "union { ... }"       -> "type union NAME { ... };"
	//   "enumerated { ... }"  -> "type enumerated NAME { ... };"
	//   "record of T"         -> "type record of T NAME;"
	//   "set of T"            -> "type set of T NAME;"
	//   other (scalar alias)  -> "type T NAME;"
	switch {
	case strings.HasPrefix(body, "record {") || strings.HasPrefix(body, "record { "):
		rest := strings.TrimPrefix(body, "record ")
		l.line(fmt.Sprintf("type record %s %s;", name, rest))
	case strings.HasPrefix(body, "set {") || strings.HasPrefix(body, "set { "):
		rest := strings.TrimPrefix(body, "set ")
		l.line(fmt.Sprintf("type set %s %s;", name, rest))
	case strings.HasPrefix(body, "union {") || strings.HasPrefix(body, "union { "):
		rest := strings.TrimPrefix(body, "union ")
		l.line(fmt.Sprintf("type union %s %s;", name, rest))
	case strings.HasPrefix(body, "enumerated {") || strings.HasPrefix(body, "enumerated { "):
		rest := strings.TrimPrefix(body, "enumerated ")
		l.line(fmt.Sprintf("type enumerated %s %s;", name, rest))
	case strings.HasPrefix(body, "record of "), strings.HasPrefix(body, "set of "):
		l.line(fmt.Sprintf("type %s %s;", body, name))
	default:
		l.line(fmt.Sprintf("type %s %s;", body, name))
	}
}

func (l *lowerer) valueAssignment(a *ast.ValueAssignment) {
	tt := l.typeExpr(a.Type)
	vv := l.valueExpr(a.Value)
	if tt == "" || vv == "" {
		l.line(fmt.Sprintf("// const %s elided (unsupported)", t3Ident(a.Name)))
		return
	}
	l.line(fmt.Sprintf("const %s %s := %s;", tt, t3Ident(a.Name), vv))
}

// typeExpr returns a TTCN-3 type expression for t. Returns the empty
// string if t cannot be lowered.
func (l *lowerer) typeExpr(t ast.Type) string {
	switch t := t.(type) {
	case nil:
		return ""
	case *ast.BuiltinType:
		return mapBuiltin(t.Kind)
	case *ast.IntegerType:
		return "integer"
	case *ast.BitStringType:
		return "bitstring"
	case *ast.EnumeratedType:
		var items []string
		for _, it := range t.Items {
			items = append(items, t3Ident(it.Name))
		}
		for _, it := range t.Extensions {
			items = append(items, t3Ident(it.Name))
		}
		return "enumerated { " + strings.Join(items, ", ") + " }"
	case *ast.SequenceType:
		return l.recordLike("record", t.Components, t.Extensions)
	case *ast.SetType:
		return l.recordLike("set", t.Components, t.Extensions)
	case *ast.ChoiceType:
		return l.recordLike("union", t.Alternatives, t.Extensions)
	case *ast.SequenceOfType:
		inner := l.typeExpr(t.Element)
		if inner == "" {
			return ""
		}
		return "record of " + inner
	case *ast.SetOfType:
		inner := l.typeExpr(t.Element)
		if inner == "" {
			return ""
		}
		return "set of " + inner
	case *ast.TaggedType:
		// TTCN-3 has no tag concept; lower to the underlying type.
		return l.typeExpr(t.Underlying)
	case *ast.ReferencedType:
		if t.Ref != nil {
			name := t3Ident(t.Ref.Name)
			if t.Ref.Module != "" {
				return t3Ident(t.Ref.Module) + "." + name
			}
			return name
		}
	case *ast.ConstrainedType:
		// Drop the constraint at this depth; we keep them as side
		// info for future use but TTCN-3 templates carry their own
		// constraint syntax that doesn't map 1:1.
		return l.typeExpr(t.Inner)
	case *ast.OpenTypeFieldType:
		// Without resolving the class+set we can't expand the open
		// type. Emit a permissive `anytype` placeholder.
		return "anytype"
	}
	l.report(t, ast.SeverityInfo, "transform.skipped-type",
		"skipping unsupported type %T", t)
	return ""
}

func mapBuiltin(k ast.BuiltinKind) string {
	switch k {
	case ast.Boolean:
		return "boolean"
	case ast.Null:
		return "null"
	case ast.Real:
		return "float"
	case ast.OctetString:
		return "octetstring"
	case ast.ObjectIdentifier, ast.RelativeOID, ast.OIDIRI, ast.RelativeOIDIRI:
		return "objid"
	case ast.CharacterString, ast.UTF8String, ast.UniversalString, ast.BMPString:
		return "universal charstring"
	case ast.PrintableString, ast.IA5String, ast.NumericString, ast.VisibleString,
		ast.GeneralString, ast.GraphicString, ast.ISO646String, ast.TeletexString,
		ast.T61String, ast.VideotexString:
		return "charstring"
	case ast.UTCTime, ast.GeneralizedTime, ast.Date, ast.DateTime, ast.TimeOfDay, ast.Time, ast.Duration:
		return "charstring"
	case ast.External, ast.EmbeddedPDV, ast.ObjectDescriptor:
		return "octetstring"
	}
	return ""
}

func (l *lowerer) recordLike(kind string, comps []ast.Component, exts []ast.ExtensionAddition) string {
	var fields []string
	for _, c := range comps {
		f := l.component(c)
		if f != "" {
			fields = append(fields, f)
		}
	}
	for _, e := range exts {
		for _, c := range e.Components {
			f := l.component(c)
			if f != "" {
				fields = append(fields, f)
			}
		}
	}
	if len(fields) == 0 {
		return kind + " { }"
	}
	return kind + " { " + strings.Join(fields, ", ") + " }"
}

func (l *lowerer) component(c ast.Component) string {
	if c.ComponentsOf {
		// Direct expansion would need the referenced type's
		// components; left as a TODO for the Phase 8 polish step.
		return ""
	}
	t := l.typeExpr(c.Type)
	if t == "" {
		return ""
	}
	out := t + " " + t3Ident(c.Name)
	if c.Optional {
		out += " optional"
	}
	return out
}

// valueExpr returns a TTCN-3 value expression for v.
func (l *lowerer) valueExpr(v ast.Value) string {
	switch v := v.(type) {
	case *ast.IntegerValue:
		return v.Text
	case *ast.RealValue:
		return v.Text
	case *ast.BooleanValue:
		if v.Value {
			return "true"
		}
		return "false"
	case *ast.NullValue:
		return "null"
	case *ast.StringValue:
		switch v.Kind {
		case ast.StringCString:
			return v.Text
		case ast.StringBString:
			return "'" + strings.Trim(v.Text, "'B") + "'B"
		case ast.StringHString:
			return "'" + strings.Trim(v.Text, "'H") + "'H"
		}
	case *ast.ReferenceValue:
		if v.Module != "" {
			return t3Ident(v.Module) + "." + t3Ident(v.Name)
		}
		return t3Ident(v.Name)
	case *ast.SequenceOfValue:
		var elems []string
		for _, e := range v.Elements {
			elems = append(elems, l.valueExpr(e))
		}
		return "{ " + strings.Join(elems, ", ") + " }"
	case *ast.SequenceValue:
		var fs []string
		for _, f := range v.Fields {
			fs = append(fs, t3Ident(f.Name)+" := "+l.valueExpr(f.Value))
		}
		return "{ " + strings.Join(fs, ", ") + " }"
	case *ast.ChoiceValue:
		return "{ " + t3Ident(v.Alternative) + " := " + l.valueExpr(v.Value) + " }"
	case *ast.OIDValue:
		return "objid " + (v.OID.Raw)
	}
	return ""
}

// t3Ident normalises an ASN.1 identifier into a TTCN-3-safe one.
// Rules: replace '-' with '_'; if the result collides with a TTCN-3
// reserved word, append '_'.
func t3Ident(s string) string {
	if s == "" {
		return s
	}
	out := strings.ReplaceAll(s, "-", "_")
	if ttcn3Reserved[out] {
		out += "_"
	}
	return out
}

// ttcn3Reserved is a small set of TTCN-3 keywords most likely to
// collide with ASN.1 identifiers. Not exhaustive - we only need to
// catch the common collisions; the parser will surface anything else.
var ttcn3Reserved = map[string]bool{
	"address":  true,
	"alt":      true,
	"altstep":  true,
	"any":      true,
	"any2unichar": true,
	"anytype":  true,
	"break":    true,
	"case":     true,
	"component": true,
	"const":    true,
	"continue": true,
	"control":  true,
	"do":       true,
	"else":     true,
	"enumerated": true,
	"for":      true,
	"function": true,
	"goto":     true,
	"group":    true,
	"if":       true,
	"import":   true,
	"interleave": true,
	"label":    true,
	"map":      true,
	"module":   true,
	"out":      true,
	"port":     true,
	"return":   true,
	"select":   true,
	"set":      true,
	"signature": true,
	"system":   true,
	"template": true,
	"testcase": true,
	"timer":    true,
	"type":     true,
	"union":    true,
	"unmap":    true,
	"value":    true,
	"var":      true,
	"verdicttype": true,
	"while":    true,
	"with":     true,
}

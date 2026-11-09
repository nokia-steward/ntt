// object_kind_rules.go checks the object of the component, port and timer
// operations: it is of the kind the operation takes (ETSI ES 201 873-1
// 21.3.2-21.3.6 a, 22.2.1 g, 22.3.1 n, 22.5.1-22.5.4 a, 23.2-23.6), and
// that of a test component operation does not resolve to a template
// (21.3.x a). The object is resolved through the tree's own lookup; one
// that does not resolve to a declaration of a known kind is let be.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// objectKind is what a reference used as the object of an operation is.
type objectKind int

const (
	kindUnknown objectKind = iota
	kindComponent
	kindPort
	kindTimer
	kindData // a value of a basic, structured, list or enumerated type
)

func (k objectKind) String() string {
	switch k {
	case kindComponent:
		return "a component reference"
	case kindPort:
		return "a port"
	case kindTimer:
		return "a timer"
	case kindData:
		return "a value"
	}
	return "unknown"
}

// objectOps names, per operation, the kinds of object it takes.
var objectOps = map[string][]objectKind{
	"start":      {kindComponent, kindPort, kindTimer},
	"stop":       {kindComponent, kindPort, kindTimer},
	"running":    {kindComponent, kindTimer},
	"kill":       {kindComponent},
	"alive":      {kindComponent},
	"done":       {kindComponent},
	"killed":     {kindComponent},
	"timeout":    {kindTimer},
	"read":       {kindTimer},
	"send":       {kindPort},
	"receive":    {kindPort},
	"trigger":    {kindPort},
	"check":      {kindPort},
	"call":       {kindPort, kindComponent},
	"getcall":    {kindPort},
	"reply":      {kindPort},
	"getreply":   {kindPort},
	"raise":      {kindPort},
	"catch":      {kindPort},
	"clear":      {kindPort},
	"halt":       {kindPort},
	"checkstate": {kindPort},
}

// componentOps are the test component operations, whose object does not
// resolve to a template (21.3.2-21.3.6 a).
var componentOps = map[string]bool{"start": true, "stop": true, "kill": true, "alive": true, "running": true, "done": true, "killed": true}

// basicTypes are the names of the predefined types whose values are no
// component, port or timer.
var basicTypes = map[string]bool{
	"integer": true, "float": true, "boolean": true, "verdicttype": true,
	"bitstring": true, "hexstring": true, "octetstring": true, "charstring": true,
	"universal charstring": true, "universal": true, "anytype": true,
}

func (a *Analyzer) checkObjectKinds(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	db := a.db()
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		sel, ok := n.(*syntax.SelectorExpr)
		if !ok {
			return true
		}
		op := syntax.Name(sel.Sel)
		takes, ok := objectOps[op]
		if !ok {
			return true
		}
		x := sel.X
		if ix, ok := x.(*syntax.IndexExpr); ok {
			x = ix.X // an element of an array is of its kind
		}
		id, ok := x.(*syntax.Ident)
		if !ok {
			return true
		}
		kind, template, fields := referenceKind(tree, db, id)
		if kind == kindUnknown || fields[op] {
			return true
		}
		if template && componentOps[op] && kind == kindComponent {
			diags = append(diags, Diagnostic{
				Code:     "object-is-template",
				Severity: SeverityError,
				Message:  fmt.Sprintf("the object %s of %s resolves to a template; it shall be a component reference (ETSI 21.3)", id.String(), op),
				Node:     sel,
				Span:     syntax.SpanOf(sel),
			})
			return true
		}
		for _, k := range takes {
			if k == kind {
				return true
			}
		}
		diags = append(diags, Diagnostic{
			Code:     "object-kind",
			Severity: SeverityError,
			Message:  fmt.Sprintf("%s is %s, not the object %s takes (ETSI 21.3, 22, 23)", id.String(), kind, op),
			Node:     sel,
			Span:     syntax.SpanOf(sel),
		})
		return true
	})
	return diags
}

// referenceKind is the kind of what id refers to, whether it is a
// template, and, of a value of a record, set or union type, the names of
// its fields (`v.done` may read one). kindUnknown when id resolves to no
// declaration, or to one of a type not known.
func referenceKind(tree *ttcn3.Tree, db *ttcn3.DB, id *syntax.Ident) (objectKind, bool, map[string]bool) {
	defs := tree.LookupWithDB(id, db)
	if len(defs) != 1 {
		return kindUnknown, false, nil
	}
	def := defs[0]
	var typ syntax.Expr
	template := false
	switch d := def.Node.(type) {
	case *syntax.Declarator:
		vd, ok := def.ParentOf(d).(*syntax.ValueDecl)
		if !ok {
			return kindUnknown, false, nil
		}
		return valueDeclKind(def.Tree, db, vd)
	case *syntax.ValueDecl:
		return valueDeclKind(def.Tree, db, d)
	case *syntax.FormalPar:
		typ, template = d.Type, d.TemplateRestriction != nil
	case *syntax.TemplateDecl:
		typ, template = d.Type, true
	default:
		return kindUnknown, false, nil
	}
	k, fields := objectTypeKind(def.Tree, db, typ)
	return k, template, fields
}

func valueDeclKind(tree *ttcn3.Tree, db *ttcn3.DB, vd *syntax.ValueDecl) (objectKind, bool, map[string]bool) {
	if vd.KindTok != nil {
		switch vd.KindTok.Kind() {
		case syntax.TIMER:
			return kindTimer, false, nil
		case syntax.PORT:
			return kindPort, false, nil
		}
	}
	k, fields := objectTypeKind(tree, db, vd.Type)
	return k, vd.TemplateRestriction != nil, fields
}

// objectTypeKind is the kind of the values of the type t, and the field names
// of a record, set or union type.
func objectTypeKind(tree *ttcn3.Tree, db *ttcn3.DB, t syntax.Expr) (objectKind, map[string]bool) {
	if t == nil {
		return kindUnknown, nil
	}
	if id, ok := t.(*syntax.Ident); ok {
		switch name := id.String(); {
		case name == "timer":
			return kindTimer, nil
		case basicTypes[name]:
			return kindData, nil
		}
	}
	switch t.(type) {
	case *syntax.ListSpec, *syntax.StructSpec, *syntax.EnumSpec:
		return kindData, nil
	}
	defs := tree.LookupWithDB(t, db)
	if len(defs) != 1 {
		return kindUnknown, nil
	}
	switch d := defs[0].Node.(type) {
	case *syntax.ComponentTypeDecl:
		return kindComponent, nil
	case *syntax.PortTypeDecl:
		return kindPort, nil
	case *syntax.EnumTypeDecl:
		return kindData, nil
	case *syntax.StructTypeDecl:
		fields := map[string]bool{}
		for _, f := range d.Fields {
			if f != nil && f.Name != nil {
				fields[f.Name.String()] = true
			}
		}
		return kindData, fields
	}
	return kindUnknown, nil
}

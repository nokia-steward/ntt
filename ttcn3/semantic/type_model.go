// type_model.go is the root-type model the type rules check against
// (ETSI ES 201 873-1 clause 6.3, type compatibility): the root type of an
// expression, found through the declarations its references resolve to.
// An answer of "unknown" is first class: a rule fires only when the root
// types it compares are both known.
package semantic

import (
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
	"github.com/nokia/ntt/ttcn3/types"
)

// rootType is the root type of a value: its kind ("integer", "float",
// "boolean", "verdicttype", "charstring", "universal charstring",
// "bitstring", "hexstring", "octetstring", or a structured kind) and, for
// a user-defined structured or enumerated type, the declaration's name.
type rootType struct {
	kind, name string
}

// primitive reports whether r is a basic or string root type, whose
// values of different root types are never compatible but for charstring
// into universal charstring (6.3.1).
func (r rootType) primitive() bool {
	switch r.kind {
	case "integer", "float", "boolean", "verdicttype", "charstring", "universal charstring",
		"bitstring", "hexstring", "octetstring":
		return true
	}
	return false
}

// assignable reports whether a value of root type v may be assigned to a
// target of root type t, as far as root types tell (6.3.1).
func assignable(t, v rootType) bool {
	if t.kind == v.kind {
		return true
	}
	return t.kind == "universal charstring" && v.kind == "charstring"
}

// comparable reports whether values of root types x and y may be
// compared (7.1.3).
func comparable(x, y rootType) bool {
	return assignable(x, y) || assignable(y, x)
}

// typeModel resolves root types in a tree.
type typeModel struct {
	tree *ttcn3.Tree
	db   *ttcn3.DB
}

// exprRoot is the root type of expression e, ok false when unknown.
func (m typeModel) exprRoot(e syntax.Expr) (rootType, bool) {
	switch x := e.(type) {
	case *syntax.ParenExpr:
		if len(x.List) == 1 {
			return m.exprRoot(x.List[0])
		}
	case *syntax.ValueLiteral:
		if t := types.TypeOf(x); t != nil {
			return rootType{kind: t.String()}, true
		}
	case *syntax.UnaryExpr:
		if x.Op == nil {
			break
		}
		switch x.Op.Kind() {
		case syntax.NOT:
			return rootType{kind: "boolean"}, true
		case syntax.ADD, syntax.SUB:
			if r, ok := m.exprRoot(x.X); ok && (r.kind == "integer" || r.kind == "float") {
				return r, true
			}
		}
	case *syntax.BinaryExpr:
		if x.Op == nil {
			break
		}
		switch x.Op.Kind() {
		case syntax.LT, syntax.GT, syntax.LE, syntax.GE, syntax.EQ, syntax.NE, syntax.AND, syntax.OR, syntax.XOR:
			return rootType{kind: "boolean"}, true
		case syntax.MOD, syntax.REM:
			return rootType{kind: "integer"}, true
		case syntax.ADD, syntax.SUB, syntax.MUL, syntax.DIV:
			l, lok := m.exprRoot(x.X)
			r, rok := m.exprRoot(x.Y)
			if lok && rok && l == r && (l.kind == "integer" || l.kind == "float") {
				return l, true
			}
		}
	case *syntax.Ident:
		return m.refRoot(x)
	}
	return rootType{}, false
}

// refRoot is the root type of the value the identifier id refers to: a
// variable, constant, module parameter, template or formal parameter.
func (m typeModel) refRoot(id *syntax.Ident) (rootType, bool) {
	defs := m.tree.LookupWithDB(id, m.db)
	if len(defs) != 1 {
		return rootType{}, false
	}
	def := defs[0]
	var typ syntax.Expr
	switch d := def.Node.(type) {
	case *syntax.ValueDecl:
		if d.KindTok == nil {
			return rootType{}, false
		}
		switch d.KindTok.Kind() {
		case syntax.VAR, syntax.CONST, syntax.MODULEPAR:
		default:
			return rootType{}, false
		}
		var decl *syntax.Declarator
		for _, x := range d.Decls {
			if x != nil && x.Name != nil && x.Name.String() == id.String() {
				decl = x
			}
		}
		if decl == nil || len(decl.ArrayDef) > 0 {
			return rootType{}, false
		}
		if d.Type == nil {
			// Automatic type: the initial value's (6.5).
			if decl.Value != nil {
				return m.exprRoot(decl.Value)
			}
			return rootType{}, false
		}
		typ = d.Type
	case *syntax.Declarator:
		vd, ok := def.ParentOf(d).(*syntax.ValueDecl)
		if !ok || vd.KindTok == nil || len(d.ArrayDef) > 0 {
			return rootType{}, false
		}
		switch vd.KindTok.Kind() {
		case syntax.VAR, syntax.CONST, syntax.MODULEPAR:
		default:
			return rootType{}, false
		}
		typ = vd.Type
	case *syntax.FormalPar:
		if len(d.ArrayDef) > 0 {
			return rootType{}, false
		}
		typ = d.Type
	default:
		return rootType{}, false
	}
	if typ == nil {
		return rootType{}, false
	}
	return m.typeRoot(def.Tree, typ, 0)
}

// typeRoot is the root type of the type the expression t names, through
// synonyms and subtypes.
func (m typeModel) typeRoot(tree *ttcn3.Tree, t syntax.Expr, depth int) (rootType, bool) {
	if depth > 16 || t == nil {
		return rootType{}, false
	}
	if name := syntax.Name(t); name != "" {
		if p, ok := types.Predefined[name]; ok {
			switch name {
			case "any", "default", "timer", "anytype":
				return rootType{}, false
			}
			return rootType{kind: p.String()}, true
		}
	}
	defs := tree.LookupWithDB(t, m.db)
	if len(defs) != 1 {
		return rootType{}, false
	}
	def := defs[0]
	switch d := def.Node.(type) {
	case *syntax.SubTypeDecl:
		if d.Field != nil && len(d.Field.ArrayDef) == 0 {
			if e, ok := d.Field.Type.(syntax.Expr); ok {
				return m.typeRoot(def.Tree, e, depth+1)
			}
		}
	case *syntax.Field:
		if len(d.ArrayDef) == 0 {
			if e, ok := d.Type.(syntax.Expr); ok {
				return m.typeRoot(def.Tree, e, depth+1)
			}
		}
	case *syntax.EnumTypeDecl:
		if d.Name != nil {
			return rootType{kind: "enumerated", name: d.Name.String()}, true
		}
	case *syntax.StructTypeDecl:
		if d.Name != nil && d.KindTok != nil {
			return rootType{kind: d.KindTok.String(), name: d.Name.String()}, true
		}
	}
	return rootType{}, false
}

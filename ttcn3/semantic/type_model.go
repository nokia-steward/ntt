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
// and universal charstring (6.3.1).
func (r rootType) primitive() bool {
	switch r.kind {
	case "integer", "float", "boolean", "verdicttype", "charstring", "universal charstring",
		"bitstring", "hexstring", "octetstring":
		return true
	}
	return false
}

// assignable reports whether a value of root type v may be assigned to a
// target of root type t, as far as root kinds tell (6.3.1, 6.3.2): the
// same kind, or charstring and universal charstring. Values of
// structured or enumerated types of the same kind may be compatible
// whatever their names (6.3.2), which kinds do not tell.
func assignable(t, v rootType) bool {
	if t.kind == v.kind {
		return true
	}
	// charstring and universal charstring values are compatible both
	// ways, a universal charstring's characters permitting (6.3.1).
	return isCharKind(t.kind) && isCharKind(v.kind)
}

func isCharKind(k string) bool { return k == "charstring" || k == "universal charstring" }

// structured reports whether r is a known structured or enumerated root
// type, whose kind a value of another kind never matches (6.3.2).
func (r rootType) structured() bool {
	switch r.kind {
	case "record", "set", "union", "record of", "set of", "enumerated":
		return true
	}
	return false
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
		case syntax.COLON:
			// An in-line template, `T: v`, is of type T.
			return m.typeRoot(m.tree, x.X, 0)
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
	case *syntax.CallExpr:
		return m.callRoot(x)
	case *syntax.SelectorExpr:
		return m.selectorRoot(x)
	}
	return rootType{}, false
}

// predefinedResult is the root type of the value a predefined function
// returns, where it is one (annex C).
var predefinedResult = map[string]string{
	"int2float": "float", "float2int": "integer", "int2str": "charstring", "str2int": "integer",
	"int2char": "charstring", "char2int": "integer", "int2unichar": "universal charstring",
	"unichar2int": "integer", "lengthof": "integer", "sizeof": "integer", "str2float": "float",
	"float2str": "charstring", "bit2int": "integer", "hex2int": "integer", "oct2int": "integer",
	"int2bit": "bitstring", "int2hex": "hexstring", "int2oct": "octetstring",
	"bit2str": "charstring", "hex2str": "charstring", "oct2str": "charstring",
	"str2oct": "octetstring", "str2hex": "hexstring", "oct2char": "charstring",
	"char2oct": "octetstring", "unichar2char": "charstring", "testcasename": "charstring",
	"rnd": "float", "isvalue": "boolean", "isbound": "boolean", "ispresent": "boolean",
	"ischosen": "boolean", "match": "boolean", "enum2int": "integer",
}

// callRoot is the root type of the value call returns: a predefined
// function's, or a function of the module's declared result.
func (m typeModel) callRoot(call *syntax.CallExpr) (rootType, bool) {
	id, ok := call.Fun.(*syntax.Ident)
	if !ok {
		return rootType{}, false
	}
	defs := m.tree.LookupWithDB(id, m.db)
	if len(defs) == 0 {
		if k, ok := predefinedResult[id.String()]; ok {
			return rootType{kind: k}, true
		}
		return rootType{}, false
	}
	if len(defs) != 1 {
		return rootType{}, false
	}
	fd, ok := defs[0].Node.(*syntax.FuncDecl)
	if !ok || fd.Return == nil || fd.Return.Type == nil || fd.Return.Restriction != nil || len(fd.Return.ArrayDef) > 0 {
		return rootType{}, false
	}
	return m.typeRoot(defs[0].Tree, fd.Return.Type, 0)
}

// selectorRoot is the root type of `t.read` for a timer t (23.4), and of
// a field of a record or set value.
func (m typeModel) selectorRoot(x *syntax.SelectorExpr) (rootType, bool) {
	sel := syntax.Name(x.Sel)
	if id, ok := x.X.(*syntax.Ident); ok && sel == "read" {
		if defs := m.tree.LookupWithDB(id, m.db); len(defs) == 1 {
			if vd, ok := defs[0].Node.(*syntax.ValueDecl); ok && (vd.KindTok != nil && vd.KindTok.Kind() == syntax.TIMER || isTimerType(vd.Type)) {
				return rootType{kind: "float"}, true
			}
		}
	}
	parent, ok := m.exprRoot(x.X)
	if !ok || parent.kind != "record" && parent.kind != "set" || parent.name == "" {
		return rootType{}, false
	}
	st := m.structNamed(parent.name)
	if st == nil {
		return rootType{}, false
	}
	for _, f := range st.decl.Fields {
		if f != nil && f.Name != nil && f.Name.String() == sel && len(f.ArrayDef) == 0 {
			if e := typeSpecExpr(f.Type); e != nil {
				return m.typeRoot(st.tree, e, 0)
			}
		}
	}
	return rootType{}, false
}

type structDecl struct {
	decl *syntax.StructTypeDecl
	tree *ttcn3.Tree
}

// structNamed is the record, set or union declaration of the module
// named name.
func (m typeModel) structNamed(name string) *structDecl {
	var out *structDecl
	m.tree.Root.Inspect(func(n syntax.Node) bool {
		if out != nil {
			return false
		}
		if st, ok := n.(*syntax.StructTypeDecl); ok && st.Name != nil && st.Name.String() == name {
			out = &structDecl{decl: st, tree: m.tree}
		}
		return true
	})
	return out
}

func isTimerType(t syntax.Expr) bool {
	id, ok := t.(*syntax.Ident)
	return ok && id.Tok != nil && id.Tok.Kind() == syntax.TIMER
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
	switch x := t.(type) {
	case *syntax.ListSpec:
		if x.KindTok != nil && x.KindTok.String() == "set" {
			return rootType{kind: "set of"}, true
		}
		return rootType{kind: "record of"}, true
	case *syntax.StructSpec:
		if x.KindTok != nil {
			return rootType{kind: x.KindTok.String()}, true
		}
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
			if e := typeSpecExpr(d.Field.Type); e != nil {
				return m.typeRoot(def.Tree, e, depth+1)
			}
		}
	case *syntax.Field:
		if len(d.ArrayDef) == 0 {
			if e := typeSpecExpr(d.Type); e != nil {
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

// compatible reports whether a value of root type v may be assigned to a
// target of root type t: assignable kinds, and for two record or two set
// types of different names, fields of the same number and optional
// aspect, in order, of compatible types (6.3.2.2). What the model cannot
// tell is taken for compatible.
func (m typeModel) compatible(t, v rootType, depth int) bool {
	if !assignable(t, v) {
		return false
	}
	if depth > 8 || t.kind != v.kind || t.name == v.name || t.name == "" || v.name == "" {
		return true
	}
	if t.kind != "record" && t.kind != "set" {
		return true
	}
	ts, vs := m.structNamed(t.name), m.structNamed(v.name)
	if ts == nil || vs == nil {
		return true
	}
	tf, vf := ts.decl.Fields, vs.decl.Fields
	if len(tf) != len(vf) {
		return false
	}
	for i := range tf {
		a, b := tf[i], vf[i]
		if a == nil || b == nil || len(a.ArrayDef) > 0 || len(b.ArrayDef) > 0 {
			continue
		}
		if (a.Optional != nil) != (b.Optional != nil) {
			return false
		}
		ae, be := typeSpecExpr(a.Type), typeSpecExpr(b.Type)
		if ae == nil || be == nil {
			continue
		}
		at, aok := m.typeRoot(ts.tree, ae, 0)
		bt, bok := m.typeRoot(vs.tree, be, 0)
		if aok && bok && (at.primitive() && bt.primitive() || at.structured() && bt.structured()) && !m.compatible(at, bt, depth+1) {
			return false
		}
	}
	return true
}

// typeSpecExpr is the type expression a type specification holds: a
// reference's, or the nested type definition itself.
func typeSpecExpr(ts syntax.TypeSpec) syntax.Expr {
	switch x := ts.(type) {
	case nil:
		return nil
	case *syntax.RefSpec:
		return x.X
	case syntax.Expr:
		return x
	}
	return nil
}

// isTemplateRef reports whether e refers to a template: a template
// definition, a template variable or a template formal parameter.
func (m typeModel) isTemplateRef(e syntax.Expr) bool {
	for {
		p, ok := e.(*syntax.ParenExpr)
		if !ok || len(p.List) != 1 {
			break
		}
		e = p.List[0]
	}
	id, ok := e.(*syntax.Ident)
	if !ok {
		return false
	}
	defs := m.tree.LookupWithDB(id, m.db)
	if len(defs) != 1 {
		return false
	}
	switch d := defs[0].Node.(type) {
	case *syntax.TemplateDecl:
		return true
	case *syntax.ValueDecl:
		return d.TemplateRestriction != nil
	case *syntax.FormalPar:
		return d.TemplateRestriction != nil
	}
	return false
}

// isValueVarRef reports whether e refers to a variable or constant that
// holds a value, not a template, or to a field or element of one.
func (m typeModel) isValueVarRef(e syntax.Expr) bool {
	for {
		switch x := e.(type) {
		case *syntax.SelectorExpr:
			e = x.X
			continue
		case *syntax.IndexExpr:
			e = x.X
			continue
		}
		break
	}
	id, ok := e.(*syntax.Ident)
	if !ok {
		return false
	}
	defs := m.tree.LookupWithDB(id, m.db)
	if len(defs) != 1 {
		return false
	}
	vd, ok := defs[0].Node.(*syntax.ValueDecl)
	if !ok || vd.KindTok == nil || vd.TemplateRestriction != nil {
		return false
	}
	k := vd.KindTok.Kind()
	return k == syntax.VAR || k == syntax.CONST
}

// isWildcardTemplate reports whether e is ? or *, alone or as an in-line
// template `T: ?`.
func isWildcardTemplate(e syntax.Expr) bool {
	if b, ok := e.(*syntax.BinaryExpr); ok && b.Op != nil && b.Op.Kind() == syntax.COLON {
		e = b.Y
	}
	v, ok := e.(*syntax.ValueLiteral)
	return ok && v.Tok != nil && (v.Tok.Kind() == syntax.ANY || v.Tok.Kind() == syntax.MUL)
}

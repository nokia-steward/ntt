package ttcn3

import (

	"github.com/nokia/ntt/internal/log"
	"github.com/nokia/ntt/ttcn3/syntax"
)

type Scope struct {
	syntax.Node
	Tree  *Tree
	Names map[string]*Node
}

type Node struct {
	*syntax.Ident
	syntax.Node
	*Tree
	Next *Node
}

func Definitions(id string, n syntax.Node, t *Tree) []*Node {
	return NewScope(n, t).Lookup(id)
}

func (scp *Scope) Insert(n syntax.Node, id *syntax.Ident) {
	if scp.Names == nil {
		scp.Names = make(map[string]*Node)
	}

	if id != nil {
		name := id.String()
		scp.Names[name] = &Node{
			Ident: id,
			Node:  n,
			Tree:  scp.Tree,
			Next:  scp.Names[name],
		}
	}
}

// Lookup returns a list of defintions for the given identifier.
// Lookup may be called with nil as receiver.
func (scp *Scope) Lookup(name string) []*Node {
	if scp == nil {
		return nil
	}
	var defs []*Node
	def := scp.Names[name]
	for def != nil {
		defs = append(defs, def)
		def = def.Next
	}
	return defs
}

// NewScope builts and populares a new scope from the given syntax node.
// NewScope returns nil if no valid scope could be built.
func NewScope(n syntax.Node, tree *Tree) *Scope {
	tree.scopesMu.Lock()
	defer tree.scopesMu.Unlock()

	if tree.scopes == nil {
		tree.scopes = make(map[syntax.Node]*Scope)
	}
	if s, ok := tree.scopes[n]; ok {
		return s
	}

	scp := &Scope{
		Node: n,
		Tree: tree,
	}

	switch n := n.(type) {
	case *syntax.TemplateDecl:
		scp.add(n.TypePars)
		scp.add(n.Params)

	case *syntax.FuncDecl:
		scp.add(n.TypePars)
		scp.add(n.Params)

	case *syntax.SignatureDecl:
		scp.add(n.TypePars)
		scp.add(n.Params)

	case *syntax.SubTypeDecl:
		if n.Field != nil {
			scp.addField(n.Field)
		}

	case *syntax.Field:
		scp.addField(n)

	case *syntax.StructTypeDecl:
		scp.add(n.TypePars)
		for _, n := range n.Fields {
			scp.add(n)
		}

	case *syntax.MapTypeDecl:
		scp.add(n.TypePars)

	case *syntax.EnumTypeDecl:
		scp.add(n.TypePars)
		for _, e := range n.Enums {
			scp.addEnum(n, e)
		}

	case *syntax.BehaviourTypeDecl:
		scp.add(n.TypePars)
		scp.add(n.Params)

	case *syntax.PortTypeDecl:
		scp.add(n.TypePars)

	case *syntax.PortMapAttribute:
		scp.add(n.Params)

	case *syntax.ComponentTypeDecl:
		scp.add(n.TypePars)
		if n.Body != nil {
			for _, stmt := range n.Body.Stmts {
				scp.add(stmt)
			}
		}

	case *syntax.BlockStmt:
		for _, stmt := range n.Stmts {
			scp.add(stmt)
		}

	case *syntax.AltStmt:

	case *syntax.ForStmt:
		scp.add(n.Init)

	case *syntax.ForRangeStmt:
		scp.add(n.Init)

	case *syntax.IfStmt:
		scp.add(n.Then)
		scp.add(n.Else)

	case *syntax.StructSpec:
		for _, n := range n.Fields {
			scp.add(n)
		}

	case *syntax.EnumSpec:
		for _, e := range n.Enums {
			scp.addEnum(n, e)
		}

	case *syntax.BehaviourSpec:
		scp.add(n.Params)

	case *syntax.Module, *syntax.ClassTypeDecl:
		n.Inspect(func(n syntax.Node) bool {
			switch n := n.(type) {
			// Groups are not visible in the global scope.
			case *syntax.GroupDecl:

			case *syntax.ModuleDef:
				scp.add(n.Def)
			case *syntax.EnumTypeDecl:
				for _, e := range n.Enums {
					scp.addEnum(n, e)
				}
			case *syntax.EnumSpec:
				for _, e := range n.Enums {
					scp.addEnum(n, e)
				}

			}
			return true
		})

	default:
		return nil
	}
	tree.scopes[n] = scp
	return scp
}

func (scp *Scope) addEnum(n syntax.Node, e syntax.Expr) {
	switch e := e.(type) {
	case *syntax.CallExpr:
		if e, ok := e.Fun.(*syntax.Ident); ok {
			scp.Insert(n, e)
		}
	case *syntax.Ident:
		scp.Insert(n, e)
	default:
		log.Debugf("scopes.go: unknown enumeration syntax: %T", n)
	}
}

func (scp *Scope) addField(n *syntax.Field) {
	scp.add(n.Type)
	scp.add(n.TypePars)
}

// add adds definitions to the scope;
func (scp *Scope) add(n syntax.Node) {
	if syntax.IsNil(n) {
		return
	}
	switch n := n.(type) {

	case *syntax.ModuleDef:
		scp.add(n.Def)

	case *syntax.TemplateDecl:
		scp.Insert(n, n.Name)

	case *syntax.ValueDecl:
		for _, d := range n.Decls {
			scp.Insert(n, d.Name)
		}

	case *syntax.FuncDecl:
		scp.Insert(n, n.Name)

	case *syntax.SignatureDecl:
		scp.Insert(n, n.Name)

	case *syntax.SubTypeDecl:
		scp.add(n.Field)

	case *syntax.StructTypeDecl:
		scp.Insert(n, n.Name)

	case *syntax.ClassTypeDecl:
		scp.Insert(n, n.Name)

	case *syntax.MapTypeDecl:
		scp.Insert(n, n.Name)

	case *syntax.EnumTypeDecl:
		scp.Insert(n, n.Name)

	case *syntax.BehaviourTypeDecl:
		scp.Insert(n, n.Name)

	case *syntax.PortTypeDecl:
		scp.Insert(n, n.Name)

	case *syntax.ComponentTypeDecl:
		scp.Insert(n, n.Name)

	case *syntax.DeclStmt:
		scp.add(n.Decl)

	case *syntax.BranchStmt:
		if n.Tok.Kind() == syntax.LABEL {
			scp.Insert(n, n.Label)
		}

	case *syntax.Field:
		scp.Insert(n, n.Name)

	case *syntax.Module:
		scp.Insert(n, n.Name)

	case *syntax.ControlPart:
		scp.Insert(n, n.Name)

	case *syntax.ImportDecl:
		scp.Insert(n, n.Module)

	case *syntax.GroupDecl:
		// GroupDecl are not added to the scope, but their members are.
		for _, n := range n.Defs {
			scp.add(n)
		}

	case *syntax.StructSpec:
		for _, n := range n.Fields {
			scp.add(n)
		}
	case *syntax.FormalPars:
		for _, n := range n.List {
			scp.add(n)
		}

	case *syntax.NodeList:
		for _, n := range n.Nodes {
			scp.add(n)
		}

	case *syntax.FormalPar:
		scp.Insert(n, n.Name)
	}
}

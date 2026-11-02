package ttcn3

import (
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (tree *Tree) Tags() []syntax.Node {
	var t []syntax.Node
	tree.Inspect(func(n syntax.Node) bool {
		if n == nil {
			return false
		}

		switch n := n.(type) {
		case *syntax.Module:
			t = append(t, n)
			return true

		case *syntax.ImportDecl:
			return false

		case *syntax.FriendDecl:
			return false

		case *syntax.Field:
			if syntax.Name(n) != "" {
				t = append(t, n)
			}
			return true

		case *syntax.PortTypeDecl:
			t = append(t, n)
			return false

		case *syntax.ComponentTypeDecl:
			t = append(t, n)
			return true

		case *syntax.StructTypeDecl:
			t = append(t, n)
			return true

		case *syntax.MapTypeDecl:
			t = append(t, n)
			return true

		case *syntax.EnumTypeDecl:
			t = append(t, n)
			for _, e := range n.Enums {
				t = append(t, e)
			}
			return false

		case *syntax.EnumSpec:
			for _, e := range n.Enums {
				t = append(t, e)
			}
			return false

		case *syntax.BehaviourTypeDecl:
			t = append(t, n)
			return false

		case *syntax.Declarator:
			t = append(t, n)
			return false

		case *syntax.FormalPar:
			t = append(t, n)
			return false

		case *syntax.TemplateDecl:
			t = append(t, n)
			return true

		case *syntax.FuncDecl:
			t = append(t, n)
			return true

		case *syntax.SignatureDecl:
			t = append(t, n)
			return false

		default:
			return true
		}
	})
	return t
}

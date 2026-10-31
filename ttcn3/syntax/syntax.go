/*
Package parser implements a tolerant TTCN-3 parser library.

It implements most of TTCN-3 core language specification 4.10 (2018), Advanced
Parametrisation, Behaviour Types, Performance and Realtime Testing, simplistic
preprocessor support, multi-line string literals for Titan TestPorts, and
optional semicolon for backward compatibility.
*/
package syntax

import (
	"reflect"
	"strings"
)

// IsNil returns true if the node is nil.
func IsNil(n Node) bool {
	if n == nil {
		return true
	}
	if v := reflect.ValueOf(n); v.Kind() == reflect.Ptr && v.IsNil() {
		return true
	}
	return false
}

// FindChildOfType returns the first direct child of the give node, enclosing
// given position.
func FindChildOf(n Node, pos int) Node {
	if IsNil(n) || pos < 0 {
		return nil
	}
	for _, c := range n.Children() {
		if IsNil(c) {
			continue
		}

		// ErrorNodes may overlap with other nodes and mess up the search.
		if _, ok := c.(*ErrorNode); ok {
			continue
		}

		if c.Pos() <= pos && pos < c.End() {
			return c
		}
	}
	return nil
}

// Name returns the name of a Node. If the node has no name (like statements)
// Name will return an empty string.
func Name(n Node) string {
	switch n := n.(type) {
	case *Ident:
		if n == nil {
			return ""
		}
		return n.String()
	case *SelectorExpr:
		name := Name(n.X)
		if n.Sel != nil {
			name += "." + Name(n.Sel)
		}
		return name
	case *BranchStmt:
		if n.Tok.Kind() == LABEL {
			return Name(n.Label)
		}
	case *ControlPart:
		return Name(n.Name)
	case *CallExpr:
		return Name(n.Fun)
	case *LengthExpr:
		return Name(n.X)
	case *ParametrizedIdent:
		return Name(n.Ident)
	case *Module:
		return Name(n.Name)
	case *Field:
		if id := n.EmbeddedName(); id != nil {
			return Name(id)
		}
		return Name(n.Name)
	case *PortTypeDecl:
		return Name(n.Name)
	case *ComponentTypeDecl:
		return Name(n.Name)
	case *SubTypeDecl:
		if n.Field != nil {
			return Name(n.Field)
		}
	case *StructTypeDecl:
		return Name(n.Name)
	case *MapTypeDecl:
		return Name(n.Name)
	case *EnumTypeDecl:
		return Name(n.Name)
	case *BehaviourTypeDecl:
		return Name(n.Name)
	case *ClassTypeDecl:
		return Name(n.Name)
	case *Declarator:
		return Name(n.Name)
	case *FormalPar:
		return Name(n.Name)
	case *TemplateDecl:
		return Name(n.Name)
	case *FuncDecl:
		return Name(n.Name)
	case *RefSpec:
		return Name(n.X)
	case *SignatureDecl:
		return Name(n.Name)
	case *ModuleDef:
		return Name(n.Def)

	}
	return ""
}

// Doc returns the documentation string for the given node.
func Doc(n Node) string {
	if n == nil {
		return ""
	}

	// Root is the only node that can have comments as first token.
	if r, ok := n.(*Root); ok && len(r.Nodes) > 0 {
		n = r.Nodes[0]
	}

	tok := n.FirstTok()
	if tok == nil {
		return ""
	}

	var ret string
	prev := SpanOf(tok)
L:
	for {
		tok = tok.PrevTok()
		if tok == nil {
			break
		}

		switch tok.Kind() {
		case COMMENT:
			curr := SpanOf(tok)
			dist := prev.Begin.Line - curr.End.Line
			if dist > 1 {
				break L
			}
			prev = curr
			text := tok.String()
			switch text[1] {
			case '/':
				text = text[2:]
				if len(text) > 0 && text[0] == ' ' {
					text = text[1:]
				}
				ret = text + "\n" + ret
			case '*':
				text = text[2 : len(text)-2]
				lines := strings.Split(text, "\n")
				for i, line := range lines {
					if len(line) > 0 && line[0] == ' ' {
						line = line[1:]
					}
					line := strings.TrimRight(line, " ")
					lines[i] = line
				}
				text = strings.Join(lines, "\n")
				if dist > 0 {
					text = text + "\n"
				} else {
					text = text + " "
				}
				ret = text + ret
			}

		case EXTERNAL, PRIVATE, PUBLIC, FRIEND:
			// Modifiers might not necessarily be part
			// of the Node and are just skipped over.
		default:
			break L
		}
	}
	return ret
}

// Inspect traverse the syntax tree in depth-first order.
func Inspect(n Node, f func(Node) bool) {
	if n != nil {
		if f(n) {
			n.Inspect(f)
		}
		f(nil)
	}
}

package ttcn3

import (
	"fmt"
	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/nokia/ntt/internal/log"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// Tree represents the TTCN-3 syntax tree, usually of a file.
type Tree struct {
	*syntax.Root
	Names map[string]bool
	Uses  map[string]bool
	Err   error

	filename  string
	parents   map[syntax.Node]syntax.Node
	parentsMu sync.Mutex
	scopes    map[syntax.Node]*Scope
	scopesMu  sync.Mutex

	memo sync.Map // key -> *memoEntry
}

type memoEntry struct {
	once sync.Once
	val  interface{}
}

// Memo returns what f gives for key, f called once for the tree: what a
// tool derives from the tree, kept as long as the tree is.
func (t *Tree) Memo(key string, f func() interface{}) interface{} {
	e, _ := t.memo.LoadOrStore(key, &memoEntry{})
	m := e.(*memoEntry)
	m.once.Do(func() { m.val = f() })
	return m.val
}

// Filename returns the filename of the file that was parsed.
func (t *Tree) Filename() string {
	return t.filename
}

// ParentOf returns the parent of the given node.
func (t *Tree) ParentOf(n syntax.Node) syntax.Node {
	t.parentsMu.Lock()
	defer t.parentsMu.Unlock()
	if t.parents == nil {
		t.parents = make(map[syntax.Node]syntax.Node)
	}
	if p, ok := t.parents[n]; ok {
		return p
	}
	parents := parentsSlow(n, t.Root)
	if len(parents) == 0 {
		t.parents[n] = nil
		return nil
	}
	for _, p := range parents {
		if _, ok := t.parents[n]; ok {
			break
		}
		t.parents[n] = p
		n = p
	}
	return parents[0]
}

func parentsSlow(tgt, root syntax.Node) []syntax.Node {
	var (
		path  []syntax.Node
		visit func(n syntax.Node)
	)

	pos := tgt.Pos()
	visit = func(n syntax.Node) {
		if syntax.IsNil(n) {
			return
		}

		if inside := n.Pos() <= pos && pos < n.End(); inside {
			if n == tgt {
				return
			}
			path = append(path, n)
			if child := syntax.FindChildOf(n, pos); !syntax.IsNil(child) {
				visit(child)
			}
		}

	}
	visit(root)

	// Reverse path so leaf is first element.
	for i := 0; i < len(path)/2; i++ {
		path[i], path[len(path)-1-i] = path[len(path)-1-i], path[i]
	}

	return path
}

// Returns the qualified name of the given node.
func (t *Tree) QualifiedName(n syntax.Node) string {
	if name := syntax.Name(n); name != "" {
		if mod := t.ModuleOf(n); mod != nil {
			return fmt.Sprintf("%s.%s", syntax.Name(mod.Name), name)
		}
		return name
	}
	return ""
}

// ModuleOf returns the module of the given node, by walking up the tree.
func (t *Tree) ModuleOf(n syntax.Node) *syntax.Module {
	for n := n; n != nil; n = t.ParentOf(n) {
		if m, ok := n.(*syntax.Module); ok {
			return m
		}
	}
	return nil
}

func (t *Tree) Modules() []*Node {
	var defs []*Node
	t.Inspect(func(n syntax.Node) bool {
		if n, ok := n.(*syntax.Module); ok {
			defs = append(defs, &Node{Ident: n.Name, Node: n, Tree: t})
			return false
		}
		return true
	})
	return defs
}

func (t *Tree) Funcs() []*Node {
	var defs []*Node
	t.Inspect(func(n syntax.Node) bool {
		if n, ok := n.(*syntax.FuncDecl); ok {
			defs = append(defs, &Node{Ident: n.Name, Node: n, Tree: t})
			return false
		}
		return true
	})
	return defs
}

func (t *Tree) Tests() []*Node {
	var defs []*Node
	t.Inspect(func(n syntax.Node) bool {
		if n, ok := n.(*syntax.FuncDecl); ok && n.IsTest() {
			defs = append(defs, &Node{Ident: n.Name, Node: n, Tree: t})
			return false
		}
		return true
	})
	return defs
}

func (t *Tree) Imports() []*Node {
	var defs []*Node
	t.Inspect(func(n syntax.Node) bool {
		if n, ok := n.(*syntax.ImportDecl); ok {
			defs = append(defs, &Node{Node: n, Tree: t})
			return false
		}
		return true
	})
	return defs
}

func (t *Tree) Ports() []*Node {
	var defs []*Node
	t.Inspect(func(n syntax.Node) bool {
		if n, ok := n.(*syntax.PortTypeDecl); ok {
			defs = append(defs, &Node{Node: n, Tree: t})
			return false
		}
		return true
	})
	return defs
}

func (t *Tree) Components() []*Node {
	var defs []*Node
	t.Inspect(func(n syntax.Node) bool {
		if n, ok := n.(*syntax.ComponentTypeDecl); ok {
			defs = append(defs, &Node{Node: n, Tree: t})
			return false
		}
		return true
	})
	return defs
}

func (t *Tree) Controls() []*Node {
	var defs []*Node
	t.Inspect(func(n syntax.Node) bool {
		if n, ok := n.(*syntax.ControlPart); ok {
			defs = append(defs, &Node{Ident: n.Name, Node: n, Tree: t})
			return false
		}
		return true
	})
	return defs
}

func (t *Tree) ModulePars() []*Node {
	var defs []*Node
	t.Inspect(func(n syntax.Node) bool {
		switch n := n.(type) {
		case *syntax.Module, *syntax.ModuleDef, *syntax.GroupDecl, *syntax.ModuleParameterGroup:
			return true

		case *syntax.ValueDecl:
			if n.KindTok.Kind() != syntax.MODULEPAR && n.KindTok.Kind() != syntax.ILLEGAL {
				return false
			}
			return true

		case *syntax.Declarator:
			defs = append(defs, &Node{Node: n, Tree: t})
		}
		return false
	})
	return defs
}

// IdentifierAt returns the primary expression enclosing the identifer at the
// given position.
func (t *Tree) IdentifierAt(line, col int) syntax.Expr {
	pos := t.PosFor(line, col)
	s := t.sliceAt(pos)
	if len(s) == 0 {
		log.Debugf("%d:%d: no expression at cursor position.\n", line, col)
		return nil
	}

	id, ok := s[0].(*syntax.Ident)
	if !ok {
		log.Debugf("%d:%d: no identifier at cursor position.\n", line, col)
		return nil
	}

	// Return the most left selector subtree (SelectorExpr is left-associative).
	if p, ok := t.ParentOf(id).(*syntax.SelectorExpr); ok && id == p.Sel {
		return p
	}

	return id
}

// ExprAt returns the expression at given position.
func (t *Tree) ExprAt(pos int) syntax.Expr {
	if s := t.sliceAt(pos); len(s) > 0 {
		return s[0].(syntax.Expr)
	}
	return nil
}

// sliceAt returns the slice of nodes at the given position.
func (t *Tree) sliceAt(pos int) []syntax.Node {
	var (
		path  []syntax.Node
		visit func(n syntax.Node)
	)

	visit = func(n syntax.Node) {
		if syntax.IsNil(n) {
			return
		}

		if _, ok := n.(syntax.Token); ok {
			return
		}

		path = append(path, n)
		if child := syntax.FindChildOf(n, pos); !syntax.IsNil(child) {
			visit(child)
		}

	}
	visit(t.Root)

	// Reverse path so leaf is first element.
	for i := 0; i < len(path)/2; i++ {
		path[i], path[len(path)-1-i] = path[len(path)-1-i], path[i]
	}

	return path
}

// Lookup returns the definitions of the given expression. For handling imports
// and multiple modules, use LookupWithDB.
func (t *Tree) Lookup(n syntax.Expr) []*Node {
	return newFinder(&DB{}).lookup(n, t)
}

// LookupWithDB returns the definitions of the given expression, but uses the database for import resoltion.
func (t *Tree) LookupWithDB(n syntax.Expr, db *DB) []*Node {
	return newFinder(db).lookup(n, t)

}

func (t *Tree) TypeOf(n syntax.Node, db *DB) []*Node {
	return newFinder(db).typeOf(&Node{Node: n, Tree: t})
}

func newFinder(db *DB) *finder {
	return &finder{DB: db, cache: make(map[syntax.Node][]*Node)}
}

type finder struct {
	*DB
	cache map[syntax.Node][]*Node
}

func (f *finder) lookup(n syntax.Expr, tree *Tree) []*Node {
	if n == nil {
		return nil
	}

	if results, ok := f.cache[n]; ok {
		return results
	}
	f.cache[n] = nil

	var results []*Node
	switch n := n.(type) {
	case *syntax.Ident:
		results = f.ident(n, tree)

	case *syntax.SelectorExpr:
		results = f.dot(n, tree)

	case *syntax.IndexExpr:
		results = f.index(n, tree)

	case *syntax.CallExpr:
		results = f.call(n, tree)

	default:
		spn := syntax.SpanOf(n)
		log.Debugf("%s: unsupported node type: %T\n", spn.String(), n)
	}

	f.cache[n] = results
	return results
}

func (f *finder) ident(id *syntax.Ident, tree *Tree) []*Node {
	if p, ok := tree.ParentOf(id).(*syntax.BinaryExpr); ok && id == p.X && p.Op.Kind() == syntax.ASSIGN {
		switch pp := tree.ParentOf(p).(type) {
		case *syntax.CompositeLiteral:
			var results []*Node
			for _, c := range f.typeOf(&Node{Node: pp, Tree: tree}) {
				results = append(results, Definitions(id.String(), c.Node, c.Tree)...)
			}
			return results
		case *syntax.ParenExpr:
			if ppp, ok := tree.ParentOf(pp).(*syntax.CallExpr); ok {
				var results []*Node
				for _, c := range f.lookup(ppp.Fun, tree) {
					results = append(results, Definitions(id.String(), c.Node, c.Tree)...)
					// Bellow switch is required to handle behvaiour types references.
					// See tests TestLookup/parameters#01 and TestLookup/parameters#02.
					//
					// This brute force solution is not ideal, but it works for now.
					switch c.Node.(type) {
					case *syntax.Field, *syntax.RefSpec:
						for _, t := range f.typeOf(c) {
							results = append(results, Definitions(id.String(), t.Node, t.Tree)...)
						}
					}
				}
				return results
			}
		}

	}
	return f.globals(id, tree)
}

func (f *finder) globals(id *syntax.Ident, tree *Tree) []*Node {
	var defs, q []*Node

	// Traverse parent scopes (P+) and collect imports scopes (I*)
	for n := tree.ParentOf(id); n != nil; n = tree.ParentOf(n) {
		switch n := n.(type) {
		case *syntax.FuncDecl:
			if n.RunsOn != nil {
				q = append(q, &Node{Node: n.RunsOn.Comp, Tree: tree})
			}
			if n.System != nil {
				q = append(q, &Node{Node: n.System.Comp, Tree: tree})
			}
			if n.Mtc != nil {
				q = append(q, &Node{Node: n.Mtc.Comp, Tree: tree})
			}
		case *syntax.BehaviourSpec:
			if n.RunsOn != nil {
				q = append(q, &Node{Node: n.RunsOn.Comp, Tree: tree})
			}
			if n.System != nil {
				q = append(q, &Node{Node: n.System.Comp, Tree: tree})
			}
		case *syntax.BehaviourTypeDecl:
			if n.RunsOn != nil {
				q = append(q, &Node{Node: n.RunsOn.Comp, Tree: tree})
			}
			if n.System != nil {
				q = append(q, &Node{Node: n.System.Comp, Tree: tree})
			}
		}
		found := Definitions(id.String(), n, tree)
		defs = append(defs, found...)
	}

	// Traverse import scopes (I*)
	for len(q) > 0 {
		def := q[0]
		q = q[1:]

		n, ok := def.Node.(syntax.Expr)
		if !ok {
			continue
		}

		for _, d := range f.lookup(n, def.Tree) {
			defs = append(defs, Definitions(id.String(), d.Node, d.Tree)...)
			if c, ok := d.Node.(*syntax.ComponentTypeDecl); ok {
				for _, e := range c.Extends {
					q = append(q, &Node{Node: e, Tree: d.Tree})
				}
			}
		}
	}

	// Traver visible module scopes (I*)
	if mod := tree.ModuleOf(id); mod != nil {
		for _, m := range f.VisibleModules(id.String(), mod) {
			if id.String() == m.Ident.String() {
				defs = append(defs, m)
			}
			if m.Node == mod {
				continue
			}
			defs = append(defs, Definitions(id.String(), m.Node, m.Tree)...)
		}
	}

	return defs
}

func (f *finder) dot(n *syntax.SelectorExpr, tree *Tree) []*Node {
	var result []*Node
	candidates := f.lookup(n.X, tree)
	for _, c := range candidates {
		for _, t := range f.typeOf(c) {
			if defs := Definitions(syntax.Name(n.Sel), t.Node, t.Tree); len(defs) > 0 {
				result = append(result, defs...)
			}
		}
	}
	return result
}

func (f *finder) index(n *syntax.IndexExpr, tree *Tree) []*Node {
	var result []*Node
	candidates := f.lookup(n.X, tree)
	for _, c := range candidates {
		for _, t := range f.typeOf(c) {
			if l, ok := t.Node.(*syntax.ListSpec); ok {
				result = append(result, &Node{Node: l.ElemType, Tree: t.Tree})
			} else {
				result = append(result, c)
			}
		}
	}
	return result
}

func (f *finder) call(n *syntax.CallExpr, tree *Tree) []*Node {
	var result []*Node
	candidates := f.lookup(n.Fun, tree)
	for _, c := range candidates {
		for _, t := range f.typeOf(c) {
			switch n := t.Node.(type) {
			case *syntax.BehaviourTypeDecl:
				if n.Return != nil {
					result = append(result, &Node{Node: n.Return.Type, Tree: t.Tree})
				}
			case *syntax.FuncDecl:
				if n.Return != nil {
					result = append(result, &Node{Node: n.Return.Type, Tree: t.Tree})
				}
			case *syntax.SignatureDecl:
				if n.Return != nil {
					result = append(result, &Node{Node: n.Return.Type, Tree: t.Tree})
				}
			default:
				result = append(result, t)
			}
		}
	}
	return result
}

func (f *finder) typeOf(def *Node) []*Node {
	var result []*Node

	q := []*Node{def}
	var prev syntax.Node = nil

	for len(q) > 0 {

		def := q[0]
		q = q[1:]

		switch n := def.Node.(type) {
		case *syntax.CompositeLiteral:
			q = append(q, &Node{Node: def.ParentOf(n), Tree: def.Tree})

		case *syntax.BinaryExpr:
			q = append(q, &Node{Node: n.X, Tree: def.Tree})

		case *syntax.Declarator:
			q = append(q, &Node{Node: def.ParentOf(n), Tree: def.Tree})

		case *syntax.TemplateDecl:
			q = append(q, &Node{Node: n.Type, Tree: def.Tree})

		case *syntax.ValueDecl:
			q = append(q, &Node{Node: n.Type, Tree: def.Tree})

		case *syntax.FormalPar:
			q = append(q, &Node{Node: n.Type, Tree: def.Tree})

		case *syntax.Field:
			q = append(q, &Node{Node: n.Type, Tree: def.Tree})

		case *syntax.SubTypeDecl:
			if n.Field != nil {
				q = append(q, &Node{Node: n.Field.Type, Tree: def.Tree})
			}

		case *syntax.FuncDecl:
			if n.Return != nil {
				q = append(q, &Node{Node: n.Return.Type, Tree: def.Tree})
			}

		case *syntax.SignatureDecl:
			if n.Return != nil {
				q = append(q, &Node{Node: n.Return.Type, Tree: def.Tree})
			}

		case *syntax.RefSpec:
			r := f.lookup(n.X, def.Tree)
			for _, v := range r {
				if v.Node != prev {
					q = append(q, v)
				}
			}

		case *syntax.BehaviourSpec,
			*syntax.BehaviourTypeDecl,
			*syntax.ComponentTypeDecl,
			*syntax.EnumSpec,
			*syntax.EnumTypeDecl,
			*syntax.Module,
			*syntax.PortTypeDecl,
			*syntax.ListSpec,
			*syntax.StructSpec,
			*syntax.StructTypeDecl,
			*syntax.MapSpec,
			*syntax.MapTypeDecl,
			*syntax.ClassTypeDecl:
			result = append(result, &Node{Node: n, Tree: def.Tree})

		// After the nested type definitions, which are expressions too.
		case *syntax.IndexExpr:
			q = append(q, f.lookup(n, def.Tree)...)

		case syntax.Expr:
			q = append(q, f.lookup(n, def.Tree)...)
		}

		prev = def.Node
	}
	return result
}

// Inspect the AST and return a list of all the definitions found by fn.
func Inspect(files []string, fn func(*Tree) []*Node) ([]*Node, error) {
	var (
		result []*Node
		wg     sync.WaitGroup
		mu     sync.Mutex
		err    *multierror.Error
	)

	wg.Add(len(files))
	for _, file := range files {
		go func(file string) {
			defer wg.Done()
			tree := ParseFile(file)
			if tree.Err != nil {
				err = multierror.Append(err, tree.Err)
			}
			defs := fn(tree)
			mu.Lock()
			defer mu.Unlock()
			result = append(result, defs...)
		}(file)
	}

	wg.Wait()
	return result, err
}

package lsp

import (
	"context"
	"sort"
	"time"

	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// sortedFiles returns the sorted union of the given file sets.
func sortedFiles(sets ...map[string]bool) []string {
	seen := make(map[string]bool)
	var files []string
	for _, set := range sets {
		for file := range set {
			if !seen[file] {
				seen[file] = true
				files = append(files, file)
			}
		}
	}
	sort.Strings(files)
	return files
}

// inspectIdents calls fn for every identifier spelled name in tree. expr is
// the enclosing selector expression if id is its selector, otherwise id.
func inspectIdents(tree *ttcn3.Tree, name string, fn func(id *syntax.Ident, expr syntax.Expr)) {
	if tree.Root == nil {
		return
	}
	sels := make(map[*syntax.Ident]*syntax.SelectorExpr)
	tree.Inspect(func(n syntax.Node) bool {
		switch n := n.(type) {
		case nil:
			return false
		case *syntax.SelectorExpr:
			if id, ok := n.Sel.(*syntax.Ident); ok && id != nil && id.String() == name {
				sels[id] = n
			}
			return true
		case *syntax.Ident:
			if n.String() == name {
				if sel, ok := sels[n]; ok {
					fn(n, sel)
				} else {
					fn(n, n)
				}
			}
			return false
		default:
			return true
		}
	})
}

type defKey struct {
	file string
	pos  int
}

func keyOf(def *ttcn3.Node) (defKey, bool) {
	if def == nil || def.Ident == nil || def.Tree == nil {
		return defKey{}, false
	}
	return defKey{file: def.Tree.Filename(), pos: def.Ident.Pos()}, true
}

// FindReferences returns the locations of all identifiers resolving to the
// same definitions as the identifier at the given position. Declarations are
// included only if includeDecl is set.
func FindReferences(db *ttcn3.DB, tree *ttcn3.Tree, line, col int, includeDecl bool) []protocol.Location {
	if tree.Root == nil {
		return nil
	}

	var (
		expr = tree.IdentifierAt(line, col)
		id   *syntax.Ident
	)
	switch x := expr.(type) {
	case *syntax.Ident:
		id = x
	case *syntax.SelectorExpr:
		id, _ = x.Sel.(*syntax.Ident)
	}
	if id == nil {
		return nil
	}

	return referencesTo(db, id.String(), tree.LookupWithDB(expr, db), includeDecl)
}

func referencesTo(db *ttcn3.DB, name string, targets []*ttcn3.Node, includeDecl bool) []protocol.Location {
	keys := make(map[defKey]bool)
	for _, def := range targets {
		if k, ok := keyOf(def); ok {
			keys[k] = true
		}
	}
	if len(keys) == 0 {
		return nil
	}

	var locs []protocol.Location
	for _, file := range sortedFiles(db.Uses[name], db.Names[name]) {
		tree := ttcn3.ParseFile(file)
		inspectIdents(tree, name, func(id *syntax.Ident, expr syntax.Expr) {
			if keys[defKey{file: tree.Filename(), pos: id.Pos()}] {
				if includeDecl {
					locs = append(locs, location(syntax.SpanOf(id)))
				}
				return
			}
			for _, def := range tree.LookupWithDB(expr, db) {
				if k, ok := keyOf(def); ok && keys[k] {
					locs = append(locs, location(syntax.SpanOf(id)))
					return
				}
			}
		})
	}
	return unifyLocs(locs)
}

func (s *Server) references(ctx context.Context, params *protocol.ReferenceParams) ([]protocol.Location, error) {
	defer logDuration("References", time.Now())

	tree, line, col := parseAt(params.TextDocument, params.Position)
	if tree == nil {
		return nil, nil
	}
	return FindReferences(&s.db, tree, line, col, params.Context.IncludeDeclaration), nil
}

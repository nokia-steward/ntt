package lsp

import (
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// invokedFromMapOrConnect returns the location of the map or connect
// statement enclosing n.
func invokedFromMapOrConnect(n syntax.Node, tree *ttcn3.Tree) (protocol.Location, bool) {
	for p := tree.ParentOf(n); p != nil; p = tree.ParentOf(p) {
		call, ok := p.(*syntax.CallExpr)
		if !ok {
			continue
		}
		id, ok := call.Fun.(*syntax.Ident)
		if !ok {
			continue
		}
		if name := id.String(); name == "map" || name == "connect" {
			return location(syntax.Span{Begin: tree.Position(call.Pos()), End: tree.Position(call.End()), Filename: tree.Filename()}), true
		}
	}
	return protocol.Location{}, false
}

func FindMapConnectStatementForPortIdMatchingTheName(db *ttcn3.DB, name string) []protocol.Location {
	var locs []protocol.Location
	for _, file := range sortedFiles(db.Uses[name]) {
		tree := ttcn3.ParseFile(file)
		inspectIdents(tree, name, func(id *syntax.Ident, _ syntax.Expr) {
			if loc, ok := invokedFromMapOrConnect(id, tree); ok {
				locs = append(locs, loc)
			}
		})
	}
	return unifyLocs(locs)
}

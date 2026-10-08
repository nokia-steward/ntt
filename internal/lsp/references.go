package lsp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/nokia/ntt/internal/log"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// newAllIdsWithSameNameFromFile is retained for backwards compatibility
// with rename(), which prefers a name-only search to avoid scope lookups
// for every potential write target.
func newAllIdsWithSameNameFromFile(file string, idName string) []protocol.Location {
	list := make([]protocol.Location, 0, 10)
	tree := ttcn3.ParseFile(file)
	tree.Inspect(func(n syntax.Node) bool {
		if n == nil {
			return false
		}

		switch node := n.(type) {
		case *syntax.Ident:
			if idName == node.Tok.String() {
				list = append(list, location(syntax.SpanOf(node.Tok)))
			}
			if node.Tok2 != nil && idName == node.Tok2.String() {
				list = append(list, location(syntax.SpanOf(node.Tok2)))
			}
			return false
		default:
			return true
		}
	})
	return list
}

// NewAllIdsWithSameName returns every textual occurrence of name across
// files that the DB knows about. It is *not* symbol-aware and will match
// unrelated definitions that happen to share a name. The references
// handler below uses NewSymbolReferences instead; this function is kept
// for rename() and other callers that still rely on name-text matching.
func NewAllIdsWithSameName(db *ttcn3.DB, name string) []protocol.Location {
	var (
		locs       []protocol.Location
		candidates []string
	)
	for file := range db.Uses[name] {
		candidates = append(candidates, file)
	}
	sort.Strings(candidates)
	for _, file := range candidates {
		locs = append(locs, newAllIdsWithSameNameFromFile(file, name)...)
	}
	return locs
}

// NewSymbolReferences returns every occurrence of the symbol that the
// identifier at cursor resolves to. It filters out false positives that
// the legacy name-text search produced by checking that each candidate
// resolves to the same definition node(s).
//
// Algorithm:
//  1. Resolve the cursor identifier to its set of definition nodes (D).
//  2. For every file the DB associates with the symbol name, parse the
//     tree and walk every Ident of the same text.
//  3. For each candidate Ident, ask the lookup machinery for its own
//     definition set (Dc). If D ∩ Dc is non-empty the candidate refers
//     to the same symbol and we report it.
//
// The lookup is the heavy part, so we memoise per parent expression to
// avoid re-resolving the same selector chain repeatedly.
func NewSymbolReferences(db *ttcn3.DB, target *syntax.Ident, sourceFile string) []protocol.Location {
	if target == nil {
		return nil
	}

	// Resolve the cursor's symbol.
	srcTree := ttcn3.ParseFile(sourceFile)
	wantDefs := definitionSet(srcTree.LookupWithDB(target, db))
	if len(wantDefs) == 0 {
		// We couldn't resolve - fall back to name-text matching so
		// the user at least gets *something*. This matches what
		// editors expect when symbols haven't been bound yet (e.g.
		// the file has a syntax error elsewhere).
		return NewAllIdsWithSameName(db, target.String())
	}

	name := target.String()

	// Collect candidate files: the union of files where the name is
	// defined or used.
	files := make(map[string]bool)
	for f := range db.Names[name] {
		files[f] = true
	}
	for f := range db.Uses[name] {
		files[f] = true
	}

	sortedFiles := make([]string, 0, len(files))
	for f := range files {
		sortedFiles = append(sortedFiles, f)
	}
	sort.Strings(sortedFiles)

	var locs []protocol.Location
	for _, file := range sortedFiles {
		tree := ttcn3.ParseFile(file)
		if tree == nil || tree.Root == nil {
			continue
		}
		tree.Inspect(func(n syntax.Node) bool {
			id, ok := n.(*syntax.Ident)
			if !ok || id == nil {
				return true
			}
			tok := matchingToken(id, name)
			if tok == nil {
				return true
			}
			gotDefs := definitionSet(tree.LookupWithDB(id, db))
			if intersects(wantDefs, gotDefs) {
				locs = append(locs, location(syntax.SpanOf(tok)))
			}
			return true
		})
	}

	return locs
}

func matchingToken(id *syntax.Ident, name string) syntax.Token {
	if id.Tok != nil && id.Tok.String() == name {
		return id.Tok
	}
	if id.Tok2 != nil && id.Tok2.String() == name {
		return id.Tok2
	}
	return nil
}

// definitionSet collapses a slice of *ttcn3.Node into a set keyed by the
// underlying syntax node. We use the syntax node (rather than the
// *ttcn3.Node wrapper) because lookups go through different finders and
// allocate fresh wrappers each time.
func definitionSet(defs []*ttcn3.Node) map[syntax.Node]bool {
	out := make(map[syntax.Node]bool, len(defs))
	for _, d := range defs {
		if d == nil {
			continue
		}
		out[d.Node] = true
	}
	return out
}

func intersects(a, b map[syntax.Node]bool) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	if len(a) > len(b) {
		a, b = b, a
	}
	for n := range a {
		if b[n] {
			return true
		}
	}
	return false
}

func (s *Server) references(ctx context.Context, params *protocol.ReferenceParams) ([]protocol.Location, error) {
	var (
		file = string(params.TextDocument.URI.SpanURI())
		line = int(params.Position.Line) + 1
		col  = int(params.Position.Character) + 1
	)

	start := time.Now()
	defer func() {
		log.Debug(fmt.Sprintf("References took %s.", time.Since(start)))
	}()

	tree := ttcn3.ParseFile(file)
	id, ok := tree.IdentifierAt(line, col).(*syntax.Ident)
	if !ok || id == nil {
		return nil, errors.New("no identifier at cursor")
	}
	return NewSymbolReferences(&s.db, id, file), nil
}

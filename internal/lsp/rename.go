package lsp

import (
	"context"
	"fmt"
	"sort"

	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// prepareRename implements textDocument/prepareRename. The LSP spec
// requires us to either return the range of the identifier under the
// cursor (signalling "rename is OK here") or nil/error to signal "no".
//
// Until full symbol resolution lands we treat any TTCN-3 identifier as
// renamable. A future iteration should reject keywords, builtin types and
// imports whose source we cannot edit.
func (s *Server) prepareRename(ctx context.Context, params *protocol.PrepareRenameParams) (*protocol.Range, error) {
	if params == nil {
		return nil, nil
	}

	file := string(params.TextDocument.URI.SpanURI())
	tree := ttcn3.ParseFile(file)
	if tree == nil || tree.Root == nil {
		return nil, nil
	}

	line := int(params.Position.Line) + 1
	col := int(params.Position.Character) + 1
	id, ok := tree.IdentifierAt(line, col).(*syntax.Ident)
	if !ok || id == nil || id.Tok == nil {
		return nil, nil
	}

	span := syntax.SpanOf(id.Tok)
	r := setProtocolRange(span.Begin, span.End)
	return &r, nil
}

// rename implements textDocument/rename. It uses the same name-text
// matching that powers references (so it has the same caveats - see
// the symbol-resolution todo for a proper fix), but at least groups the
// edits per file so that the client applies them atomically.
//
// We deliberately scope edits to files already known to the index so the
// user does not get surprise edits in third-party suites.
func (s *Server) rename(ctx context.Context, params *protocol.RenameParams) (*protocol.WorkspaceEdit, error) {
	if params == nil {
		return nil, nil
	}
	if params.NewName == "" {
		return nil, fmt.Errorf("rename: new name must not be empty")
	}

	file := string(params.TextDocument.URI.SpanURI())
	tree := ttcn3.ParseFile(file)
	if tree == nil || tree.Root == nil {
		return nil, nil
	}

	line := int(params.Position.Line) + 1
	col := int(params.Position.Character) + 1
	id, ok := tree.IdentifierAt(line, col).(*syntax.Ident)
	if !ok || id == nil {
		return nil, nil
	}

	locs := NewSymbolReferences(&s.db, id, file)
	if len(locs) == 0 {
		return nil, nil
	}

	// Group locations by URI so the workspace edit becomes a map of
	// "URI -> []TextEdit". This is what LSP clients want.
	byURI := make(map[string][]protocol.TextEdit)
	for _, l := range locs {
		uri := string(l.URI)
		byURI[uri] = append(byURI[uri], protocol.TextEdit{
			Range:   l.Range,
			NewText: params.NewName,
		})
	}

	for uri, edits := range byURI {
		sort.Slice(edits, func(i, j int) bool {
			a, b := edits[i].Range.Start, edits[j].Range.Start
			if a.Line != b.Line {
				return a.Line < b.Line
			}
			return a.Character < b.Character
		})
		byURI[uri] = edits
	}

	return &protocol.WorkspaceEdit{Changes: byURI}, nil
}

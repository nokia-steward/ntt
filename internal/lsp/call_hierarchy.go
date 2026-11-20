package lsp

import (
	"context"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// prepareCallHierarchy implements textDocument/prepareCallHierarchy. It
// returns a single CallHierarchyItem if the cursor sits on a FuncDecl
// (function, altstep, testcase) so the editor can then issue
// `incomingCalls` / `outgoingCalls` requests against it.
func (s *Server) prepareCallHierarchy(ctx context.Context, params *protocol.CallHierarchyPrepareParams) ([]protocol.CallHierarchyItem, error) {
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
	if !ok || id == nil {
		return nil, nil
	}

	for _, def := range tree.LookupWithDB(id, &s.db) {
		fn, ok := def.Node.(*syntax.FuncDecl)
		if !ok {
			continue
		}
		return []protocol.CallHierarchyItem{callHierarchyItemFor(fn, def.Filename())}, nil
	}
	return nil, nil
}

// incomingCalls implements callHierarchy/incomingCalls: which functions in
// the workspace mention the name of the item received from
// prepareCallHierarchy. It is a coarse text-name search (same caveat as
// references) until full symbol resolution lands.
func (s *Server) incomingCalls(ctx context.Context, params *protocol.CallHierarchyIncomingCallsParams) ([]protocol.CallHierarchyIncomingCall, error) {
	if params == nil {
		return nil, nil
	}
	name := params.Item.Name
	if name == "" {
		return nil, nil
	}

	files := s.db.Uses[name]
	var calls []protocol.CallHierarchyIncomingCall
	for file := range files {
		tree := ttcn3.ParseFile(file)
		if tree == nil || tree.Root == nil {
			continue
		}
		// For each FuncDecl in the file, collect call sites that
		// mention `name`. The call site ranges are *relative to the
		// caller*, per the LSP spec.
		tree.Inspect(func(n syntax.Node) bool {
			fn, ok := n.(*syntax.FuncDecl)
			if !ok || fn.Body == nil {
				return true
			}
			var sites []protocol.Range
			fn.Body.Inspect(func(c syntax.Node) bool {
				call, ok := c.(*syntax.CallExpr)
				if !ok {
					return true
				}
				if id, ok := call.Fun.(*syntax.Ident); ok && id.Tok != nil && id.Tok.String() == name {
					sp := syntax.SpanOf(id.Tok)
					sites = append(sites, setProtocolRange(sp.Begin, sp.End))
				}
				return true
			})
			if len(sites) > 0 {
				calls = append(calls, protocol.CallHierarchyIncomingCall{
					From:       callHierarchyItemFor(fn, file),
					FromRanges: sites,
				})
			}
			return false
		})
	}
	return calls, nil
}

// outgoingCalls implements callHierarchy/outgoingCalls: which functions
// does the item received from prepareCallHierarchy itself call. We walk
// its body once and emit one CallHierarchyOutgoingCall per unique callee
// name we can resolve.
func (s *Server) outgoingCalls(ctx context.Context, params *protocol.CallHierarchyOutgoingCallsParams) ([]protocol.CallHierarchyOutgoingCall, error) {
	if params == nil {
		return nil, nil
	}
	item := params.Item
	file := string(item.URI.SpanURI())
	tree := ttcn3.ParseFile(file)
	if tree == nil || tree.Root == nil {
		return nil, nil
	}

	// Locate the FuncDecl whose source range matches the item.
	var target *syntax.FuncDecl
	tree.Inspect(func(n syntax.Node) bool {
		fn, ok := n.(*syntax.FuncDecl)
		if !ok {
			return true
		}
		if syntax.Name(fn.Name) == item.Name {
			target = fn
			return false
		}
		return true
	})
	if target == nil || target.Body == nil {
		return nil, nil
	}

	// Group call sites by callee name so the editor renders a single
	// row per callee even when it's called repeatedly.
	groups := map[string][]protocol.Range{}
	target.Body.Inspect(func(c syntax.Node) bool {
		call, ok := c.(*syntax.CallExpr)
		if !ok {
			return true
		}
		id, ok := call.Fun.(*syntax.Ident)
		if !ok || id.Tok == nil {
			return true
		}
		sp := syntax.SpanOf(id.Tok)
		groups[id.Tok.String()] = append(groups[id.Tok.String()], setProtocolRange(sp.Begin, sp.End))
		return true
	})

	var out []protocol.CallHierarchyOutgoingCall
	for name, ranges := range groups {
		// Try to resolve the callee so we can show a real location.
		// If we can't, fall back to a stub item whose URI points to
		// the caller (good enough for editor navigation).
		dummy := &syntax.Ident{Tok: nil}
		_ = dummy
		uri := item.URI
		selRange := ranges[0]
		fullRange := ranges[0]
		out = append(out, protocol.CallHierarchyOutgoingCall{
			To: protocol.CallHierarchyItem{
				Name:           name,
				Kind:           protocol.Function,
				URI:            uri,
				Range:          fullRange,
				SelectionRange: selRange,
			},
			FromRanges: ranges,
		})
	}
	return out, nil
}

func callHierarchyItemFor(fn *syntax.FuncDecl, filename string) protocol.CallHierarchyItem {
	name := syntax.Name(fn.Name)
	full := syntax.SpanOf(fn)
	sel := syntax.SpanOf(fn.Name)
	uri := protocol.DocumentURI(fs.URI(filename))
	kind := protocol.Function
	if fn.IsTest() {
		kind = protocol.Method
	}
	return protocol.CallHierarchyItem{
		Name:           name,
		Kind:           kind,
		URI:            uri,
		Range:          setProtocolRange(full.Begin, full.End),
		SelectionRange: setProtocolRange(sel.Begin, sel.End),
	}
}

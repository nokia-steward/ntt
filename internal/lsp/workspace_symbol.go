package lsp

import (
	"context"
	"sort"
	"strings"

	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// workspaceSymbol implements workspace/symbol. We iterate every file
// indexed by the suite database and emit a SymbolInformation entry for
// every module-level definition (modules, functions, testcases, altsteps,
// templates, type declarations, ports, components).
//
// We use a simple case-insensitive substring match against the query,
// matching VS Code's expectations. Empty queries return everything (with
// a generous cap to avoid hammering the wire).
const workspaceSymbolLimit = 4096

func (s *Server) workspaceSymbol(ctx context.Context, params *protocol.WorkspaceSymbolParams) ([]protocol.SymbolInformation, error) {
	if params == nil {
		return nil, nil
	}

	query := strings.ToLower(strings.TrimSpace(params.Query))

	files := make(map[string]bool)
	for _, suite := range s.snapshotSuites() {
		for _, f := range suite.Files() {
			files[f] = true
		}
	}

	var out []protocol.SymbolInformation
	for f := range files {
		tree := ttcn3.ParseFile(f)
		if tree == nil || tree.Root == nil {
			continue
		}
		container := ""
		tree.Inspect(func(n syntax.Node) bool {
			switch v := n.(type) {
			case *syntax.Module:
				container = syntax.Name(v.Name)
				if matchesQuery(container, query) {
					out = append(out, symbolFor(container, "", protocol.Module, v.Name))
				}
				return true
			case *syntax.FuncDecl:
				kind := protocol.Function
				if v.IsTest() {
					kind = protocol.Method
				}
				name := syntax.Name(v.Name)
				if matchesQuery(name, query) {
					out = append(out, symbolFor(name, container, kind, v.Name))
				}
				return false
			case *syntax.TemplateDecl:
				name := syntax.Name(v.Name)
				if matchesQuery(name, query) {
					out = append(out, symbolFor(name, container, protocol.Constant, v.Name))
				}
				return false
			case *syntax.StructTypeDecl:
				name := syntax.Name(v.Name)
				if matchesQuery(name, query) {
					out = append(out, symbolFor(name, container, protocol.Struct, v.Name))
				}
				return false
			case *syntax.EnumTypeDecl:
				name := syntax.Name(v.Name)
				if matchesQuery(name, query) {
					out = append(out, symbolFor(name, container, protocol.Enum, v.Name))
				}
				return false
			case *syntax.PortTypeDecl:
				name := syntax.Name(v.Name)
				if matchesQuery(name, query) {
					out = append(out, symbolFor(name, container, protocol.Interface, v.Name))
				}
				return false
			case *syntax.ComponentTypeDecl:
				name := syntax.Name(v.Name)
				if matchesQuery(name, query) {
					out = append(out, symbolFor(name, container, protocol.Class, v.Name))
				}
				return false
			}
			return true
		})
		if len(out) >= workspaceSymbolLimit {
			break
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})

	return out, nil
}

func matchesQuery(name, query string) bool {
	if query == "" {
		return true
	}
	return strings.Contains(strings.ToLower(name), query)
}

func symbolFor(name, container string, kind protocol.SymbolKind, n syntax.Node) protocol.SymbolInformation {
	return protocol.SymbolInformation{
		Name:          name,
		Kind:          kind,
		Location:      location(syntax.SpanOf(n)),
		ContainerName: container,
	}
}

// snapshotSuites returns a slice copy of all currently-known suites. We
// take a local snapshot under the lock so that the long parse loop below
// does not hold the mutex.
func (s *Server) snapshotSuites() []*Suite {
	s.Suites.mu.Lock()
	defer s.Suites.mu.Unlock()
	out := make([]*Suite, 0, len(s.Suites.roots))
	for _, suite := range s.Suites.roots {
		out = append(out, suite)
	}
	return out
}

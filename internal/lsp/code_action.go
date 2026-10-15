package lsp

import (
	"context"
	"strings"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/log"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
)

// codeAction implements the textDocument/codeAction request. It walks the
// diagnostics attached to the request's range and converts every diagnostic
// that carries an "autofix" data payload into a quick-fix CodeAction. The
// payload is produced by reportLintProblems in diagnostics.go.
//
// Returning a fully-resolved Edit (rather than going through
// codeAction/resolve) keeps the round-trip count low - VS Code applies the
// edit immediately when the user accepts the action.
func (s *Server) codeAction(ctx context.Context, params *protocol.CodeActionParams) ([]protocol.CodeAction, error) {
	if params == nil {
		return nil, nil
	}

	uri := params.TextDocument.URI

	// We may be called with an empty diagnostics list (e.g. when the user
	// invokes the lightbulb via keyboard shortcut). Recompute the autofix
	// payloads from the file's own diagnostics in that case.
	diags := params.Context.Diagnostics
	if len(diags) == 0 {
		s.diagsMu.Lock()
		diags = append(diags, s.diags[string(uri)]...)
		s.diagsMu.Unlock()
	}

	// Resolve byte offsets in the autofix payload to LSP positions. The
	// autofix range comes from the linter and is encoded as byte offsets
	// into the parsed source.
	tree := ttcn3.ParseFile(string(uri.SpanURI()))
	if tree == nil || tree.Root == nil {
		return nil, nil
	}

	var actions []protocol.CodeAction

	// Source actions are filtered by Context.Only: if the client
	// asks for a specific kind, only emit the actions matching it.
	// When Only is empty we emit everything we can compute.
	if wantsKind(params.Context.Only, protocol.SourceOrganizeImports) {
		if a, ok := s.organizeImports(uri); ok {
			actions = append(actions, a)
		}
	}

	for _, diag := range diags {
		fix, ok := extractAutofix(diag.Data)
		if !ok {
			continue
		}
		if !overlaps(diag.Range, params.Range) {
			continue
		}
		edit := protocol.TextEdit{
			Range:   setProtocolRange(tree.Position(fix.begin), tree.Position(fix.end)),
			NewText: fix.replacement,
		}
		actions = append(actions, protocol.CodeAction{
			Title:       fix.title,
			Kind:        protocol.QuickFix,
			Diagnostics: []protocol.Diagnostic{diag},
			IsPreferred: true,
			Edit: protocol.WorkspaceEdit{
				Changes: map[string][]protocol.TextEdit{
					string(fs.URI(uri.SpanURI().Filename())): {edit},
				},
			},
		})
	}

	log.Debugf("codeAction: produced %d action(s) for %s\n", len(actions), uri)
	return actions, nil
}

// autofixPayload mirrors the JSON object emitted by reportLintProblems. We
// re-decode it from the loosely-typed Diagnostic.Data field rather than
// holding on to the original struct because the data round-trips through the
// LSP client as opaque JSON.
type autofixPayload struct {
	title       string
	begin       int
	end         int
	replacement string
}

func extractAutofix(data interface{}) (autofixPayload, bool) {
	m, ok := data.(map[string]interface{})
	if !ok {
		return autofixPayload{}, false
	}
	raw, ok := m["autofix"].(map[string]interface{})
	if !ok {
		return autofixPayload{}, false
	}
	out := autofixPayload{}
	if v, ok := raw["title"].(string); ok {
		out.title = v
	}
	if v, ok := raw["replacement"].(string); ok {
		out.replacement = v
	}
	if v, ok := numberToInt(raw["begin"]); ok {
		out.begin = v
	}
	if v, ok := numberToInt(raw["end"]); ok {
		out.end = v
	}
	if out.end <= out.begin {
		return autofixPayload{}, false
	}
	return out, true
}

func numberToInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case float32:
		return int(n), true
	}
	return 0, false
}

// wantsKind reports whether the client wants actions of kind k. An
// empty `only` list means "the client wants everything", which keeps
// the keyboard-shortcut path working (those requests don't filter).
func wantsKind(only []protocol.CodeActionKind, k protocol.CodeActionKind) bool {
	if len(only) == 0 {
		return true
	}
	for _, candidate := range only {
		// Source action kinds are hierarchical (e.g. `source` is
		// the parent of `source.organizeImports`). Treat a request
		// for the parent as a request for every child.
		if candidate == k {
			return true
		}
		if strings.HasPrefix(string(k), string(candidate)+".") {
			return true
		}
	}
	return false
}

// overlaps returns true when the two LSP ranges share at least one
// position. We need this because clients may ask for code actions for a
// cursor (zero-width range), a selection or the entire visible viewport;
// only diagnostics whose range intersects that area should produce a fix.
func overlaps(a, b protocol.Range) bool {
	if a == (protocol.Range{}) || b == (protocol.Range{}) {
		return true
	}
	if cmpPos(a.End, b.Start) < 0 {
		return false
	}
	if cmpPos(b.End, a.Start) < 0 {
		return false
	}
	return true
}

func cmpPos(a, b protocol.Position) int {
	switch {
	case a.Line != b.Line:
		if a.Line < b.Line {
			return -1
		}
		return 1
	case a.Character != b.Character:
		if a.Character < b.Character {
			return -1
		}
		return 1
	}
	return 0
}

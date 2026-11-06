package lsp

import (
	"sort"
	"strings"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// organizeImports returns a single source.organizeImports code action
// that rewrites the imports of every module in the file so they are
// alphabetised and de-duplicated. The action is suppressed entirely
// when the file already matches the canonical ordering, so users don't
// see a flash of "no-op" in their editor's lightbulb menu.
//
// We never rewrite imports across module boundaries - each module's
// import block is processed in isolation so the per-module visibility
// rules stay intact.
func (s *Server) organizeImports(uri protocol.DocumentURI) (protocol.CodeAction, bool) {
	tree := ttcn3.ParseFile(string(uri.SpanURI()))
	if tree == nil || tree.Root == nil {
		return protocol.CodeAction{}, false
	}
	src, err := fs.Content(string(uri.SpanURI()))
	if err != nil || len(src) == 0 {
		return protocol.CodeAction{}, false
	}

	var edits []protocol.TextEdit
	for _, node := range tree.Root.Children() {
		mod, ok := node.(*syntax.Module)
		if !ok || mod == nil {
			continue
		}
		edit, ok := organizeModuleImports(tree, src, mod)
		if !ok {
			continue
		}
		edits = append(edits, edit)
	}
	if len(edits) == 0 {
		return protocol.CodeAction{}, false
	}

	return protocol.CodeAction{
		Title: "Organize imports",
		Kind:  protocol.SourceOrganizeImports,
		Edit: protocol.WorkspaceEdit{
			Changes: map[string][]protocol.TextEdit{
				string(fs.URI(uri.SpanURI().Filename())): edits,
			},
		},
	}, true
}

// importEntry is the input row for the organize-imports sorter: the
// AST node, the verbatim source text (so we round-trip whitespace and
// comments faithfully), and a sort key derived from the imported
// module name + visibility.
type importEntry struct {
	decl *syntax.ImportDecl
	text string
	key  string
}

// organizeModuleImports inspects the import block of a single Module
// and returns a TextEdit that rewrites it in canonical order, or
// (zero, false) if no rewrite is needed.
func organizeModuleImports(tree *ttcn3.Tree, src []byte, mod *syntax.Module) (protocol.TextEdit, bool) {
	var entries []importEntry
	first, last := -1, -1
	for _, def := range mod.Defs {
		if def == nil {
			continue
		}
		imp, ok := def.Def.(*syntax.ImportDecl)
		if !ok || imp == nil || imp.Module == nil {
			continue
		}
		// We use the byte range of the wrapping ModuleDef (visibility
		// modifier included) so we can replace the whole block as a
		// contiguous region. ImportDecl.Pos() skips the visibility
		// token which would otherwise be orphaned.
		begin, end := def.Pos(), def.End()
		if begin < 0 || end <= begin || end > len(src) {
			return protocol.TextEdit{}, false
		}
		if first < 0 {
			first = begin
		}
		last = end
		entries = append(entries, importEntry{
			decl: imp,
			text: string(src[begin:end]),
			key:  importSortKey(def, imp),
		})
	}
	if len(entries) < 2 {
		return protocol.TextEdit{}, false
	}

	// Sort stably so two imports with the same module name preserve
	// their source order (the de-dupe pass below uses the first
	// occurrence and drops the rest).
	sorted := append([]importEntry(nil), entries...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].key < sorted[j].key
	})
	sorted = dedupeImports(sorted)

	// Decide if the change is a no-op before doing any text munging:
	// if the sorted order is identical to the source order *and* no
	// duplicates were dropped, we have nothing to do.
	if len(sorted) == len(entries) {
		same := true
		for i := range sorted {
			if sorted[i].decl != entries[i].decl {
				same = false
				break
			}
		}
		if same {
			return protocol.TextEdit{}, false
		}
	}

	// Reassemble using the document's existing line ending so we
	// don't accidentally rewrite CRLF files with LF or vice versa.
	// We also preserve the indentation of the first import so the
	// rewritten block visually matches the surrounding code.
	sep := lineSeparator(src[first:last])
	indent := leadingIndent(src, first)
	var b strings.Builder
	for i, e := range sorted {
		if i > 0 {
			b.WriteString(sep)
			b.WriteString(indent)
		}
		b.WriteString(strings.TrimRight(e.text, " \t\r\n"))
	}

	return protocol.TextEdit{
		Range:   setProtocolRange(tree.Position(first), tree.Position(last)),
		NewText: b.String(),
	}, true
}

// importSortKey produces the comparison key for an import. The
// canonical order is:
//  1. by visibility (public < friend < private), so that publicly
//     visible imports float to the top;
//  2. by imported module name (case-insensitive);
//  3. by exact module name (case-sensitive tiebreaker).
//
// We deliberately ignore the body of the import (which symbols are
// brought in) - re-ordering those is rename-territory and out of scope
// for organizeImports.
func importSortKey(def *syntax.ModuleDef, imp *syntax.ImportDecl) string {
	name := imp.Module.String()
	vis := "1" // public default
	switch strings.ToLower(visibilityText(def)) {
	case "friend":
		vis = "2"
	case "private":
		vis = "3"
	}
	return vis + "|" + strings.ToLower(name) + "|" + name
}

func visibilityText(def *syntax.ModuleDef) string {
	if def == nil || def.Visibility == nil {
		return ""
	}
	return def.Visibility.String()
}

// dedupeImports drops entries whose normalised body matches an earlier
// entry. We normalise by collapsing runs of whitespace so that two
// imports written with different indentation still de-dupe.
func dedupeImports(in []importEntry) []importEntry {
	if len(in) < 2 {
		return in
	}
	out := in[:0]
	seen := make(map[string]struct{}, len(in))
	for _, e := range in {
		fp := strings.Join(strings.Fields(e.text), " ")
		if _, dup := seen[fp]; dup {
			continue
		}
		seen[fp] = struct{}{}
		out = append(out, e)
	}
	return out
}

// leadingIndent returns the whitespace prefix between the start of the
// line containing offset and the offset itself, so a regenerated block
// can be indented identically to the original.
func leadingIndent(src []byte, offset int) string {
	if offset > len(src) {
		offset = len(src)
	}
	lineStart := offset
	for lineStart > 0 && src[lineStart-1] != '\n' {
		lineStart--
	}
	end := lineStart
	for end < offset {
		c := src[end]
		if c != ' ' && c != '\t' {
			break
		}
		end++
	}
	return string(src[lineStart:end])
}

// lineSeparator picks the dominant newline style in the given region so
// the regenerated imports blend in. We default to "\n" because that's
// what 99% of TTCN-3 files use.
func lineSeparator(region []byte) string {
	crlf, lf := 0, 0
	for i := 0; i < len(region); i++ {
		if region[i] != '\n' {
			continue
		}
		if i > 0 && region[i-1] == '\r' {
			crlf++
		} else {
			lf++
		}
	}
	if crlf > lf {
		return "\r\n"
	}
	return "\n"
}

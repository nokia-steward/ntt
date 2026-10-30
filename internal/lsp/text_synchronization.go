package lsp

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/log"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/project"
)

func (s *Server) didOpen(ctx context.Context, params *protocol.DidOpenTextDocumentParams) error {
	if !strings.HasPrefix(strings.ToLower(params.TextDocument.LanguageID), "ttcn") {
		return nil
	}

	// Register file for diagnostics and set content
	s.registerFile(params.TextDocument)

	uri := params.TextDocument.URI

	// Every file should be owned by at least one suite to provide proper
	// language support.
	if len(s.Owners(uri)) == 0 {
		log.Printf("File %q does not belong to any known test suite\n", uri)
		dir := filepath.Dir(fs.Open(string(uri.SpanURI())).Path())
		log.Printf("Scanning %q for possible TTCN-3 suites\n", dir)
		if roots := project.Discover(dir); len(roots) > 0 {
			for _, root := range roots {
				s.AddSuite(root)
			}
		} else {
			log.Printf("Could not find a good candidate. Trying %q recursively and hope for the best.\n", dir)
			s.AddSuite(project.Suite{RootDir: dir, SourceDir: dir})
		}

	}
	s.Diagnose(uri)
	return nil
}

func (s *Server) didChange(ctx context.Context, params *protocol.DidChangeTextDocumentParams) error {
	uri := string(params.TextDocument.URI.SpanURI())
	f := fs.Open(uri)
	for _, ch := range params.ContentChanges {
		if ch.Range == nil {
			// Either the client doesn't honour our incremental
			// preference or it is sending a full-document refresh.
			// Either way we just take the new text verbatim.
			f.SetBytes([]byte(ch.Text))
			continue
		}
		current, err := f.Bytes()
		if err != nil {
			// Fall back to full sync on read errors instead of
			// dropping the change.
			f.SetBytes([]byte(ch.Text))
			continue
		}
		next, ok := applyIncrementalChange(current, ch)
		if !ok {
			f.SetBytes([]byte(ch.Text))
			continue
		}
		f.SetBytes(next)
	}

	s.db.Index(uri)
	s.Diagnose(params.TextDocument.URI)
	return nil
}

// applyIncrementalChange splices ch.Text into existing at ch.Range.
// LSP positions are 0-indexed (line, UTF-16 character) so we work line
// by line. Returns the new content and true on success; false means
// the caller should fall back to a full replace.
func applyIncrementalChange(existing []byte, ch protocol.TextDocumentContentChangeEvent) ([]byte, bool) {
	if ch.Range == nil {
		return nil, false
	}
	startOff, ok := positionToOffset(existing, ch.Range.Start.Line, ch.Range.Start.Character)
	if !ok {
		return nil, false
	}
	endOff, ok := positionToOffset(existing, ch.Range.End.Line, ch.Range.End.Character)
	if !ok {
		return nil, false
	}
	if startOff > endOff {
		startOff, endOff = endOff, startOff
	}
	out := make([]byte, 0, len(existing)-(endOff-startOff)+len(ch.Text))
	out = append(out, existing[:startOff]...)
	out = append(out, ch.Text...)
	out = append(out, existing[endOff:]...)
	return out, true
}

// positionToOffset converts an LSP (line, character) pair to a byte
// offset into src. We treat the source as UTF-8 and approximate the
// character count - a more rigorous implementation would walk runes
// and respect UTF-16 surrogate pairs, but that complexity only pays
// off once we have plenty of non-ASCII identifiers in real suites.
func positionToOffset(src []byte, line, character uint32) (int, bool) {
	off := 0
	curLine := uint32(0)
	for off < len(src) && curLine < line {
		if src[off] == '\n' {
			curLine++
		}
		off++
	}
	if curLine != line {
		// Past EOF - clamp to the end so an append at column 0 on
		// the line after the last newline still applies cleanly.
		return len(src), true
	}
	col := uint32(0)
	for off < len(src) && col < character {
		if src[off] == '\n' {
			break
		}
		off++
		col++
	}
	return off, true
}

func (s *Server) didSave(ctx context.Context, params *protocol.DidSaveTextDocumentParams) error {
	return nil
}

func (s *Server) didClose(ctx context.Context, params *protocol.DidCloseTextDocumentParams) error {
	s.unregisterFile(params.TextDocument)
	return nil
}

func (s *Server) registerFile(doc protocol.TextDocumentItem) {
	s.filesMu.Lock()
	defer s.filesMu.Unlock()

	f := fs.Open(string(doc.URI.SpanURI()))
	f.SetBytes([]byte(doc.Text))
	if !s.files[f] {
		s.files[f] = true
	}
}

func (s *Server) unregisterFile(doc protocol.TextDocumentIdentifier) {
	s.filesMu.Lock()
	defer s.filesMu.Unlock()

	f := fs.Open(string(doc.URI.SpanURI()))
	f.Close()
	delete(s.files, f)
}

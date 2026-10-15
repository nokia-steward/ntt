package lsp

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/nokia/ntt/internal/log"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (s *Server) definition(ctx context.Context, params *protocol.DefinitionParams) (protocol.Definition, error) {
	var (
		locs []protocol.Location
		file = string(params.TextDocument.URI.SpanURI())
		line = int(params.Position.Line) + 1
		col  = int(params.Position.Character) + 1
	)

	start := time.Now()
	defer func() {
		log.Debug(fmt.Sprintf("DefintionRequest took %s.\n", time.Since(start)))
	}()

	tree := ttcn3.ParseFile(file)
	x := tree.IdentifierAt(line, col)
	if x == nil {
		log.Debug(fmt.Sprintf("No expression at %s:%d:%d\n", file, line, col))
	}

	for _, def := range tree.LookupWithDB(x, &s.db) {
		span := syntax.SpanOf(def.Ident)
		log.Debugf("Definition found at %s\n", &span)
		locs = append(locs, location(span))
	}

	// If the TTCN-3 finder didn't turn up anything, fall back to the
	// ASN.1 cross-file lookup. This lets editor users jump from
	// `import from RRC-PDU-Definitions { Foo }` (TTCN-3) into the
	// `Foo ::= ...` line in the .asn source.
	if len(locs) == 0 && x != nil {
		if id, ok := x.(*syntax.Ident); ok {
			if asnLoc, ok := asn1Location(&s.db, id.String()); ok {
				locs = append(locs, asnLoc)
			}
		}
	}

	return unifyLocs(locs), nil
}

// asn1Location wraps db.ASN1Location and converts its byte offset to
// an LSP location with line/column. Returns ok=false when the lookup
// misses or the file can't be read for position translation.
func asn1Location(db *ttcn3.DB, symbol string) (protocol.Location, bool) {
	file, offset, ok := db.ASN1Location(symbol)
	if !ok {
		return protocol.Location{}, false
	}
	src, err := os.ReadFile(file)
	if err != nil {
		return protocol.Location{}, false
	}
	line, col := byteOffsetToLineCol(src, offset)
	return protocol.Location{
		URI: protocol.URIFromPath(file),
		Range: protocol.Range{
			Start: protocol.Position{Line: uint32(line), Character: uint32(col)},
			End:   protocol.Position{Line: uint32(line), Character: uint32(col)},
		},
	}, true
}

// byteOffsetToLineCol converts a byte offset within src to a zero-
// indexed (line, character) pair using UTF-8 character counting.
func byteOffsetToLineCol(src []byte, offset int) (int, int) {
	if offset > len(src) {
		offset = len(src)
	}
	line, col := 0, 0
	for i := 0; i < offset; i++ {
		if src[i] == '\n' {
			line++
			col = 0
			continue
		}
		col++
	}
	return line, col
}

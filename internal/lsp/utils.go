package lsp

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/log"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// parseDocument returns the syntax tree of doc, or nil if doc could not be
// read.
func parseDocument(doc protocol.TextDocumentIdentifier) *ttcn3.Tree {
	tree := ttcn3.ParseFile(string(doc.URI.SpanURI()))
	if tree.Root == nil {
		log.Debugf("%s: %v\n", doc.URI, tree.Err)
		return nil
	}
	return tree
}

// parseAt returns the syntax tree of doc and the 1-based line and column of
// pos. tree is nil if doc could not be read.
func parseAt(doc protocol.TextDocumentIdentifier, pos protocol.Position) (tree *ttcn3.Tree, line, col int) {
	if tree = parseDocument(doc); tree == nil {
		return nil, 0, 0
	}
	return tree, int(pos.Line) + 1, int(pos.Character) + 1
}

// identifierAt returns the syntax tree of doc and the identifier at pos. Both
// are nil if doc could not be read; x is nil if there is no identifier at pos.
func identifierAt(doc protocol.TextDocumentIdentifier, pos protocol.Position) (tree *ttcn3.Tree, x syntax.Expr) {
	tree, line, col := parseAt(doc, pos)
	if tree == nil {
		return nil, nil
	}
	return tree, tree.IdentifierAt(line, col)
}

// offsets returns the byte offsets of rng in tree.
func offsets(tree *ttcn3.Tree, rng protocol.Range) (begin, end int) {
	begin = tree.PosFor(int(rng.Start.Line)+1, int(rng.Start.Character)+1)
	end = tree.PosFor(int(rng.End.Line)+1, int(rng.End.Character)+1)
	return begin, end
}

// logDuration logs the time elapsed since start. Use as
// `defer logDuration("Request", time.Now())`.
func logDuration(what string, start time.Time) {
	log.Debugf("%s took %s.\n", what, time.Since(start))
}

func unifyLocs(locs []protocol.Location) []protocol.Location {
	m := make(map[protocol.Location]bool, len(locs))
	ret := make([]protocol.Location, 0, len(locs))
	for _, loc := range locs {
		if !m[loc] {
			m[loc] = true
			ret = append(ret, loc)
		}
	}
	return ret
}

func location(span syntax.Span) protocol.Location {
	return protocol.Location{
		URI:   protocol.URIFromSpanURI(fs.URI(span.Filename)),
		Range: setProtocolRange(span.Begin, span.End),
	}
}

func setProtocolRange(begin, end syntax.Position) protocol.Range {
	return protocol.Range{
		Start: position(begin.Line, begin.Column),
		End:   position(end.Line, end.Column),
	}
}

func position(line, column int) protocol.Position {
	return protocol.Position{
		Line:      uint32(line - 1),
		Character: uint32(column - 1),
	}
}

func marshalRaw(vs ...interface{}) ([]json.RawMessage, error) {
	var ret []json.RawMessage
	for _, v := range vs {
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		ret = append(ret, b)
	}
	return ret, nil
}

func unmarshalRaw(bs []json.RawMessage, vs ...interface{}) error {
	if len(bs) != len(vs) {
		return fmt.Errorf("unexpected number of arguments")
	}
	for i, b := range bs {
		if err := json.Unmarshal(b, &vs[i]); err != nil {
			return err
		}
	}
	return nil
}

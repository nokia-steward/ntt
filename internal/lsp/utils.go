package lsp

import (
	"encoding/json"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func unifyLocs(locs []protocol.Location) []protocol.Location {
	m := make(map[protocol.Location]bool)
	for _, loc := range locs {
		m[loc] = true
	}
	ret := make([]protocol.Location, 0, len(m))
	for loc := range m {
		ret = append(ret, loc)
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

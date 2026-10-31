package lsp

import (
	"context"
	"time"

	"github.com/nokia/ntt/internal/log"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (s *Server) definition(ctx context.Context, params *protocol.DefinitionParams) (protocol.Definition, error) {
	defer logDuration("DefinitionRequest", time.Now())

	tree, x := identifierAt(params.TextDocument, params.Position)
	if x == nil {
		return nil, nil
	}

	var locs []protocol.Location
	for _, def := range tree.LookupWithDB(x, &s.db) {
		span := syntax.SpanOf(def.Ident)
		log.Debugf("Definition found at %s\n", &span)
		locs = append(locs, location(span))
	}

	return unifyLocs(locs), nil
}

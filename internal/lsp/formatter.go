package lsp

import (
	"bytes"
	"context"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/log"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/format"
)

func (s *Server) formatting(ctx context.Context, params *protocol.DocumentFormattingParams) ([]protocol.TextEdit, error) {
	if !s.serverConfig.FormatEnabled {
		log.Verbose("formatting: disabled")
		return nil, nil
	}

	uri := string(params.TextDocument.URI)
	b, err := fs.Content(uri)
	if err != nil {
		log.Debug("formatting: ", err.Error())
		return nil, nil
	}
	if len(b) < 1 {
		log.Debugln("formatting: zero length file")
		return nil, nil
	}

	// Use the new width-aware wrapping formatter, configured from the
	// client's FormattingOptions. We still fall through to the
	// canonical printer (via WrappingFormatter) so existing
	// regressions are picked up.
	opts := format.DefaultOptions()
	if params.Options.TabSize > 0 {
		opts.TabWidth = int(params.Options.TabSize)
	}
	if params.Options.InsertSpaces {
		opts.UseSpaces = true
	}
	// Manifest-level overrides take precedence over the client's
	// formatting options because the project owner knows best.
	if cfg := s.FmtConfig(); cfg.PrintWidth > 0 {
		opts.PrintWidth = cfg.PrintWidth
	}
	if cfg := s.FmtConfig(); cfg.TabWidth > 0 {
		opts.TabWidth = cfg.TabWidth
	}
	if cfg := s.FmtConfig(); cfg.UseSpaces {
		opts.UseSpaces = true
	}
	if cfg := s.FmtConfig(); cfg.MaxEmptyLines > 0 {
		opts.MaxEmptyLines = cfg.MaxEmptyLines
	}
	if s.serverConfig.FormatPrintWidth > 0 {
		opts.PrintWidth = s.serverConfig.FormatPrintWidth
	}

	var out bytes.Buffer
	if err := format.NewWrappingFormatter(opts).Fprint(&out, b); err != nil {
		log.Debug("formatting:", err.Error())
		return nil, nil
	}

	tree := ttcn3.ParseFile(uri)
	if tree.Err != nil {
		log.Debug("skip formatting: ", tree.Err.Error())
		return nil, nil
	}
	begin := tree.Position(0)
	end := tree.Position(len(b)) // The end position is exclusive.

	if b[len(b)-1] == '\n' {
		end.Line++
		end.Column = 1
	}

	return []protocol.TextEdit{{
		Range:   setProtocolRange(begin, end),
		NewText: out.String(),
	}}, nil
}

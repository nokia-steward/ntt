package lsp

import (
	"context"

	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// foldingRange implements textDocument/foldingRange. We fold any
// curly-brace block that spans more than a single line: modules, groups,
// blocks, struct/enum bodies, etc. The implementation is intentionally
// AST-based rather than text-based so it follows code structure even
// inside macros / preprocessor blocks.
func (s *Server) foldingRange(ctx context.Context, params *protocol.FoldingRangeParams) ([]protocol.FoldingRange, error) {
	if params == nil {
		return nil, nil
	}

	file := string(params.TextDocument.URI.SpanURI())
	tree := ttcn3.ParseFile(file)
	if tree == nil || tree.Root == nil {
		return nil, nil
	}

	var ranges []protocol.FoldingRange
	push := func(open, close syntax.Token, kind string) {
		if open == nil || close == nil {
			return
		}
		o := syntax.Begin(open)
		c := syntax.Begin(close)
		if c.Line <= o.Line {
			return
		}
		ranges = append(ranges, protocol.FoldingRange{
			StartLine:      uint32(o.Line - 1),
			StartCharacter: uint32(o.Column - 1),
			EndLine:        uint32(c.Line - 1),
			EndCharacter:   uint32(c.Column - 1),
			Kind:           kind,
		})
	}

	tree.Inspect(func(n syntax.Node) bool {
		switch v := n.(type) {
		case *syntax.Module:
			push(v.LBrace, v.RBrace, "region")
		case *syntax.GroupDecl:
			push(v.LBrace, v.RBrace, "region")
		case *syntax.BlockStmt:
			push(v.LBrace, v.RBrace, "region")
		case *syntax.CompositeLiteral:
			push(v.LBrace, v.RBrace, "")
		case *syntax.StructSpec:
			push(v.LBrace, v.RBrace, "")
		case *syntax.EnumSpec:
			push(v.LBrace, v.RBrace, "")
		case *syntax.StructTypeDecl:
			push(v.LBrace, v.RBrace, "")
		case *syntax.ImportDecl:
			push(v.LBrace, v.RBrace, "imports")
		}
		return true
	})

	return ranges, nil
}

package lsp

import (
	"context"

	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (s *Server) inlayHint(ctx context.Context, params *protocol.InlayHintParams) ([]protocol.InlayHint, error) {
	if !s.serverConfig.InlayHintEnabled {
		return nil, nil
	}

	tree := parseDocument(params.TextDocument)
	if tree == nil {
		return nil, nil
	}
	begin, end := offsets(tree, params.Range)
	return ProcessInlayHint(tree, &s.db, begin, end), nil
}

func ProcessInlayHint(tree *ttcn3.Tree, db *ttcn3.DB, begin int, end int) []protocol.InlayHint {
	var hints []protocol.InlayHint

	tree.Inspect(func(n syntax.Node) bool {
		if n == nil || n.End() < begin || end < n.Pos() {
			return false
		}

		if callExpr, ok := n.(*syntax.CallExpr); ok {

			for _, decl := range tree.LookupWithDB(callExpr.Fun, db) {

				if params := getDeclarationParams(decl.Node); params != nil {

					for idx, arg := range callExpr.Args.List {

						// Stop processing further arguments after the first assignment notation.
						// Value arguments are not allowed: ES 201 873-1, 5.4.2, Restrictions, point o).
						if binaryExpr, ok := arg.(*syntax.BinaryExpr); ok {
							if binaryExpr.Op.String() == ":=" {
								break
							}
						}

						if idx >= len(params.List) || params.List[idx].Variadic != nil {
							break
						}

						name := params.List[idx].Name.Tok.String()
						lbl := protocol.InlayHintLabelPart{Value: name + " :="}
						pos := syntax.Begin(arg)
						hint := protocol.InlayHint{
							Position:     position(pos.Line, pos.Column),
							Label:        []protocol.InlayHintLabelPart{lbl},
							Kind:         protocol.Parameter,
							PaddingRight: true,
						}
						hints = append(hints, hint)

					}

					// Stop after the first declaration is processed.
					break
				}
			}
		}
		return true
	})
	return hints
}

func getDeclarationParams(node syntax.Node) *syntax.FormalPars {
	if decl, ok := node.(*syntax.FuncDecl); ok {
		return decl.Params
	}
	if decl, ok := node.(*syntax.TemplateDecl); ok {
		return decl.Params
	}
	return nil
}

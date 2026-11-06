package lsp

import (
	"bytes"
	"context"
	"fmt"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// signatureHelp implements textDocument/signatureHelp.
//
// We resolve the cursor to its enclosing CallExpr or TemplateDecl call
// (using the existing lookup machinery in ttcn3) and synthesise a
// SignatureInformation from the callee's FormalPars. The active parameter
// is the index of the argument that contains the cursor.
//
// We deliberately keep the implementation small: it does not yet handle
// overload resolution (no two TTCN-3 callees share a name and signature in
// practice), and it does not yet support struct field signature help
// (planned for a later iteration).
func (s *Server) signatureHelp(ctx context.Context, params *protocol.SignatureHelpParams) (*protocol.SignatureHelp, error) {
	if params == nil {
		return nil, nil
	}

	file := string(params.TextDocument.URI.SpanURI())
	tree := ttcn3.ParseFile(file)
	if tree == nil || tree.Root == nil {
		return nil, nil
	}

	line := int(params.Position.Line) + 1
	col := int(params.Position.Character) + 1
	pos := tree.PosFor(line, col)
	if pos < 0 {
		return nil, nil
	}

	// Find the innermost CallExpr / ParenExpr containing the cursor.
	call, paren := callContext(tree, pos)
	if call == nil || paren == nil {
		return nil, nil
	}

	// Find the callee declaration. We can reuse the existing finder via
	// Tree.LookupWithDB; it already resolves identifiers, imports, and
	// the like.
	candidates := tree.LookupWithDB(call.Fun, &s.db)
	if len(candidates) == 0 {
		return nil, nil
	}

	for _, def := range candidates {
		formal := getDeclarationParams(def.Node)
		if formal == nil {
			continue
		}

		label, paramRanges := buildSignatureLabel(def, formal)
		paramInfos := make([]protocol.ParameterInformation, 0, len(paramRanges))
		for _, pr := range paramRanges {
			// The generated protocol bindings type Label as a plain
			// string, so we use the substring form instead of byte
			// offsets. This is still a substring of the signature
			// label and editors will highlight it correctly.
			paramInfos = append(paramInfos, protocol.ParameterInformation{
				Label: label[pr[0]:pr[1]],
			})
		}

		sig := protocol.SignatureInformation{
			Label:      label,
			Parameters: paramInfos,
		}
		help := &protocol.SignatureHelp{
			Signatures:      []protocol.SignatureInformation{sig},
			ActiveSignature: 0,
			ActiveParameter: uint32(activeArgIndex(paren, pos, len(paramInfos))),
		}
		return help, nil
	}

	return nil, nil
}

// callContext returns the innermost CallExpr that wraps the cursor as well
// as the ParenExpr holding its arguments. We use the ParenExpr separately
// because we need its byte range to identify the active argument.
func callContext(tree *ttcn3.Tree, pos int) (*syntax.CallExpr, *syntax.ParenExpr) {
	var (
		call  *syntax.CallExpr
		paren *syntax.ParenExpr
	)

	tree.Inspect(func(n syntax.Node) bool {
		if n == nil {
			return false
		}
		// Skip subtrees that don't contain the cursor.
		if n.Pos() > pos || pos > n.End() {
			return false
		}

		if c, ok := n.(*syntax.CallExpr); ok && c.Args != nil {
			if c.Args.Pos() <= pos && pos <= c.Args.End() {
				call = c
				paren = c.Args
			}
		}
		return true
	})

	return call, paren
}

func activeArgIndex(paren *syntax.ParenExpr, pos int, total int) int {
	if paren == nil || total == 0 {
		return 0
	}
	for i, arg := range paren.List {
		if arg == nil {
			continue
		}
		if pos <= arg.End() {
			return i
		}
	}
	last := len(paren.List)
	if last >= total {
		return total - 1
	}
	return last
}

// buildSignatureLabel renders a single-line signature from the callee
// declaration's source text. We avoid re-walking the AST and instead splice
// from the original source bytes, which preserves the user's preferred
// spacing.
func buildSignatureLabel(def *ttcn3.Node, formal *syntax.FormalPars) (string, [][2]int) {
	var (
		buf    bytes.Buffer
		ranges [][2]int
	)

	switch n := def.Node.(type) {
	case *syntax.FuncDecl:
		if n.KindTok != nil {
			fmt.Fprintf(&buf, "%s ", n.KindTok.String())
		}
		buf.WriteString(syntax.Name(n.Name))
	case *syntax.TemplateDecl:
		buf.WriteString("template ")
		buf.WriteString(syntax.Name(n.Name))
	case *syntax.SignatureDecl:
		buf.WriteString("signature ")
		buf.WriteString(syntax.Name(n.Name))
	default:
		buf.WriteString(syntax.Name(def.Node))
	}

	buf.WriteByte('(')
	src := fileBytes(def.Filename())
	for i, p := range formal.List {
		if p == nil {
			continue
		}
		if i > 0 {
			buf.WriteString(", ")
		}
		begin := buf.Len()
		if src != nil {
			buf.Write(src[p.Pos():p.End()])
		} else {
			buf.WriteString(syntax.Name(p.Name))
		}
		end := buf.Len()
		ranges = append(ranges, [2]int{begin, end})
	}
	buf.WriteByte(')')

	if fn, ok := def.Node.(*syntax.FuncDecl); ok && fn.Return != nil {
		buf.WriteString(" return ")
		if fn.Return.Type != nil {
			buf.WriteString(syntax.Name(fn.Return.Type))
		}
	}
	return buf.String(), ranges
}

func fileBytes(filename string) []byte {
	if filename == "" {
		return nil
	}
	b, err := fs.Content(filename)
	if err != nil {
		return nil
	}
	return b
}

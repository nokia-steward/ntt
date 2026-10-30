package lsp

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/log"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// removeDuplicateNodes
// NOTE: this function is a workaround for a scoping problem which
// occurs inside `tree.LookupWithDB`.
// following test is provoking the situation:
// TestPlainTextHoverForPortDefFromDecl (hover_test.go)
func removeDuplicateNodes(nodes []*ttcn3.Node) []*ttcn3.Node {
	uNodes := make(map[*syntax.Ident]bool, len(nodes))
	res := make([]*ttcn3.Node, 0, len(nodes))
	for _, n := range nodes {
		if _, ok := uNodes[n.Ident]; !ok {
			uNodes[n.Ident] = true
			res = append(res, n)
		}
	}
	return res
}

func (md *MarkdownHover) Print(sign string, comment string, posRef string) protocol.MarkupContent {
	// make line breaks conform to markdown spec
	comment = strings.ReplaceAll(comment, "\n", "  \n")
	res := "```ttcn3\n" + sign + "\n```\n"
	if len(comment) > 0 {
		res += " - - -\n" + comment
	}
	if len(md.mapOrConnectLinks) > 0 {
		res += "_possible map / connect statements_\n - - -\n" + md.mapOrConnectLinks
	}
	if len(posRef) > 0 {
		res += "\n - - -\n" + posRef
	}
	return protocol.MarkupContent{Kind: "markdown", Value: res}
}

func (md *MarkdownHover) LinkForNode(def *ttcn3.Node) string {
	p := syntax.Begin(def.Node)
	return fmt.Sprintf("[module %s](%s#L%dC%d)", def.ModuleOf(def.Node).Name.String(), def.Filename(), p.Line, p.Column)
}

func (md *MarkdownHover) GatherMapOrConnectLinks(fileName string, line uint32, uri protocol.DocumentURI) {
	md.mapOrConnectLinks += fmt.Sprintf("[%s:%d](%s#L%d)  \n", fileName, line+1, uri, line+1)
}

func (md *MarkdownHover) Reset() {
	md.mapOrConnectLinks = ""
}

func (pt *PlainTextHover) Print(sign string, comment string, posRef string) protocol.MarkupContent {
	val := sign
	if len(val) > 0 && val[len(val)-1] != '\n' {
		val += "\n"
	}
	if len(comment) > 0 {
		val += "__________\n" + comment

		if val[len(val)-1] != '\n' {
			val += "\n"
		}
	}
	if len(pt.mapOrConnectRefs) > 0 {
		val += "possible map / connect statements\n_________________________________\n" + pt.mapOrConnectRefs
	}
	if len(posRef) > 0 {
		val += strings.Repeat("_", len(posRef)+2) + "\n" + posRef
	}
	return protocol.MarkupContent{Kind: "plaintext", Value: val}
}

func (pt *PlainTextHover) LinkForNode(def *ttcn3.Node) string {
	return fmt.Sprintf("[module %s]", def.ModuleOf(def.Node).Name.String())
}

func (pt *PlainTextHover) GatherMapOrConnectLinks(fileName string, line uint32, uri protocol.DocumentURI) {
	pt.mapOrConnectRefs += fmt.Sprintf("%s:%d\n", fileName, line+1)
}

func (pt *PlainTextHover) Reset() {
	pt.mapOrConnectRefs = ""
}

func getSignature(def *ttcn3.Node) string {
	var sig bytes.Buffer
	var prefix = ""
	fh := fs.Open(def.Filename())
	content, _ := fh.Bytes()
	switch node := def.Node.(type) {
	case *syntax.FuncDecl:
		if tok := node.KindTok; tok != nil {
			sig.WriteString(node.KindTok.String() + " ")
		}
		sig.WriteString(node.Name.String())
		sig.Write(content[node.Params.Pos():node.Params.End()])
		if node.RunsOn != nil {
			sig.WriteString("\n  ")
			sig.Write(content[node.RunsOn.Pos():node.RunsOn.End()])
		}
		if node.System != nil {
			sig.WriteString("\n  ")
			sig.Write(content[node.System.Pos():node.System.End()])
		}
		if node.Return != nil {
			sig.WriteString("\n  ")
			sig.Write(content[node.Return.Pos():node.Return.End()])
		}
	case *syntax.ClassTypeDecl:
		if node.LBrace != nil {
			sig.Write(content[node.Pos() : node.LBrace.Pos()-1])
		}
	case *syntax.ValueDecl, *syntax.TemplateDecl, *syntax.FormalPar, *syntax.StructTypeDecl, *syntax.MapTypeDecl, *syntax.ComponentTypeDecl, *syntax.EnumTypeDecl, *syntax.PortTypeDecl:
		sig.Write(content[def.Node.Pos():def.Node.End()])
	case *syntax.Field:
		if parent := def.ParentOf(node); parent != nil {
			if _, ok := parent.(*syntax.SubTypeDecl); ok {
				prefix = "type "
			}
		}
		sig.Write(content[def.Node.Pos():def.Node.End()])
	case *syntax.Module:
		fmt.Fprintf(&sig, "module %s\n", node.Name)
	default:
		log.Debugf("getSignature: unknown Type:%T\n", node)
	}
	return prefix + sig.String()
}

func (s *Server) hover(ctx context.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	return ProcessHover(params, &s.db, s.clientCapability.HoverContent)
}

func ProcessHover(params *protocol.HoverParams, db *ttcn3.DB, capability HoverContentProvider) (*protocol.Hover, error) {
	var (
		file      = string(params.TextDocument.URI.SpanURI())
		line      = int(params.Position.Line) + 1
		col       = int(params.Position.Character) + 1
		comment   string
		signature string
		posRef    string
		defFound  = false
	)

	tree := ttcn3.ParseFile(file)
	x := tree.IdentifierAt(line, col)
	if x == nil {
		return nil, nil
	}

	for _, def := range removeDuplicateNodes(tree.LookupWithDB(x, db)) {
		defFound = true
		comment = syntax.Doc(def.Node)
		signature = getSignature(def)
		if tree.Root != def.Root {
			posRef = capability.LinkForNode(def)
		}

		if firstTok := def.Node.FirstTok(); firstTok == nil {
			continue
		} else {
			if node, ok := def.Node.(*syntax.ValueDecl); ok {
				if tok := node.KindTok; tok != nil && tok.Kind() == syntax.PORT {
					locations := FindMapConnectStatementForPortIdMatchingTheName(db, syntax.Name(x))
					for _, l := range locations {
						capability.GatherMapOrConnectLinks(l.URI.SpanURI().Filename(), l.Range.Start.Line, l.URI)
					}
				}
			}
		}
	}
	if !defFound {
		// look for predefined functions
		if id := syntax.Name(x); len(id) > 0 {
			for _, predef := range PredefinedFunctions {
				if predef.Label == id+"(...)" {
					comment = predef.Documentation
					signature = predef.Signature
				}
			}
		}
	}

	hoverContents := capability.Print(signature, comment, posRef)
	hover := &protocol.Hover{Contents: hoverContents}
	capability.Reset()
	return hover, nil
}

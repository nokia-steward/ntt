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

func (md *MarkdownHover) Print(sign string, comment string, posRef string, mapOrConnect []protocol.Location) protocol.MarkupContent {
	// make line breaks conform to markdown spec
	comment = strings.ReplaceAll(comment, "\n", "  \n")
	res := "```ttcn3\n" + sign + "\n```\n"
	if len(comment) > 0 {
		res += " - - -\n" + comment
	}
	if len(mapOrConnect) > 0 {
		res += "_possible map / connect statements_\n - - -\n"
		for _, l := range mapOrConnect {
			line := l.Range.Start.Line + 1
			res += fmt.Sprintf("[%s:%d](%s#L%d)  \n", l.URI.SpanURI().Filename(), line, l.URI, line)
		}
	}
	if len(posRef) > 0 {
		res += "\n - - -\n" + posRef
	}
	return protocol.MarkupContent{Kind: "markdown", Value: res}
}

func (md *MarkdownHover) LinkForNode(def *ttcn3.Node) string {
	p := syntax.Begin(def.Node)
	return fmt.Sprintf("[%s](%s#L%dC%d)", moduleLabel(def), def.Filename(), p.Line, p.Column)
}

func (pt *PlainTextHover) Print(sign string, comment string, posRef string, mapOrConnect []protocol.Location) protocol.MarkupContent {
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
	if len(mapOrConnect) > 0 {
		val += "possible map / connect statements\n_________________________________\n"
		for _, l := range mapOrConnect {
			val += fmt.Sprintf("%s:%d\n", l.URI.SpanURI().Filename(), l.Range.Start.Line+1)
		}
	}
	if len(posRef) > 0 {
		val += strings.Repeat("_", len(posRef)+2) + "\n" + posRef
	}
	return protocol.MarkupContent{Kind: "plaintext", Value: val}
}

func (pt *PlainTextHover) LinkForNode(def *ttcn3.Node) string {
	return fmt.Sprintf("[%s]", moduleLabel(def))
}

// moduleLabel returns "module <name>" for the module enclosing def, or the
// file name of def if there is none.
func moduleLabel(def *ttcn3.Node) string {
	if mod := def.ModuleOf(def.Node); mod != nil && mod.Name != nil {
		return "module " + mod.Name.String()
	}
	return def.Filename()
}

func getSignature(def *ttcn3.Node) string {
	var sig bytes.Buffer
	var prefix = ""
	content, err := fs.Open(def.Filename()).Bytes()
	if err != nil {
		log.Debugf("getSignature: %s\n", err)
		return ""
	}
	switch node := def.Node.(type) {
	case *syntax.FuncDecl:
		if node.KindTok != nil {
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

func isPortDecl(n syntax.Node) bool {
	decl, ok := n.(*syntax.ValueDecl)
	return ok && decl.KindTok != nil && decl.KindTok.Kind() == syntax.PORT
}

func (s *Server) hover(ctx context.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	return ProcessHover(params, &s.db, s.clientCapability.HoverContent)
}

func ProcessHover(params *protocol.HoverParams, db *ttcn3.DB, capability HoverContentProvider) (*protocol.Hover, error) {
	tree, x := identifierAt(params.TextDocument, params.Position)
	if x == nil {
		return nil, nil
	}

	var (
		comment      string
		signature    string
		posRef       string
		mapOrConnect []protocol.Location
	)
	if defs := tree.LookupWithDB(x, db); len(defs) > 0 {
		def := defs[0]
		comment = syntax.Doc(def.Node)
		signature = getSignature(def)
		if tree.Root != def.Root {
			posRef = capability.LinkForNode(def)
		}
		if isPortDecl(def.Node) {
			mapOrConnect = FindMapConnectStatementForPortIdMatchingTheName(db, syntax.Name(x))
		}
	} else if id := syntax.Name(x); len(id) > 0 {
		for _, predef := range PredefinedFunctions {
			if predef.Label == id+"(...)" {
				comment = predef.Documentation
				signature = predef.Signature
				break
			}
		}
	}

	return &protocol.Hover{Contents: capability.Print(signature, comment, posRef, mapOrConnect)}, nil
}

package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"html"
	"os"
	"strings"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func dot(n syntax.Node) {
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()

	_, _ = w.WriteString(`digraph {
	rankdir=LR
`)
	q := []syntax.Node{n}
	toks := []syntax.Token{}

	for len(q) > 0 {
		n := q[0]
		q = q[1:]
		if syntax.IsNil(n) {
			continue
		}
		if tok, ok := n.(syntax.Token); ok {
			toks = append(toks, tok)
			continue
		}
		fmt.Fprintf(w, "\t%s %s;\n", nodeID(n), nodeProps(n))
		for _, child := range n.Children() {
			if !syntax.IsNil(child) {
				fmt.Fprintf(w, "\t%s -> %s;\n", nodeID(n), nodeID(child))
				q = append(q, child)
			}
		}
	}

	_, _ = w.WriteString("	{ \n")
	for _, tok := range toks {
		fmt.Fprintf(w, "\t%s %s;\n", nodeID(tok), nodeProps(tok))
	}
	_, _ = w.WriteString("	}")
	_, _ = w.WriteString("}")
}

func nodeID(n syntax.Node) string {
	if tok, ok := n.(syntax.Token); ok {
		return fmt.Sprintf("t%d", tok.Pos())
	}
	return fmt.Sprintf("n%p%s", n, base64.RawStdEncoding.EncodeToString([]byte(fmt.Sprintf("%T", n))))
}

func nodeProps(n syntax.Node) string {
	if tok, ok := n.(syntax.Token); ok {
		label := fmt.Sprintf("%v", tok.Kind())
		if tok.Kind().IsLiteral() {
			label = tok.String()
		}
		return fmt.Sprintf("[label=<<B>%s</B>>; shape=box; style=filled; fillcolor=lightgrey]", escape(label))
	}
	label := strings.TrimPrefix(fmt.Sprintf("%T", n), "*syntax.")
	return fmt.Sprintf("[label=\"%s\"]", label)
}

func escape(s string) string {
	s = html.EscapeString(s)
	s = strings.Replace(s, "[", " [", -1)
	s = strings.Replace(s, "|", " |", -1)
	s = strings.Replace(s, "]", " ]", -1)
	return s
}

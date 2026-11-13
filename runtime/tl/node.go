package tl

import (
	"encoding/xml"
	"io"
	"strings"
)

// NS is one of the TCI-TL XML schemas. Every element belongs to the
// namespace of the schema that declares it (all Annex B schemas are
// elementFormDefault="qualified"), which is not always the namespace of its
// type: an event's parameter elements are declared by the Events schema even
// when their type comes from Types or Values.
type NS uint8

const (
	NSLog NS = iota
	NSEvents
	NSTypes
	NSValues
	NSTemplates
)

// Namespace URIs and the prefixes the XML writer declares for them, from
// ES 201 873-6 Annex B.
var namespaces = [...]struct{ Prefix, URI string }{
	NSLog:       {"tli", "http://uri.etsi.org/ttcn-3/tci/TLI_v4_10_1.xsd"},
	NSEvents:    {"ev", "http://uri.etsi.org/ttcn-3/tci/Events_v4_10_1.xsd"},
	NSTypes:     {"ty", "http://uri.etsi.org/ttcn-3/tci/Types_v4_10_1.xsd"},
	NSValues:    {"va", "http://uri.etsi.org/ttcn-3/tci/Values_v4_10_1.xsd"},
	NSTemplates: {"te", "http://uri.etsi.org/ttcn-3/tci/Templates_v4_10_1.xsd"},
}

// URI returns the namespace URI of ns.
func (ns NS) URI() string { return namespaces[ns].URI }

// Attr is an XML attribute.
type Attr struct {
	Name, Value string
}

// Node is an XML element of a TCI-TL log.
type Node struct {
	NS    NS
	Tag   string
	Attrs []Attr
	Text  string
	Kids  []*Node
}

// Content is what goes inside an element whose name the caller supplies:
// an event parameter's content is built by a constructor that knows the
// parameter's schema type, and wrapped in an element named after the
// parameter by the encoder.
type Content struct {
	Attrs []Attr
	Text  string
	Kids  []*Node
}

func (c Content) in(ns NS, tag string) *Node {
	return &Node{NS: ns, Tag: tag, Attrs: c.Attrs, Text: c.Text, Kids: c.Kids}
}

func el(ns NS, tag string, kids ...*Node) *Node {
	return &Node{NS: ns, Tag: tag, Kids: kids}
}

func text(ns NS, tag, s string) *Node {
	return &Node{NS: ns, Tag: tag, Text: s}
}

// writeXML writes n and its subtree with namespace prefixes.
func (n *Node) writeXML(w io.Writer) error {
	var b strings.Builder
	n.appendXML(&b)
	_, err := io.WriteString(w, b.String())
	return err
}

func (n *Node) appendXML(b *strings.Builder) {
	name := namespaces[n.NS].Prefix + ":" + n.Tag
	b.WriteByte('<')
	b.WriteString(name)
	for _, a := range n.Attrs {
		b.WriteByte(' ')
		b.WriteString(a.Name)
		b.WriteString(`="`)
		escape(b, a.Value)
		b.WriteByte('"')
	}
	if n.Text == "" && len(n.Kids) == 0 {
		b.WriteString("/>")
		return
	}
	b.WriteByte('>')
	escape(b, n.Text)
	for _, k := range n.Kids {
		k.appendXML(b)
	}
	b.WriteString("</")
	b.WriteString(name)
	b.WriteByte('>')
}

func escape(b *strings.Builder, s string) {
	var sb strings.Builder
	_ = xml.EscapeText(&sb, []byte(s))
	b.WriteString(sb.String())
}

// jsonNode is the JSON Lines form of a Node: the same tree, one object per
// element, so a log converts between the two formats (XML cannot carry the
// characters XML 1.0 excludes; see doc.go). Namespaces
// are left implicit; Annex B fixes each element's namespace by its position.
type jsonNode struct {
	Tag   string            `json:"tag"`
	Attrs map[string]string `json:"attrs,omitempty"`
	Text  string            `json:"text,omitempty"`
	Kids  []*jsonNode       `json:"kids,omitempty"`
}

func (n *Node) toJSON() *jsonNode {
	j := &jsonNode{Tag: n.Tag, Text: n.Text}
	if len(n.Attrs) > 0 {
		j.Attrs = make(map[string]string, len(n.Attrs))
		for _, a := range n.Attrs {
			j.Attrs[a.Name] = a.Value
		}
	}
	for _, k := range n.Kids {
		j.Kids = append(j.Kids, k.toJSON())
	}
	return j
}

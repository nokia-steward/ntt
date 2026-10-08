package tl

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// Log is a TCI-TL log read back: its header and its events, each the
// element tree it was written as. Namespaces are dropped; Annex B fixes
// each element's by its position.
type Log struct {
	Header *Node
	Events []*Node

	// XML is set for a log read from the XML format, which cannot carry
	// every character (see Compare).
	XML bool
	// Truncated is set for a log that ends mid-event, as the log of a
	// killed run does: an XML log without its closing tags, or a JSON
	// Lines log whose last line is cut short. Events holds the complete
	// events.
	Truncated bool
}

// ReadLog reads a log in either format NewXMLWriter and NewJSONLWriter
// write, telling them apart by the first character.
func ReadLog(r io.Reader) (*Log, error) {
	br := bufio.NewReader(r)
	// A UTF-8 byte order mark is not part of the log.
	if b, err := br.Peek(3); err == nil && bytes.Equal(b, []byte{0xEF, 0xBB, 0xBF}) {
		_, _ = br.Discard(3)
	}
	for {
		b, err := br.Peek(1)
		if err != nil {
			return nil, fmt.Errorf("tl: empty log")
		}
		switch b[0] {
		case ' ', '\t', '\r', '\n':
			_, _ = br.ReadByte()
			continue
		case '<':
			return readXML(br)
		case '{':
			return readJSONL(br)
		}
		return nil, fmt.Errorf("tl: not a TCI-TL log (starts with %q)", b[0])
	}
}

func readJSONL(r *bufio.Reader) (*Log, error) {
	l := &Log{}
	line := 0
	// A line that does not parse is an error, unless it is the last one:
	// the log of a killed run ends in a line cut short. Lines are as long
	// as their event: a value may be megabytes.
	var pending error
	for {
		b, rerr := r.ReadBytes('\n')
		if rerr != nil && rerr != io.EOF {
			return nil, fmt.Errorf("tl: line %d: %w", line+1, rerr)
		}
		if len(b) == 0 && rerr == io.EOF {
			break
		}
		line++
		if len(bytes.TrimSpace(b)) == 0 {
			if rerr == io.EOF {
				break
			}
			continue
		}
		if pending != nil {
			return nil, pending
		}
		var j jsonNode
		if err := json.Unmarshal(b, &j); err != nil {
			pending = fmt.Errorf("tl: line %d: %w", line, err)
			if rerr == io.EOF {
				break
			}
			continue
		}
		n, err := j.node()
		if err != nil {
			return nil, fmt.Errorf("tl: line %d: %w", line, err)
		}
		if l.Header == nil {
			if n.Tag != "header" {
				return nil, fmt.Errorf("tl: line %d: a TCI-TL log begins with its header, not %q", line, n.Tag)
			}
			l.Header = n
			continue
		}
		l.Events = append(l.Events, n)
		if rerr == io.EOF {
			break
		}
	}
	if l.Header == nil {
		if pending != nil {
			return nil, pending
		}
		return nil, fmt.Errorf("tl: empty log")
	}
	l.Truncated = pending != nil
	return l, nil
}

func (j *jsonNode) node() (*Node, error) {
	if j == nil {
		return nil, fmt.Errorf("an element is null")
	}
	if j.Tag == "" {
		return nil, fmt.Errorf("an element has no tag")
	}
	n := &Node{Tag: j.Tag, Text: j.Text}
	for k, v := range j.Attrs {
		n.Attrs = append(n.Attrs, Attr{k, v})
	}
	for _, k := range j.Kids {
		kn, err := k.node()
		if err != nil {
			return nil, err
		}
		n.Kids = append(n.Kids, kn)
	}
	return n, nil
}

func readXML(r io.Reader) (*Log, error) {
	er := &eofReader{r: r}
	d := xml.NewDecoder(er)
	var stack []*Node
	var root *Node
	truncated := false
	for {
		tok, err := d.Token()
		if err == io.EOF {
			if len(stack) > 0 {
				truncated = true
			}
			break
		}
		if err != nil {
			// A log cut off by a killed run ends inside an element — or
			// inside a character, which the decoder reports as invalid
			// UTF-8 — so an error once all the input is read, after the
			// log began, is the log's end.
			if se, ok := err.(*xml.SyntaxError); ok && root != nil && len(stack) > 0 &&
				(strings.Contains(se.Msg, "unexpected EOF") || er.eof && strings.Contains(se.Msg, "invalid UTF-8")) {
				truncated = true
				break
			}
			return nil, fmt.Errorf("tl: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if root != nil && len(stack) == 0 {
				return nil, fmt.Errorf("tl: content after the logfile element")
			}
			n := &Node{Tag: t.Name.Local}
			for _, a := range t.Attr {
				if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" {
					continue
				}
				n.Attrs = append(n.Attrs, Attr{a.Name.Local, a.Value})
			}
			if len(stack) > 0 {
				p := stack[len(stack)-1]
				p.Kids = append(p.Kids, n)
			} else {
				root = n
			}
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].Text += string(t)
			}
		}
	}
	if root == nil || root.Tag != "logfile" {
		return nil, fmt.Errorf("tl: no logfile element")
	}
	l := &Log{XML: true, Truncated: truncated}
	for _, k := range root.Kids {
		switch k.Tag {
		case "header":
			l.Header = k
		case "body":
			l.Events = k.Kids
		}
	}
	// In a truncated log the event being written when the run was killed
	// is still open: stack[0] is the logfile, stack[1] the body, stack[2]
	// that event.
	if truncated && len(stack) > 2 && len(l.Events) > 0 && l.Events[len(l.Events)-1] == stack[2] {
		l.Events = l.Events[:len(l.Events)-1]
	}
	// Element content in the written log is either text or elements, not
	// both; whitespace between elements is layout.
	for _, e := range l.Events {
		trimLayout(e)
	}
	return l, nil
}

// eofReader records that its reader reached the end of the input.
type eofReader struct {
	r   io.Reader
	eof bool
}

func (e *eofReader) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	if err == io.EOF {
		e.eof = true
	}
	return n, err
}

func trimLayout(n *Node) {
	if len(n.Kids) > 0 {
		n.Text = strings.TrimSpace(n.Text)
	}
	for _, k := range n.Kids {
		trimLayout(k)
	}
}

// Attr returns the value of attribute name, or "".
func (n *Node) Attr(name string) string {
	for _, a := range n.Attrs {
		if a.Name == name {
			return a.Value
		}
	}
	return ""
}

// Kid returns the first child element named tag, or nil.
func (n *Node) Kid(tag string) *Node {
	for _, k := range n.Kids {
		if k.Tag == tag {
			return k
		}
	}
	return nil
}

package syntax

import (
	"fmt"
	"reflect"
	"sort"
)

// EventKind describes the kind of a serialization event.
type EventKind uint8

const (
	TokenEvent EventKind = iota // A token, including comments, commas and semicolons
	OpenEvent                   // Begin of a syntax node
	CloseEvent                  // End of a syntax node
)

func (k EventKind) String() string {
	switch k {
	case TokenEvent:
		return "Token"
	case OpenEvent:
		return "Open"
	case CloseEvent:
		return "Close"
	default:
		return fmt.Sprintf("EventKind(%d)", k)
	}
}

// Event is a single element of a serialized syntax tree.
type Event struct {
	Kind EventKind

	// Node is the opened or closed syntax node for OpenEvent and
	// CloseEvent. For TokenEvent Node is the token.
	Node Node
}

// Token returns the token of a TokenEvent and nil otherwise.
func (e Event) Token() Token {
	if e.Kind != TokenEvent {
		return nil
	}
	tok, _ := e.Node.(Token)
	return tok
}

func (e Event) String() string {
	if tok := e.Token(); tok != nil {
		return fmt.Sprintf("%s %s %q", e.Kind, tok.Kind(), tok.String())
	}
	if IsNil(e.Node) {
		return e.Kind.String()
	}
	return fmt.Sprintf("%s %s", e.Kind, reflect.Indirect(reflect.ValueOf(e.Node)).Type().Name())
}

// Serialize flattens the syntax tree n into a slice of open-, close- and
// token-events in source order.
//
// Tokens not referenced by the syntax tree, such as comments, commas,
// semicolons, preprocessor directives and tokens skipped during error
// recovery, are included as well. They are emitted before the open-event of
// the node owning the following token, so separators and comments between
// two nodes belong to the innermost node enclosing both. When n is a *Root,
// leading and trailing comments of the file are included too; otherwise only
// tokens between the first and the last token of n are serialized.
//
// Every token is emitted exactly once: tokens referenced by multiple nodes,
// which may happen during error recovery, are only emitted for the first
// node. The EOF token is never emitted.
func Serialize(n Node) []Event {
	if IsNil(n) {
		return nil
	}
	s := serializer{next: -1}
	if r, ok := n.(*Root); ok {
		s.root = r
		s.next = 0
		s.events = make([]Event, 0, 2*len(r.tokens))
	}
	s.node(n)
	s.open()
	return s.events
}

type serializer struct {
	root *Root
	next int // index of the next unserialized token in root.tokens

	// pending holds the events of nodes which have not emitted a token
	// yet. They are deferred until the first token, so that preceding
	// comments and separators do not end up inside them.
	pending []Event
	gen     int // incremented whenever pending events are emitted

	events []Event
}

func (s *serializer) node(n Node) {
	if tok, ok := n.(*tokenNode); ok {
		s.token(tok)
		return
	}

	// FirstTok is not reliable for locating the first token of a node,
	// because it is derived from field order and not source order (e.g.
	// postfix ifpresent).
	if r, ok := n.(*Root); ok && r == s.root {
		s.events = append(s.events, Event{Kind: OpenEvent, Node: n})
		for _, c := range sortedChildren(n) {
			s.node(c)
		}
		s.open()
		s.flush(len(r.tokens))
		s.events = append(s.events, Event{Kind: CloseEvent, Node: n})
		return
	}

	gen := s.gen
	s.pending = append(s.pending, Event{Kind: OpenEvent, Node: n})
	for _, c := range sortedChildren(n) {
		s.node(c)
	}
	if gen == s.gen {
		s.pending = append(s.pending, Event{Kind: CloseEvent, Node: n})
	} else {
		s.open()
		s.events = append(s.events, Event{Kind: CloseEvent, Node: n})
	}
}

func (s *serializer) token(tok *tokenNode) {
	if tok.Kind() == EOF {
		return
	}
	if s.root == nil {
		s.root = tok.Root
		s.next = tok.idx
	}

	// Tokens shared between nodes are emitted only once, but still mark
	// the begin of the nodes referencing them.
	if tok.idx < s.next {
		s.open()
		return
	}
	s.flush(tok.idx)
	s.open()
	s.events = append(s.events, Event{Kind: TokenEvent, Node: tok})
	s.next = tok.idx + 1
}

// open emits the pending events.
func (s *serializer) open() {
	if len(s.pending) == 0 {
		return
	}
	s.events = append(s.events, s.pending...)
	s.pending = s.pending[:0]
	s.gen++
}

// flush emits all tokens up to, but excluding, token index end.
func (s *serializer) flush(end int) {
	if s.root == nil {
		return
	}
	for ; s.next < end; s.next++ {
		if s.root.tokens[s.next].Kind == EOF {
			continue
		}
		s.events = append(s.events, Event{Kind: TokenEvent, Node: &tokenNode{Root: s.root, idx: s.next}})
	}
}

// sortedChildren returns the non-nil children of n in source order. It reuses
// the slice returned by Children, which is expected to be newly allocated.
func sortedChildren(n Node) []Node {
	children := n.Children()
	i := 0
	for _, c := range children {
		if !IsNil(c) {
			children[i] = c
			i++
		}
	}
	children = children[:i]
	less := func(i, j int) bool { return children[i].Pos() < children[j].Pos() }
	if !sort.SliceIsSorted(children, less) {
		sort.SliceStable(children, less)
	}
	return children
}

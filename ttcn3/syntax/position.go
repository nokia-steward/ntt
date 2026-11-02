package syntax

import (
	"fmt"
)

// Span is the text span in the source code.
type Span struct {
	Filename   string
	Begin, End Position
}

// String returns the string representation of the text span.
func (s Span) String() string {
	ret := fmt.Sprint(s.Begin.String())
	if name := s.Filename; name != "" {
		ret = fmt.Sprintf("%s:", s.Filename) + ret
	}
	if s.Begin != s.End && s.End.IsValid() {
		ret += fmt.Sprintf("-%s", s.End.String())
	}
	return ret
}

// Position is a cursor position in a source file. Lines and columns are 1-based.
type Position struct {
	Line, Column int
}

// IsValid returns true if the position is valid.
func (pos *Position) IsValid() bool { return pos.Line > 0 }

// After returns true if the position is after other given position.
func (pos *Position) After(other Position) bool {
	return pos.Line > other.Line || pos.Line == other.Line && pos.Column > other.Column
}

// Before returns true if the position is before the other given position.
func (pos *Position) Before(other Position) bool {
	return pos.Line < other.Line || pos.Line == other.Line && pos.Column < other.Column
}

// String returns the position's string representation.
func (pos Position) String() string {
	if !pos.IsValid() {
		return "-"
	}
	s := fmt.Sprintf("%d", pos.Line)
	if pos.Column != 0 {
		s += fmt.Sprintf(":%d", pos.Column)
	}
	return s
}

func SpanOf(n Node) Span {
	return Span{
		Filename: Filename(n),
		Begin:    Begin(n),
		End:      End(n),
	}
}

func Filename(n Node) string {
	if tok := n.FirstTok(); tok != nil {
		return tok.(*tokenNode).Root.Filename
	}
	return ""
}

func Begin(n Node) Position {
	if tok := n.FirstTok(); tok != nil {
		return tok.(*tokenNode).Root.Position(tok.Pos())
	}
	return Position{}
}

func End(n Node) Position {
	if tok := n.LastTok(); tok != nil {
		return tok.(*tokenNode).Root.Position(tok.End())
	}
	return Position{}
}

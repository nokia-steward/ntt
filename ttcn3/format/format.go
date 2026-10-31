package format

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/nokia/ntt/ttcn3/syntax"
)

type whiteSpace byte

const (
	ignore   = whiteSpace(0)
	blank    = whiteSpace(' ')
	newline  = whiteSpace('\n')
	cell     = whiteSpace('|')
	indent   = whiteSpace('>')
	unindent = whiteSpace('<')
)

// Fprint formats src in canonical TTCN-3 style and writes the result to w or
// returns an (I/O or syntax) error. src is expected to be syntactically
// correct TTCN-3 source text. The formatting rules are described in the
// package documentation.
func Fprint(w io.Writer, src interface{}) error {
	return NewCanonicalPrinter(w).Fprint(src)
}

// CanonicalPrinter is a simple formatter that only fixes indentation and
// various whitespace issues. The formatting rules are described in the
// package documentation.
type CanonicalPrinter struct {
	// UseSpaces controls whether to use spaces instead of tabs for indenting.
	UseSpaces bool

	// TabWidth is the width of a tab character (equivalent to the number
	// of spaces, default is 8).
	TabWidth int

	// Indent is the level of indentation at which to start.
	Indent int

	w                io.Writer
	lastPos, currPos syntax.Position
	whiteBuf         string
	firstToken       bool
	stack            []frame
}

// frame is one open syntax node.
//
// A line break inside an object indents its continuation one level, once
// for that object. covers means the object already contributes that level,
// because its opening bracket was printed or its continuation indent was
// applied (cont).
type frame struct {
	node    syntax.Node
	started bool // a token of this object has been printed
	covers  bool
	cont    bool
}

// NewCanonicalPrinter returns a new printer that formats source code.
func NewCanonicalPrinter(w io.Writer) *CanonicalPrinter {
	return &CanonicalPrinter{
		Indent:   0,
		TabWidth: 8,
		w:        w,
	}
}

func (p *CanonicalPrinter) Fprint(v interface{}) error {
	minwidth := p.TabWidth
	twmode := tabwriter.DiscardEmptyColumns | tabwriter.StripEscape
	if !p.UseSpaces {
		minwidth = 0
		twmode |= tabwriter.TabIndent
	}
	p.w = tabwriter.NewWriter(p.w, minwidth, p.TabWidth, 1, ' ', twmode)

	b, err := toBytes(v)
	if err != nil {
		return err
	}

	// The simple formatting rules do not need much context information.
	// This allows us to use the tokeniser and release initial
	// formatting experiments even before the parser is ready.
	n := syntax.Parse(b)

	// Only pretty print if there are no syntax errors.
	if n.Err() != nil {
		return n.Err()
	}

	return p.tree(n)
}

// tree prints the syntax tree by interspersing the token stream with spacing
// information.
func (p *CanonicalPrinter) tree(n syntax.Node) error {

	// Prime the position tracker with the first token to avoid printing
	// white-spaces before the first token.
	p.firstToken = true
	p.stack = p.stack[:0]

	for _, e := range syntax.Serialize(n) {
		switch e.Kind {
		case syntax.OpenEvent:
			p.stack = append(p.stack, frame{node: e.Node})
		case syntax.CloseEvent:
			p.closeFrame()
		case syntax.TokenEvent:
			p.printToken(e.Token())
		}
	}

	// Terminate the last line with a newline.
	if !p.firstToken {
		fmt.Fprint(p.w, "\n")
	}

	if tw, ok := p.w.(*tabwriter.Writer); ok {
		return tw.Flush()
	}

	return nil
}

func (p *CanonicalPrinter) printToken(n syntax.Token) {
	// Incorporate user-defined line breaks and token separators
	// into output stream.
	currPos := syntax.SpanOf(n)
	switch {
	case currPos.Begin.Line > p.lastPos.Line:
		p.print(newline)
		if currPos.Begin.Line-p.lastPos.Line > 1 {
			p.print(newline)
		}
		// The line before the first token is not a broken line.
		if !p.firstToken {
			p.continueIndent()
		}
	case currPos.Begin.Column > p.lastPos.Column:
		p.print(blank)
	}
	p.lastPos = currPos.End

	switch k, s := n.Kind(), n.String(); {

	// Increment indentation after opening {, [, (.
	// That level is the object's one continuation indent.
	case k == syntax.LBRACE, k == syntax.LBRACK, k == syntax.LPAREN:
		p.print(s, indent)
		p.markBracket()

	// Decrement indentation before closing }, ], ).
	case k == syntax.RBRACE, k == syntax.RBRACK, k == syntax.RPAREN:
		p.print(unindent, s)

	// Add space after comma
	case k == syntax.COMMA:
		p.print(",", blank)

	// Align assignments.
	case k == syntax.ASSIGN:
		p.print(blank, ":=", blank)

	// Every line of a comment has to be indented individually.
	case k == syntax.COMMENT:
		p.print(cell)

		// Before we split a comment into its lines, we have to
		// remove the trailing newline of `//` comments.
		//
		// This makes the logic of p.comment easier, because
		// printing `//` comments is then identical to printing
		// single line `/*` comments.
		p.comment(currPos.Begin.Column-1, strings.Split(strings.TrimSpace(s), "\n"))
		if strings.HasSuffix(s, "\n") {
			p.print(newline)
		}

	// Only literals may contain newlines and \t and need to be quoted.
	case n.Kind().IsLiteral():
		p.print(quote(s))

	// All other tokens are printed as is.
	default:
		p.print(s)
	}
	p.markStarted()
}

// continueIndent indents a broken line one level for the innermost object
// that contains the break and has not already contributed an indent level.
// Further breaks inside that same object do not indent again. Nested
// objects each contribute their own level.
func (p *CanonicalPrinter) continueIndent() {
	for i := len(p.stack) - 1; i >= 0; i-- {
		f := &p.stack[i]
		if !f.started {
			continue
		}
		if f.covers {
			return
		}
		if !isContinuationObject(f.node) {
			continue
		}
		f.covers = true
		f.cont = true
		p.print(indent)
		return
	}
}

// markBracket records that the innermost open object is delimited by a
// bracket which already incremented the indent.
func (p *CanonicalPrinter) markBracket() {
	if len(p.stack) == 0 {
		return
	}
	p.stack[len(p.stack)-1].covers = true
}

func (p *CanonicalPrinter) markStarted() {
	for i := range p.stack {
		p.stack[i].started = true
	}
}

func (p *CanonicalPrinter) closeFrame() {
	if len(p.stack) == 0 {
		return
	}
	f := p.stack[len(p.stack)-1]
	p.stack = p.stack[:len(p.stack)-1]
	if f.cont {
		p.print(unindent)
	}
}

// isContinuationObject reports whether a broken line inside n is indented.
// Bracket-delimited objects get that level from {, [, or ( instead.
func isContinuationObject(n syntax.Node) bool {
	if syntax.IsNil(n) {
		return false
	}
	switch n.(type) {
	case syntax.Expr, *syntax.FormalPars, *syntax.FormalPar, *syntax.Declarator, *syntax.ReturnStmt:
		return true
	default:
		return false
	}
}

// comment prints a comment line by line. It removes white-space prefixes from
// multi-line comments, so they can be properly indented.
func (p *CanonicalPrinter) comment(firstLineIndent int, lines []string) {
	minNrOfWs := findLeastIndentation(firstLineIndent, lines)
	line, lines := lines[0], lines[1:]
	// first line shall alway get aligned to the current indentation level
	// or one space after the previous non-white space character
	p.print(quote(strings.TrimSpace(line)))

	if len(lines) == 0 {
		return
	}

	for _, line := range lines {
		p.print(newline)
		// omit empty lines
		if s := strings.TrimSpace(line); s != "" {
			indent := strings.Repeat(" ", len(line)-len(strings.TrimLeft(line, " "))-minNrOfWs)
			p.print(indent, quote(s))
		}
	}
}

func (p *CanonicalPrinter) print(args ...interface{}) {
	for _, arg := range args {
		switch arg := arg.(type) {
		case whiteSpace:

			switch arg {
			case ignore:
				continue
			case indent:
				p.Indent++
			case unindent:
				p.Indent--
			case blank:
				if p.whiteBuf == "" {
					p.whiteBuf = " "
				}
			case cell:
				if p.whiteBuf == " " {
					p.whiteBuf = ""
				}
				if strings.Count(p.whiteBuf, "\t") == len(p.whiteBuf) {
					p.whiteBuf += "\t"
				}
			case newline:
				if strings.Count(p.whiteBuf, "\n") != len(p.whiteBuf) {
					p.whiteBuf = ""
				}
				p.whiteBuf += "\n"
			}
		default:
			if p.whiteBuf != "" {
				p.printSpace()
				p.whiteBuf = ""
			}
			fmt.Fprint(p.w, arg)
		}

	}
}

// printSpace prints the white-space buffer and indentation.
func (p *CanonicalPrinter) printSpace() {
	if p.firstToken {
		p.firstToken = false
		return
	}
	fmt.Fprint(p.w, p.whiteBuf)
	if strings.HasSuffix(p.whiteBuf, "\n") {
		p.printIndent()
	}
}

// printIndent writes the current indentation. Tabs are escaped so a tabwriter
// cell, such as the one before a comment, does not treat them as columns.
func (p *CanonicalPrinter) printIndent() {
	if p.Indent <= 0 {
		return
	}
	if p.UseSpaces {
		fmt.Fprint(p.w, strings.Repeat(" ", p.Indent*p.TabWidth))
		return
	}
	fmt.Fprint(p.w, "\xff", strings.Repeat("\t", p.Indent), "\xff")
}

func toBytes(v interface{}) ([]byte, error) {
	switch v := v.(type) {
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	case io.Reader:
		return io.ReadAll(v)
	default:
		return nil, fmt.Errorf("printer: unsupported type %T", v)
	}
}

func quote(s string) string {
	return fmt.Sprintf("\xff%s\xff", s)
}

// findLeastIndentation returns the min amount of
// prefix white spaces for the supplied lines
func findLeastIndentation(firstLineIndent int, lines []string) int {
	min := firstLineIndent
	for _, l := range lines[1:] {
		woLeadingSpaces := strings.TrimLeft(l, " ")
		if min > len(l)-len(woLeadingSpaces) {
			if len(woLeadingSpaces) > 0 {
				// omit empty lines (or ones filled with white spaces)
				min = len(l) - len(woLeadingSpaces)
			}
		}
	}
	return min
}

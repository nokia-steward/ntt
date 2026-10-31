package format

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

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
	out              io.Writer
	src              []byte
	lastPos, currPos syntax.Position
	whiteBuf         string
	firstToken       bool
	stack            []frame

	// lineIndent is the indent level of the current output line.
	// lineInToken is set when the current line starts inside a
	// multi-line token, such as a comment or a string literal.
	lineIndent  int
	lineInToken bool

	// col is the output column of the next character. align is set when the
	// line being started aligns a formal parameter with the first one.
	col   int
	align *frame
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

	// A formal parameter list aligns later parameters with the first when
	// that one follows the opening delimiter on the same line. level is the
	// indent level of the line with the delimiter and col the column of the
	// first parameter.
	level   int
	col     int
	params  int
	aligned bool
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

	minwidth := p.TabWidth
	twmode := tabwriter.DiscardEmptyColumns | tabwriter.StripEscape
	if !p.UseSpaces {
		minwidth = 0
		twmode |= tabwriter.TabIndent
	}
	tw := tabwriter.NewWriter(p.w, minwidth, p.TabWidth, 1, ' ', twmode)
	p.out = tw
	p.src = b
	defer func(indent int) {
		p.Indent = indent
		p.out = nil
		p.src = nil
	}(p.Indent)

	p.tree(n)
	return tw.Flush()
}

// tree prints the syntax tree by interspersing the token stream with spacing
// information.
func (p *CanonicalPrinter) tree(n syntax.Node) {

	// Prime the position tracker with the first token to avoid printing
	// white-spaces before the first token.
	p.firstToken = true
	p.lastPos = syntax.Position{}
	p.whiteBuf = ""
	p.stack = p.stack[:0]
	p.lineIndent = p.Indent
	p.lineInToken = false
	p.col = 0
	p.align = nil

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
		fmt.Fprint(p.out, "\n")
	}
}

func (p *CanonicalPrinter) printToken(n syntax.Token) {
	// Incorporate user-defined line breaks and token separators
	// into output stream.
	currPos := syntax.SpanOf(n)
	fp := p.paramStart()
	switch {
	case currPos.Begin.Line > p.lastPos.Line:
		p.print(newline)
		if currPos.Begin.Line-p.lastPos.Line > 1 {
			p.print(newline)
		}
		// The line before the first token is not a broken line.
		if !p.firstToken && (fp == nil || !fp.aligned) {
			p.continueIndent(n)
		}
		p.lineInToken = false
	case currPos.Begin.Column > p.lastPos.Column:
		p.print(blank)
	}
	p.lastPos = currPos.End
	p.alignParam(fp)

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
		anchor := 0
		if !p.lineInToken {
			anchor = p.lineWidth(n.Pos())
		}
		p.comment(anchor, strings.Split(strings.TrimSpace(s), "\n"))
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
	p.markDelimiter()
	p.markStarted()

	if currPos.End.Line > currPos.Begin.Line {
		p.lineInToken = true
		p.lineIndent = 0
	}
}

// continueIndent indents a broken line one level for the innermost object
// that contains the break and has not already contributed an indent level.
// Further breaks inside that same object do not indent again. Nested
// objects each contribute their own level.
func (p *CanonicalPrinter) continueIndent(tok syntax.Token) {
	if tok.Kind() == syntax.LBRACE || p.opensBody(tok) {
		return
	}
	if p.opensClause() {
		return
	}
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

// opensBody reports whether tok is the with of an attribute list, which is
// laid out like the { of a function body.
func (p *CanonicalPrinter) opensBody(tok syntax.Token) bool {
	if len(p.stack) == 0 {
		return false
	}
	_, ok := p.stack[len(p.stack)-1].node.(*syntax.WithSpec)
	return ok && tok.Kind() == syntax.WITH
}

// opensClause indents a header clause (runs on, system, mtc, return) that
// starts on a new line. The indent ends when the clause closes.
func (p *CanonicalPrinter) opensClause() bool {
	if len(p.stack) == 0 {
		return false
	}
	f := &p.stack[len(p.stack)-1]
	if f.started {
		return false
	}
	switch f.node.(type) {
	case *syntax.RunsOnSpec, *syntax.SystemSpec, *syntax.MtcSpec, *syntax.ReturnSpec:
		f.covers = true
		f.cont = true
		p.print(indent)
		return true
	default:
		return false
	}
}

// paramStart returns the formal parameter list whose next parameter starts
// with the token about to be printed, or nil.
func (p *CanonicalPrinter) paramStart() *frame {
	for i := len(p.stack) - 1; i > 0; i-- {
		if p.stack[i].started {
			return nil
		}
		if _, ok := p.stack[i].node.(*syntax.FormalPar); ok {
			if _, ok := p.stack[i-1].node.(*syntax.FormalPars); ok && p.stack[i-1].started {
				return &p.stack[i-1]
			}
			return nil
		}
	}
	return nil
}

// alignParam remembers the column of the first parameter and prepares the
// alignment of later ones.
func (p *CanonicalPrinter) alignParam(fp *frame) {
	if fp == nil {
		return
	}
	if fp.params == 0 && !strings.ContainsAny(p.whiteBuf, "\n\t") {
		fp.col = p.col + len(p.whiteBuf)
		fp.aligned = true
	}
	fp.params++
	if fp.aligned && strings.HasSuffix(p.whiteBuf, "\n") {
		p.align = fp
	}
}

// markDelimiter records the line of the opening delimiter of a formal
// parameter list.
func (p *CanonicalPrinter) markDelimiter() {
	if len(p.stack) == 0 {
		return
	}
	if f := &p.stack[len(p.stack)-1]; !f.started {
		if _, ok := f.node.(*syntax.FormalPars); ok {
			f.level = p.lineIndent
		}
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
	case syntax.Expr, *syntax.FormalPars, *syntax.FormalPar, *syntax.Declarator, *syntax.ReturnStmt,
		*syntax.ValueDecl, *syntax.TemplateDecl:
		return true
	default:
		return false
	}
}

// comment prints a comment line by line. anchor is the source column of the
// line the comment starts on. The least of anchor and the indentation of the
// other lines is aligned to the indent level of the current output line; the
// rest of each line's indentation is kept as spaces.
func (p *CanonicalPrinter) comment(anchor int, lines []string) {
	line, lines := lines[0], lines[1:]
	// first line shall alway get aligned to the current indentation level
	// or one space after the previous non-white space character
	p.print(quote(strings.TrimSpace(line)))

	for _, line := range lines {
		if strings.TrimSpace(line) != "" && p.width(line) < anchor {
			anchor = p.width(line)
		}
	}

	indent := p.Indent
	p.Indent = p.lineIndent
	for _, line := range lines {
		p.print(newline)
		// omit empty lines
		if s := strings.TrimSpace(line); s != "" {
			p.print(strings.Repeat(" ", p.width(line)-anchor), quote(s))
		}
	}
	p.Indent = indent
}

// lineWidth returns the width of the white-space that starts the source line
// containing offset pos.
func (p *CanonicalPrinter) lineWidth(pos int) int {
	i := pos
	for i > 0 && !isLineBreak(p.src[i-1]) {
		i--
	}
	return p.width(string(p.src[i:pos]))
}

// width returns the width of the leading white-space of s. A tab advances to
// the next multiple of TabWidth.
func (p *CanonicalPrinter) width(s string) int {
	n := 0
	for _, c := range s {
		switch c {
		case '\t':
			if p.TabWidth > 0 {
				n += p.TabWidth - n%p.TabWidth
			}
		case ' ', '\r', '\v', '\f':
			n++
		default:
			return n
		}
	}
	return n
}

func isLineBreak(c byte) bool {
	return c == '\n' || c == '\v' || c == '\f'
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
			p.write(fmt.Sprint(arg))
		}

	}
}

// write prints s and tracks the output column. \xff only delimits escaped
// text and has no width.
func (p *CanonicalPrinter) write(s string) {
	fmt.Fprint(p.out, s)
	s = strings.ReplaceAll(s, "\xff", "")
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		s = s[i+1:]
		p.col = 0
	}
	p.col += utf8.RuneCountInString(s)
}

// printSpace prints the white-space buffer and indentation.
func (p *CanonicalPrinter) printSpace() {
	if p.firstToken {
		p.firstToken = false
		p.lineIndent = p.Indent
		p.col = p.Indent * p.TabWidth
		return
	}
	p.write(p.whiteBuf)
	if strings.HasSuffix(p.whiteBuf, "\n") {
		if a := p.align; a != nil {
			p.align = nil
			p.lineIndent = a.level
			p.printAligned(a.level, a.col)
			return
		}
		p.lineIndent = p.Indent
		p.printIndent()
	}
}

// printAligned writes level indents followed by spaces up to column col.
func (p *CanonicalPrinter) printAligned(level, col int) {
	if min := level * p.TabWidth; col < min {
		col = min
	}
	if p.UseSpaces {
		p.write(strings.Repeat(" ", col))
		return
	}
	p.write("\xff" + strings.Repeat("\t", level) + strings.Repeat(" ", col-level*p.TabWidth) + "\xff")
	p.col = col
}

// printIndent writes the current indentation. Tabs are escaped so a tabwriter
// cell, such as the one before a comment, does not treat them as columns.
func (p *CanonicalPrinter) printIndent() {
	if p.Indent <= 0 {
		return
	}
	if p.UseSpaces {
		p.write(strings.Repeat(" ", p.Indent*p.TabWidth))
		return
	}
	p.write("\xff" + strings.Repeat("\t", p.Indent) + "\xff")
	p.col += p.Indent * (p.TabWidth - 1)
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

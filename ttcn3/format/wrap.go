package format

import (
	"bytes"
	"io"
	"regexp"
	"strings"
)

// WrappingFormatter is a width-aware companion to CanonicalPrinter. It
// post-processes the canonical output to break selected constructs
// across multiple lines when they would exceed Options.PrintWidth.
//
// The implementation intentionally operates on the already-canonical
// text rather than on the AST: the canonical printer guarantees a
// stable token spacing, so a small regexp-driven pass is enough to
// reflow function-parameter lists and composite literals. Building a
// fully AST-aware printer is tracked in the longer-term plan; this
// keeps the LSP and CLI honest about line width today.
type WrappingFormatter struct {
	Options Options
}

// NewWrappingFormatter returns a formatter configured with opts. If opts
// is the zero value DefaultOptions() is used.
func NewWrappingFormatter(opts Options) *WrappingFormatter {
	if opts == (Options{}) {
		opts = DefaultOptions()
	}
	return &WrappingFormatter{Options: opts}
}

// Fprint formats src and writes the result to w. src must be one of the
// types CanonicalPrinter.Fprint accepts ([]byte, string or io.Reader).
func (f *WrappingFormatter) Fprint(w io.Writer, src interface{}) error {
	var buf bytes.Buffer
	cp := NewCanonicalPrinter(&buf)
	cp.TabWidth = f.Options.TabWidth
	cp.UseSpaces = f.Options.UseSpaces
	if err := cp.Fprint(src); err != nil {
		return err
	}
	out := f.wrapLines(buf.String())
	out = f.collapseEmptyLines(out)
	_, err := io.WriteString(w, out)
	return err
}

// wrapLines breaks lines that exceed PrintWidth at the most plausible
// split points - currently top-level commas inside parameter lists and
// composite literals. The function preserves the original indentation
// and adds one level of nesting for the wrapped continuations.
func (f *WrappingFormatter) wrapLines(in string) string {
	if f.Options.PrintWidth <= 0 {
		return in
	}
	var b strings.Builder
	scanner := strings.Split(in, "\n")
	for i, line := range scanner {
		if i > 0 {
			b.WriteByte('\n')
		}
		if f.visualLength(line) <= f.Options.PrintWidth {
			b.WriteString(line)
			continue
		}
		wrapped := f.wrapOneLine(line)
		b.WriteString(wrapped)
	}
	return b.String()
}

// wrapOneLine tries to find a balanced opener / closer pair on the line
// and reflows the contents across multiple lines. We only handle the
// outermost pair to keep the algorithm linear; nested wraps happen on a
// subsequent pass if needed.
func (f *WrappingFormatter) wrapOneLine(line string) string {
	openIdx, closeIdx := outerBracketPair(line)
	if openIdx < 0 || closeIdx <= openIdx+1 {
		return line
	}
	prefix := line[:openIdx+1]
	inner := line[openIdx+1 : closeIdx]
	suffix := line[closeIdx:]

	// Don't wrap if the inner content has no top-level commas - we'd
	// just turn one long line into another long line with extra
	// noise.
	parts := splitTopLevel(inner, ',')
	if len(parts) <= 1 {
		return line
	}

	baseIndent := leadingIndent(line)
	contIndent := baseIndent + f.indentUnit()

	var b strings.Builder
	b.WriteString(prefix)
	for i, part := range parts {
		b.WriteByte('\n')
		b.WriteString(contIndent)
		b.WriteString(strings.TrimSpace(part))
		if i < len(parts)-1 {
			b.WriteByte(',')
		}
	}
	b.WriteByte('\n')
	b.WriteString(baseIndent)
	b.WriteString(strings.TrimLeft(suffix, " \t"))
	return b.String()
}

var emptyLineRun = regexp.MustCompile(`\n{2,}`)

func (f *WrappingFormatter) collapseEmptyLines(s string) string {
	if f.Options.MaxEmptyLines <= 0 {
		return s
	}
	limit := f.Options.MaxEmptyLines + 1 // number of \n that separates blocks
	return emptyLineRun.ReplaceAllStringFunc(s, func(run string) string {
		if len(run) <= limit {
			return run
		}
		return strings.Repeat("\n", limit)
	})
}

func (f *WrappingFormatter) visualLength(line string) int {
	n := 0
	tab := f.Options.TabWidth
	if tab <= 0 {
		tab = 8
	}
	for _, r := range line {
		if r == '\t' {
			n += tab - (n % tab)
			continue
		}
		n++
	}
	return n
}

func (f *WrappingFormatter) indentUnit() string {
	if f.Options.UseSpaces {
		tab := f.Options.TabWidth
		if tab <= 0 {
			tab = 4
		}
		return strings.Repeat(" ", tab)
	}
	return "\t"
}

func leadingIndent(line string) string {
	for i, r := range line {
		if r != ' ' && r != '\t' {
			return line[:i]
		}
	}
	return line
}

// outerBracketPair returns the byte offsets of the outermost matched
// `(` / `)` or `{` / `}` pair on the line, or (-1, -1) if no balanced
// pair exists at the top level.
func outerBracketPair(line string) (int, int) {
	openIdx := -1
	depth := 0
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch c {
		case '(', '{':
			if depth == 0 && openIdx < 0 {
				openIdx = i
			}
			depth++
		case ')', '}':
			depth--
			if depth == 0 && openIdx >= 0 {
				return openIdx, i
			}
		case '"':
			// Skip strings - they can contain unbalanced brackets.
			j := i + 1
			for j < len(line) && line[j] != '"' {
				if line[j] == '\\' && j+1 < len(line) {
					j += 2
					continue
				}
				j++
			}
			i = j
		}
	}
	return -1, -1
}

// splitTopLevel splits s on the given byte, ignoring occurrences inside
// nested brackets and string literals.
func splitTopLevel(s string, sep byte) []string {
	var (
		out   []string
		start int
		depth int
	)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '(', '{', '[':
			depth++
		case ')', '}', ']':
			depth--
		case '"':
			j := i + 1
			for j < len(s) && s[j] != '"' {
				if s[j] == '\\' && j+1 < len(s) {
					j += 2
					continue
				}
				j++
			}
			i = j
		case sep:
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	out = append(out, s[start:])
	return out
}

package format

import (
	"io"
	"strings"
)

// This file implements a small Wadler/Lindig-style pretty-printing
// combinator library. It exists primarily for the wrapping printer
// that reflows over-long parameter lists and composite literals. The
// combinators are intentionally minimal: Text, Line, Nest, Group,
// Concat and HardLine, with the SoftLine / HardLine / Group / Nest
// vocabulary that pretty-printers in this lineage typically expose.

// Doc is the abstract document type that combinators build.
type Doc interface {
	docNode()
}

type (
	textDoc     struct{ s string }
	lineDoc     struct{ alt string } // alt is what appears when the group fits
	hardLineDoc struct{}
	nestDoc     struct {
		indent int
		d      Doc
	}
	groupDoc struct{ d Doc }
	concatDoc struct{ ds []Doc }
)

func (textDoc) docNode()     {}
func (lineDoc) docNode()     {}
func (hardLineDoc) docNode() {}
func (nestDoc) docNode()     {}
func (groupDoc) docNode()    {}
func (concatDoc) docNode()   {}

// Text is a literal piece of text with no wrapping behaviour.
func Text(s string) Doc { return textDoc{s} }

// SoftLine prints a single space when the enclosing group fits on a line,
// or a newline otherwise.
func SoftLine() Doc { return lineDoc{alt: " "} }

// SoftLineEmpty prints nothing when the enclosing group fits, or a
// newline otherwise. Useful between an opening bracket and its first
// element.
func SoftLineEmpty() Doc { return lineDoc{alt: ""} }

// HardLine forces a line break and resets the current column to the
// outer indentation.
func HardLine() Doc { return hardLineDoc{} }

// Nest increases the indentation for any line breaks produced inside d.
func Nest(indent int, d Doc) Doc { return nestDoc{indent: indent, d: d} }

// Group marks d as a candidate for fitting on a single line. The
// renderer will collapse soft lines inside d when the entire group
// fits within the configured PrintWidth.
func Group(d Doc) Doc { return groupDoc{d} }

// Concat joins several Docs into one.
func Concat(ds ...Doc) Doc { return concatDoc{ds: ds} }

// Join inserts sep between every consecutive pair of ds.
func Join(sep Doc, ds []Doc) Doc {
	if len(ds) == 0 {
		return Concat()
	}
	out := make([]Doc, 0, 2*len(ds)-1)
	for i, d := range ds {
		if i > 0 {
			out = append(out, sep)
		}
		out = append(out, d)
	}
	return Concat(out...)
}

// Render writes d to w using the given options. The algorithm is the
// "linear-time, lazy" variant described by Lindig, which is what
// clang-format and Prettier are also based on.
func Render(w io.Writer, d Doc, opts Options) error {
	if opts.PrintWidth <= 0 {
		opts.PrintWidth = 100
	}
	if opts.TabWidth <= 0 {
		opts.TabWidth = 4
	}
	r := renderer{
		w:    w,
		opts: opts,
	}
	r.render(d, 0, modeBreak)
	return r.err
}

// renderMode tells the renderer how to interpret soft lines inside
// the current group.
type renderMode int

const (
	modeFlat  renderMode = iota // soft lines render as their alt string
	modeBreak                   // soft lines render as a real line break
)

type renderer struct {
	w     io.Writer
	opts  Options
	col   int
	err   error
	atBOL bool
}

func (r *renderer) render(d Doc, indent int, mode renderMode) {
	if r.err != nil {
		return
	}
	switch x := d.(type) {
	case textDoc:
		r.writeString(x.s)
	case lineDoc:
		if mode == modeFlat {
			r.writeString(x.alt)
		} else {
			r.newline(indent)
		}
	case hardLineDoc:
		r.newline(indent)
	case nestDoc:
		r.render(x.d, indent+x.indent, mode)
	case groupDoc:
		if r.fits(x.d, indent, r.opts.PrintWidth-r.col) {
			r.render(x.d, indent, modeFlat)
		} else {
			r.render(x.d, indent, modeBreak)
		}
	case concatDoc:
		for _, c := range x.ds {
			r.render(c, indent, mode)
		}
	}
}

// fits returns true if d can be rendered in flat mode without exceeding
// the remaining width on the current line.
func (r *renderer) fits(d Doc, indent, remaining int) bool {
	if remaining < 0 {
		return false
	}
	return fitsRec(d, indent, remaining) >= 0
}

func fitsRec(d Doc, indent, remaining int) int {
	if remaining < 0 {
		return -1
	}
	switch x := d.(type) {
	case textDoc:
		return remaining - len(x.s)
	case lineDoc:
		return remaining - len(x.alt)
	case hardLineDoc:
		return -1
	case nestDoc:
		return fitsRec(x.d, indent+x.indent, remaining)
	case groupDoc:
		return fitsRec(x.d, indent, remaining)
	case concatDoc:
		for _, c := range x.ds {
			remaining = fitsRec(c, indent, remaining)
			if remaining < 0 {
				return -1
			}
		}
		return remaining
	}
	return remaining
}

func (r *renderer) writeString(s string) {
	if r.err != nil || s == "" {
		return
	}
	if _, err := io.WriteString(r.w, s); err != nil {
		r.err = err
		return
	}
	// Naive column tracking: treat tabs as TabWidth, count characters
	// otherwise. Good enough for ASCII source.
	if strings.IndexByte(s, '\n') >= 0 {
		idx := strings.LastIndexByte(s, '\n')
		r.col = len(s) - idx - 1
	} else {
		r.col += len(s)
	}
}

func (r *renderer) newline(indent int) {
	if r.err != nil {
		return
	}
	indentStr := r.indentString(indent)
	if _, err := io.WriteString(r.w, "\n"+indentStr); err != nil {
		r.err = err
		return
	}
	r.col = indent
}

func (r *renderer) indentString(indent int) string {
	if r.opts.UseSpaces {
		return strings.Repeat(" ", indent)
	}
	// Round indentation down to whole tabs and pad with spaces.
	tabs := indent / r.opts.TabWidth
	rem := indent % r.opts.TabWidth
	return strings.Repeat("\t", tabs) + strings.Repeat(" ", rem)
}

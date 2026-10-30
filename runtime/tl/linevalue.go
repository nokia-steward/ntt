package tl

// linevalue.go writes values and templates for the log of lines, compactly
// and without spaces:
//
//	integer      -200
//	float        1.0, 10000.0, 12.6999999999999992895 (21 significant
//	             digits, fixed notation, the double's exact value)
//	charstring   "a ""b""" — printable characters quoted, a quote doubled;
//	             any other character as char(g, p, r, c), joined with &:
//	             "a"&char(0,0,0,10)&"b"
//	strings      '0101'B '0AF'H '0AFF'O
//	record       {1,"a",omit}, its fields in order; set {a:=1,b:=2}
//	record of    {1,2,3}; map {["k"]:=1}
//	union        {alt:=1}
//	unbound      #
//	templates    ? * (1,2) complement(1) (1..3) "pattern text"
//	             with " length(n)" and " ifpresent" after them
//	component    its name and state: worker(running)

import (
	"math"
	"strconv"
	"strings"
)

// lineValue writes a Values element.
func (lw *LinesWriter) lineValue(n *Node) string {
	if n == nil {
		return "#"
	}
	if !isValueKind(n.Tag) {
		// A parameter element holding one value, or a template.
		switch n.Tag {
		case "any":
			return "?"
		case "anyoromit":
			return "*"
		case "omit":
			return "omit"
		case "null":
			return "null"
		}
		if len(n.Kids) == 1 {
			return lw.lineValue(n.Kids[0])
		}
		return "#"
	}
	var s string
	switch {
	case n.Kid("omit") != nil:
		return "omit"
	case n.Kid("null") != nil:
		switch n.Tag {
		case "component", "default", "address":
			s = "null"
		default:
			// An unbound value is logged as the schema's null.
			return "#"
		}
	case n.Kid("matching_symbol") != nil:
		s = lw.lineMatching(n, n.Kid("matching_symbol"))
		if s == "?" && n.Kid("ifpresent") != nil {
			// `? ifpresent` is AnyValueOrNone.
			return "*" + lineLength(n)
		}
		if s == "?" || s == "*" {
			return s + lineLength(n)
		}
	case n.Tag == "component":
		s = lw.componentValue(n)
	case n.Kid("value") != nil:
		s = lineScalar(n.Tag, n.Kid("value").Text)
	case n.Tag == "record_of" && n.Attr("type") == "map":
		var parts []string
		for _, pair := range n.Kids {
			if len(pair.Kids) == 2 {
				parts = append(parts, "["+lw.lineValue(pair.Kids[0])+"]:="+lw.lineValue(pair.Kids[1]))
			}
		}
		s = "{" + strings.Join(parts, ",") + "}"
	default:
		named := n.Tag == "set" || n.Tag == "union" || n.Tag == "anytype"
		var parts []string
		for _, k := range n.Kids {
			if !isValueKind(k.Tag) {
				continue
			}
			p := lw.lineValue(k)
			if nm := k.Attr("name"); named && nm != "" {
				p = nm + ":=" + p
			}
			parts = append(parts, p)
		}
		s = "{" + strings.Join(parts, ",") + "}"
	}
	return s + lineLength(n) + lineIfPresent(n)
}

func lineLength(n *Node) string {
	l := n.Kid("length")
	if l == nil {
		return ""
	}
	lo, hi := "0", "infinity"
	if v := l.Kid("lower"); v != nil {
		lo = v.Text
	}
	if v := l.Kid("upper"); v != nil {
		hi = v.Text
	}
	if lo == hi {
		return " length(" + lo + ")"
	}
	return " length(" + lo + ".." + hi + ")"
}

func lineIfPresent(n *Node) string {
	if n.Kid("ifpresent") != nil {
		return " ifpresent"
	}
	return ""
}

// lineMatching writes the matching symbol ms of value n.
func (lw *LinesWriter) lineMatching(n, ms *Node) string {
	if len(ms.Kids) != 1 {
		return "?"
	}
	m := ms.Kids[0]
	switch m.Tag {
	case "any_value", "any_element":
		return "?"
	case "any_value_or_none", "any_element_or_none":
		return "*"
	case "range":
		lo, hi := "-infinity", "infinity"
		if l := m.Kid("lower"); l != nil && len(l.Kids) == 1 {
			lo = lw.lineValue(l.Kids[0])
		}
		if h := m.Kid("upper"); h != nil && len(h.Kids) == 1 {
			hi = lw.lineValue(h.Kids[0])
		}
		if m.Kid("excludeLower") != nil {
			lo = "!" + lo
		}
		if m.Kid("excludeUpper") != nil {
			hi = "!" + hi
		}
		return "(" + lo + ".." + hi + ")"
	case "pattern":
		// A pattern is written as the text of it.
		if len(m.Kids) == 1 && m.Kids[0].Kid("value") != nil {
			return lineCharstring(m.Kids[0].Kid("value").Text)
		}
		return `""`
	}
	var parts []string
	for _, k := range m.Kids {
		parts = append(parts, lw.lineValue(k))
	}
	if m.Tag == "list" {
		return "(" + strings.Join(parts, ",") + ")"
	}
	return m.Tag + "(" + strings.Join(parts, ",") + ")"
}

// lineScalar writes a scalar of kind.
func lineScalar(kind, text string) string {
	switch kind {
	case "bitstring", "hexstring", "octetstring":
		// A template matching inside the string is written as its text.
		if strings.ContainsAny(text, "?*") {
			return lineCharstring(text)
		}
	}
	switch kind {
	case "charstring", "universal_charstring":
		return lineCharstring(text)
	case "bitstring":
		return "'" + text + "'B"
	case "hexstring":
		return "'" + strings.ToUpper(text) + "'H"
	case "octetstring":
		return "'" + strings.ToUpper(text) + "'O"
	case "float":
		return lineFloat(text)
	}
	return text
}

// lineCharstring quotes the printable characters of s, doubling a quote,
// and writes every other character as char(group, plane, row, cell),
// the pieces joined with &.
func lineCharstring(s string) string {
	if s == "" {
		return `""`
	}
	var pieces []string
	var run strings.Builder
	flush := func() {
		if run.Len() > 0 {
			pieces = append(pieces, `"`+run.String()+`"`)
			run.Reset()
		}
	}
	for _, r := range s {
		if r >= 32 && r <= 126 {
			if r == '"' {
				run.WriteString(`""`)
			} else {
				run.WriteRune(r)
			}
			continue
		}
		flush()
		w := uint32(r)
		pieces = append(pieces, "char("+strconv.Itoa(int(w>>24&255))+","+strconv.Itoa(int(w>>16&255))+","+
			strconv.Itoa(int(w>>8&255))+","+strconv.Itoa(int(w&255))+")")
	}
	flush()
	return strings.Join(pieces, "&")
}

// lineFloat writes the float text gives in fixed notation with up to 21
// significant digits, trailing zeros removed: the double's exact value,
// 12.7 being 12.6999999999999992895.
func lineFloat(text string) string {
	f, err := strconv.ParseFloat(text, 64)
	switch {
	case err != nil:
		return text
	case math.IsInf(f, 1):
		return "infinity"
	case math.IsInf(f, -1):
		return "-infinity"
	case math.IsNaN(f):
		return "not_a_number"
	case f == 0:
		return "0.0"
	}
	sign := ""
	if f < 0 {
		sign, f = "-", -f
	}
	e := strconv.FormatFloat(f, 'e', 20, 64) // d.dddddddddddddddddddde±XX
	mant, exp, _ := strings.Cut(e, "e")
	digits := strings.TrimRight(strings.Replace(mant, ".", "", 1), "0")
	x, _ := strconv.Atoi(exp)
	x++ // digits before the point
	switch {
	case x >= len(digits):
		return sign + digits + strings.Repeat("0", x-len(digits)) + ".0"
	case x > 0:
		return sign + digits[:x] + "." + digits[x:]
	}
	return sign + "0." + strings.Repeat("0", -x) + digits
}

// lineTypeName names the type of a Values element: its own name when the
// log has it, module-qualified, else the name of the built-in type or the
// kind of structure it is.
func lineTypeName(n *Node) string {
	if n == nil {
		return "-"
	}
	if !isValueKind(n.Tag) {
		if len(n.Kids) == 1 {
			return lineTypeName(n.Kids[0])
		}
		return "-"
	}
	if t := n.Attr("type"); t != "" && t != "map" {
		if m := n.Attr("module"); m != "" {
			return m + "." + t
		}
		return t
	}
	switch n.Tag {
	case "universal_charstring":
		return "universal charstring"
	case "record_of":
		if n.Attr("type") == "map" {
			return "map"
		}
		return "record of"
	case "set_of":
		return "set of"
	}
	return n.Tag
}

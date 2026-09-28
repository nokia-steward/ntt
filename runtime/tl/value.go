package tl

import "strconv"

// Value is a TTCN-3 value or template in the TCI-TL mapping (ES 201 873-6
// clause 11.3.3, Annex B.3 and B.4).
//
// Kind is the element of the schema's Value group: integer, float, boolean,
// verdicttype, bitstring, hexstring, octetstring, charstring,
// universal_charstring, record, record_of, set, set_of, enumerated, union,
// anytype, address, component, port, default or timer. Name, Type and
// Module are the optional value attributes; a record field, set field or
// union alternative carries its field name in Name.
//
// A scalar's Text is its value in TTCN-3 notation without literal
// decoration, the element already naming the type: 5, 1.5, true, pass,
// 0101 for a bitstring, 0A1B for an octetstring, the characters of a
// charstring, the identifier of an enumerated value.
//
// A template is a Value whose Match replaces the value, or that carries
// IfPresent or a Length restriction.
type Value struct {
	Kind               string
	Name, Type, Module string

	Text      string
	Elems     []Value
	Omit      bool
	Null      bool
	Match     *Matching
	IfPresent bool
	Length    *Length
}

// Matching is a matching symbol used instead of a value (Templates
// MatchingSymbol). Symbol is one of any_value (?), any_value_or_none (*),
// any_element, any_element_or_none, range, list, complement, subset,
// superset, permutation and pattern.
type Matching struct {
	Symbol               string
	List                 []Value
	Lower, Upper         *Value
	ExclLower, ExclUpper bool
	Pattern              string
	Universal            bool // the pattern is a universal charstring
}

// Length is a length restriction; Upper is nil for infinity.
type Length struct {
	Lower int
	Upper *int
}

func (v Value) attrs() []Attr {
	var a []Attr
	if v.Name != "" {
		a = append(a, Attr{"name", v.Name})
	}
	if v.Type != "" {
		a = append(a, Attr{"type", v.Type})
	}
	if v.Module != "" {
		a = append(a, Attr{"module", v.Module})
	}
	return a
}

// node returns v as the element of the Value group its Kind names. A
// value whose type is not known — an omitted field, a `?` inside a
// structure, an unbound value — is an anytype, the one element of the group
// that can hold any of them without claiming a type.
func (v Value) node() *Node {
	v = v.normalized()
	n := &Node{NS: NSValues, Tag: v.Kind, Attrs: v.attrs()}
	n.Kids = v.body()
	return n
}

// normalized resolves what the schema cannot express as given: an unknown
// kind becomes anytype, and a record_of or set_of whose elements are not
// all of one kind — the schema requires that — is taken for what it most
// likely is, a record or set value held positionally. Elements of no kind
// in an otherwise uniform list take the list's kind.
func (v Value) normalized() Value {
	if v.Kind == "" {
		v.Kind = "anytype"
	}
	if v.Kind == "anytype" && v.Match == nil && !v.Omit && !v.Null && len(v.Elems) == 0 {
		// AnytypeValue holds a value of some type, not bare text.
		v.Elems = []Value{{Kind: "charstring", Text: v.Text}}
	}
	if v.Kind != "record_of" && v.Kind != "set_of" {
		return v
	}
	kinds := map[string]bool{}
	for _, e := range v.Elems {
		if e.Kind != "" {
			kinds[e.Kind] = true
		}
	}
	if len(kinds) > 1 {
		if v.Kind == "record_of" {
			v.Kind = "record"
		} else {
			v.Kind = "set"
		}
		return v
	}
	for k := range kinds {
		elems := make([]Value, len(v.Elems))
		for i, e := range v.Elems {
			if e.Kind == "" {
				e.Kind = k
			}
			elems[i] = e
		}
		v.Elems = elems
	}
	return v
}

// body is the content of v's element, per its type in Annex B.3.
func (v Value) body() []*Node {
	if v.Null {
		return []*Node{el(NSValues, "null")}
	}
	if v.Omit {
		return []*Node{el(NSValues, "omit")}
	}
	switch v.Kind {
	case "port", "timer", "default":
		// PortValue, TimerValue, DefaultValue: a value, no matching.
		return []*Node{text(NSValues, "value", v.Text)}
	case "address":
		// AddressValue holds exactly one value of the address type.
		if len(v.Elems) == 1 {
			return []*Node{v.Elems[0].node()}
		}
		return []*Node{Value{Kind: "charstring", Text: v.Text}.node()}
	}
	var kids []*Node
	switch {
	case v.Match != nil:
		kids = append(kids, v.Match.node())
	case isStructured(v.Kind):
		for _, e := range v.Elems {
			kids = append(kids, e.node())
		}
	default:
		kids = append(kids, text(NSValues, "value", v.Text))
	}
	if v.IfPresent {
		kids = append(kids, el(NSValues, "ifpresent"))
	}
	// RecordValue, SetValue and UnionValue have no length element.
	if v.Length != nil && !isRecordLike(v.Kind) {
		l := el(NSValues, "length", text(NSValues, "lower", strconv.Itoa(v.Length.Lower)))
		if v.Length.Upper != nil {
			l.Kids = append(l.Kids, text(NSValues, "upper", strconv.Itoa(*v.Length.Upper)))
		}
		kids = append(kids, l)
	}
	return kids
}

func isStructured(kind string) bool {
	switch kind {
	case "record", "set", "record_of", "set_of", "union", "anytype":
		return true
	}
	return false
}

func isRecordLike(kind string) bool {
	switch kind {
	case "record", "set", "union", "anytype":
		return true
	}
	return false
}

func (m *Matching) node() *Node {
	ms := el(NSValues, "matching_symbol")
	switch m.Symbol {
	case "range":
		r := el(NSTemplates, "range")
		if m.ExclLower {
			r.Kids = append(r.Kids, el(NSTemplates, "excludeLower"))
		}
		if m.Lower != nil {
			r.Kids = append(r.Kids, el(NSTemplates, "lower", m.Lower.node()))
		}
		if m.ExclUpper {
			r.Kids = append(r.Kids, el(NSTemplates, "excludeUpper"))
		}
		if m.Upper != nil {
			r.Kids = append(r.Kids, el(NSTemplates, "upper", m.Upper.node()))
		}
		ms.Kids = append(ms.Kids, r)
	case "list", "complement", "subset", "superset", "permutation":
		l := el(NSTemplates, m.Symbol)
		for _, e := range m.List {
			l.Kids = append(l.Kids, e.node())
		}
		ms.Kids = append(ms.Kids, l)
	case "pattern":
		kind := "charstring"
		if m.Universal {
			kind = "universal_charstring"
		}
		// Pattern declares its charstring element itself, so it is in the
		// Templates namespace; its content is a Values:CharstringValue.
		p := el(NSTemplates, "pattern", el(NSTemplates, kind, text(NSValues, "value", m.Pattern)))
		ms.Kids = append(ms.Kids, p)
	default:
		ms.Kids = append(ms.Kids, el(NSTemplates, m.Symbol))
	}
	return ms
}

// valueContent returns v as the content of an element of type Values:Value.
func (v Value) valueContent() Content {
	return Content{Kids: []*Node{v.node()}}
}

// AsValue returns v as the content of a Values:Value parameter.
func (v Value) AsValue() Content { return v.valueContent() }

// AsTemplate returns v as the content of a Templates:TciValueTemplate
// parameter. A bare `?` or `*` of unknown type uses the special templates
// any and anyoromit; everything else is a value with matching symbols.
func (v Value) AsTemplate() Content {
	if v.Kind == "" && v.Match != nil {
		switch v.Match.Symbol {
		case "any_value":
			return Content{Kids: []*Node{el(NSTemplates, "any")}}
		case "any_value_or_none":
			return Content{Kids: []*Node{el(NSTemplates, "anyoromit")}}
		}
	}
	if v.Kind == "" && v.Omit {
		return Content{Kids: []*Node{el(NSTemplates, "omit")}}
	}
	return v.valueContent()
}

// NonValueTemplate kinds for Templates:TciNonValueTemplate parameters such
// as fromTmpl, compTmpl and timerTmpl.
func AnyTemplate() Content  { return Content{Kids: []*Node{el(NSTemplates, "any")}} }
func AllTemplate() Content  { return Content{Kids: []*Node{el(NSTemplates, "all")}} }
func NullTemplate() Content { return Content{Kids: []*Node{el(NSTemplates, "null")}} }

// ValueTemplate is a non-value template given by a concrete value, e.g.
// the component a `from` clause names.
func ValueTemplate(v Value) Content { return v.valueContent() }

// Diff is one difference between a value and the template it did not match
// (Templates:TciValueDifference): XPath expressions locating the differing
// parts in the logged value and template, and an optional description.
type Diff struct {
	Val, Tmpl string
	Desc      string
}

// Diffs is a Templates:TciValueDifferenceList. The schema requires at least
// one difference; with none, a single one describing the whole value is
// used.
func Diffs(ds ...Diff) Content {
	if len(ds) == 0 {
		ds = []Diff{{Val: "/", Tmpl: "/"}}
	}
	var kids []*Node
	for _, d := range ds {
		n := el(NSTemplates, "diff", text(NSTemplates, "val", d.Val), text(NSTemplates, "tmpl", d.Tmpl))
		if d.Desc != "" {
			n.Attrs = append(n.Attrs, Attr{"desc", d.Desc})
		}
		kids = append(kids, n)
	}
	return Content{Kids: kids}
}

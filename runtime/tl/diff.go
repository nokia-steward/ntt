package tl

import (
	"encoding/xml"
	"fmt"
	"html"
	"path"
	"sort"
	"strconv"
	"strings"
)

// Comparing two runs: what each component did, testcase by testcase.
//
// Two correct runs of the same testcase can interleave their components
// differently, and differ in when a message arrived relative to what a
// component was doing. So the comparison is per component, and leaves out
// the events that record arrivals and their consequences rather than
// actions: a message or procedure envelope arriving in a queue, a receiving
// operation failing to match whatever happened to be at its head, an alt
// round that found nothing. Everything else — every send and receive with
// its value and template, every verdict, timer, component and port
// operation, every alt entered and left — must agree. The elapsed time a
// timer read reports is left out too: it measures the clock.
//
// Components are matched by the name they were created with, numbered in
// creation order when several share it; the MTC, the system and a control
// part by their roles. Their numeric ids, which two runs may assign
// differently, are left out, and a value that refers to a component is
// compared as that component. A component whose behaviour outlived its
// testcase is compared with the other events outside any testcase.

// ArrivalTiming reports whether op records when something arrived, not what
// a component did, so that it may differ between correct runs.
func ArrivalTiming(op string) bool {
	switch {
	case strings.Contains(op, "Detected"), strings.Contains(op, "Mismatch"):
		return true
	case op == "tliANomatch", op == "tliADefaults", op == "tliAWait":
		return true
	}
	return false
}

// Run is one testcase of a log: each component's events in order.
type Run struct {
	Testcase   string             // Module.testcase, with #n for its n-th run in the log
	Components map[string][]*Node // by component key (see componentKeys)
	keys       map[string]string  // component id -> key
	used       map[string]bool
	xml        bool // the log is XML; see Compare
}

// Outside is the name of the run holding the events outside any testcase.
const Outside = "(outside any testcase)"

func newRun(name string, xml bool) *Run {
	return &Run{Testcase: name, Components: map[string][]*Node{}, keys: map[string]string{},
		used: map[string]bool{"mtc": true, "system": true, "control": true}, xml: xml}
}

// Runs splits a log into its testcases, framed by tliTcStart and
// tliTcTerminated. Events outside any testcase — a control part's, or those
// of a component that outlived its testcase — form a run named Outside.
func (l *Log) Runs() []*Run {
	var runs []*Run
	seen := map[string]int{}
	var cur *Run
	outside := newRun(Outside, l.XML)
	for _, e := range l.Events {
		if e.Tag == "tliTcStart" {
			name := testcaseName(e)
			seen[name]++
			if seen[name] > 1 {
				name += "#" + strconv.Itoa(seen[name])
			}
			cur = newRun(name, l.XML)
			runs = append(runs, cur)
		}
		r := cur
		if r == nil || e.Attr("id") == LateID {
			r = outside
		}
		r.add(e)
		if e.Tag == "tliTcTerminated" {
			cur = nil
		}
	}
	if len(outside.Components) > 0 {
		runs = append([]*Run{outside}, runs...)
	}
	return runs
}

// add files e under its component's key. A tliCCreate names the component
// it creates: its key is the name it was created with, or "(unnamed)",
// numbered from the second component of that name on, in creation order.
// The MTC, the system and a control part, which are not created, keep their
// role names, which no created component can take.
func (r *Run) add(e *Node) {
	if e.Tag == "tliCCreate" {
		if id, _ := compID(e.Kid("comp")); id != "" {
			base := "(unnamed)"
			if n := e.Kid("name"); n != nil && n.Text != "" {
				base = n.Text
			}
			key := base
			for i := 2; r.used[key]; i++ {
				key = base + "#" + strconv.Itoa(i)
			}
			r.used[key] = true
			r.keys[id] = key
		}
	}
	r.Components[r.key(e.Attr("id"), e.Attr("name"))] = append(r.Components[r.key(e.Attr("id"), e.Attr("name"))], e)
}

// key returns the key of the component with this id, falling back to its
// logged name for a component the run saw no creation of.
func (r *Run) key(id, name string) string {
	if k, ok := r.keys[id]; ok {
		return k
	}
	return name
}

// compID returns the id and name a Types:TriComponentIdType identifies.
func compID(n *Node) (id, name string) {
	if n == nil {
		return "", ""
	}
	if i := n.Kid("id"); i != nil {
		if v := i.Kid("id"); v != nil {
			id = v.Text
		}
		if v := i.Kid("name"); v != nil {
			name = v.Text
		}
	}
	return id, name
}

func testcaseName(e *Node) string {
	if tc := e.Kid("tcId"); tc != nil {
		if n := tc.Kid("name"); n != nil {
			return n.Attr("moduleName") + "." + n.Attr("baseName")
		}
	}
	return "?"
}

// Difference is the first place one component's actions differ between
// two runs. A or B is nil when that run has no event there. CanonA and
// CanonB are what was compared, for when the summaries read alike.
type Difference struct {
	Component      string
	Index          int // position among the component's compared events
	A, B           *Node
	CanonA, CanonB string
}

// Result compares one testcase of two logs. Missing names the log that
// has no such testcase ("a" or "b"); otherwise Differences lists, per
// component, the first difference, and Compared the events compared.
type Result struct {
	Testcase    string
	Missing     string
	Compared    int
	Differences []Difference
}

// Same reports whether the testcase did the same in both runs.
func (r Result) Same() bool { return r.Missing == "" && len(r.Differences) == 0 }

// Compare compares two logs testcase by testcase, in the order of a, then
// the testcases only b has.
//
// XML cannot carry every character: those XML 1.0 excludes, such as most
// control characters, are written as U+FFFD. When either log is XML, text
// is compared as the XML form would hold it, so that a log compares the
// same with its own XML and JSON Lines forms; two values differing only in
// such characters then compare the same.
func Compare(a, b *Log) []Result {
	ra, rb := a.Runs(), b.Runs()
	lossy := a.XML || b.XML
	byName := map[string]*Run{}
	for _, r := range rb {
		byName[r.Testcase] = r
	}
	var out []Result
	done := map[string]bool{}
	for _, x := range ra {
		done[x.Testcase] = true
		y, ok := byName[x.Testcase]
		if !ok {
			out = append(out, Result{Testcase: x.Testcase, Missing: "b"})
			continue
		}
		out = append(out, compareRuns(x, y, lossy))
	}
	for _, y := range rb {
		if !done[y.Testcase] {
			out = append(out, Result{Testcase: y.Testcase, Missing: "a"})
		}
	}
	return out
}

func compareRuns(x, y *Run, lossy bool) Result {
	res := Result{Testcase: x.Testcase}
	names := map[string]bool{}
	for n := range x.Components {
		names[n] = true
	}
	for n := range y.Components {
		names[n] = true
	}
	var sorted []string
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)
	for _, comp := range sorted {
		ea, eb := actions(x.Components[comp]), actions(y.Components[comp])
		n := len(ea)
		if len(eb) > n {
			n = len(eb)
		}
		for i := 0; i < n; i++ {
			var a, b *Node
			var ca, cb string
			if i < len(ea) {
				a = ea[i]
				ca = canonical(a, x, lossy)
			}
			if i < len(eb) {
				b = eb[i]
				cb = canonical(b, y, lossy)
			}
			if a != nil && b != nil && ca == cb {
				res.Compared++
				continue
			}
			res.Differences = append(res.Differences, Difference{Component: comp, Index: i, A: a, B: b, CanonA: ca, CanonB: cb})
			break
		}
	}
	return res
}

func actions(events []*Node) []*Node {
	var out []*Node
	for _, e := range events {
		if !ArrivalTiming(e.Tag) {
			out = append(out, e)
		}
	}
	return out
}

// canonical renders what is compared of an event: its operation, where in
// the test specification it was performed (the file's base name, since the
// same file may sit at different paths, and the line), and its content,
// with components identified by key and a timer read's elapsed time left
// out.
func canonical(e *Node, r *Run, lossy bool) string {
	var b strings.Builder
	b.WriteString(e.Tag)
	b.WriteString("@")
	b.WriteString(path.Base(strings.ReplaceAll(e.Attr("src"), `\`, "/")))
	b.WriteString(":")
	b.WriteString(e.Attr("line"))
	for _, k := range e.Kids {
		if k.Tag == "am" || (e.Tag == "tliTRead" && k.Tag == "elapsed") {
			continue
		}
		b.WriteByte(' ')
		canon(&b, k, r, lossy)
	}
	return b.String()
}

func canon(b *strings.Builder, n *Node, r *Run, lossy bool) {
	// A component identifier or a component value: the component's key,
	// not its numeric id.
	if id, name := compID(n); id != "" && len(n.Kids) == 1 {
		b.WriteString(n.Tag + "(->" + strconv.Quote(r.key(id, name)) + ")")
		return
	}
	if n.Tag == "component" && len(n.Kids) == 1 && n.Kids[0].Tag == "value" {
		if key, ok := r.keys[n.Kids[0].Text]; ok {
			b.WriteString(n.Tag + "(->" + strconv.Quote(key) + ")")
			return
		}
	}
	b.WriteString(n.Tag)
	attrs := append([]Attr(nil), n.Attrs...)
	sort.Slice(attrs, func(i, j int) bool { return attrs[i].Name < attrs[j].Name })
	for _, a := range attrs {
		v := a.Value
		if lossy {
			v = xmlText(v)
		}
		b.WriteString(" " + a.Name + "=" + strconv.Quote(v))
	}
	if t := n.Text; t != "" {
		if lossy {
			t = xmlText(t)
		}
		b.WriteString(" " + strconv.Quote(t))
	}
	b.WriteByte('(')
	for _, k := range n.Kids {
		canon(b, k, r, lossy)
	}
	b.WriteByte(')')
}

// xmlText is s as an XML log holds it: characters XML 1.0 cannot carry
// replaced by U+FFFD, as encoding/xml's escaping does.
func xmlText(s string) string {
	var sb strings.Builder
	_ = xml.EscapeText(&sb, []byte(s))
	// EscapeText also escapes markup; undo that, only the replacement
	// matters here.
	return html.UnescapeString(sb.String())
}

// Summary renders an event in one line: its operation, its component and
// where it was performed, and its content in brief, values in TTCN-3
// notation.
func Summary(e *Node) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s [%s", e.Tag, e.Attr("name"))
	if l := e.Attr("line"); l != "" {
		fmt.Fprintf(&b, ", line %s", l)
	}
	b.WriteString("]")
	for _, k := range e.Kids {
		if k.Tag == "am" {
			continue
		}
		if v := brief(k); v != "" {
			fmt.Fprintf(&b, " %s=%s", k.Tag, v)
		}
	}
	return b.String()
}

// brief renders one element of an event compactly, "" when it holds
// nothing worth showing.
func brief(n *Node) string {
	// Identifiers.
	if comp, port := n.Kid("comp"), n.Kid("port"); comp != nil && port != nil {
		return brief(comp) + ":" + portName(port)
	}
	if n.Tag == "port" && n.Kid("id") != nil {
		return portName(n)
	}
	if _, name := compID(n); name != "" && len(n.Kids) == 1 {
		return name
	}
	if n.Attr("baseName") != "" {
		return n.Attr("moduleName") + "." + n.Attr("baseName")
	}
	if nm := n.Kid("name"); nm != nil && nm.Attr("baseName") != "" && len(n.Kids) == 1 {
		return brief(nm)
	}
	switch n.Tag {
	case "tciPars":
		var ps []string
		for _, par := range n.Kids {
			ps = append(ps, par.Attr("name")+" := "+brief(par.Kid("val")))
		}
		if len(ps) == 0 {
			return ""
		}
		return "(" + strings.Join(ps, ", ") + ")"
	case "diffs":
		var ds []string
		for _, d := range n.Kids {
			s := d.Attr("desc")
			if v := d.Kid("val"); v != nil && v.Text != "." {
				if s != "" {
					s += " "
				}
				s += "at " + v.Text
			}
			ds = append(ds, s)
		}
		return strings.Join(ds, "; ")
	}
	if len(n.Kids) == 0 {
		switch {
		case n.Text != "":
			return n.Text
		case n.Attr("val") != "":
			return n.Attr("val")
		}
		return map[string]string{"omit": "omit", "null": "null", "any": "?", "anyoromit": "*", "all": "all"}[n.Tag]
	}
	// A parameter element (msgValue, msgTmpl, ...) holding one value.
	if len(n.Kids) == 1 && !isValueKind(n.Tag) {
		return brief(n.Kids[0])
	}
	if isValueKind(n.Tag) {
		return value(n)
	}
	var parts []string
	for _, k := range n.Kids {
		parts = append(parts, brief(k))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func portName(p *Node) string {
	name := ""
	if id := p.Kid("id"); id != nil {
		if nm := id.Kid("name"); nm != nil {
			name = nm.Text
		}
	}
	if ix := p.Kid("index"); ix != nil {
		name += "[" + ix.Text + "]"
	}
	return name
}

var valueKinds = map[string]bool{
	"integer": true, "float": true, "boolean": true, "verdicttype": true, "bitstring": true,
	"hexstring": true, "octetstring": true, "charstring": true, "universal_charstring": true,
	"record": true, "record_of": true, "set": true, "set_of": true, "enumerated": true,
	"union": true, "anytype": true, "address": true, "component": true, "port": true,
	"default": true, "timer": true,
}

func isValueKind(tag string) bool { return valueKinds[tag] }

// value renders a Values element in TTCN-3 notation: its content, or the
// matching symbol that replaces it, then its ifpresent and length
// attributes, prefixed by its type when the log names one.
func value(n *Node) string {
	var s string
	switch {
	case n.Kid("omit") != nil:
		s = "omit"
	case n.Kid("null") != nil:
		s = "null"
	case n.Kid("matching_symbol") != nil:
		s = matchingSymbol(n.Kid("matching_symbol"))
	case n.Kid("value") != nil:
		s = literal(n.Tag, n.Kid("value").Text)
	default:
		var parts []string
		for _, k := range n.Kids {
			if !isValueKind(k.Tag) {
				continue
			}
			p := value(k)
			if nm := k.Attr("name"); nm != "" {
				p = nm + " := " + p
			}
			parts = append(parts, p)
		}
		s = "{" + strings.Join(parts, ", ") + "}"
	}
	if n.Kid("ifpresent") != nil {
		s += " ifpresent"
	}
	if l := n.Kid("length"); l != nil {
		lo, hi := "", "infinity"
		if v := l.Kid("lower"); v != nil {
			lo = v.Text
		}
		if v := l.Kid("upper"); v != nil {
			hi = v.Text
		}
		if lo == hi {
			s += " length(" + lo + ")"
		} else {
			s += " length(" + lo + " .. " + hi + ")"
		}
	}
	if t := n.Attr("type"); t != "" {
		s = t + ": " + s
	}
	if n.Tag == "component" {
		s = "component " + s
	}
	return s
}

// literal writes a scalar in TTCN-3 notation.
func literal(kind, text string) string {
	switch kind {
	case "charstring", "universal_charstring":
		return strconv.Quote(text)
	case "bitstring":
		return "'" + text + "'B"
	case "hexstring":
		return "'" + text + "'H"
	case "octetstring":
		return "'" + text + "'O"
	case "float":
		if !strings.ContainsAny(text, ".eE") && text != "infinity" && text != "-infinity" && text != "not_a_number" {
			return text + ".0"
		}
	}
	return text
}

func matchingSymbol(ms *Node) string {
	if len(ms.Kids) != 1 {
		return "?"
	}
	n := ms.Kids[0]
	switch n.Tag {
	case "any_value":
		return "?"
	case "any_value_or_none":
		return "*"
	case "any_element":
		return "?"
	case "any_element_or_none":
		return "*"
	case "range":
		lo, hi := "-infinity", "infinity"
		if l := n.Kid("lower"); l != nil && len(l.Kids) == 1 {
			lo = value(l.Kids[0])
		}
		if h := n.Kid("upper"); h != nil && len(h.Kids) == 1 {
			hi = value(h.Kids[0])
		}
		if n.Kid("excludeLower") != nil {
			lo = "!" + lo
		}
		if n.Kid("excludeUpper") != nil {
			hi = "!" + hi
		}
		return "(" + lo + " .. " + hi + ")"
	case "pattern":
		if len(n.Kids) == 1 && n.Kids[0].Kid("value") != nil {
			return "pattern " + strconv.Quote(n.Kids[0].Kid("value").Text)
		}
		return "pattern"
	}
	var parts []string
	for _, k := range n.Kids {
		parts = append(parts, value(k))
	}
	switch n.Tag {
	case "list":
		return "(" + strings.Join(parts, ", ") + ")"
	}
	return n.Tag + "(" + strings.Join(parts, ", ") + ")"
}

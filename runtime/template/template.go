// Package template implements TTCN-3 template matching (Core Language
// clause 15) as a small, side-effect-free engine that the runtime, the
// interpreter and the future MIR backends can all share.
//
// A Template is a pattern that decides whether a concrete value matches.
// The interesting bits are the matching primitives: AnyValue (`?`),
// AnyOrNone (`*`), Range, ValueList, Complement, plus the template
// algebra (And, Or, Not). Composite templates (records, lists) reuse the
// same Template interface for their fields so matching is recursive by
// construction.
//
// The package is intentionally decoupled from `runtime.Object`: we work
// off the small Value interface defined here so tests can drive the
// engine with plain Go values without dragging the whole interpreter in.
// The interpreter has thin adapters in `runtime/template/adapter.go`
// that bridge to its concrete object representation.
package template

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Value is anything that can be matched against a template. The engine
// only needs three operations: comparing two values for equality,
// pretty-printing for diagnostics, and dispatching by Kind so we can
// validate that a Range template doesn't get applied to a record.
type Value interface {
	Kind() ValueKind
	Inspect() string
	Equal(Value) bool
}

// ValueKind enumerates the shapes of values the engine understands.
// The kinds intentionally mirror the TTCN-3 base types so the matching
// rules stay close to clause 15 of the standard.
type ValueKind int

const (
	KindUnknown ValueKind = iota
	KindInt
	KindFloat
	KindBool
	KindCharstring
	KindBitstring
	KindHexstring
	KindOctetstring
	KindEnum
	KindVerdict
	KindRecord
	KindRecordOf
	KindSet
	KindSetOf
	KindUnion
	KindOmit
	KindNull
)

// Template is the matching primitive. Match reports whether v matches
// the template; Inspect returns the TTCN-3 textual form for diagnostics.
type Template interface {
	Match(v Value) bool
	Inspect() string
}

// MatchResult carries the verdict plus a human-readable path to the
// first mismatch. The match engine populates Path for nested record /
// list templates so error messages can point at exactly the offending
// field. Match returns the boolean result; Detail returns the result
// plus the path.
type MatchResult struct {
	OK     bool
	Path   string
	Reason string
}

// Detail runs the template's Match plus auxiliary diagnostics for
// non-OK matches. Implementations that don't override Detail get a
// generic mismatch message; record/list templates supply a richer path.
func Detail(t Template, v Value) MatchResult {
	type detailer interface {
		Detail(v Value) MatchResult
	}
	if d, ok := t.(detailer); ok {
		return d.Detail(v)
	}
	if t.Match(v) {
		return MatchResult{OK: true}
	}
	return MatchResult{
		OK:     false,
		Reason: fmt.Sprintf("value %s does not match %s", v.Inspect(), t.Inspect()),
	}
}

// ---------------------------------------------------------------------------
// Leaf templates
// ---------------------------------------------------------------------------

// AnyValue is the `?` wildcard. It matches any value of the same kind
// when Kind is set; an unconstrained `?` matches every value.
type AnyValue struct {
	WantKind ValueKind
}

// Match implements Template.
func (a AnyValue) Match(v Value) bool {
	if a.WantKind == KindUnknown {
		return true
	}
	return v.Kind() == a.WantKind
}

// Inspect implements Template.
func (a AnyValue) Inspect() string { return "?" }

// AnyOrNoneValue is the `*` wildcard. At the leaf level it matches any
// value plus "omit"; inside record-of/set-of templates it has additional
// list-level semantics handled by RecordOfTemplate.
type AnyOrNoneValue struct{}

// Match implements Template.
func (AnyOrNoneValue) Match(Value) bool { return true }

// Inspect implements Template.
func (AnyOrNoneValue) Inspect() string { return "*" }

// Literal wraps a concrete value as a single-value template.
type Literal struct {
	Value Value
}

// Match implements Template.
func (l Literal) Match(v Value) bool {
	if l.Value == nil {
		return v == nil
	}
	return l.Value.Equal(v)
}

// Inspect implements Template.
func (l Literal) Inspect() string {
	if l.Value == nil {
		return "null"
	}
	return l.Value.Inspect()
}

// Range matches integer/float values in [Lower, Upper]. Either bound may
// be nil, meaning -infinity / +infinity respectively (the TTCN-3 syntax
// allows `-infinity..N` and `N..infinity`).
type Range struct {
	Lower, Upper Value
}

// Match implements Template.
func (r Range) Match(v Value) bool {
	if r.Lower != nil && less(v, r.Lower) {
		return false
	}
	if r.Upper != nil && less(r.Upper, v) {
		return false
	}
	return true
}

// Inspect implements Template.
func (r Range) Inspect() string {
	lo, hi := "-infinity", "infinity"
	if r.Lower != nil {
		lo = r.Lower.Inspect()
	}
	if r.Upper != nil {
		hi = r.Upper.Inspect()
	}
	return lo + ".." + hi
}

// ValueList matches if v equals any of Values. Empty list never matches,
// which mirrors TTCN-3's `( )` semantics.
type ValueList struct {
	Values []Value
}

// Match implements Template.
func (vl ValueList) Match(v Value) bool {
	for _, x := range vl.Values {
		if x.Equal(v) {
			return true
		}
	}
	return false
}

// Inspect implements Template.
func (vl ValueList) Inspect() string {
	parts := make([]string, len(vl.Values))
	for i, v := range vl.Values {
		parts[i] = v.Inspect()
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// Complement matches any value that does NOT match Inner. It maps to
// TTCN-3's `complement (...)` template.
type Complement struct {
	Inner Template
}

// Match implements Template.
func (c Complement) Match(v Value) bool { return !c.Inner.Match(v) }

// Inspect implements Template.
func (c Complement) Inspect() string { return "complement(" + c.Inner.Inspect() + ")" }

// And matches when every sub-template matches. Empty And always matches,
// which makes it a useful identity element for builders.
type And struct {
	Parts []Template
}

// Match implements Template.
func (a And) Match(v Value) bool {
	for _, p := range a.Parts {
		if !p.Match(v) {
			return false
		}
	}
	return true
}

// Inspect implements Template.
func (a And) Inspect() string {
	parts := make([]string, len(a.Parts))
	for i, p := range a.Parts {
		parts[i] = p.Inspect()
	}
	return "(" + strings.Join(parts, " and ") + ")"
}

// Or matches when at least one sub-template matches.
type Or struct {
	Parts []Template
}

// Match implements Template.
func (o Or) Match(v Value) bool {
	for _, p := range o.Parts {
		if p.Match(v) {
			return true
		}
	}
	return false
}

// Inspect implements Template.
func (o Or) Inspect() string {
	parts := make([]string, len(o.Parts))
	for i, p := range o.Parts {
		parts[i] = p.Inspect()
	}
	return "(" + strings.Join(parts, " or ") + ")"
}

// Length wraps Inner with a length restriction, e.g. `?length(1..3)`. The
// inner template is matched first; if it accepts, Length checks that the
// value's length is in [Min, Max] (Max == -1 means no upper bound).
type Length struct {
	Inner    Template
	Min, Max int
}

// Match implements Template.
func (l Length) Match(v Value) bool {
	if !l.Inner.Match(v) {
		return false
	}
	n := lengthOf(v)
	if n < 0 {
		// Length doesn't apply to scalar values.
		return false
	}
	if n < l.Min {
		return false
	}
	if l.Max != -1 && n > l.Max {
		return false
	}
	return true
}

// Inspect implements Template.
func (l Length) Inspect() string {
	rng := fmt.Sprintf("length(%d", l.Min)
	if l.Max == -1 {
		rng += "..infinity"
	} else if l.Max != l.Min {
		rng += fmt.Sprintf("..%d", l.Max)
	}
	rng += ")"
	return l.Inner.Inspect() + " " + rng
}

// IfPresent matches when the value is absent (omit) OR when Inner
// matches. It corresponds to TTCN-3's `ifpresent` qualifier.
type IfPresent struct {
	Inner Template
}

// Match implements Template.
func (i IfPresent) Match(v Value) bool {
	if v == nil || v.Kind() == KindOmit {
		return true
	}
	return i.Inner.Match(v)
}

// Inspect implements Template.
func (i IfPresent) Inspect() string {
	return i.Inner.Inspect() + " ifpresent"
}

// ---------------------------------------------------------------------------
// Composite templates
// ---------------------------------------------------------------------------

// RecordTemplate matches record / set values field by field. Fields not
// listed in Fields default to AnyValue. Unknown fields in the value are
// ignored (TTCN-3 records carry exactly the declared fields).
type RecordTemplate struct {
	Fields map[string]Template
}

// Match implements Template.
func (r RecordTemplate) Match(v Value) bool {
	rec, ok := v.(RecordValue)
	if !ok {
		return false
	}
	for name, t := range r.Fields {
		fv, present := rec.Field(name)
		if !present {
			if !t.Match(omitValue{}) {
				return false
			}
			continue
		}
		if !t.Match(fv) {
			return false
		}
	}
	return true
}

// Detail returns the first mismatching field, with a dotted path that
// upstream layers can chain.
func (r RecordTemplate) Detail(v Value) MatchResult {
	rec, ok := v.(RecordValue)
	if !ok {
		return MatchResult{Reason: fmt.Sprintf("value %s is not a record", v.Inspect())}
	}
	// Iterate deterministically so error messages are reproducible.
	names := make([]string, 0, len(r.Fields))
	for n := range r.Fields {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		t := r.Fields[name]
		fv, present := rec.Field(name)
		if !present {
			if !t.Match(omitValue{}) {
				return MatchResult{Reason: fmt.Sprintf("field %s missing, expected %s", name, t.Inspect()), Path: name}
			}
			continue
		}
		sub := Detail(t, fv)
		if !sub.OK {
			sub.Path = joinPath(name, sub.Path)
			return sub
		}
	}
	return MatchResult{OK: true}
}

// Inspect implements Template.
func (r RecordTemplate) Inspect() string {
	names := make([]string, 0, len(r.Fields))
	for n := range r.Fields {
		names = append(names, n)
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = n + " := " + r.Fields[n].Inspect()
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// RecordOfTemplate matches list-shaped values. AnyOrNone elements (the
// `*` wildcard) absorb zero or more consecutive values, which makes the
// matching algorithm a small state machine.
type RecordOfTemplate struct {
	Elements []Template
	// LengthMin / LengthMax can constrain the value's length on top of
	// the element-by-element match. LengthMax == -1 means no upper bound.
	LengthMin, LengthMax int
}

// Match implements Template.
func (r RecordOfTemplate) Match(v Value) bool {
	lv, ok := v.(ListValue)
	if !ok {
		return false
	}
	n := lv.Length()
	if n < r.LengthMin {
		return false
	}
	if r.LengthMax != -1 && r.LengthMax >= 0 && n > r.LengthMax {
		return false
	}
	values := make([]Value, n)
	for i := 0; i < n; i++ {
		values[i] = lv.Element(i)
	}
	return matchList(r.Elements, values)
}

// Inspect implements Template.
func (r RecordOfTemplate) Inspect() string {
	parts := make([]string, len(r.Elements))
	for i, e := range r.Elements {
		parts[i] = e.Inspect()
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// RecordValue / ListValue are the structural interfaces the engine
// needs. Implementations live in adapter packages so the engine doesn't
// have to know about runtime.Object.
type RecordValue interface {
	Value
	Field(name string) (Value, bool)
}

// ListValue is the structural interface for record-of / set-of values.
type ListValue interface {
	Value
	Length() int
	Element(i int) Value
}

// omitValue is the placeholder we hand to templates when a field is
// absent. It only ever participates in matching, never in user-visible
// values.
type omitValue struct{}

func (omitValue) Kind() ValueKind { return KindOmit }
func (omitValue) Inspect() string { return "omit" }
func (omitValue) Equal(v Value) bool {
	_, ok := v.(omitValue)
	return ok
}

// Omit returns the canonical omit value. The engine uses it when a
// caller wants to model "field is absent" without inventing their own
// sentinel.
func Omit() Value { return omitValue{} }

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// matchList runs the TTCN-3 list-matching rules: every element template
// must consume one value, except for AnyOrNoneValue which consumes zero
// or more. We use a small recursive backtracking algorithm because the
// element count rarely exceeds a handful; for hot paths the IR layer can
// emit a compiled matcher.
func matchList(elems []Template, values []Value) bool {
	if len(elems) == 0 {
		return len(values) == 0
	}
	head, rest := elems[0], elems[1:]
	if _, ok := head.(AnyOrNoneValue); ok {
		// Try consuming 0, 1, ..., len(values) values for the wildcard.
		for i := 0; i <= len(values); i++ {
			if matchList(rest, values[i:]) {
				return true
			}
		}
		return false
	}
	if len(values) == 0 {
		return false
	}
	if !head.Match(values[0]) {
		return false
	}
	return matchList(rest, values[1:])
}

// less reports whether a < b for numeric values. The implementation is
// deliberately small: the runtime supplies richer comparison via the
// Value interface for non-numeric types.
func less(a, b Value) bool {
	type comparable interface {
		Less(Value) bool
	}
	if c, ok := a.(comparable); ok {
		return c.Less(b)
	}
	// Fall back to deep equality: if values are equal, neither is less.
	if reflect.DeepEqual(a, b) {
		return false
	}
	return false
}

// lengthOf returns the length of v in TTCN-3 terms or -1 if v is a
// scalar that doesn't have a length.
func lengthOf(v Value) int {
	type lengther interface {
		Length() int
	}
	if l, ok := v.(lengther); ok {
		return l.Length()
	}
	type charLen interface {
		CharCount() int
	}
	if l, ok := v.(charLen); ok {
		return l.CharCount()
	}
	return -1
}

// joinPath chains dotted field paths, skipping empty components so the
// resulting string never contains leading or trailing dots.
func joinPath(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + "." + b
	}
}

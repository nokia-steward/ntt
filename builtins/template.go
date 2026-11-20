// Template values: the algebra of TTCN-3 templates the interpreter
// applies to evaluated values, with no syntax tree: whether a template
// holds a matching mechanism or is a value, its value and presentness
// (15.14, 15.15), modified templates merged (15.5), elements of record of
// templates and of character patterns referenced and assigned (15.6), and
// whether a template matches omit as a whole (15.8).

package builtins

import (
	"strings"

	"github.com/nokia/ntt/runtime"
)

// maxTemplateScan bounds the lists searched for the matching symbols
// that change how an element is referenced: a record of template with a
// permutation or AnyElementsOrNone is written out by hand, and a value
// never holds one, so a longer list is taken to be a value rather than
// every element of it being searched at every reference.
const maxTemplateScan = 256

// MatchesOmitAsAWhole reports whether the template v as a whole matches
// omit, which the present restriction forbids (ETSI 15.8, table 12):
// omit, AnyValueOrNone, a template with ifpresent, or a template list one
// of these is in.
func MatchesOmitAsAWhole(v runtime.Object) bool {
	switch x := v.(type) {
	case *runtime.IfPresent:
		return true
	case *runtime.List:
		if x.ListType != runtime.VALUE_LIST {
			return false
		}
		for _, e := range x.Elements {
			if MatchesOmitAsAWhole(e) {
				return true
			}
		}
		return false
	}
	return v == runtime.Omit || v == runtime.AnyOrNone
}

// MergeTemplateMod implements the runtime side of `template T t2
// modifies t1 := { fieldX := X, ... }`. Field assignments in the
// modifier overlay the base record's field map; unspecified fields, and
// those the modifier leaves unchanged with `-`, inherit from the base
// (ETSI ES 201 873-1 15.5). Either side may be written positionally.
// Returns (merged, true) on success, or (nil, false) if the shapes don't
// line up with the record-based model we implement here.
func MergeTemplateMod(base, mod runtime.Object) (runtime.Object, bool) {
	br, ok := TemplateRecord(base, nil)
	if !ok {
		return nil, false
	}
	mr, ok := TemplateRecord(mod, br.Names())
	if !ok {
		return nil, false
	}
	out := runtime.NewRecord()
	for _, k := range br.Names() {
		out.Set(k, br.Fields[k])
	}
	for _, k := range mr.Names() {
		if v := mr.Fields[k]; v != runtime.Undefined {
			out.Set(k, v)
		}
	}
	return out, true
}

// TemplateRecord is the record a template of a record type is: itself, or
// the fields of one held positionally, named by its type or, failing
// that, by names, the base's field names in order.
func TemplateRecord(v runtime.Object, names []string) (*runtime.Record, bool) {
	switch x := v.(type) {
	case *runtime.Record:
		return x, true
	case *runtime.List:
		if len(x.FieldNames) > 0 {
			names = x.FieldNames
		}
		if x.ListType != "" || len(names) == 0 || len(names) != len(x.Elements) {
			return nil, false
		}
		r := runtime.NewRecord()
		for i, name := range names {
			r.Set(name, x.Elements[i])
		}
		return r, true
	}
	return nil, false
}

// roItem is one position of the transformed form of a record of template
// (ETSI ES 201 873-1 15.6.3 g): obj at it, the element of the template it
// comes from, and whether it was made by the transformation, is an
// AnyElementsOrNone, or is sheltered by a permutation, one holding
// AnyElementsOrNone if permStar.
type roItem struct {
	obj            runtime.Object
	orig           int
	made, star     bool
	perm, permStar bool
}

// IsAnyElementsOrNone reports whether e is `*`, with a length or not.
func IsAnyElementsOrNone(e runtime.Object) (*runtime.LengthRestricted, bool) {
	if e == runtime.AnyOrNone {
		return nil, true
	}
	if lr, ok := e.(*runtime.LengthRestricted); ok && lr.Inner == runtime.AnyOrNone {
		return lr, true
	}
	return nil, false
}

// transformedRecordOf is the transformed form of a record of template
// holding a permutation or AnyElementsOrNone: every `* length(n..m)`
// replaced by n AnyElements followed, if m is greater than n, by `*
// length(0..m-n)`. ok is false for any other list.
func transformedRecordOf(list *runtime.List) ([]roItem, bool) {
	if len(list.FieldNames) > 0 || len(list.Elements) > maxTemplateScan {
		return nil, false
	}
	wild := false
	for _, e := range list.Elements {
		_, star := IsAnyElementsOrNone(e)
		if p, ok := e.(*runtime.List); star || ok && p.ListType == runtime.PERMUTATION {
			wild = true
			break
		}
	}
	if !wild {
		return nil, false
	}
	var items []roItem
	for k, e := range list.Elements {
		if p, ok := e.(*runtime.List); ok && p.ListType == runtime.PERMUTATION {
			permStar := false
			for _, pe := range p.Elements {
				_, st := IsAnyElementsOrNone(pe)
				permStar = permStar || st
			}
			for _, pe := range p.Elements {
				items = append(items, roItem{obj: pe, orig: k, perm: true, permStar: permStar})
			}
			continue
		}
		lr, star := IsAnyElementsOrNone(e)
		switch {
		case star && lr != nil:
			for j := 0; j < lr.Min; j++ {
				items = append(items, roItem{obj: runtime.Any, orig: k, made: true})
			}
			if lr.Max < 0 || lr.Max > lr.Min {
				rest := runtime.Object(runtime.AnyOrNone)
				if lr.Max >= 0 {
					rest = &runtime.LengthRestricted{Inner: runtime.AnyOrNone, Min: 0, Max: lr.Max - lr.Min}
				}
				items = append(items, roItem{obj: rest, orig: k, made: true, star: true})
			}
		case star:
			items = append(items, roItem{obj: e, orig: k, star: true})
		default:
			items = append(items, roItem{obj: e, orig: k})
		}
	}
	return items, true
}

// ElementOfRecordOfTemplate is element i of a record of template holding a
// permutation or AnyElementsOrNone, on the right hand side (ETSI ES 201
// 873-1 15.6.3 e, g): an element a permutation shelters, or one at or past
// an AnyElementsOrNone of the transformed form, is an error. ok is false
// for any other list, whose element i is its own.
func ElementOfRecordOfTemplate(list *runtime.List, i int) (runtime.Object, bool) {
	items, ok := transformedRecordOf(list)
	if !ok {
		return nil, false
	}
	for pos, it := range items {
		switch {
		case it.permStar:
			return runtime.Errorf("referencing element %d, which a permutation holding * shelters (ETSI 15.6.3 e)", i), true
		case it.star && pos <= i:
			return runtime.Errorf("referencing element %d, at or past * (ETSI 15.6.3 g)", i), true
		case pos == i && it.perm:
			return runtime.Errorf("referencing element %d, which a permutation shelters (ETSI 15.6.3 e)", i), true
		case pos == i:
			return it.obj, true
		}
	}
	return runtime.Undefined, true
}

// AssignedRecordOfTemplate is the record of template list holding a
// permutation or AnyElementsOrNone after val is assigned to element i
// (ETSI ES 201 873-1 15.6.3 e, g). An element a permutation shelters is
// an error. Before the first AnyElementsOrNone of the transformed form the
// element is replaced, one the transformation made once the elements up
// to it are written out as the transformed form has them. Past that of the
// last element, the elements between are AnyElement, as many as its
// length allows and unbound beyond, and an AnyElementsOrNone with what its
// length leaves follows. Past one of another element is an error. ok is
// false for any other list.
func AssignedRecordOfTemplate(list *runtime.List, i int, val runtime.Object) (runtime.Object, bool) {
	items, ok := transformedRecordOf(list)
	if !ok {
		return nil, false
	}
	sp := -1
	for pos, it := range items {
		if it.permStar && pos <= i {
			return runtime.Errorf("assigning element %d, which a permutation holding * shelters (ETSI 15.6.3 e)", i), true
		}
		if it.perm && pos == i {
			return runtime.Errorf("assigning element %d, which a permutation shelters (ETSI 15.6.3 e)", i), true
		}
		if it.star && sp < 0 {
			sp = pos
		}
	}
	// writtenOut is the elements before element k of the template, as
	// they are but element expand, written out as the transformed form
	// has it, and where each position of that is in it.
	writtenOut := func(k, expand int) ([]runtime.Object, map[int]int) {
		var out []runtime.Object
		at := map[int]int{}
		for pos, it := range items {
			if it.orig >= k {
				break
			}
			if it.perm || it.made && it.orig != expand {
				if pos == 0 || items[pos-1].orig != it.orig {
					out = append(out, list.Elements[it.orig])
				}
				continue
			}
			at[pos] = len(out)
			out = append(out, it.obj)
		}
		return out, at
	}
	result := func(out []runtime.Object) (runtime.Object, bool) {
		return &runtime.List{ListType: list.ListType, Elements: out}, true
	}
	last := len(list.Elements) - 1
	if sp < 0 || i < sp {
		if i >= len(items) {
			out, _ := writtenOut(last+1, -1)
			for gap := i - len(items); gap > 0; gap-- {
				out = append(out, runtime.Undefined)
			}
			return result(append(out, val))
		}
		target := items[i].orig
		var out []runtime.Object
		var at map[int]int
		if items[i].made {
			out, at = writtenOut(target+1, target)
		} else {
			out, at = writtenOut(target, -1)
			at[i] = len(out)
			out = append(out, list.Elements[target])
		}
		out = append(out, list.Elements[target+1:]...)
		out[at[i]] = val
		return result(out)
	}
	if items[sp].orig != last {
		return runtime.Errorf("assigning element %d, past * that is not the last element (ETSI 15.6.3 g)", i), true
	}
	out, _ := writtenOut(last, -1)
	start := 0
	for start < len(items) && items[start].orig < last {
		start++
	}
	lo, hi := 0, -1
	if lr, _ := IsAnyElementsOrNone(list.Elements[last]); lr != nil {
		lo, hi = lr.Min, lr.Max
	}
	for p := start; p < i; p++ {
		if hi < 0 || p-start < hi {
			out = append(out, runtime.Any)
		} else {
			out = append(out, runtime.Undefined)
		}
	}
	out = append(out, val)
	used := i - start + 1
	if lo -= used; lo < 0 {
		lo = 0
	}
	switch {
	case hi < 0 && lo == 0:
		out = append(out, runtime.AnyOrNone)
	case hi < 0:
		out = append(out, &runtime.LengthRestricted{Inner: runtime.AnyOrNone, Min: lo, Max: -1})
	case hi > used:
		out = append(out, &runtime.LengthRestricted{Inner: runtime.AnyOrNone, Min: lo, Max: hi - used})
	}
	return result(out)
}

// WithPatternItem is template s with item i set to r (ETSI ES 201 873-1
// 15.6.1 e-h): a specific value becomes the pattern that matches it
// alone; past the end, as many `?` as fill the gap come first; and a
// pattern of more than one item becomes one group.
func WithPatternItem(s *runtime.String, i int, r *runtime.String) runtime.Object {
	var items [][]rune
	var isRef []bool
	if s.IsPattern {
		items, isRef = PatternItems(s.Value, s.Refs)
	} else {
		for _, c := range s.Value {
			items = append(items, escapePatternRune(c))
		}
	}
	if !r.IsPattern && r.Len() != 1 {
		return runtime.Errorf("assigning a string of length %d to an item of a pattern (ETSI 15.6.1)", r.Len())
	}
	var item []rune
	if r.IsPattern {
		parts, _ := PatternItems(r.Value, r.Refs)
		item = append([]rune(nil), r.Value...)
		if len(parts) > 1 {
			item = append(append([]rune{'('}, item...), ')')
		}
	} else {
		for _, c := range r.Value {
			item = append(item, escapePatternRune(c)...)
		}
	}
	for len(items) < i {
		items = append(items, []rune{'?'})
	}
	if i < len(items) {
		items[i] = item
	} else {
		items = append(items, item)
	}
	// The references among the items left stay one item each.
	var out []rune
	var refs [][2]int
	for k, it := range items {
		if k != i && k < len(isRef) && isRef[k] {
			refs = append(refs, [2]int{len(out), len(out) + len(it)})
		}
		out = append(out, it...)
	}
	return &runtime.String{Value: out, IsPattern: true, NoCase: s.NoCase || r.NoCase,
		PatternComplex: s.PatternComplex || r.PatternComplex || r.IsPattern && len(item) > 1, Refs: refs}
}

// escapePatternRune is c as a pattern matches it alone.
func escapePatternRune(c rune) []rune {
	if strings.ContainsRune(`?*\[]{}()#+|"`, c) {
		return []rune{'\\', c}
	}
	return []rune{c}
}

// ExtractTemplateValue is `t.value` (ETSI 15.15): the string of a pattern
// and the value of a template holding one, with any matching attribute -
// ifpresent, length - removed. Any other template is an error.
func ExtractTemplateValue(o runtime.Object) runtime.Object {
	switch v := o.(type) {
	case *runtime.IfPresent:
		return ExtractTemplateValue(v.Inner)
	case *runtime.LengthRestricted:
		return ExtractTemplateValue(v.Inner)
	case *runtime.String:
		if v.IsPattern {
			return runtime.NewCharstringFromRunes(append([]rune(nil), v.Value...))
		}
	case *runtime.Binarystring:
		// An empty string shares the negative Value of a template
		// with matching symbols; the symbols tell them apart.
		if v.Value == nil || (v.Value.Sign() < 0 && strings.ContainsAny(v.String, "?*")) {
			return runtime.Errorf("value extraction: %s is not a value (ETSI 15.15)", v.Inspect())
		}
		return v
	}
	if !IsConcreteValue(o) {
		what := "an unbound template"
		if o != nil && o != runtime.Undefined {
			what = o.Inspect()
		}
		return runtime.Errorf("value extraction: %s holds neither a value nor a pattern (ETSI 15.15)", what)
	}
	return o
}

// PresentnessConversion is `t.present` (ETSI 15.14): AnyValueOrNone
// becomes AnyValue, an ifpresent attribute is dropped, a template list
// loses its omit alternatives and has the others converted, and a
// template that matches omit alone is an error. Others are unchanged.
func PresentnessConversion(o runtime.Object) runtime.Object {
	switch v := o.(type) {
	case *runtime.IfPresent:
		return PresentnessConversion(v.Inner)
	case *runtime.LengthRestricted:
		inner := PresentnessConversion(v.Inner)
		if runtime.IsError(inner) {
			return inner
		}
		return &runtime.LengthRestricted{Inner: inner, Min: v.Min, Max: v.Max}
	case *runtime.List:
		if v.ListType != runtime.VALUE_LIST {
			return o
		}
		out := &runtime.List{ListType: runtime.VALUE_LIST}
		for _, e := range v.Elements {
			if e == runtime.Omit {
				continue
			}
			c := PresentnessConversion(e)
			if runtime.IsError(c) {
				return c
			}
			out.Elements = append(out.Elements, c)
		}
		if len(out.Elements) == 0 {
			return runtime.Errorf("presentness conversion of a template that matches only omit (ETSI 15.14)")
		}
		if len(out.Elements) == 1 {
			return out.Elements[0]
		}
		return out
	}
	switch o {
	case runtime.AnyOrNone:
		return runtime.Any
	case runtime.Omit:
		return runtime.Errorf("presentness conversion of a template that matches only omit (ETSI 15.14)")
	}
	return o
}

// HasMatchingMechanism reports whether o, or a field or an element of it at
// any depth, is a matching mechanism rather than a value (ETSI B.1): `?`,
// `*`, a range, a length restriction, ifpresent, a pattern or a binary
// string with wildcards, an enumerated value of match ranges, a value
// list, complement, subset, superset or permutation.
func HasMatchingMechanism(o runtime.Object) bool {
	switch v := o.(type) {
	case nil:
		return false
	case *runtime.Range, *runtime.LengthRestricted, *runtime.IfPresent:
		return true
	case *runtime.EnumValue:
		return v.IsTemplate()
	case *runtime.String:
		return v.IsPattern
	case *runtime.Binarystring:
		return v.Value != nil && v.Value.Sign() < 0
	case *runtime.List:
		if v.ListType != runtime.RECORD_OF && v.ListType != runtime.SET_OF {
			return true
		}
		for _, e := range v.Elements {
			if HasMatchingMechanism(e) {
				return true
			}
		}
		return false
	case *runtime.Record:
		for _, f := range v.Fields {
			if HasMatchingMechanism(f) {
				return true
			}
		}
		return false
	}
	return o == runtime.Any || o == runtime.AnyOrNone
}

// IsConcreteValue reports whether o is a fully-initialised concrete
// value rather than a template still carrying a matching mechanism.
// Used by `isvalue` (ETSI 7.1.8.3) and `.value` (15.15): `?`, `*`,
// charstring patterns, value ranges, length restrictions and the other
// matching mechanisms are templates, not values; `null` and ordinary
// scalars / structures are values.
func IsConcreteValue(o runtime.Object) bool {
	if o == nil || o == runtime.Undefined || o == runtime.Omit || o == runtime.Any || o == runtime.AnyOrNone {
		return false
	}
	switch v := o.(type) {
	case *runtime.String:
		return !v.IsPattern
	case *runtime.Range:
		return false
	case *runtime.LengthRestricted:
		return false
	case *runtime.List:
		// A record/set value (field-name metadata present) is a
		// concrete value only when every declared field is supplied;
		// a missing trailing field makes it partially initialised and
		// therefore not a value (ETSI 7.1.8.3, Sem_1901_002).
		if len(v.FieldNames) > 0 && len(v.Elements) < len(v.FieldNames) {
			return false
		}
	}
	// Nor is a value list, complement, subset, superset or permutation,
	// an ifpresent, a bit, hex or octet string with wildcards, an enum
	// template, or a structure with a matching mechanism in it (7.1.8.3).
	return !HasMatchingMechanism(o)
}

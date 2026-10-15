package template_test

import (
	"fmt"
	"testing"

	"github.com/nokia/ntt/runtime/template"
)

// The tests use minimal hand-rolled Value implementations so the engine
// can be exercised without the runtime/object package. The actual
// runtime ships an adapter in template/adapter.go that wraps
// runtime.Object.

type intV int

func (i intV) Kind() template.ValueKind { return template.KindInt }
func (i intV) Inspect() string          { return fmt.Sprintf("%d", int(i)) }
func (i intV) Equal(v template.Value) bool {
	o, ok := v.(intV)
	return ok && o == i
}
func (i intV) Less(v template.Value) bool {
	o, ok := v.(intV)
	return ok && int(i) < int(o)
}

type charV string

func (c charV) Kind() template.ValueKind { return template.KindCharstring }
func (c charV) Inspect() string          { return fmt.Sprintf("%q", string(c)) }
func (c charV) Equal(v template.Value) bool {
	o, ok := v.(charV)
	return ok && o == c
}
func (c charV) Length() int    { return len(c) }
func (c charV) CharCount() int { return len(c) }

type recV map[string]template.Value

func (r recV) Kind() template.ValueKind { return template.KindRecord }
func (r recV) Inspect() string          { return fmt.Sprintf("%v", map[string]template.Value(r)) }
func (r recV) Equal(v template.Value) bool {
	o, ok := v.(recV)
	if !ok || len(o) != len(r) {
		return false
	}
	for k, vv := range r {
		ov, present := o[k]
		if !present || !ov.Equal(vv) {
			return false
		}
	}
	return true
}
func (r recV) Field(name string) (template.Value, bool) {
	v, ok := r[name]
	return v, ok
}

type listV []template.Value

func (l listV) Kind() template.ValueKind { return template.KindRecordOf }
func (l listV) Inspect() string          { return fmt.Sprintf("%v", []template.Value(l)) }
func (l listV) Equal(v template.Value) bool {
	o, ok := v.(listV)
	if !ok || len(o) != len(l) {
		return false
	}
	for i, x := range l {
		if !x.Equal(o[i]) {
			return false
		}
	}
	return true
}
func (l listV) Length() int                  { return len(l) }
func (l listV) Element(i int) template.Value { return l[i] }

func TestAnyValue(t *testing.T) {
	if !(template.AnyValue{}).Match(intV(42)) {
		t.Error("unconstrained ? should match anything")
	}
	if !(template.AnyValue{WantKind: template.KindInt}).Match(intV(42)) {
		t.Error("?-of-int should match an int")
	}
	if (template.AnyValue{WantKind: template.KindInt}).Match(charV("hi")) {
		t.Error("?-of-int should not match a string")
	}
}

func TestLiteralAndAnyOrNone(t *testing.T) {
	if !(template.Literal{Value: intV(1)}).Match(intV(1)) {
		t.Error("literal should match equal value")
	}
	if (template.Literal{Value: intV(1)}).Match(intV(2)) {
		t.Error("literal should not match unequal value")
	}
	if !(template.AnyOrNoneValue{}).Match(intV(99)) {
		t.Error("* must match every value")
	}
}

func TestRange(t *testing.T) {
	r := template.Range{Lower: intV(1), Upper: intV(5)}
	for _, n := range []int{1, 3, 5} {
		if !r.Match(intV(n)) {
			t.Errorf("%d should be in 1..5", n)
		}
	}
	for _, n := range []int{0, 6} {
		if r.Match(intV(n)) {
			t.Errorf("%d should not be in 1..5", n)
		}
	}
}

func TestValueList(t *testing.T) {
	vl := template.ValueList{Values: []template.Value{intV(1), intV(2), intV(3)}}
	if !vl.Match(intV(2)) {
		t.Error("value list should match present value")
	}
	if vl.Match(intV(4)) {
		t.Error("value list should not match missing value")
	}
}

func TestComplement(t *testing.T) {
	c := template.Complement{Inner: template.Literal{Value: intV(5)}}
	if !c.Match(intV(4)) {
		t.Error("complement should match non-equal value")
	}
	if c.Match(intV(5)) {
		t.Error("complement should not match equal value")
	}
}

func TestAndOr(t *testing.T) {
	pos := template.Range{Lower: intV(0), Upper: intV(10)}
	even := template.ValueList{Values: []template.Value{intV(0), intV(2), intV(4), intV(6), intV(8), intV(10)}}
	combo := template.And{Parts: []template.Template{pos, even}}
	if !combo.Match(intV(4)) {
		t.Error("4 should match (0..10) and even")
	}
	if combo.Match(intV(3)) {
		t.Error("3 should not match (0..10) and even")
	}

	either := template.Or{Parts: []template.Template{template.Literal{Value: intV(-1)}, even}}
	if !either.Match(intV(-1)) || !either.Match(intV(4)) {
		t.Error("or should match each branch")
	}
	if either.Match(intV(3)) {
		t.Error("or should not match neither branch")
	}
}

func TestLength(t *testing.T) {
	l := template.Length{Inner: template.AnyValue{}, Min: 2, Max: 4}
	if !l.Match(charV("hi")) {
		t.Error("length(2..4) should match length 2")
	}
	if l.Match(charV("h")) {
		t.Error("length(2..4) should not match length 1")
	}
	if l.Match(charV("hellos")) {
		t.Error("length(2..4) should not match length 6")
	}
}

func TestIfPresent(t *testing.T) {
	ip := template.IfPresent{Inner: template.Literal{Value: intV(7)}}
	if !ip.Match(template.Omit()) {
		t.Error("ifpresent should match omit")
	}
	if !ip.Match(intV(7)) {
		t.Error("ifpresent should match inner")
	}
	if ip.Match(intV(8)) {
		t.Error("ifpresent should not match non-inner value")
	}
}

func TestRecordTemplate(t *testing.T) {
	r := template.RecordTemplate{Fields: map[string]template.Template{
		"a": template.Literal{Value: intV(1)},
		"b": template.AnyValue{},
	}}
	if !r.Match(recV{"a": intV(1), "b": intV(2)}) {
		t.Error("record template should match")
	}
	if r.Match(recV{"a": intV(2), "b": intV(2)}) {
		t.Error("record template should not match mismatching field")
	}
}

func TestRecordTemplate_Detail(t *testing.T) {
	r := template.RecordTemplate{Fields: map[string]template.Template{
		"a": template.Literal{Value: intV(1)},
		"b": template.Literal{Value: intV(2)},
	}}
	v := recV{"a": intV(1), "b": intV(99)}
	res := template.Detail(r, v)
	if res.OK {
		t.Fatal("expected mismatch")
	}
	if res.Path != "b" {
		t.Errorf("Path = %q, want %q", res.Path, "b")
	}
}

func TestRecordOfTemplate_Wildcard(t *testing.T) {
	r := template.RecordOfTemplate{
		Elements: []template.Template{
			template.Literal{Value: intV(1)},
			template.AnyOrNoneValue{},
			template.Literal{Value: intV(9)},
		},
		LengthMax: -1,
	}
	tests := []struct {
		name  string
		input listV
		want  bool
	}{
		{"empty middle", listV{intV(1), intV(9)}, true},
		{"single middle", listV{intV(1), intV(5), intV(9)}, true},
		{"multi middle", listV{intV(1), intV(5), intV(6), intV(9)}, true},
		{"missing tail", listV{intV(1), intV(2)}, false},
		{"missing head", listV{intV(0), intV(9)}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := r.Match(tc.input); got != tc.want {
				t.Errorf("Match(%v) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestRecordOfTemplate_LengthConstraint(t *testing.T) {
	r := template.RecordOfTemplate{
		Elements:  []template.Template{template.AnyOrNoneValue{}},
		LengthMin: 2,
		LengthMax: 3,
	}
	if !r.Match(listV{intV(1), intV(2)}) {
		t.Error("length 2 should match")
	}
	if r.Match(listV{intV(1)}) {
		t.Error("length 1 should not match")
	}
	if r.Match(listV{intV(1), intV(2), intV(3), intV(4)}) {
		t.Error("length 4 should not match")
	}
}

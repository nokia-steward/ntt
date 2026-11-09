package runtime

import (
	"math"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUndefined(t *testing.T) {
	assert.Equal(t, "undefined", Undefined{}.String())
}

func TestIsCompatible(t *testing.T) {
	assert.NoError(t, IsCompatible(42, nil))
	assert.NoError(t, IsCompatible(Undefined{}, IntegerType))

	assert.NoError(t, IsCompatible(42, IntegerType))
	assert.NoError(t, IsCompatible(true, BooleanType))
	assert.NoError(t, IsCompatible(1.5, FloatType))
	assert.NoError(t, IsCompatible("hi", CharstringType))
	assert.NoError(t, IsCompatible("hi", UniversalCharstringType))
	assert.NoError(t, IsCompatible(42, &NamedType{Name: "alias", Type: IntegerType}))

	assert.Error(t, IsCompatible("x", IntegerType))
	assert.Error(t, IsCompatible(42, BooleanType))

	ro := &RecordOfType{ElementType: IntegerType}
	assert.NoError(t, IsCompatible([]any{1, 2}, ro))
	assert.Error(t, IsCompatible(42, ro))

	mp := &MapType{KeyType: IntegerType, ValueType: CharstringType}
	assert.NoError(t, IsCompatible(map[any]any{1: "a"}, mp))
	assert.Error(t, IsCompatible([]any{}, mp))

	t.Run("MapKeys", func(t *testing.T) {
		m := map[any]any{"b": 2, "a": 1}
		keys := MapKeys(m, CharstringType)
		assert.Equal(t, []any{"a", "b"}, keys)
		assert.Equal(t, []any{1, 2}, MapValues(m, IntegerType))
	})

	assert.NoError(t, IsCompatible([]any{1}, RecordOfType{ElementType: IntegerType}))
	rec := RecordType{Fields: []*Var{{Name: "x", Type: IntegerType}}}
	assert.NoError(t, IsCompatible(map[string]any{"x": 1}, rec))
	assert.Error(t, IsCompatible(map[string]any{"x": 1, "y": 2}, rec))
	assert.Error(t, IsCompatible(42, rec))

	assert.NoError(t, IsCompatible(1, Subtype{BaseType: IntegerType}))
}

func TestTemplateValuesAndCompatibility(t *testing.T) {
	assert.Equal(t, "omit", OmitVal.String())
	assert.Equal(t, "?", Any.String())
	assert.Equal(t, "*", AnyOrNone.String())
	assert.Equal(t, "infinity", PosInfinity.String())
	assert.Equal(t, "-infinity", NegInfinity.String())
	assert.Equal(t, "(1..10)", Range{Lo: 1, Hi: 10}.String())
	assert.Equal(t, `"abc*"`, Pattern("abc*").String())
	assert.Equal(t, "A", EnumValue{Name: "A", Val: 0}.String())

	ut := UnionType{Fields: []*Var{{Name: "a", Type: IntegerType, Index: 9}}}
	assert.Len(t, UnionFields(ut), 1)
	assert.Len(t, UnionFields(&ut), 1)
	assert.Nil(t, UnionFields(IntegerType))
	assert.NoError(t, IsCompatible([]any{1}, ut))
	assert.Error(t, IsCompatible(42, ut))
	assert.Error(t, IsCompatible([]any{Any}, ut))

	// ApplyIfPresent normalization
	assert.Equal(t, AnyOrNone, ApplyIfPresent(Any))
	assert.Equal(t, AnyOrNone, ApplyIfPresent(AnyOrNone))
	assert.Equal(t, ModifiedTemplate{Val: AnyOrNone, Length: Range{Lo: 1, Hi: 4}}, ApplyIfPresent(ApplyLength(Any, Range{Lo: 1, Hi: 4})))
	assert.Equal(t, ModifiedTemplate{Val: 2, IfPresent: true}, ApplyIfPresent(2))
	assert.Equal(t, ModifiedTemplate{Val: "abc", Length: 3, IfPresent: true}, ApplyIfPresent(ApplyLength("abc", 3)))

	// Unrestricted template (permitt)
	ttInt := TemplateType{BaseType: IntegerType}
	assert.NoError(t, IsCompatible(42, ttInt))
	assert.NoError(t, IsCompatible(Undefined{}, ttInt))
	assert.NoError(t, IsCompatible(OmitVal, ttInt))
	assert.NoError(t, IsCompatible(Any, ttInt))
	assert.NoError(t, IsCompatible(AnyOrNone, ttInt))
	assert.NoError(t, IsCompatible(Range{Lo: NegInfinity, Hi: 10}, ttInt))
	assert.NoError(t, IsCompatible(ValueList{1, 2, 3}, ttInt))
	assert.NoError(t, IsCompatible(TemplateList{Kind: "complement", Items: []any{1, 2}}, ttInt))
	assert.NoError(t, IsCompatible(ApplyIfPresent(2), ttInt))
	assert.Error(t, IsCompatible("not-an-int", ttInt))
	assert.Error(t, IsCompatible(ApplyIfPresent("not-an-int"), ttInt))

	// template(omit) (permito)
	ttOmit := TemplateType{Flags: Omit, BaseType: IntegerType}
	assert.NoError(t, IsCompatible(42, ttOmit))
	assert.NoError(t, IsCompatible(Undefined{}, ttOmit))
	assert.NoError(t, IsCompatible(OmitVal, ttOmit))
	assert.Error(t, IsCompatible(Any, ttOmit))
	assert.Error(t, IsCompatible(AnyOrNone, ttOmit))
	assert.Error(t, IsCompatible(Range{Lo: 1, Hi: 5}, ttOmit))
	assert.Error(t, IsCompatible(ValueList{1, 2}, ttOmit))
	assert.Error(t, IsCompatible(TemplateList{Kind: "complement", Items: []any{1}}, ttOmit))
	assert.Error(t, IsCompatible(ApplyIfPresent(1), ttOmit))

	// template(present) (permitp)
	ttPres := TemplateType{Flags: Present, BaseType: IntegerType}
	assert.NoError(t, IsCompatible(42, ttPres))
	assert.NoError(t, IsCompatible(Undefined{}, ttPres))
	assert.NoError(t, IsCompatible(Any, ttPres))
	assert.NoError(t, IsCompatible(Range{Lo: 1, Hi: 5}, ttPres))
	assert.NoError(t, IsCompatible(ValueList{1, 2}, ttPres))
	assert.NoError(t, IsCompatible(TemplateList{Kind: "complement", Items: []any{1}}, ttPres))
	assert.NoError(t, IsCompatible(ApplyLength(Any, 3), ttPres))
	assert.Error(t, IsCompatible(OmitVal, ttPres))
	assert.Error(t, IsCompatible(AnyOrNone, ttPres))
	assert.Error(t, IsCompatible(ApplyIfPresent(2), ttPres))
	assert.Error(t, IsCompatible(ApplyLength(AnyOrNone, 3), ttPres))

	// Compound template and non-template record with optional field
	rec := RecordType{Fields: []*Var{
		{Name: "a", Type: IntegerType, Index: 0},
		{Name: "b", Type: IntegerType, Index: 1, Flags: Optional},
	}}
	assert.NoError(t, IsCompatible([]any{1, OmitVal}, rec))
	assert.NoError(t, IsCompatible([]any{1, Undefined{}}, rec))
	assert.Error(t, IsCompatible([]any{OmitVal, 1}, rec))
	assert.Error(t, IsCompatible([]any{1, Any}, rec))

	assert.NoError(t, IsCompatible([]any{1, OmitVal}, TemplateType{Flags: Omit, BaseType: rec}))
	assert.Error(t, IsCompatible([]any{1, Any}, TemplateType{Flags: Omit, BaseType: rec}))
	assert.NoError(t, IsCompatible([]any{1, AnyOrNone}, TemplateType{Flags: Present, BaseType: rec}))
	assert.Error(t, IsCompatible(OmitVal, TemplateType{Flags: Present, BaseType: rec}))
}

func TestSetVerdictRank(t *testing.T) {
	c := NewComponent("c")
	assert.Equal(t, VerdictNone, c.Verdict())

	order := []Verdict{VerdictNone, VerdictPass, VerdictInconc, VerdictFail, VerdictError}
	for i, v := range order {
		assert.Equal(t, order[max(i-1, 0)], c.SetVerdict(v))
		assert.Equal(t, v, c.Verdict())
		for _, weaker := range order[:i] {
			assert.Equal(t, v, c.SetVerdict(weaker))
			assert.Equal(t, v, c.Verdict())
		}
	}

	assert.Equal(t, VerdictError, c.SetVerdict(""))
	assert.Equal(t, VerdictError, c.Verdict())
	assert.Equal(t, VerdictError, c.SetVerdict(VerdictError))
	assert.Equal(t, VerdictError, c.Verdict())
}

func TestSetVerdictZeroRanksAsNone(t *testing.T) {
	c := &Component{done: make(chan struct{})}
	assert.Equal(t, Verdict(""), c.Verdict())
	assert.Equal(t, Verdict(""), c.SetVerdict(VerdictNone))
	assert.Equal(t, Verdict(""), c.Verdict())
	assert.Equal(t, Verdict(""), c.SetVerdict(""))
	assert.Equal(t, Verdict(""), c.Verdict())

	assert.Equal(t, Verdict(""), c.SetVerdict(VerdictPass))
	assert.Equal(t, VerdictPass, c.Verdict())
	assert.Equal(t, VerdictPass, c.SetVerdict(""))
	assert.Equal(t, VerdictPass, c.Verdict())
	assert.Equal(t, VerdictPass, c.SetVerdict(VerdictNone))
	assert.Equal(t, VerdictPass, c.Verdict())
}

func TestComponentFinish(t *testing.T) {
	c := NewComponent("c")
	assert.False(t, c.Done())
	c.Finish()
	assert.True(t, c.Done())
	assert.NotPanics(t, c.Finish)
	assert.True(t, c.Done())
}

func TestComponentStop(t *testing.T) {
	c := NewComponent("c")
	assert.False(t, c.Stopped())
	c.Stop()
	assert.True(t, c.Stopped())
	c.Stop()
	assert.True(t, c.Stopped())
}

func TestComponentConcurrent(t *testing.T) {
	c := NewComponent("c")
	var wg sync.WaitGroup
	verdicts := []Verdict{"", VerdictNone, VerdictPass, VerdictInconc, VerdictFail, VerdictError}
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c.SetVerdict(verdicts[i%len(verdicts)])
			_ = c.Verdict()
			c.Stop()
			_ = c.Stopped()
			c.Finish()
			_ = c.Done()
		}(i)
	}
	wg.Wait()
	assert.Equal(t, VerdictError, c.Verdict())
	assert.True(t, c.Stopped())
	assert.True(t, c.Done())
}

func TestFormatFloat(t *testing.T) {
	assert.Equal(t, "2.75", FormatFloat(2.75))
	assert.Equal(t, "0.0100000000000000002082", FormatFloat(0.01))
	assert.Equal(t, "-1.0", FormatFloat(-1))
	assert.Equal(t, "0.0", FormatFloat(0))
	assert.Equal(t, "infinity", FormatFloat(math.Inf(1)))
}

func TestBinary(t *testing.T) {
	b := func(unit byte, digits string) Binary { return Binary{Unit: unit, Digits: digits} }
	and, err := Bitwise(b('B', "1100"), b('B', "1010"), func(x, y uint64) uint64 { return x & y })
	assert.NoError(t, err)
	assert.Equal(t, "'1000'B", and.String())
	not, err := Bitwise(b('O', "0F"), Binary{}, func(x, _ uint64) uint64 { return ^x })
	assert.NoError(t, err)
	assert.Equal(t, "'F0'O", not.String())

	shifted, err := Shift(b('B', "1100"), 1, true, false)
	assert.NoError(t, err)
	assert.Equal(t, b('B', "1000"), shifted)
	rotated, err := Shift(b('B', "1100"), 1, true, true)
	assert.NoError(t, err)
	assert.Equal(t, b('B', "1001"), rotated)
	rotated, err = Shift("abc", 1, false, true)
	assert.NoError(t, err)
	assert.Equal(t, "cab", rotated)
}

func TestConvert(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []any
		want any
	}{
		{"int2str", []any{42}, "42"},
		{"str2int", []any{"-7"}, -7},
		{"int2bit", []any{5, 4}, Binary{Unit: 'B', Digits: "0101"}},
		{"oct2int", []any{Binary{Unit: 'O', Digits: "FF"}}, 255},
		{"char2int", []any{"A"}, 65},
		{"float2int", []any{2.7}, 2},
	} {
		got, err := Convert(tt.name, tt.args...)
		assert.NoError(t, err, tt.name)
		assert.Equal(t, tt.want, got, tt.name)
	}
}

func TestPatternMatch(t *testing.T) {
	ref := func(name string) any { return map[string]any{"cs": "b+"}[name] }
	for _, tt := range []struct {
		pattern, s string
		want       bool
	}{
		{"a{cs}c", "abbc", true},
		{"a{cs}c", "ac", false},
		{`a\d`, "a1", true},
		{"a[A-Z]#(2)", "aZZ", true},
		{"[^x]", "x", false},
	} {
		got, err := Pattern(tt.pattern).Match(tt.s, ref)
		assert.NoError(t, err, tt.pattern)
		assert.Equal(t, tt.want, got, tt.pattern+" ~ "+tt.s)
	}
}

func TestTimer(t *testing.T) {
	tm := &Timer{Name: "t", Default: 0.5}
	assert.Equal(t, "{inactive,0.5}", tm.String())
	assert.Error(t, tm.Start(-1))
	assert.NoError(t, tm.Start(10))
	assert.True(t, tm.Running())
	assert.True(t, tm.Stop())
	assert.False(t, tm.Running())
	ready, running, _ := tm.Timeout()
	assert.False(t, ready || running)

	assert.NoError(t, tm.Start(0))
	ready, _, _ = tm.Timeout()
	assert.True(t, ready)
	ready, running, _ = tm.Timeout()
	assert.False(t, ready || running)
}

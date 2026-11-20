package builtins_test

import (
	"testing"

	"github.com/nokia/ntt/builtins"
	"github.com/nokia/ntt/runtime"
)

func valueList(objs ...runtime.Object) *runtime.List {
	return &runtime.List{ListType: runtime.VALUE_LIST, Elements: objs}
}

// A template holds a matching mechanism, at any depth of a record or a
// record of, or is a value; values are concrete, matching mechanisms not.
func TestHasMatchingMechanism(t *testing.T) {
	one := runtime.NewInt(1)
	rec := runtime.NewRecord()
	rec.Set("a", one)
	wild := runtime.NewRecord()
	wild.Set("a", runtime.Any)
	for name, c := range map[string]struct {
		o        runtime.Object
		mm, conc bool
	}{
		"a value":            {one, false, true},
		"AnyValue":           {runtime.Any, true, false},
		"AnyValueOrNone":     {runtime.AnyOrNone, true, false},
		"ifpresent":          {&runtime.IfPresent{Inner: one}, true, false},
		"a value list":       {valueList(one, runtime.NewInt(2)), true, false},
		"a record of values": {rec, false, true},
		"a record with ?":    {wild, true, false},
		"a record of ?":      {runtime.NewList(one, runtime.Any), true, false},
	} {
		if got := builtins.HasMatchingMechanism(c.o); got != c.mm {
			t.Errorf("%s: HasMatchingMechanism = %v, want %v", name, got, c.mm)
		}
		if got := builtins.IsConcreteValue(c.o); got != c.conc {
			t.Errorf("%s: IsConcreteValue = %v, want %v", name, got, c.conc)
		}
	}
}

// A template matches omit as a whole when it is omit, AnyValueOrNone,
// with ifpresent, or a template list holding one of these (ETSI ES 201
// 873-1 15.8, table 12).
func TestMatchesOmitAsAWhole(t *testing.T) {
	one := runtime.NewInt(1)
	for name, c := range map[string]struct {
		o    runtime.Object
		want bool
	}{
		"omit":                {runtime.Omit, true},
		"AnyValueOrNone":      {runtime.AnyOrNone, true},
		"ifpresent":           {&runtime.IfPresent{Inner: one}, true},
		"a list holding omit": {valueList(one, runtime.Omit), true},
		"AnyValue":            {runtime.Any, false},
		"a value":             {one, false},
		"a list of values":    {valueList(one, runtime.NewInt(2)), false},
	} {
		if got := builtins.MatchesOmitAsAWhole(c.o); got != c.want {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
}

// The presentness of a template drops ifpresent and the omit of a value
// list (15.14); the value of a template is that of one that is a value,
// and an error otherwise (15.15).
func TestPresentnessAndValue(t *testing.T) {
	one := runtime.NewInt(1)
	if got := builtins.PresentnessConversion(&runtime.IfPresent{Inner: one}); !got.Equal(one) {
		t.Errorf("presentness of 1 ifpresent = %s, want 1", got.Inspect())
	}
	// A list of one is that one.
	if got := builtins.PresentnessConversion(valueList(one, runtime.Omit)); !got.Equal(one) {
		t.Errorf("presentness of (1, omit) = %s, want 1", got.Inspect())
	}
	got := builtins.PresentnessConversion(valueList(one, runtime.Omit, runtime.NewInt(2)))
	if l, ok := got.(*runtime.List); !ok || len(l.Elements) != 2 {
		t.Errorf("presentness of (1, omit, 2) = %s, want (1, 2)", got.Inspect())
	}
	if got := builtins.ExtractTemplateValue(&runtime.IfPresent{Inner: one}); !got.Equal(one) {
		t.Errorf("value of 1 ifpresent = %s, want 1", got.Inspect())
	}
	if got := builtins.ExtractTemplateValue(runtime.Any); !runtime.IsError(got) {
		t.Errorf("value of ? = %s, want an error", got.Inspect())
	}
}

// A modified template takes the fields its modification gives, and the
// base's for the others and those it leaves unchanged (15.5).
func TestMergeTemplateMod(t *testing.T) {
	base := runtime.NewRecord()
	base.Set("a", runtime.NewInt(1))
	base.Set("b", runtime.NewInt(2))
	mod := runtime.NewRecord()
	mod.Set("b", runtime.NewInt(3))
	mod.Set("a", runtime.Undefined)
	got, ok := builtins.MergeTemplateMod(base, mod)
	r, isRec := got.(*runtime.Record)
	if !ok || !isRec {
		t.Fatalf("MergeTemplateMod = %v, %v", got, ok)
	}
	if a, _ := r.Get("a"); !a.Equal(runtime.NewInt(1)) {
		t.Errorf("a = %s, want 1 (kept from the base)", a.Inspect())
	}
	if b, _ := r.Get("b"); !b.Equal(runtime.NewInt(3)) {
		t.Errorf("b = %s, want 3 (the modification's)", b.Inspect())
	}
}

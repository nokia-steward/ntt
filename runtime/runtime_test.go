package runtime_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/nokia/ntt/runtime"
	"github.com/stretchr/testify/assert"
)

func TestAddBuiltin(t *testing.T) {
	// Do not run this test in parallel, as it modifies global state.
	runtime.ResetBuiltins()
	assert.NoError(t, runtime.AddBuiltin("foo", nil))
	assert.True(t, errors.Is(runtime.AddBuiltin("foo", nil), runtime.ErrExists))
	assert.True(t, errors.Is(runtime.AddBuiltin("foo()", nil), runtime.ErrExists))
	assert.True(t, errors.Is(runtime.AddBuiltin("foo(integer x)", nil), runtime.ErrExists))
}

func TestBuiltinArgumentChecks(t *testing.T) {
	// Do not run this test in parallel, as it modifies global state.
	tests := []struct {
		sig  string
		args []runtime.Object
		want error
	}{
		{sig: "foo"},
		{sig: "foo", args: []runtime.Object{nil, runtime.NewInt(23), nil}},
		{sig: "foo()", args: []runtime.Object{}},
		{sig: "foo()", args: []runtime.Object{nil, runtime.NewInt(23), nil}},
		{sig: "foo(integer x)", args: []runtime.Object{nil}, want: runtime.ErrTypeMismatch},
		{sig: "foo(integer x)", args: []runtime.Object{runtime.NewInt(23)}},
		{sig: "foo(integer x, boolean y)", args: []runtime.Object{runtime.NewInt(23)}, want: runtime.ErrInvalidArgCount},
		{sig: "foo(integer x, boolean y)", args: []runtime.Object{runtime.NewInt(23), runtime.NewBool(true)}},
		{sig: "foo(integer x, bool y)", args: []runtime.Object{runtime.NewInt(23), runtime.NewBool(true)}, want: runtime.ErrNotImplemented},
		{sig: "foo(integer x[23])", args: []runtime.Object{nil}, want: runtime.ErrNotImplemented},
	}
	for _, tt := range tests {
		t.Run(t.Name(), func(t *testing.T) {
			name := tt.sig
			if i := strings.Index(tt.sig, "("); i >= 0 {
				name = tt.sig[:i]
			}
			runtime.ResetBuiltins()
			env := runtime.NewEnv(nil)
			if obj, ok := env.Get(name); ok {
				t.Errorf("unexpected object %v", obj)
			}
			assert.NoError(t, runtime.AddBuiltin(tt.sig, func(obj ...runtime.Object) runtime.Object { return nil }))
			obj, ok := env.Get(name)
			if !ok {
				t.Fatal("builtin not found")
			}

			bi, ok := obj.(*runtime.Builtin)
			if !ok {
				t.Fatalf("unexpected object: %v", obj)
			}

			switch val := bi.Fn(tt.args...).(type) {
			case nil:
				if tt.want != nil {
					t.Errorf("got nil, want %v", tt.want)
				}
			case error:
				if !errors.Is(val, tt.want) {
					t.Errorf("got %v, want %v", val, tt.want)
				}

			default:
				t.Errorf("unexpected return value: %v", val)
			}
		})
	}
}

// TestSetOfEqual: two set of values are equal when their elements pair
// up one to one, in any order; a value list compared with a set of is
// one of its type. Order mattered: only lists of one repeated element
// were equal.
func TestSetOfEqual(t *testing.T) {
	i := func(v ...int) []runtime.Object {
		var l []runtime.Object
		for _, x := range v {
			l = append(l, runtime.NewInt(x))
		}
		return l
	}
	for _, c := range []struct {
		a, b *runtime.List
		want bool
	}{
		{runtime.NewSetOf(i(1, 2)...), runtime.NewSetOf(i(2, 1)...), true},
		{runtime.NewSetOf(i(1, 1, 2)...), runtime.NewSetOf(i(1, 2, 2)...), false},
		{runtime.NewSetOf(i(1, 2)...), runtime.NewSetOf(i(1, 2, 2)...), false},
		{runtime.NewSetOf(i(0, 0, 1)...), runtime.NewList(i(0, 1, 0)...), true},
		{runtime.NewList(i(1, 2)...), runtime.NewSetOf(i(2, 1)...), true},
		{runtime.NewList(i(1, 2)...), runtime.NewList(i(2, 1)...), false},
		{runtime.NewSuperset(i(1, 2)...), runtime.NewList(i(2, 1)...), false},
	} {
		if got := c.a.Equal(c.b); got != c.want {
			t.Errorf("%s == %s: %v, want %v", c.a.Inspect(), c.b.Inspect(), got, c.want)
		}
	}
}

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

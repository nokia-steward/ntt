package syntax_test

import (
	"testing"

	"github.com/nokia/ntt/ttcn3/syntax"
	"github.com/stretchr/testify/assert"
)

func TestFieldName(t *testing.T) {
	root := syntax.Parse([]byte(`type record R { integer a, integer, M.T, P<integer> optional, record {} }`))
	if err := root.Err(); err != nil {
		t.Fatal(err)
	}
	var names []string
	root.Inspect(func(n syntax.Node) bool {
		if f, ok := n.(*syntax.Field); ok {
			names = append(names, syntax.Name(f))
			return false
		}
		return true
	})
	assert.Equal(t, []string{"a", "integer", "T", "P", ""}, names)
}

func TestDoc(t *testing.T) {
	t.Parallel()

	testDoc := func(t *testing.T, input string) string {
		root := syntax.Parse([]byte(input), syntax.WithFilename(t.Name()))
		if err := root.Err(); err != nil {
			t.Fatal(err)
		}
		return syntax.Doc(root)
	}

	t.Run("empty", func(t *testing.T) {
		input := `module Foo {}`
		want := ""
		assert.Equal(t, want, testDoc(t, input))
	})

	t.Run("comments", func(t *testing.T) {
		input := `
		// foo
		// bar
		module Foo {}`
		want := "foo\nbar\n"
		assert.Equal(t, want, testDoc(t, input))
	})

	t.Run("comments", func(t *testing.T) {
		input := `
		// foo
		// bar

		module Foo {}`
		want := ""
		got := testDoc(t, input)
		assert.Equal(t, want, got)
	})

	t.Run("comments", func(t *testing.T) {
		input := `
		// foo

		// bar
		module Foo {}`
		want := "bar\n"
		assert.Equal(t, want, testDoc(t, input))
	})

	t.Run("comments", func(t *testing.T) {
		input := `
		/* foo */
		/* bar */
		module Foo {}`
		want := "foo\nbar\n"
		assert.Equal(t, want, testDoc(t, input))
	})

	t.Run("comments", func(t *testing.T) {
		input := `
		/* foo */ /* bar */
		module Foo {}`
		want := "foo bar\n"
		assert.Equal(t, want, testDoc(t, input))
	})
}

package format

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSimpleFormatter(t *testing.T) {
	tests := []struct {
		input string
		want  interface{}
		skip  bool
	}{
		{input: "", want: ""},
		{input: "foo;", want: "foo;\n"},
		{input: "foo;\n\n", want: "foo;\n"},
		{input: "//foo", want: "//foo\n"},
		{input: "//foo\n", want: "//foo\n"},

		// Remove leading whitespace
		{input: "    leading;", want: "leading;\n"},

		// Remove trailing whitespace
		{input: "trailing;   ", want: "trailing;\n"},

		// At max one blank between tokens.
		{input: "import from M  all", want: "import from M all\n"},

		// Convert all other whitespace to blanks.
		{input: "import \tfrom  M\tall", want: "import from M all\n"},

		// Replace line breaks with \n
		{input: "foo;\r\nbar;", want: "foo;\nbar;\n"},
		{input: "foo;\rbar;", want: "foo; bar;\n"}, // \r is not a line break
		{input: "foo;\n\rbar;", want: "foo;\nbar;\n"},
		{input: "foo;\vbar;", want: "foo;\nbar;\n"},
		{input: "foo;\fbar;", want: "foo;\nbar;\n"},

		// Keep at most one newline
		{input: "foo;\r\n\r\nbar;", want: "foo;\n\nbar;\n"},

		// User defined spaces
		{input: "control{}", want: "control{}\n"},
		{input: "control {}", want: "control {}\n"},
		{input: "control {} // Foo", want: "control {} // Foo\n"},
		{input: "control  {}  ", want: "control {}\n"},
		{input: "control \n{}", want: "control\n{}\n"},
		{input: "control\n {}", want: "control\n{}\n"},

		// Verify that {, [ and ( increment the indentation level
		{input: "{foo}", want: "{foo}\n"},
		{input: "{\nfoo}", want: "{\n\tfoo}\n"},
		{input: "{\n foo}", want: "{\n\tfoo}\n"},
		{input: "{\nfoo};\nbar", want: "{\n\tfoo};\nbar\n"},
		{input: "{\n[\n(\n1,2\n)\n] p.receive\n}", want: "{\n\t[\n\t\t(\n\t\t\t1, 2\n\t\t)\n\t] p.receive\n}\n"},

		// Verify that tokens with newlines have correct indentation
		{input: "{// Foo\nBar}", want: "{ // Foo\n\tBar}\n"},                          //  Bar must be indented.
		{input: "{\n/*\n* foo\n*/}", want: "{\n\t/*\n\t* foo\n\t*/}\n"},               //  Comment must be indented, with one extra space.
		{input: "{\n      /*  \n   * foo\n*/}", want: "{\n\t/*\n\t   * foo\n\t*/}\n"}, //  the least indented line is the anchor. It gets alligned to the actual indentation level. All other lines are alligned relative to it
		{input: "{\n/*\n   * foo\n   * some prefix*/}", want: "{\n\t/*\n\t   * foo\n\t   * some prefix*/}\n"},

		// Indenting multi-line comments
		{input: "{\n  /**\n   * Foo \n   */}", want: "{\n\t/**\n\t * Foo\n\t */}\n"},
		{input: "{\n  /**\n    * Bar\n    */}", want: "{\n\t/**\n\t  * Bar\n\t  */}\n"},
		{input: "{\n  x:=25   /**\n             Bar\n           */}", want: "{\n\tx := 25 /**\n\t   Bar\n\t */}\n"},
		{input: "{\n    /*function f() {\n        var integer x;\n\n        log(x);\n    }*/}\n", want: "{\n\t/*function f() {\n\t    var integer x;\n\n\t    log(x);\n\t}*/}\n"}, // including an empty line

		// Verify that comments and := are aligned.
		{input: "p := {x := 1,\nx2:= 123}", want: "p := {x := 1,\n\tx2 := 123}\n"},
		{input: "p := {\nx := 1,\nx2:= 123}", want: "p := {\n\tx := 1,\n\tx2 := 123}\n"},
		{input: "p := {\nx := 1, // a\nx2:= 123 /* b */}", want: "p := {\n\tx := 1,   // a\n\tx2 := 123 /* b */}\n"},

		// A broken line is indented once for that object.
		{input: "var int x := a\n+\nb", want: "var int x := a\n\t+\n\tb\n"},
		{input: "var int x := a +\nb +\nc", want: "var int x := a +\n\tb +\n\tc\n"},
		{input: "var int x := a +\nb *\nc", want: "var int x := a +\n\tb *\n\t\tc\n"},
		{input: "var int x :=\n1", want: "var int x :=\n\t1\n"},

		// Parameter lists: () already indents once; <> does not.
		{input: "function f(int a,\nint b) {}", want: "function f(int a,\n\tint b) {}\n"},
		{input: "function f<type T,\ntype U>() {}", want: "function f<type T,\n\ttype U>() {}\n"},
		{input: "function f(in\nint\nx) {}", want: "function f(in\n\t\tint\n\t\tx) {}\n"},
		{input: "f(a +\nb,\nc)", want: "f(a +\n\t\tb,\n\tc)\n"},

		// A broken expression inside a block stacks on the brace indent.
		{input: "{\na +\nb\n}", want: "{\n\ta +\n\t\tb\n}\n"},

		// A header break before a brace is not a continuation of the brace.
		{input: "function f()\n{\nvar int x := 1\n}", want: "function f()\n{\n\tvar int x := 1\n}\n"},
		{input: "return\n1", want: "return\n\t1\n"},

		{input: "var int x := a + // c\nb", want: "var int x := a + // c\n\tb\n"},
		{input: "var int x := a.\nb.\nc", want: "var int x := a.\n\tb.\n\tc\n"},
		{input: "var int x := a +\n\nb", want: "var int x := a +\n\n\tb\n"},
		{input: "f(\na,\nb)", want: "f(\n\ta,\n\tb)\n"},
	}

	for _, test := range tests {
		t.Run(t.Name(), func(t *testing.T) {
			if test.skip {
				t.Skip()
			}
			var buf bytes.Buffer
			err := Fprint(&buf, test.input)
			got := buf.String()
			switch want := test.want.(type) {
			case string:
				assert.Nil(t, err)
				assert.Equal(t, want, got)
			case error:
				assert.True(t, errors.Is(want, err))
				assert.Nil(t, got)
			default:
				t.Fatalf("test implementation error: unexpected type %T", want)
			}
		})
	}
}

package format

import (
	"bytes"
	"errors"
	"fmt"
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
		{input: "{\n    /*function f() {\n        var integer x;\n\n        log(x);\n    }*/}\n", want: "{\n\t/*function f() {\n\t    var integer x;\n\n\t    log(x);\n\t}*/}\n"}, // including an empty line

		// A trailing comment is anchored at the indentation of its line.
		{input: "{\n  x:=25   /**\n             Bar\n           */}", want: "{\n\tx := 25 /**\n\t           Bar\n\t         */}\n"},

		// Tabs in comments count TabWidth columns.
		{input: "{\n\t/**\n\t * Foo\n\t */}", want: "{\n\t/**\n\t * Foo\n\t */}\n"},
		{input: "{\n\t\t/*\n\t\t * a */}", want: "{\n\t/*\n\t * a */}\n"},
		{input: "{\n/*\n\t* a\n\t* b\n\t*/\n}", want: "{\n\t/*\n\t        * a\n\t        * b\n\t        */\n}\n"},
		{input: "{\n  /*\n\t * a\n  */}", want: "{\n\t/*\n\t       * a\n\t*/}\n"},
		{input: "var int x := a +\n/* c\n * d */\nb;", want: "var int x := a +\n\t/* c\n\t * d */\n\tb;\n"},
		{input: "{ /* a\n  b */\nx}", want: "{ /* a\n  b */\n\tx}\n"},
		{input: "x := \"a\n b\" /* c\n   d */", want: "x := \"a\n b\" /* c\n   d */\n"},

		// Verify that comments and := are aligned.
		{input: "p := {x := 1,\nx2:= 123}", want: "p := {x := 1,\n\tx2 := 123}\n"},
		{input: "p := {\nx := 1,\nx2:= 123}", want: "p := {\n\tx := 1,\n\tx2 := 123}\n"},
		{input: "p := {\nx := 1, // a\nx2:= 123 /* b */}", want: "p := {\n\tx := 1,   // a\n\tx2 := 123 /* b */}\n"},

		// A broken line is indented once for that object.
		{input: "var int x := a\n+\nb", want: "var int x := a\n\t+\n\tb\n"},
		{input: "var int x := a +\nb +\nc", want: "var int x := a +\n\tb +\n\tc\n"},
		{input: "var int x := a +\nb *\nc", want: "var int x := a +\n\tb *\n\t\tc\n"},
		{input: "var int x :=\n1", want: "var int x :=\n\t1\n"},

		// A later parameter aligns with the first one after the delimiter.
		{input: "function f(int a,\nint b) {}", want: "function f(int a,\n           int b) {}\n"},
		{input: "function f<type T,\ntype U>() {}", want: "function f<type T,\n           type U>() {}\n"},
		{input: "signature S(in integer p_in,\ninout integer p_inout,\nout integer p_out);", want: "signature S(in integer p_in,\n            inout integer p_inout,\n            out integer p_out);\n"},
		{input: "module M {\nfunction f(int a,\nint b) {}\n}", want: "module M {\n\tfunction f(int a,\n\t           int b) {}\n}\n"},
		{input: "function f(int a, // c\nint b) {}", want: "function f(int a, // c\n           int b) {}\n"},
		{input: "function f(\nint a,\nint b) {}", want: "function f(\n\tint a,\n\tint b) {}\n"},
		{input: "function f(int a,\nint\nb) {}", want: "function f(int a,\n           int\n\t\tb) {}\n"},
		{input: "function f(in\nint\nx) {}", want: "function f(in\n\t\tint\n\t\tx) {}\n"},
		{input: "f(a +\nb,\nc)", want: "f(a +\n\t\tb,\n\tc)\n"},

		// A broken expression inside a block stacks on the brace indent.
		{input: "{\na +\nb\n}", want: "{\n\ta +\n\t\tb\n}\n"},

		// A header break before a brace is not a continuation of the brace.
		{input: "function f()\n{\nvar int x := 1\n}", want: "function f()\n{\n\tvar int x := 1\n}\n"},
		{input: "return\n1", want: "return\n\t1\n"},

		// A header clause on a new line is indented; the { is not.
		{input: "testcase TC()\nruns on GeneralComp system GeneralComp {}", want: "testcase TC()\n\truns on GeneralComp system GeneralComp {}\n"},
		{input: "testcase TC() runs on C\nsystem S\n{}", want: "testcase TC() runs on C\n\tsystem S\n{}\n"},
		{input: "testcase TC()\nruns on C\nsystem S\n{\nlog(1)\n}", want: "testcase TC()\n\truns on C\n\tsystem S\n{\n\tlog(1)\n}\n"},
		{input: "function f()\nreturn integer\n{\nreturn\n1\n}", want: "function f()\n\treturn integer\n{\n\treturn\n\t\t1\n}\n"},
		{input: "testcase TC()\nruns on C\nmtc M\nsystem S\n{}", want: "testcase TC()\n\truns on C\n\tmtc M\n\tsystem S\n{}\n"},
		{input: "altstep a()\nruns on C\n{}", want: "altstep a()\n\truns on C\n{}\n"},
		{input: "signature S()\nreturn integer\nexception (E);", want: "signature S()\n\treturn integer\nexception (E);\n"},
		{input: "type function F()\nruns on C\nreturn integer;", want: "type function F()\n\truns on C\n\treturn integer;\n"},
		{input: "type class C\nruns on X\nsystem S\n{\nfunction f()\nreturn int\n{\nreturn 1\n}\n}", want: "type class C\n\truns on X\n\tsystem S\n{\n\tfunction f()\n\t\treturn int\n\t{\n\t\treturn 1\n\t}\n}\n"},
		{input: "function f() runs on C\n{\nreturn\n1\n}", want: "function f() runs on C\n{\n\treturn\n\t\t1\n}\n"},

		{input: "var int x := a + // c\nb", want: "var int x := a + // c\n\tb\n"},
		{input: "var int x := a.\nb.\nc", want: "var int x := a.\n\tb.\n\tc\n"},
		{input: "var int x := a +\n\nb", want: "var int x := a +\n\n\tb\n"},
		{input: "f(\na,\nb)", want: "f(\n\ta,\n\tb)\n"},

		// Later declarators of a declaration are one continuation.
		{input: "var int x := 1,\ny := 2,\nz := 3;", want: "var int x := 1,\n\ty := 2,\n\tz := 3;\n"},
		{input: "timer t1,\nt2,\nt3;", want: "timer t1,\n\tt2,\n\tt3;\n"},
		{input: "var int x :=\n1,\ny := 2;", want: "var int x :=\n\t1,\n\ty := 2;\n"},

		// Template declarations continue like var and const.
		{input: "template charstring t :=\n\"a\";", want: "template charstring t :=\n\t\"a\";\n"},
		{input: "template T t modifies u :=\n\"a\";", want: "template T t modifies u :=\n\t\"a\";\n"},
		{input: "template T t := {\nx := 1\n}", want: "template T t := {\n\tx := 1\n}\n"},

		// A break before { is not a continuation; other values are.
		{input: "template MessageType t :=\n{\nfield1 := 1\n}", want: "template MessageType t :=\n{\n\tfield1 := 1\n}\n"},
		{input: "var R v :=\n{\na := 1\n}", want: "var R v :=\n{\n\ta := 1\n}\n"},
		{input: "var int x :=\n1", want: "var int x :=\n\t1\n"},

		// A header break before the { of a nested type is not a continuation.
		{input: "type set\n{\nint x\n} S;", want: "type set\n{\n\tint x\n} S;\n"},
		{input: "var record\n{\nint x\n} v;", want: "var record\n{\n\tint x\n} v;\n"},
		{input: "var enumerated\n{\nE1, E2\n} e;", want: "var enumerated\n{\n\tE1, E2\n} e;\n"},
		{input: "type record of set\n{\nint x\n} S;", want: "type record of set\n{\n\tint x\n} S;\n"},
		{input: "var record of\ncharstring x;", want: "var record of\n\tcharstring x;\n"},
		{input: "var record {\nint x\n} v;", want: "var record {\n\tint x\n} v;\n"},

		// Nor is a break before with attributes.
		{input: "const int x := 1\nwith { extension \"a\" }", want: "const int x := 1\nwith { extension \"a\" }\n"},
		{input: "template T t := 1\nwith {\nextension \"a\"\n}", want: "template T t := 1\nwith {\n\textension \"a\"\n}\n"},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%q", test.input), func(t *testing.T) {
			if test.skip {
				t.Skip()
			}
			got, err := format(test.input, false)
			switch want := test.want.(type) {
			case string:
				assert.Nil(t, err)
				assert.Equal(t, want, got)

				again, err := format(want, false)
				assert.Nil(t, err)
				assert.Equal(t, want, again, "not idempotent")

				spaces, err := format(test.input, true)
				assert.Nil(t, err)
				again, err = format(spaces, true)
				assert.Nil(t, err)
				assert.Equal(t, spaces, again, "not idempotent with spaces")
			case error:
				assert.True(t, errors.Is(err, want))
				assert.Empty(t, got)
			default:
				t.Fatalf("test implementation error: unexpected type %T", want)
			}
		})
	}
}

func TestReusePrinter(t *testing.T) {
	inputs := []string{
		"function f() {\nvar int x := 1\n}",
		"var int y := 2;",
		"{\n  /**\n   * Foo\n   */}",
	}
	for _, useSpaces := range []bool{false, true} {
		var buf bytes.Buffer
		p := NewCanonicalPrinter(&buf)
		p.UseSpaces = useSpaces
		p.Indent = 1
		for _, input := range inputs {
			want, err := format(input, useSpaces, func(q *CanonicalPrinter) { q.Indent = 1 })
			assert.Nil(t, err)

			buf.Reset()
			assert.Nil(t, p.Fprint(input))
			assert.Equal(t, want, buf.String())
			assert.Equal(t, 1, p.Indent)
		}
	}
}

func format(input string, useSpaces bool, opts ...func(*CanonicalPrinter)) (string, error) {
	var buf bytes.Buffer
	p := NewCanonicalPrinter(&buf)
	p.UseSpaces = useSpaces
	for _, opt := range opts {
		opt(p)
	}
	err := p.Fprint(input)
	return buf.String(), err
}

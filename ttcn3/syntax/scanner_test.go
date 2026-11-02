package syntax

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", []string{"EOF"}},
		{"\n", []string{"EOF"}},
		{"? ??? ?? ????", []string{"?", "???", "?", "?", "???", "?", "EOF"}},

		{"foo\n", []string{
			`IDENT "foo"`,
			`EOF`,
		}},
		{"//\n", []string{
			`COMMENT "//"`,
			`EOF`,
		}},
		{`"str"
				`, []string{
			`STRING "\"str\""`,
			`EOF`,
		}},
		{`"multiline
str"
`, []string{
			`STRING "\"multiline\nstr\""`,
			`EOF`,
		}},
		{`"
multiline
str"
`, []string{
			`STRING "\"\nmultiline\nstr\""`,
			`EOF`,
		}},
		{`"multiline
str
"
`, []string{
			`STRING "\"multiline\nstr\n\""`,
			`EOF`,
		}},
		{`"
multiline
str
"
`, []string{
			`STRING "\"\nmultiline\nstr\n\""`,
			`EOF`,
		}},
	}
	for _, test := range tests {
		root := Tokenize([]byte(test.input))
		if root == nil {
			t.Fatalf("Tokenize(%q) returned nil", test.input)
		}
		var got []string
		for _, tok := range root.tokens {
			s := tok.String()
			if tok.IsLiteral() {
				s += fmt.Sprintf(" %q", root.src[tok.Begin:tok.End])
			}
			got = append(got, s)
		}
		assert.Equal(t, test.want, got)
	}
}

package syntax_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/5nord/goldenfile"
	"github.com/hashicorp/go-multierror"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// TestGolden parses the input of each test in testdata/*.txt and compares
// the resulting syntax tree with the expected S-expression. Run
// `go test -update` to regenerate the expected output.
func TestGolden(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no golden files found")
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			b, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			tests, err := goldenfile.Parse(bytes.NewReader(b))
			if err != nil {
				t.Fatalf("goldenfile.Parse(%s): %s", file, err)
			}

			if *goldenfile.Update {
				var sb strings.Builder
				for i := range tests {
					tests[i].Want = parseSexpr(tests[i].Text)
					if i > 0 {
						sb.WriteString("\n")
					}
					fmt.Fprintf(&sb, "===\n%s\n===\n%s\n---\n%s\n", tests[i].Name, tests[i].Text, tests[i].Want)
				}
				if err := os.WriteFile(file, []byte(sb.String()), 0644); err != nil {
					t.Fatal(err)
				}
			}

			seen := make(map[string]bool)
			for _, test := range tests {
				if seen[test.Name] {
					t.Errorf("duplicate test name %q", test.Name)
				}
				seen[test.Name] = true
				t.Run(test.Name, func(t *testing.T) {
					if got := parseSexpr(test.Text); got != test.Want {
						t.Errorf("input:\n%s\n\ngot:\n%s\n\nwant:\n%s", test.Text, got, test.Want)
					}
				})
			}
		})
	}
}

// parseSexpr parses src and returns the syntax tree as S-expression followed
// by the syntax errors, one per line.
func parseSexpr(src string) string {
	root := syntax.Parse([]byte(src))
	var sb strings.Builder
	writeSexpr(&sb, root, 0)
	var merr *multierror.Error
	if errors.As(root.Err(), &merr) {
		for _, err := range merr.Errors {
			if e, ok := err.(syntax.Error); ok {
				spn := syntax.SpanOf(e.Node)
				fmt.Fprintf(&sb, "\nERROR %s: %s", spn.String(), e.Msg)
				continue
			}
			fmt.Fprintf(&sb, "\nERROR %s", err)
		}
	}
	return sb.String()
}

// Delimiter tokens carry no information beyond the tree structure and are
// omitted from the S-expression.
var delimiters = map[syntax.Kind]bool{
	syntax.LPAREN:    true,
	syntax.RPAREN:    true,
	syntax.LBRACK:    true,
	syntax.RBRACK:    true,
	syntax.LBRACE:    true,
	syntax.RBRACE:    true,
	syntax.COMMA:     true,
	syntax.DOT:       true,
	syntax.SEMICOLON: true,
}

// writeSexpr writes n in a tree-sitter like format:
//
//	(Type field: "token" ...
//	  field: (Child ...))
//
// Identifiers and literals are written with their text only. Delimiters are
// written only where the parser used a different token in their place, or
// where they bound an ErrorNode.
func writeSexpr(sb *strings.Builder, n syntax.Node, depth int) {
	v := reflect.Indirect(reflect.ValueOf(n))
	sb.WriteString("(" + v.Type().Name())
	switch n := n.(type) {
	case *syntax.Ident:
		text := n.Tok.String()
		if n.Tok2 != nil {
			text += " " + n.Tok2.String()
		}
		fmt.Fprintf(sb, " %q)", text)
		return
	case *syntax.ValueLiteral:
		fmt.Fprintf(sb, " %q)", n.Tok.String())
		return
	}

	_, isErr := n.(*syntax.ErrorNode)
	var children []field
	for _, f := range fields(v) {
		if tok, ok := f.node.(syntax.Token); ok {
			if isErr || !delimiters[tok.Kind()] {
				fmt.Fprintf(sb, " %s: %q", f.name, tok.String())
			}
			continue
		}
		children = append(children, f)
	}
	for _, c := range children {
		sb.WriteString("\n" + strings.Repeat("  ", depth+1) + c.name + ": ")
		writeSexpr(sb, c.node, depth+1)
	}
	sb.WriteString(")")
}

type field struct {
	name string
	node syntax.Node
}

// fields returns the non-nil syntax node fields of struct v in declaration
// order. Slices are flattened and embedded structs are inlined.
func fields(v reflect.Value) []field {
	var ret []field
	add := func(name string, fv reflect.Value) {
		if n, ok := fv.Interface().(syntax.Node); ok && !syntax.IsNil(n) {
			ret = append(ret, field{name, n})
		}
	}
	for i := 0; i < v.NumField(); i++ {
		f, fv := v.Type().Field(i), v.Field(i)
		switch {
		case !f.IsExported():
		case f.Anonymous:
			if fv = reflect.Indirect(fv); fv.Kind() == reflect.Struct {
				ret = append(ret, fields(fv)...)
			}
		case fv.Kind() == reflect.Slice:
			for j := 0; j < fv.Len(); j++ {
				add(f.Name, fv.Index(j))
			}
		default:
			add(f.Name, fv)
		}
	}
	return ret
}

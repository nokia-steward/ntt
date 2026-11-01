package syntax_test

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/5nord/goldenfile"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// TestResilience parses mutations of valid inputs: each input truncated after
// every token and each input with a single token removed. The parser must
// terminate without panic and keep every token in the tree.
func TestResilience(t *testing.T) {
	var inputs []string
	files, _ := filepath.Glob(filepath.Join("testdata", "*.txt"))
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		tests, err := goldenfile.Parse(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		for _, test := range tests {
			if syntax.Parse([]byte(test.Text)).Err() == nil {
				inputs = append(inputs, test.Text)
			}
		}
	}

	if !testing.Short() {
		dir := os.Getenv("NTT_CONFORMANCE_DIR")
		if dir == "" {
			dir = "../../testdata/ttcn3-conformance-tests/ATS/core_language"
		}
		var n int
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".ttcn") {
				return nil
			}
			if n++; n%40 != 0 {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if syntax.Parse(b).Err() == nil {
				inputs = append(inputs, string(b))
			}
			return nil
		})
		if err != nil {
			t.Logf("skipping conformance inputs: %v", err)
		}
	}

	for _, input := range inputs {
		src := []byte(input)
		root := syntax.Parse(src)
		for tok := root.FirstTok(); tok != nil && tok.Kind() != syntax.EOF; tok = tok.NextTok() {
			checkResilience(t, src[:tok.End()])
			checkResilience(t, slices.Concat(src[:tok.Pos()], src[tok.End():]))
		}
	}
}

func checkResilience(t *testing.T, src []byte) {
	t.Helper()
	type result struct {
		root  *syntax.Root
		panic any
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- result{panic: r}
			}
		}()
		done <- result{root: syntax.Parse(src)}
	}()

	var res result
	select {
	case res = <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("Parse(%q) does not terminate", src)
	}
	if res.panic != nil {
		t.Errorf("Parse(%q) panics: %v", src, res.panic)
		return
	}
	root := res.root

	var want, got []int
	for tok := root.FirstTok(); tok != nil; tok = tok.NextTok() {
		if tok.Kind() != syntax.EOF {
			want = append(want, tok.Pos())
		}
	}
	for _, e := range syntax.Serialize(root) {
		if tok := e.Token(); tok != nil {
			got = append(got, tok.Pos())
		}
	}
	if !slices.Equal(want, got) {
		t.Errorf("Serialize(Parse(%q)) is not lossless:\nwant %v\ngot  %v", src, want, got)
	}
}

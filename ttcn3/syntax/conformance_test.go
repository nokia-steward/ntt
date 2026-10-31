package syntax_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nokia/ntt/ttcn3/syntax"
)

var verdictPassAccept = regexp.MustCompile(`@verdict\s+pass\s+accept`)

// TestConformance parses all core language conformance tests, which are
// expected to be accepted by a TTCN-3 tool.
func TestConformance(t *testing.T) {
	dir := os.Getenv("NTT_CONFORMANCE_DIR")
	if dir == "" {
		dir = "../../testdata/ttcn3-conformance-tests/ATS/core_language"
	}
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("conformance tests not found: %s", err)
	}

	var files []string
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if strings.HasSuffix(path, ".ttcn") || strings.HasSuffix(path, ".ttcn3") {
			files = append(files, path)
		}
		return nil
	})

	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !verdictPassAccept.Match(b) {
			continue
		}
		rel, _ := filepath.Rel(dir, file)
		t.Run(rel, func(t *testing.T) {
			root := syntax.Parse(b, syntax.WithFilename(file))
			if err := root.Err(); err != nil {
				t.Error(err)
			}
		})
	}
}

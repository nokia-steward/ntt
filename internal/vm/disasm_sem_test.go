package vm

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/nokia/ntt/internal/t3xf"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func compileSem060100(t *testing.T) []byte {
	t.Helper()
	path := "/home/masimon/ttcn3-conformance-tests/ATS/core_language/06_types_and_values/0601_basic_types_and_values/060100_simple_basic_types_and_values/Sem_060100_SimpleBasicTypes_001.ttcn"
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skip(err)
	}
	root := syntax.Parse(b)
	if err := root.Err(); err != nil {
		t.Fatal(err)
	}
	c := t3xf.NewCompiler()
	if err := c.Compile(root); err != nil {
		t.Fatal(err)
	}
	out, err := c.Assemble()
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSem060100SimpleBasicTypes001(t *testing.T) {
	b := compileSem060100(t)
	t3xf.PatchBlocks(b)
	l := loader{Image: NewImage(b)}
	if err := l.load(Block{Begin: -4, End: len(b)}); err != nil {
		t.Fatal(err)
	}
	if len(l.Controls) == 0 {
		t.Fatalf("no control part loaded, names=%d", len(l.Names))
	}
	var log bytes.Buffer
	if err := Run(l.Image, &log); err != nil {
		t.Fatal(err)
	}
	out := log.String()
	if !strings.Contains(out, "|tcfi|") || !strings.Contains(out, "|pass") {
		t.Fatalf("expected tcfi pass, got:\n%s", out)
	}
}

func TestSem15TemplatesAndMatch(t *testing.T) {
	const base = "/home/masimon/ttcn3-conformance-tests/ATS/core_language/15_templates/"
	files := []string{
		"1503_global_and_local_templates/Sem_1503_GlobalAndLocalTemplates_001.ttcn",
		"1506_referencing_elements_of_templates_or_template_fields/150602_referencing_record_and_set_fields/Sem_150602_ReferencingRecordAndSetFields_001.ttcn",
		"1509_match_operation/Sem_1509_MatchOperation_001.ttcn",
		"1509_match_operation/Sem_1509_MatchOperation_002.ttcn",
		"1509_match_operation/Sem_1509_MatchOperation_003.ttcn",
		"1509_match_operation/Sem_1509_MatchOperation_004.ttcn",
		"1509_match_operation/Sem_1509_MatchOperation_006.ttcn",
		"1509_match_operation/Sem_1509_MatchOperation_007.ttcn",
		"1509_match_operation/Sem_1509_MatchOperation_008.ttcn",
		"1509_match_operation/Sem_1509_MatchOperation_009.ttcn",
		"1509_match_operation/Sem_1509_MatchOperation_010.ttcn",
		"1509_match_operation/Sem_1509_MatchOperation_011.ttcn",
		"1509_match_operation/Sem_1509_MatchOperation_012.ttcn",
		"1509_match_operation/Sem_1509_MatchOperation_013.ttcn",
		"1509_match_operation/Sem_1509_MatchOperation_014.ttcn",
		"1509_match_operation/Sem_1509_MatchOperation_015.ttcn",
		"1509_match_operation/Sem_1509_MatchOperation_016.ttcn",
		"1510_valueof_operation/Sem_1510_ValueOfOperation_001.ttcn",
	}
	for _, rel := range files {
		t.Run(rel, func(t *testing.T) {
			path := base + rel
			src, err := os.ReadFile(path)
			if err != nil {
				t.Skip(err)
			}
			root := syntax.Parse(src)
			if err := root.Err(); err != nil {
				t.Fatal(err)
			}
			root.Filename = path
			c := t3xf.NewCompiler()
			if err := c.Compile(root); err != nil {
				t.Fatal(err)
			}
			b, err := c.Assemble()
			if err != nil {
				t.Fatal(err)
			}
			t3xf.PatchBlocks(b)
			l := loader{Image: NewImage(b)}
			if err := l.load(Block{Begin: -4, End: len(b)}); err != nil {
				t.Fatal(err)
			}
			var log bytes.Buffer
			if err := Run(l.Image, &log); err != nil {
				t.Fatal(err)
			}
			out := log.String()
			if !strings.Contains(out, "|tcfi|") || !strings.Contains(out, "|pass") {
				t.Fatalf("expected tcfi pass, got:\n%s", out)
			}
		})
	}
}

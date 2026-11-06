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

// malformed are the conformance tests expected to be accepted that do not
// follow the TTCN-3 grammar, by what is wrong in them.
var malformed = map[string]string{
	"05_basic_language_elements/0504_parametrization/050403_variadic_parameters/Sem_050403_variadic_parameters_009.ttcn":                                  "'returns' for 'return'",
	"05_basic_language_elements/0504_parametrization/050403_variadic_parameters/Sem_050403_variadic_parameters_011.ttcn":                                  "unbalanced ')'",
	"06_types_and_values/0602_structured_types_and_values/060201_record_type_and_values/06020104_embedding_fields/Sem_06020104_embedding_fields_002.ttcn": "';' between record fields",
	"06_types_and_values/0605_automatic_type/060504_references/Sem_060504_references_007.ttcn":                                                            "function without 'function'",
	"06_types_and_values/0605_automatic_type/060504_references/Sem_060504_references_008.ttcn":                                                            "function without 'function'",
	"06_types_and_values/0605_automatic_type/060504_references/Sem_060504_references_011.ttcn":                                                            "'returns' for 'return'",
	"06_types_and_values/0605_automatic_type/060504_references/Sem_060504_references_012.ttcn":                                                            "';' between record fields",
	"06_types_and_values/0605_automatic_type/060506_return_values/Sem_060506_return_values_005.ttcn":                                                      "unbalanced '('",
	"10_constants/Sem_10_nested_types_001.ttcn":                                                                                                           "constant without a name",
	"16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/NegSem_160106_the_not_implemented_function_004.ttcn":              "return type without 'return'",
	"16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/Sem_160106_the_not_implemented_function_001.ttcn":                 "return type without 'return'",
	"16_functions_altsteps_testcases/1601_functions/1601_toplevel/Sem_1601_toplevel_004.ttcn":                                                             "'returns' for 'return'",
}

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
		if why, ok := malformed[filepath.ToSlash(rel)]; ok {
			t.Logf("%s: skipped, malformed: %s", rel, why)
			continue
		}
		t.Run(rel, func(t *testing.T) {
			root := syntax.Parse(b, syntax.WithFilename(file))
			if err := root.Err(); err != nil {
				t.Error(err)
			}
		})
	}
}

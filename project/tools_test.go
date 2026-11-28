package project

import (
	"testing"

	"github.com/nokia/ntt/internal/yaml"
)

func TestTools_UnmarshalsManifestSection(t *testing.T) {
	const src = `
name: example
sources:
  - foo.ttcn3
tools:
  fmt:
    print_width: 120
    tab_width: 4
    use_spaces: true
    max_empty_lines: 1
  lint:
    max_lines: 80
    aligned_braces: true
    require_case_else: true
    disabled_rules:
      - unused-import
      - empty-block
`
	var m Manifest
	if err := yaml.Unmarshal([]byte(src), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m.Tools.Fmt.PrintWidth != 120 {
		t.Errorf("PrintWidth = %d, want 120", m.Tools.Fmt.PrintWidth)
	}
	if !m.Tools.Fmt.UseSpaces {
		t.Errorf("UseSpaces = false, want true")
	}
	if !m.Tools.Lint.AlignedBraces {
		t.Errorf("AlignedBraces = false, want true")
	}
	if got := m.Tools.Lint.IsRuleDisabled("unused-import"); !got {
		t.Errorf("expected unused-import to be disabled")
	}
	if got := m.Tools.Lint.IsRuleDisabled("aligned-braces"); got {
		t.Errorf("aligned-braces should not be disabled")
	}
}

func TestFmtTool_Defaults(t *testing.T) {
	opts := (FmtTool{}).Options()
	if opts.PrintWidth != 100 {
		t.Errorf("default PrintWidth = %d, want 100", opts.PrintWidth)
	}
	if opts.TabWidth != 8 {
		t.Errorf("default TabWidth = %d, want 8", opts.TabWidth)
	}
}

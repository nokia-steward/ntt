package project

// Tools is the optional `tools:` section in a manifest. It collects
// per-tool configuration in one place so the LSP, the CLI and any
// downstream automation can read the same source of truth.
//
// The shape (a [tools.fmt] and [tools.lint] section per tool) is
// chosen to look familiar to anyone who has configured a Go, Rust
// or JavaScript toolchain from a single project manifest.
//
// Example YAML:
//
//	tools:
//	  fmt:
//	    print_width: 120
//	    tab_width: 4
//	    use_spaces: true
//	    max_empty_lines: 1
//	  lint:
//	    max_lines: 80
//	    aligned_braces: true
type Tools struct {
	Fmt  FmtTool  `yaml:"fmt,omitempty" json:"fmt,omitempty"`
	Lint LintTool `yaml:"lint,omitempty" json:"lint,omitempty"`
}

// FmtTool configures the `ntt format` command and the LSP formatter.
// Zero values fall back to the defaults baked into ttcn3/format.
type FmtTool struct {
	// PrintWidth is the soft right margin for the wrapping
	// formatter. Defaults to 100 columns.
	PrintWidth int `yaml:"print_width,omitempty" json:"print_width,omitempty"`

	// TabWidth is how many columns one tab counts for. Defaults to
	// 8 to match the historical canonical printer.
	TabWidth int `yaml:"tab_width,omitempty" json:"tab_width,omitempty"`

	// UseSpaces emits spaces instead of tabs for indentation.
	UseSpaces bool `yaml:"use_spaces,omitempty" json:"use_spaces,omitempty"`

	// MaxEmptyLines caps consecutive empty lines between top-level
	// declarations. Zero disables the cap.
	MaxEmptyLines int `yaml:"max_empty_lines,omitempty" json:"max_empty_lines,omitempty"`
}

// LintTool configures the standalone `ntt lint` command and the LSP
// lint diagnostics. Unset fields leave the legacy CLI behaviour
// untouched.
type LintTool struct {
	// MaxLines is the maximum number of lines allowed in a behaviour
	// body. Zero disables the check.
	MaxLines int `yaml:"max_lines,omitempty" json:"max_lines,omitempty"`

	// AlignedBraces requires `{` and `}` to share either a line or a
	// column.
	AlignedBraces bool `yaml:"aligned_braces,omitempty" json:"aligned_braces,omitempty"`

	// RequireCaseElse requires every select statement to include a
	// `case else` branch.
	RequireCaseElse bool `yaml:"require_case_else,omitempty" json:"require_case_else,omitempty"`

	// DisabledRules is an optional list of lint rule codes to
	// suppress (e.g. "unused-import"). It is consulted by both the
	// LSP and the CLI before publishing diagnostics.
	DisabledRules []string `yaml:"disabled_rules,omitempty" json:"disabled_rules,omitempty"`
}

// FmtOptions returns the FmtTool section as a project-wide overlay
// suitable for ttcn3/format. It is a small struct rather than a
// pointer-rich object so callers can easily merge it with other
// sources (e.g. LSP client FormattingOptions).
type FmtOptions struct {
	PrintWidth    int
	TabWidth      int
	UseSpaces     bool
	MaxEmptyLines int
}

// Options returns the configured FmtOptions or sensible defaults when
// the section is empty.
func (f FmtTool) Options() FmtOptions {
	out := FmtOptions{
		PrintWidth:    f.PrintWidth,
		TabWidth:      f.TabWidth,
		UseSpaces:     f.UseSpaces,
		MaxEmptyLines: f.MaxEmptyLines,
	}
	if out.PrintWidth == 0 {
		out.PrintWidth = 100
	}
	if out.TabWidth == 0 {
		out.TabWidth = 8
	}
	return out
}

// IsRuleDisabled reports whether the given rule code is listed in
// DisabledRules.
func (l LintTool) IsRuleDisabled(code string) bool {
	for _, c := range l.DisabledRules {
		if c == code {
			return true
		}
	}
	return false
}

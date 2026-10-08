package cfgvalidate_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/build/cfgvalidate"
	"github.com/nokia/ntt/runtime/cfg"
)

func parse(t *testing.T, src string) *cfg.File {
	t.Helper()
	f, diags := cfg.Parse(strings.NewReader(src))
	for _, d := range diags {
		t.Logf("parse diag: %+v", d)
	}
	return f
}

func TestValidate_DuplicateSection(t *testing.T) {
	src := `
[EXECUTE]
M.a
[EXECUTE]
M.b
`
	issues := cfgvalidate.Validate(parse(t, src))
	if !hasCode(issues, "duplicate-section") {
		t.Errorf("missing duplicate-section: %v", issues)
	}
}

func TestValidate_DuplicateKey(t *testing.T) {
	src := `
[MODULE_PARAMETERS]
x := 1
x := 2
`
	issues := cfgvalidate.Validate(parse(t, src))
	if !hasCode(issues, "duplicate-key") {
		t.Errorf("missing duplicate-key: %v", issues)
	}
}

func TestValidate_UnknownSection(t *testing.T) {
	src := `[WHATEVER]
foo := 1
`
	issues := cfgvalidate.Validate(parse(t, src))
	if !hasCode(issues, "unknown-section") {
		t.Errorf("missing unknown-section: %v", issues)
	}
}

func TestValidate_ExecuteMissingQualifier(t *testing.T) {
	src := `[EXECUTE]
bare_name
`
	issues := cfgvalidate.Validate(parse(t, src))
	if !hasCode(issues, "execute.missing-qualifier") {
		t.Errorf("missing execute.missing-qualifier: %v", issues)
	}
}

func TestValidate_HappyPath(t *testing.T) {
	src := `
[MODULE_PARAMETERS]
x := 1
[EXECUTE]
M.tc
`
	issues := cfgvalidate.Validate(parse(t, src))
	if len(issues) != 0 {
		t.Errorf("unexpected issues: %v", issues)
	}
}

func hasCode(issues []cfgvalidate.Issue, code string) bool {
	for _, i := range issues {
		if i.Code == code {
			return true
		}
	}
	return false
}

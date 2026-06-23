package cfg_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/runtime/cfg"
)

const sample = `
# Top-level comment
[MODULE_PARAMETERS]
host := "example.com"
port := 4242

[EXECUTE]
MyModule.tc_smoke
MyModule.tc_logout

[LOGGING]
FileMask := LOG_ALL | DEBUG

# trailing comment
`

func TestParse_SectionsAndSettings(t *testing.T) {
	f, diags := cfg.Parse(strings.NewReader(sample))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	if got := len(f.Sections); got != 3 {
		t.Fatalf("Sections = %d, want 3", got)
	}

	mp := f.Section("MODULE_PARAMETERS")
	if mp == nil {
		t.Fatal("MODULE_PARAMETERS missing")
	}
	host, ok := mp.Setting("host")
	if !ok {
		t.Fatal("host setting missing")
	}
	if host.Value != `"example.com"` {
		t.Errorf("host = %q, want example.com (quoted)", host.Value)
	}

	exec := f.ExecuteList()
	if len(exec) != 2 {
		t.Fatalf("ExecuteList = %v, want 2", exec)
	}
	if exec[0] != "MyModule.tc_smoke" || exec[1] != "MyModule.tc_logout" {
		t.Errorf("ExecuteList = %v", exec)
	}
}

func TestParse_KeyValSeparators(t *testing.T) {
	src := `
[MODULE_PARAMETERS]
a := 1
b = 2
`
	f, _ := cfg.Parse(strings.NewReader(src))
	mp := f.Section("MODULE_PARAMETERS")
	if got, _ := mp.Setting("a"); got.Value != "1" {
		t.Errorf("a = %v", got)
	}
	if got, _ := mp.Setting("b"); got.Value != "2" {
		t.Errorf("b = %v", got)
	}
}

func TestParse_MalformedLine(t *testing.T) {
	src := "stray = 1\n[MOD]\nkey = ok"
	f, diags := cfg.Parse(strings.NewReader(src))
	if len(diags) == 0 {
		t.Fatal("expected a diagnostic for the stray line")
	}
	if mp := f.Section("MOD"); mp == nil {
		t.Fatal("section after diagnostic should still be parsed")
	}
}

func TestModuleParametersMap(t *testing.T) {
	src := `
[MODULE_PARAMETERS]
a := 1
b := 2
`
	f, _ := cfg.Parse(strings.NewReader(src))
	m := f.ModuleParameters()
	if m["a"] != "1" || m["b"] != "2" {
		t.Errorf("ModuleParameters = %v", m)
	}
}

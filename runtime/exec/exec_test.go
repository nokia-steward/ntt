package exec_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nokia/ntt/runtime/cfg"
	"github.com/nokia/ntt/runtime/exec"
	"github.com/nokia/ntt/runtime/report"
)

// fakeDriver answers Run from a precomputed map. Tests use it so the
// executor's scheduling / reporting can be exercised without spinning
// up the real interpreter.
type fakeDriver struct {
	cases   []string
	results map[string]report.Verdict
	reasons map[string]string
	errs    map[string]error
}

func (f fakeDriver) Run(_ context.Context, name string) (report.Verdict, string, error) {
	return f.results[name], f.reasons[name], f.errs[name]
}
func (f fakeDriver) List() []string { return append([]string{}, f.cases...) }

func TestRun_ExplicitSelectors(t *testing.T) {
	driver := fakeDriver{
		cases: []string{"M.a", "M.b"},
		results: map[string]report.Verdict{
			"M.a": report.Pass,
			"M.b": report.Fail,
		},
		reasons: map[string]string{"M.b": "oops"},
	}
	suite, err := exec.Run(context.Background(), exec.Options{
		SuiteName: "sample",
		Driver:    driver,
		Selectors: []exec.Selector{{Name: "M.a"}, {Name: "M.b"}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(suite.Cases) != 2 {
		t.Fatalf("cases = %d, want 2", len(suite.Cases))
	}
	if suite.Verdict() != report.Fail {
		t.Errorf("Verdict = %v, want fail", suite.Verdict())
	}
}

func TestRun_FromCfgExecuteList(t *testing.T) {
	cfgFile, _ := cfg.Parse(strings.NewReader(`
[EXECUTE]
M.x
M.y
`))
	driver := fakeDriver{
		cases:   []string{"M.x", "M.y"},
		results: map[string]report.Verdict{"M.x": report.Pass, "M.y": report.Pass},
	}
	suite, err := exec.Run(context.Background(), exec.Options{
		SuiteName: "cfg",
		Driver:    driver,
		Config:    cfgFile,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(suite.Cases) != 2 {
		t.Fatalf("cases = %d", len(suite.Cases))
	}
}

func TestRun_PatternSelector(t *testing.T) {
	driver := fakeDriver{
		cases: []string{"M.tc_foo", "M.tc_bar", "N.tc_quux"},
		results: map[string]report.Verdict{
			"M.tc_foo": report.Pass,
			"M.tc_bar": report.Pass,
		},
	}
	suite, _ := exec.Run(context.Background(), exec.Options{
		Driver:    driver,
		Selectors: []exec.Selector{{Name: "M.tc_*", Pattern: true}},
	})
	if len(suite.Cases) != 2 {
		t.Fatalf("cases = %d, want 2 (M.*)", len(suite.Cases))
	}
}

// settingDriver is a fakeDriver that also satisfies
// exec.ModuleParamSetter so we can assert exec.Run actually pushed
// the cfg's [MODULE_PARAMETERS] map at it.
type settingDriver struct {
	fakeDriver
	got map[string]string
	err error
}

func (d *settingDriver) SetModuleParameters(p map[string]string) error {
	if d.err != nil {
		return d.err
	}
	d.got = p
	return nil
}

func TestRun_PushesModuleParameters(t *testing.T) {
	cfgFile, _ := cfg.Parse(strings.NewReader(`
[MODULE_PARAMETERS]
M.PX_HOST := "h"
M.PX_PORT := 8080
[EXECUTE]
M.tc
`))
	driver := &settingDriver{
		fakeDriver: fakeDriver{
			cases:   []string{"M.tc"},
			results: map[string]report.Verdict{"M.tc": report.Pass},
		},
	}
	if _, err := exec.Run(context.Background(), exec.Options{
		Driver: driver, Config: cfgFile,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := driver.got["M.PX_HOST"]; got != `"h"` {
		t.Errorf("PX_HOST = %q, want %q", got, `"h"`)
	}
	if got := driver.got["M.PX_PORT"]; got != "8080" {
		t.Errorf("PX_PORT = %q, want %q", got, "8080")
	}
}

func TestRun_ModuleParamSetterErrorAborts(t *testing.T) {
	cfgFile, _ := cfg.Parse(strings.NewReader(`
[MODULE_PARAMETERS]
M.PX := 1
`))
	driver := &settingDriver{
		fakeDriver: fakeDriver{cases: []string{"M.tc"}},
		err:        errors.New("bad override"),
	}
	if _, err := exec.Run(context.Background(), exec.Options{
		Driver: driver, Config: cfgFile,
		Selectors: []exec.Selector{{Name: "M.tc"}},
	}); err == nil {
		t.Fatalf("expected error from ModuleParamSetter, got nil")
	}
}

func TestRun_DriverErrorBecomesError(t *testing.T) {
	driver := fakeDriver{
		cases: []string{"M.boom"},
		errs:  map[string]error{"M.boom": errors.New("kaboom")},
	}
	suite, err := exec.Run(context.Background(), exec.Options{
		Driver:    driver,
		Selectors: []exec.Selector{{Name: "M.boom"}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if suite.Cases[0].Verdict != report.Error {
		t.Errorf("Verdict = %v, want error", suite.Cases[0].Verdict)
	}
	if suite.Cases[0].Reason != "kaboom" {
		t.Errorf("Reason = %v", suite.Cases[0].Reason)
	}
}

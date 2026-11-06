package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nokia/ntt/project"
	"github.com/nokia/ntt/runtime/tl"
	"github.com/stretchr/testify/assert"
)

func TestRunVerdictDirectives(t *testing.T) {
	for _, tt := range []struct {
		tags    [][]string
		verdict string
		found   bool
		err     bool
	}{
		{[][]string{{"@verdict", " error"}}, "error", true, false},
		{[][]string{{"@k3", "verdict pass"}}, "pass", true, false},
		{[][]string{{"@verdict", "pass accept, ttcn3verdict:inconc"}}, "inconc", true, false},
		{[][]string{{"@verdict", "maybe"}}, "", false, true},
		{[][]string{{"@verdict", "pass accept, ttcn3verdict"}}, "", false, true},
		{[][]string{{"@purpose", "x"}}, "", false, false},
	} {
		v, found, err := parseExpectedVerdict(tt.tags)
		assert.Equal(t, tt.verdict, v, "%v", tt.tags)
		assert.Equal(t, tt.found, found, "%v", tt.tags)
		assert.Equal(t, tt.err, err != nil, "%v: %v", tt.tags, err)
	}
	for _, tt := range []struct {
		spec string
		want expectation
		err  bool
	}{
		{"pass accept", expectation{kind: expectNotError}, false},
		{"pass accept, ttcn3verdict:pass", expectation{kind: expectExact, verdict: "pass"}, false},
		{"pass reject", expectation{kind: expectReject}, false},
		{"pass reject, ttcn3verdict:pass", expectation{}, true},
		{"fail accept", expectation{}, true},
	} {
		e, _, err := parsePIXIT([][]string{{"@verdict", tt.spec}})
		assert.Equal(t, tt.want, e, tt.spec)
		assert.Equal(t, tt.err, err != nil, tt.spec)
	}
}

func TestRunJobSelection(t *testing.T) {
	p := runPlan{
		controls: []runJob{{name: "M.control"}},
		tests:    []runJob{{name: "M.a", tags: [][]string{{"@stable"}}}, {name: "M.b"}},
	}
	names := func(jobs []runJob) []string {
		var out []string
		for _, j := range jobs {
			out = append(out, j.name)
		}
		return out
	}
	// Control parts run when there are any, testcases otherwise.
	assert.Equal(t, []string{"M.control"}, names(p.selected(Basket{})))
	assert.Equal(t, []string{"M.a", "M.b"}, names(runPlan{tests: p.tests}.selected(Basket{})))
	// A basket runs the testcases it matches, not the control part.
	assert.Equal(t, []string{"M.b"}, names(p.selected(Basket{NameRegex: []string{`\.b$`}})))
	assert.Equal(t, []string{"M.a"}, names(p.selected(Basket{TagsRegex: []string{"@stable"}})))
	// Test ids run as given.
	assert.Equal(t, []string{"M.b", "M.control"}, names(p.named([]string{"M.b", "M.control"})))
}

func TestRunLogFilter(t *testing.T) {
	var f logFilter
	fs := logFilterFlags(&f)
	if err := fs.Parse([]string{"-D", "-a", "tcfi|setv", "--deactivate=M:setv"}); err != nil {
		t.Fatal(err)
	}
	assert.True(t, f.allows("tcfi", ""))
	assert.False(t, f.allows("ptsd", ""))
	assert.True(t, f.allows("setv", "N"))
	assert.False(t, f.allows("setv", "M"))
	assert.Error(t, f.check([]string{"N"}))
	assert.NoError(t, f.check([]string{"M"}))
	// A pattern that matches no event id is an error, one without a module
	// before its colon too.
	assert.Error(t, logFilterFlags(&logFilter{}).Parse([]string{"-d", "zzzz"}))
	assert.Error(t, logFilterFlags(&logFilter{}).Parse([]string{"-d", ":tcfi"}))
	// No rule shows every event.
	assert.True(t, (&logFilter{}).allows("ptsd", ""))
}

func TestRunConsoleColors(t *testing.T) {
	c := consoleFormat{color: true}
	assert.Equal(t, passColor.Sprint("t|tcfi|ntt|M.a|pass"), c.format("t|tcfi|ntt|M.a|pass"))
	assert.Equal(t, failColor.Sprint("t|cofi|MTC|error"), c.format("t|cofi|MTC|error"))
	assert.Equal(t, errorColor.Sprint("t|VMIS->Verdict mismatch|ntt|M.a|pass|fail"), c.format("t|VMIS->Verdict mismatch|ntt|M.a|pass|fail"))
	assert.Equal(t, faintColor.Sprint("t|vach|MTC=m.ttcn3:1|integer|M.a.x|1"), c.format("t|vach|MTC=m.ttcn3:1|integer|M.a.x|1"))
	assert.Equal(t, "t|ptsd|MTC=m.ttcn3:1|MTC.p|SYSTEM.p|integer|1", c.format("t|ptsd|MTC=m.ttcn3:1|MTC.p|SYSTEM.p|integer|1"))
	assert.Equal(t, "t|tcfi|ntt|M.a|pass", consoleFormat{}.format("t|tcfi|ntt|M.a|pass"))
}

func TestRunVerdictCheck(t *testing.T) {
	p := runPlan{tests: []runJob{
		{name: "M.exact", loc: "m.ttcn3:3", expect: expectation{kind: expectExact, verdict: "fail"}},
		{name: "M.accept", expect: expectation{kind: expectNotError}},
		{name: "M.reject", expect: expectation{kind: expectReject}},
		{name: "M.plain"},
	}}
	l := &runLog{plan: p}
	assert.Equal(t, "", l.observe("t|tcfi|ntt|M.exact|fail"))
	assert.Equal(t, "t|VMIS->Verdict mismatch|"+tl.RuntimeID()+"=m.ttcn3:3|M.exact|fail|pass", l.observe("t|tcfi|ntt|M.exact|pass"))
	assert.Equal(t, "", l.observe("t|tcfi|ntt|M.accept|fail"))
	assert.Equal(t, "", l.observe("t|tcfi|ntt|M.reject|error"))
	assert.Equal(t, "t|VMIS->Verdict mismatch|"+tl.RuntimeID()+"|M.reject|error|pass", l.observe("t|tcfi|ntt|M.reject|pass"))
	assert.Equal(t, "", l.observe("t|tcfi|ntt|M.plain|inconc"))
	assert.Equal(t, "", l.observe("t|tcfi|ntt|M.plain|error"))
	// pass accept takes fail, not error.
	assert.Equal(t, "", l.observe("t|tcfi|ntt|M.accept|error"))
	assert.Equal(t, []string{"M.exact", "M.reject", "M.plain", "M.accept"}, l.failed)
}

// TestRunSuite runs a suite as `ntt run` does: the testcases a basket
// selects, every event in --output, the console's as the filter allows, and
// a run failing for a verdict its tag did not expect.
func TestRunSuite(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "example.ttcn3")
	if err := os.WriteFile(src, []byte(`module example {
	type component C {}
	testcase tc1() runs on C { setverdict(pass); }
	// @verdict: pass
	testcase tc2() runs on C { setverdict(fail); }
	control { execute(tc1()); }
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	conf, err := project.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	Project = conf
	defer func() { Project = nil; runLogFilter = logFilter{} }()

	out := filepath.Join(dir, "all.lines")
	var console bytes.Buffer
	RunCommand.SetOut(&console)
	defer RunCommand.SetOut(nil)
	runOutput, runConsole = out, "plain"
	defer func() { runOutput, runConsole = "", "auto" }()
	if err := RunCommand.Flags().Parse([]string{"-r", "tc", "-D", "-a", "tcfi|VMIS"}); err != nil {
		t.Fatal(err)
	}

	err = runSuite(RunCommand, []string{src})
	assert.ErrorIs(t, err, errSilentFailure)
	lines := strings.Split(strings.TrimSpace(console.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("console:\n%s", console.String())
	}
	assert.True(t, strings.HasSuffix(lines[0], "|tcfi|"+tl.RuntimeID()+"|example.tc1|pass"), lines[0])
	assert.True(t, strings.HasSuffix(lines[1], "|tcfi|"+tl.RuntimeID()+"|example.tc2|fail"), lines[1])
	assert.True(t, strings.HasSuffix(lines[2], "|VMIS->Verdict mismatch|"+tl.RuntimeID()+"=example.ttcn3:5|example.tc2|pass|fail"), lines[2])
	all, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	assert.Contains(t, string(all), "|tcst|"+tl.RuntimeID()+"|example.tc1|unlimited")
	assert.Contains(t, string(all), "|setv|")
	assert.Contains(t, string(all), "|VMIS->Verdict mismatch|")
}

// TestRunJobsTheSuiteHas: a test id names a testcase or a control part the
// suite has, a module's control only when it declares one.
func TestRunJobsTheSuiteHas(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.ttcn3")
	b := filepath.Join(dir, "b.ttcn3")
	if err := os.WriteFile(a, []byte(`module A { type component C {} testcase tc() runs on C {} control { execute(tc()); } }`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(`module B { type component C {} testcase tc() runs on C {} }`), 0o644); err != nil {
		t.Fatal(err)
	}
	d := newStaticDriver([]string{a, b})
	assert.True(t, d.hasJob("A.tc"))
	assert.True(t, d.hasJob("B.tc"))
	assert.True(t, d.hasJob("A.control"))
	assert.False(t, d.hasJob("B.control"))
	assert.False(t, d.hasJob("Nope.control"))
	assert.False(t, d.hasJob("A.nope"))
}

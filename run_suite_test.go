package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/nokia/ntt/commands/run"
	"github.com/nokia/ntt/runtime/tl"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

// TestRunSuite runs a suite as `ntt run` does with the eval backend: the
// testcases a basket selects, every event in --output, the console's as the
// filter allows, and a run failing for a verdict its tag did not expect.
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

	out := filepath.Join(dir, "all.lines")
	var console bytes.Buffer
	cmd := run.Command
	cmd.SetOut(&console)
	defer cmd.SetOut(nil)
	if err := cmd.Flags().Parse([]string{"--backend=eval", "--format=plain", "-o", out, "-r", "tc", "-D", "-a", "tcfi", src}); err != nil {
		t.Fatal(err)
	}
	// The command is the process's one. Its flags go back to their
	// defaults afterwards; the logging filter's rules, which no flag
	// value holds, stay, so a later test must not rely on them.
	defer func() {
		cmd.Flags().Visit(func(f *pflag.Flag) {
			if s, ok := f.Value.(pflag.SliceValue); ok {
				_ = s.Replace(nil)
			} else {
				_ = f.Value.Set(f.DefValue)
			}
			f.Changed = false
		})
	}()

	err := cmd.RunE(cmd, cmd.Flags().Args())
	assert.Error(t, err, "tc2 ends in fail, its tag expects pass")
	text := console.String()
	assert.Contains(t, text, "|tcfi|"+tl.RuntimeID()+"|example.tc1|pass", text)
	assert.Contains(t, text, "|tcfi|"+tl.RuntimeID()+"|example.tc2|fail", text)
	assert.NotContains(t, text, "|tcst|", text)
	assert.NotContains(t, text, "unparsed log line", text)
	all, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	assert.Contains(t, string(all), "|tcst|"+tl.RuntimeID()+"|example.tc1|")
	assert.Contains(t, string(all), "|setv|")
	assert.Contains(t, string(all), "|tcfi|"+tl.RuntimeID()+"|example.tc2|fail")
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

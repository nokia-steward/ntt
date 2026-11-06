package run

import (
	"testing"

	"github.com/nokia/ntt/internal/k3log"
	"github.com/nokia/ntt/internal/verify"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerdictCheck(t *testing.T) {
	const stamp = "20210506T092317.322187"
	p := plan{
		Controls: []Job{{Name: "Suite.control"}},
		Tests: []Job{
			{Name: "Suite.Tagged", Expect: verify.Expectation{Kind: verify.ExpectExact, Verdict: "error"}, Loc: "suite.ttcn3:3"},
			{Name: "Suite.Plain", Loc: "suite.ttcn3:4"},
			{Name: "Suite.Accept", Expect: verify.Expectation{Kind: verify.ExpectNotError}, Loc: "suite.ttcn3:5"},
			{Name: "Suite.Reject", Expect: verify.Expectation{Kind: verify.ExpectReject}, Loc: "suite.ttcn3:6"},
			{Name: "Suite.Reason", ExpectReason: "my|reason", Loc: "suite.ttcn3:7"},
			{Name: "Suite.Both", Expect: verify.Expectation{Kind: verify.ExpectExact, Verdict: "error"}, ExpectReason: "INOP->Invalid operation|MTC=suite.ttcn3:9|x", Loc: "suite.ttcn3:8"},
			{Name: "Suite.Escaped", Expect: verify.Expectation{Kind: verify.ExpectExact, Verdict: "fail"}, ExpectReason: `a\u007cb`, Loc: "suite.ttcn3:10"},
		},
	}
	tcfi := func(name, verdict string, reason ...string) k3log.Event {
		t.Helper()
		line := stamp + "|tcfi|k3r=(unknown):0|" + name + "|" + verdict
		for _, r := range reason {
			line += "|" + r
		}
		event, err := k3log.ParseEvent(line)
		require.NoError(t, err)
		return event
	}

	tests := []struct {
		name   string
		event  k3log.Event
		lines  []string
		failed []testcaseVerdict
	}{
		{
			name:  "tag matches",
			event: tcfi("Suite.Tagged", "error"),
		},
		{
			name:   "tag differs",
			event:  tcfi("Suite.Tagged()", "pass"),
			lines:  []string{mismatchLine(stamp, "suite.ttcn3:3", "Suite.Tagged", "error", "pass")},
			failed: []testcaseVerdict{{name: "Suite.Tagged", verdict: "pass", expected: "error"}},
		},
		{
			name:   "fail without tag",
			event:  tcfi("Suite.Plain", "fail"),
			failed: []testcaseVerdict{{name: "Suite.Plain", verdict: "fail"}},
		},
		{
			name:   "error without tag",
			event:  tcfi("Suite.Plain", "error"),
			failed: []testcaseVerdict{{name: "Suite.Plain", verdict: "error"}},
		},
		{
			name:  "pass without tag",
			event: tcfi("Suite.Plain", "pass"),
		},
		{
			name:  "accept keeps fail",
			event: tcfi("Suite.Accept", "fail"),
		},
		{
			name:   "accept rejects error",
			event:  tcfi("Suite.Accept", "error"),
			failed: []testcaseVerdict{{name: "Suite.Accept", verdict: "error"}},
		},
		{
			name:  "reject keeps error",
			event: tcfi("Suite.Reject", "error"),
		},
		{
			name:   "reject rejects pass",
			event:  tcfi("Suite.Reject", "pass"),
			lines:  []string{mismatchLine(stamp, "suite.ttcn3:6", "Suite.Reject", "error", "pass")},
			failed: []testcaseVerdict{{name: "Suite.Reject", verdict: "pass", expected: "error"}},
		},
		{
			name:  "reason matches",
			event: tcfi("Suite.Reason", "inconc", "my", "reason"),
		},
		{
			name:   "reason differs",
			event:  tcfi("Suite.Reason", "inconc", "other"),
			lines:  []string{reasonMismatchLine(stamp, "suite.ttcn3:7", "Suite.Reason", "my|reason", "other")},
			failed: []testcaseVerdict{{name: "Suite.Reason", verdict: "inconc", reason: "other", expectedReason: "my|reason"}},
		},
		{
			name:   "reason missing on tcfi",
			event:  tcfi("Suite.Reason", "pass"),
			lines:  []string{reasonMismatchLine(stamp, "suite.ttcn3:7", "Suite.Reason", "my|reason", "")},
			failed: []testcaseVerdict{{name: "Suite.Reason", verdict: "pass", expectedReason: "my|reason"}},
		},
		{
			name:   "reason alone keeps the default verdict check",
			event:  tcfi("Suite.Reason", "fail", "my", "reason"),
			failed: []testcaseVerdict{{name: "Suite.Reason", verdict: "fail"}},
		},
		{
			name:  "verdict and reason match",
			event: tcfi("Suite.Both", "error", "INOP->Invalid operation", "MTC=suite.ttcn3:9", "x"),
		},
		{
			name:  "verdict and reason differ",
			event: tcfi("Suite.Both", "pass", "done"),
			lines: []string{
				mismatchLine(stamp, "suite.ttcn3:8", "Suite.Both", "error", "pass"),
				reasonMismatchLine(stamp, "suite.ttcn3:8", "Suite.Both", "INOP->Invalid operation|MTC=suite.ttcn3:9|x", "done"),
			},
			failed: []testcaseVerdict{{name: "Suite.Both", verdict: "pass", expected: "error", reason: "done", expectedReason: "INOP->Invalid operation|MTC=suite.ttcn3:9|x"}},
		},
		{
			name:   "verdict differs, reason matches",
			event:  tcfi("Suite.Both", "fail", "INOP->Invalid operation", "MTC=suite.ttcn3:9", "x"),
			lines:  []string{mismatchLine(stamp, "suite.ttcn3:8", "Suite.Both", "error", "fail")},
			failed: []testcaseVerdict{{name: "Suite.Both", verdict: "fail", expected: "error"}},
		},
		{
			name:  "escaped bar matches the escaped tag",
			event: tcfi("Suite.Escaped", "fail", `a\u007cb`),
		},
		{
			name:   "separate arguments do not match an escaped bar",
			event:  tcfi("Suite.Escaped", "fail", "a", "b"),
			lines:  []string{reasonMismatchLine(stamp, "suite.ttcn3:10", "Suite.Escaped", `a\u007cb`, "a|b")},
			failed: []testcaseVerdict{{name: "Suite.Escaped", verdict: "fail", reason: "a|b", expectedReason: `a\u007cb`}},
		},
		{
			name:   "testcase unknown to the sources",
			event:  tcfi("Other.called", "fail"),
			failed: []testcaseVerdict{{name: "Other.called", verdict: "fail"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := verdictCheck{plan: p}
			assert.Equal(t, tt.lines, check.observe(tt.event))
			assert.Equal(t, tt.failed, check.failed)
		})
	}

	t.Run("reason mismatch line keeps one field per reason", func(t *testing.T) {
		line := reasonMismatchLine(stamp, "a.ttcn3:3", "a.tc", "x|y", "z")
		assert.Equal(t, stamp+`|RMIS->Reason mismatch|k3=a.ttcn3:3|a.tc|x\u007cy|z`, line)
		event, err := k3log.ParseEvent(line)
		require.NoError(t, err)
		assert.True(t, event.Category.IsError())
		assert.Equal(t, "RMIS", event.ID())
		assert.Equal(t, `x\u007cy`, event.Field(4))
		assert.Equal(t, "z", event.Field(5))
		assert.Empty(t, event.Field(6))
	})

	t.Run("ignores events other than tcfi", func(t *testing.T) {
		event, err := k3log.ParseEvent(stamp + "|tcst|k3r=(unknown):0|Suite.Plain")
		require.NoError(t, err)
		check := verdictCheck{plan: p}
		assert.Empty(t, check.observe(event))
		assert.Empty(t, check.failed)
	})
}

func TestVerdictCheckFinish(t *testing.T) {
	const stamp = "20210506T092317.322187"
	exact := verify.Expectation{Kind: verify.ExpectExact, Verdict: "pass"}
	p := plan{
		Controls: []Job{{Name: "Suite.control"}},
		Tests: []Job{
			{Name: "Suite.Verdict", Expect: exact, Loc: "suite.ttcn3:3"},
			{Name: "Suite.Reason", ExpectReason: "my|reason", Loc: "suite.ttcn3:4"},
			{Name: "Suite.Both", Expect: exact, ExpectReason: "ok", Loc: "suite.ttcn3:5"},
			{Name: "Suite.Plain", Loc: "suite.ttcn3:6"},
			{Name: "Suite.Accept", Expect: verify.Expectation{Kind: verify.ExpectNotError}, Loc: "suite.ttcn3:7"},
			{Name: "Suite.Reject", Expect: verify.Expectation{Kind: verify.ExpectReject}, Loc: "suite.ttcn3:8"},
			{Name: "Suite.Generic", Expect: exact, Loc: "suite.ttcn3:9"},
		},
	}
	tcfi := func(name, verdict string, reason ...string) k3log.Event {
		t.Helper()
		line := stamp + "|tcfi|k3r=(unknown):0|" + name + "|" + verdict
		for _, r := range reason {
			line += "|" + r
		}
		event, err := k3log.ParseEvent(line)
		require.NoError(t, err)
		return event
	}
	tagged := []string{"Suite.Verdict", "Suite.Reason", "Suite.Both", "Suite.Generic"}
	missing := map[string][]string{
		"Suite.Verdict": {mismatchLine(stamp, "suite.ttcn3:3", "Suite.Verdict", "pass", "")},
		"Suite.Reason":  {reasonMismatchLine(stamp, "suite.ttcn3:4", "Suite.Reason", "my|reason", "")},
		"Suite.Both": {
			mismatchLine(stamp, "suite.ttcn3:5", "Suite.Both", "pass", ""),
			reasonMismatchLine(stamp, "suite.ttcn3:5", "Suite.Both", "ok", ""),
		},
		"Suite.Generic": {mismatchLine(stamp, "suite.ttcn3:9", "Suite.Generic", "pass", "")},
	}
	failed := map[string]testcaseVerdict{
		"Suite.Verdict": {name: "Suite.Verdict", expected: "pass"},
		"Suite.Reason":  {name: "Suite.Reason", expectedReason: "my|reason"},
		"Suite.Both":    {name: "Suite.Both", expected: "pass", expectedReason: "ok"},
		"Suite.Generic": {name: "Suite.Generic", expected: "pass"},
	}
	// expect returns the lines and failures of finish when only the given
	// tagged testcases are missing.
	expect := func(names ...string) ([]string, []testcaseVerdict) {
		var lines []string
		var tests []testcaseVerdict
		for _, name := range names {
			lines = append(lines, missing[name]...)
			tests = append(tests, failed[name])
		}
		return lines, tests
	}

	t.Run("reports every tagged testcase that never finished", func(t *testing.T) {
		check := newVerdictCheck(p, true, p.Controls)
		lines, tests := expect(tagged...)
		assert.Equal(t, lines, check.finish(stamp))
		assert.Equal(t, tests, check.failed)
	})

	t.Run("matching tcfi events satisfy the tags", func(t *testing.T) {
		check := newVerdictCheck(p, true, p.Controls)
		for _, event := range []k3log.Event{
			tcfi("Suite.Verdict", "pass"),
			tcfi("Suite.Reason()", "pass", "my", "reason"),
			tcfi("Suite.Both", "pass", "ok"),
			tcfi("Suite.Generic", "pass"),
		} {
			assert.Empty(t, check.observe(event))
		}
		assert.Empty(t, check.finish(stamp))
		assert.Empty(t, check.failed)
	})

	t.Run("a mismatching tcfi is judged once and still counts as a run", func(t *testing.T) {
		check := newVerdictCheck(p, true, p.Controls)
		check.observe(tcfi("Suite.Verdict", "fail"))
		check.observe(tcfi("Suite.Verdict", "pass"))
		lines, tests := expect("Suite.Reason", "Suite.Both", "Suite.Generic")
		assert.Equal(t, lines, check.finish(stamp))
		assert.Equal(t, append([]testcaseVerdict{{name: "Suite.Verdict", verdict: "fail", expected: "pass"}}, tests...), check.failed)
	})

	t.Run("generic instances count as a run of the testcase", func(t *testing.T) {
		for _, name := range []string{"Suite.Generic<S>", "Suite.Generic<S>()", "Suite.Generic<integer>(1)"} {
			t.Run(name, func(t *testing.T) {
				check := newVerdictCheck(p, true, p.Controls)
				assert.Empty(t, check.observe(tcfi(name, "inconc")))
				assert.Empty(t, check.failed, "the instance verdict stays unmatched")
				lines, tests := expect("Suite.Verdict", "Suite.Reason", "Suite.Both")
				assert.Equal(t, lines, check.finish(stamp))
				assert.Equal(t, tests, check.failed)
			})
		}
	})

	t.Run("filtered run requires only started testcases", func(t *testing.T) {
		started := []Job{p.Tests[1], p.Tests[3]}
		check := newVerdictCheck(p, false, started)
		lines, tests := expect("Suite.Reason")
		assert.Equal(t, lines, check.finish(stamp))
		assert.Equal(t, tests, check.failed)
	})

	t.Run("missing lines keep an empty actual field", func(t *testing.T) {
		lines := newVerdictCheck(p, true, p.Controls).finish(stamp)
		assert.Equal(t, stamp+"|VMIS->Verdict mismatch|k3=suite.ttcn3:3|Suite.Verdict|pass|", lines[0])
		assert.Equal(t, stamp+`|RMIS->Reason mismatch|k3=suite.ttcn3:4|Suite.Reason|my\u007creason|`, lines[1])
		event, err := k3log.ParseEvent(lines[0])
		require.NoError(t, err)
		assert.Equal(t, "VMIS", event.ID())
		assert.Equal(t, "pass", event.Field(4))
		assert.Empty(t, event.Field(5))
	})
}

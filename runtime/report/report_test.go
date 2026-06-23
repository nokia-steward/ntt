package report_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/nokia/ntt/runtime/report"
)

func sampleSuite() *report.Suite {
	return &report.Suite{
		Name:  "sample",
		Start: time.Unix(0, 0),
		End:   time.Unix(0, int64(150*time.Millisecond)),
		Cases: []report.Case{
			{Module: "M", Name: "tc_pass", Verdict: report.Pass, Duration: 50 * time.Millisecond},
			{Module: "M", Name: "tc_fail", Verdict: report.Fail, Duration: 80 * time.Millisecond, Reason: "expected 1, got 2"},
			{Module: "M", Name: "tc_skip", Verdict: report.None, Duration: 0},
		},
	}
}

func TestAggregate(t *testing.T) {
	if got := report.Aggregate(report.Pass, report.Fail, report.None); got != report.Fail {
		t.Errorf("Aggregate = %v, want fail", got)
	}
	if got := report.Aggregate(); got != report.None {
		t.Errorf("Aggregate empty = %v, want none", got)
	}
}

func TestVerdictFromString(t *testing.T) {
	cases := []string{"pass", "Fail", " inconc ", "ERROR"}
	for _, c := range cases {
		if _, err := report.VerdictFromString(c); err != nil {
			t.Errorf("VerdictFromString(%q) error: %v", c, err)
		}
	}
	if _, err := report.VerdictFromString("oops"); err == nil {
		t.Error("expected error on unknown verdict")
	}
}

func TestRenderJUnit(t *testing.T) {
	var buf bytes.Buffer
	if err := report.RenderJUnit(&buf, sampleSuite()); err != nil {
		t.Fatalf("RenderJUnit: %v", err)
	}
	s := buf.String()
	if !strings.Contains(s, `<testsuite`) {
		t.Errorf("missing testsuite element: %s", s)
	}
	if !strings.Contains(s, `name="tc_pass"`) {
		t.Errorf("missing tc_pass case: %s", s)
	}
	if !strings.Contains(s, `<failure`) {
		t.Errorf("missing <failure> for tc_fail: %s", s)
	}
	if !strings.Contains(s, `<skipped`) {
		t.Errorf("missing <skipped>: %s", s)
	}
}

func TestRenderTAP(t *testing.T) {
	var buf bytes.Buffer
	if err := report.RenderTAP(&buf, sampleSuite()); err != nil {
		t.Fatalf("RenderTAP: %v", err)
	}
	s := buf.String()
	if !strings.HasPrefix(s, "TAP version 13\n1..3\n") {
		t.Errorf("missing TAP header: %s", s)
	}
	if !strings.Contains(s, "not ok 2 - M.tc_fail") {
		t.Errorf("missing fail line: %s", s)
	}
	if !strings.Contains(s, "ok 3 - M.tc_skip # SKIP") {
		t.Errorf("missing skip line: %s", s)
	}
}

func TestRenderJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := report.RenderJSON(&buf, sampleSuite()); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	s := buf.String()
	if !strings.Contains(s, `"verdict": "fail"`) {
		t.Errorf("missing aggregate verdict: %s", s)
	}
	if !strings.Contains(s, `"name": "tc_pass"`) {
		t.Errorf("missing tc_pass: %s", s)
	}
	if !strings.Contains(s, `"counts"`) {
		t.Errorf("missing counts: %s", s)
	}
}

func TestRenderHTML(t *testing.T) {
	var buf bytes.Buffer
	if err := report.RenderHTML(&buf, sampleSuite()); err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	s := buf.String()
	if !strings.Contains(s, "<title>sample test report</title>") {
		t.Errorf("missing title: %s", s[:200])
	}
	if !strings.Contains(s, "tc_pass") || !strings.Contains(s, "tc_fail") {
		t.Errorf("missing testcases: %s", s)
	}
}

func TestSuite_VerdictAndCounts(t *testing.T) {
	s := sampleSuite()
	if got := s.Verdict(); got != report.Fail {
		t.Errorf("Verdict = %v, want fail", got)
	}
	counts := s.CountBy()
	if counts[report.Pass] != 1 || counts[report.Fail] != 1 || counts[report.None] != 1 {
		t.Errorf("CountBy = %v", counts)
	}
}

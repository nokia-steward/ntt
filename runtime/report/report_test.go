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

func TestLatencyStatsFromSamples(t *testing.T) {
	// Empty -> zero stats.
	if got := report.LatencyStatsFromSamples(nil); got.Count != 0 {
		t.Fatalf("empty: count=%d, want 0", got.Count)
	}
	// 1..10 ms. Nearest-rank: p50=idx ceil(.5*10)-1=4 -> 5ms; p90 idx 8 -> 9ms;
	// p99 idx 9 -> 10ms; min 1ms, max 10ms, mean 5.5ms.
	var s []time.Duration
	for i := 1; i <= 10; i++ {
		s = append(s, time.Duration(i)*time.Millisecond)
	}
	got := report.LatencyStatsFromSamples(s)
	want := report.LatencyStats{
		Count: 10,
		Min:   1 * time.Millisecond,
		Max:   10 * time.Millisecond,
		Mean:  5500 * time.Microsecond,
		P50:   5 * time.Millisecond,
		P90:   9 * time.Millisecond,
		P99:   10 * time.Millisecond,
	}
	if got != want {
		t.Fatalf("stats=%+v, want %+v", got, want)
	}
}

func TestRenderProfile(t *testing.T) {
	s := &report.Suite{
		Name:  "prof",
		Start: time.Unix(0, 0),
		End:   time.Unix(1, 0),
		Cases: []report.Case{{
			Module:   "m",
			Name:     "tc",
			Verdict:  report.Pass,
			Duration: time.Second,
			Metrics: &report.Metrics{Ports: []report.PortMetric{{
				Port:       "p",
				Sends:      3,
				Receives:   3,
				Throughput: 3.0,
				Latency:    report.LatencyStatsFromSamples([]time.Duration{time.Millisecond, 2 * time.Millisecond, 3 * time.Millisecond}),
			}}},
		}},
	}
	var buf bytes.Buffer
	if err := report.RenderProfile(&buf, s); err != nil {
		t.Fatalf("RenderProfile: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"m.tc", "port", "p", "recv/s"} {
		if !strings.Contains(out, want) {
			t.Fatalf("profile output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderProfileNoMetrics(t *testing.T) {
	// A functional-only suite must not render silently empty.
	var buf bytes.Buffer
	if err := report.RenderProfile(&buf, sampleSuite()); err != nil {
		t.Fatalf("RenderProfile: %v", err)
	}
	if !strings.Contains(buf.String(), "no per-port metrics") {
		t.Fatalf("expected an explicit no-metrics note:\n%s", buf.String())
	}
}

func TestRenderHTMLIncludesMetrics(t *testing.T) {
	s := &report.Suite{
		Name:  "prof",
		Start: time.Unix(0, 0),
		End:   time.Unix(1, 0),
		Cases: []report.Case{{
			Module:   "app",
			Name:     "tc",
			Verdict:  report.Pass,
			Duration: time.Second,
			Metrics: &report.Metrics{Ports: []report.PortMetric{{
				Port:       "p",
				Sends:      6,
				Receives:   6,
				Throughput: 5644.9,
				Latency:    report.LatencyStatsFromSamples([]time.Duration{time.Millisecond, 2 * time.Millisecond, 3 * time.Millisecond}),
			}}},
		}},
	}
	var buf bytes.Buffer
	if err := report.RenderHTML(&buf, s); err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"Performance profile", "app.tc", ">p<", "recv/s", "p99", "5644.9"} {
		if !strings.Contains(out, want) {
			t.Fatalf("HTML profile section missing %q:\n%s", want, out)
		}
	}
}

func TestRenderHTMLNoMetricsSection(t *testing.T) {
	// A functional (non-profiling) run must not grow an empty profile table.
	var buf bytes.Buffer
	if err := report.RenderHTML(&buf, sampleSuite()); err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	if strings.Contains(buf.String(), "Performance profile") {
		t.Fatalf("unexpected profile section in a functional report:\n%s", buf.String())
	}
}

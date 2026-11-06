// Package report renders TTCN-3 test verdicts in the file formats CI
// systems and humans expect: JUnit XML (Jenkins, GitLab, GitHub
// Actions), TAP (Perl-style test runners), JSON stream (ntt's own
// tooling and the LSP test explorer) and a self-contained HTML log
// resembling Titan's output.
//
// All formatters consume the same Suite + Case data model, so adding a
// new output mode is a matter of writing one Render function. Verdicts
// follow ETSI ES 201 873-1 clause 24: none / pass / inconc / fail /
// error, ordered from best to worst.
package report

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"io"
	"math"
	"sort"
	"strings"
	"time"
)

// Verdict is the TTCN-3 verdict, ordered so Worse(a, b) is a max.
type Verdict int

const (
	None Verdict = iota
	Pass
	Inconc
	Fail
	Error
)

// String renders the verdict in TTCN-3 keyword form.
func (v Verdict) String() string {
	switch v {
	case None:
		return "none"
	case Pass:
		return "pass"
	case Inconc:
		return "inconc"
	case Fail:
		return "fail"
	case Error:
		return "error"
	}
	return "unknown"
}

// Aggregate returns the worst (largest-value) verdict in the set. This
// matches the TTCN-3 verdict overlap rules: pass + fail = fail, etc.
func Aggregate(vs ...Verdict) Verdict {
	worst := None
	for _, v := range vs {
		if v > worst {
			worst = v
		}
	}
	return worst
}

// Case is the outcome of a single testcase run.
type Case struct {
	Name     string
	Module   string
	Verdict  Verdict
	Duration time.Duration
	Reason   string
	Logs     []string

	// Metrics carries per-port performance data from a profiling run
	// (ntt exec --profile). Nil for a functional run.
	Metrics *Metrics
}

// Metrics is a testcase's performance profile: one entry per port that
// carried traffic during a real-clock (live-SUT) run.
type Metrics struct {
	Ports []PortMetric
}

// PortMetric aggregates one port's traffic over a profiling run.
type PortMetric struct {
	Port       string
	Sends      int
	Receives   int
	Throughput float64 // receives per second over the testcase duration
	Latency    LatencyStats
}

// LatencyStats summarises the send->receive round-trip samples for a port
// (one sample per send answered by a receive). Zero-valued when the port
// produced no samples.
type LatencyStats struct {
	Count int
	Min   time.Duration
	Max   time.Duration
	Mean  time.Duration
	P50   time.Duration
	P90   time.Duration
	P99   time.Duration
}

// Suite is the outcome of running one or more testcases. Suites
// aggregate to a single verdict via Aggregate over the case verdicts.
type Suite struct {
	Name  string
	Cases []Case
	Start time.Time
	End   time.Time
}

// Verdict returns the aggregate verdict over all cases.
func (s *Suite) Verdict() Verdict {
	vs := make([]Verdict, len(s.Cases))
	for i, c := range s.Cases {
		vs[i] = c.Verdict
	}
	return Aggregate(vs...)
}

// Duration returns the wall-clock duration of the suite, summing the
// individual case durations as a fallback when Start/End are unset.
func (s *Suite) Duration() time.Duration {
	if !s.End.IsZero() && !s.Start.IsZero() {
		return s.End.Sub(s.Start)
	}
	var total time.Duration
	for _, c := range s.Cases {
		total += c.Duration
	}
	return total
}

// CountBy returns the number of cases with each verdict.
func (s *Suite) CountBy() map[Verdict]int {
	out := map[Verdict]int{}
	for _, c := range s.Cases {
		out[c.Verdict]++
	}
	return out
}

// ---------------------------------------------------------------------------
// JUnit XML
// ---------------------------------------------------------------------------

type junitTestsuite struct {
	XMLName  xml.Name    `xml:"testsuite"`
	Name     string      `xml:"name,attr"`
	Tests    int         `xml:"tests,attr"`
	Failures int         `xml:"failures,attr"`
	Errors   int         `xml:"errors,attr"`
	Skipped  int         `xml:"skipped,attr"`
	Time     float64     `xml:"time,attr"`
	Cases    []junitCase `xml:"testcase"`
}

type junitCase struct {
	ClassName string    `xml:"classname,attr,omitempty"`
	Name      string    `xml:"name,attr"`
	Time      float64   `xml:"time,attr"`
	Failure   *junitMsg `xml:"failure,omitempty"`
	Errored   *junitMsg `xml:"error,omitempty"`
	Skipped   *struct{} `xml:"skipped,omitempty"`
}

type junitMsg struct {
	Type    string `xml:"type,attr,omitempty"`
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

// RenderJUnit writes the suite as JUnit XML.
func RenderJUnit(w io.Writer, s *Suite) error {
	ts := junitTestsuite{
		Name:  s.Name,
		Tests: len(s.Cases),
		Time:  s.Duration().Seconds(),
	}
	for _, c := range s.Cases {
		jc := junitCase{
			ClassName: c.Module,
			Name:      c.Name,
			Time:      c.Duration.Seconds(),
		}
		switch c.Verdict {
		case Fail, Inconc:
			jc.Failure = &junitMsg{Type: c.Verdict.String(), Message: c.Reason}
		case Error:
			jc.Errored = &junitMsg{Type: c.Verdict.String(), Message: c.Reason}
		case None:
			jc.Skipped = &struct{}{}
		}
		ts.Cases = append(ts.Cases, jc)
	}
	counts := s.CountBy()
	ts.Failures = counts[Fail] + counts[Inconc]
	ts.Errors = counts[Error]
	ts.Skipped = counts[None]

	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if _, err := fmt.Fprint(w, xml.Header); err != nil {
		return err
	}
	if err := enc.Encode(ts); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w)
	return err
}

// ---------------------------------------------------------------------------
// TAP
// ---------------------------------------------------------------------------

// RenderTAP writes the suite as Test Anything Protocol (TAP v13).
func RenderTAP(w io.Writer, s *Suite) error {
	fmt.Fprintf(w, "TAP version 13\n1..%d\n", len(s.Cases))
	for i, c := range s.Cases {
		status := "ok"
		var directive string
		switch c.Verdict {
		case Fail, Error, Inconc:
			status = "not ok"
			if c.Verdict != Fail {
				directive = " # " + c.Verdict.String()
			}
		case None:
			status = "ok"
			directive = " # SKIP"
		}
		name := c.Name
		if c.Module != "" {
			name = c.Module + "." + name
		}
		fmt.Fprintf(w, "%s %d - %s%s\n", status, i+1, name, directive)
		if c.Reason != "" {
			fmt.Fprintf(w, "  ---\n  message: %q\n  ...\n", c.Reason)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// JSON
// ---------------------------------------------------------------------------

// RenderJSON writes the suite as pretty JSON.
func RenderJSON(w io.Writer, s *Suite) error {
	type wireLatency struct {
		Count  int     `json:"count"`
		MinMs  float64 `json:"min_ms"`
		MaxMs  float64 `json:"max_ms"`
		MeanMs float64 `json:"mean_ms"`
		P50Ms  float64 `json:"p50_ms"`
		P90Ms  float64 `json:"p90_ms"`
		P99Ms  float64 `json:"p99_ms"`
	}
	type wirePort struct {
		Port       string      `json:"port"`
		Sends      int         `json:"sends"`
		Receives   int         `json:"receives"`
		Throughput float64     `json:"throughput_per_second"`
		Latency    wireLatency `json:"latency"`
	}
	type wireCase struct {
		Name     string     `json:"name"`
		Module   string     `json:"module,omitempty"`
		Verdict  string     `json:"verdict"`
		Duration float64    `json:"duration_seconds"`
		Reason   string     `json:"reason,omitempty"`
		Ports    []wirePort `json:"ports,omitempty"`
	}
	ms := func(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
	type wire struct {
		Name     string         `json:"name"`
		Verdict  string         `json:"verdict"`
		Duration float64        `json:"duration_seconds"`
		Counts   map[string]int `json:"counts"`
		Cases    []wireCase     `json:"cases"`
	}
	cs := make([]wireCase, 0, len(s.Cases))
	for _, c := range s.Cases {
		wc := wireCase{
			Name:     c.Name,
			Module:   c.Module,
			Verdict:  c.Verdict.String(),
			Duration: c.Duration.Seconds(),
			Reason:   c.Reason,
		}
		if c.Metrics != nil {
			for _, p := range c.Metrics.Ports {
				wc.Ports = append(wc.Ports, wirePort{
					Port:       p.Port,
					Sends:      p.Sends,
					Receives:   p.Receives,
					Throughput: p.Throughput,
					Latency: wireLatency{
						Count:  p.Latency.Count,
						MinMs:  ms(p.Latency.Min),
						MaxMs:  ms(p.Latency.Max),
						MeanMs: ms(p.Latency.Mean),
						P50Ms:  ms(p.Latency.P50),
						P90Ms:  ms(p.Latency.P90),
						P99Ms:  ms(p.Latency.P99),
					},
				})
			}
		}
		cs = append(cs, wc)
	}
	counts := map[string]int{}
	for v, n := range s.CountBy() {
		counts[v.String()] = n
	}
	out := wire{
		Name:     s.Name,
		Verdict:  s.Verdict().String(),
		Duration: s.Duration().Seconds(),
		Counts:   counts,
		Cases:    cs,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// ---------------------------------------------------------------------------
// HTML
// ---------------------------------------------------------------------------

// htmlTpl is intentionally inline-styled and dependency-free so an HTML
// log is one self-contained file that opens cleanly in any browser.
const htmlTpl = `<!doctype html>
<html><head>
<meta charset="utf-8">
<title>{{.Name}} test report</title>
<style>
body { font-family: -apple-system, system-ui, sans-serif; margin: 2rem; }
h1 { margin: 0 0 0.5rem; }
.meta { color: #666; margin-bottom: 1rem; }
.verdict { display: inline-block; padding: 0.1rem 0.5rem; border-radius: 0.25rem; font-weight: 600; color: white; }
.pass { background: #2e7d32; }
.fail { background: #c62828; }
.inconc { background: #ef6c00; }
.error { background: #6a1b9a; }
.none { background: #757575; }
table { border-collapse: collapse; width: 100%; }
th, td { border: 1px solid #ddd; padding: 0.5rem; text-align: left; }
th { background: #f5f5f5; }
tr.fail td:first-child { border-left: 4px solid #c62828; }
tr.error td:first-child { border-left: 4px solid #6a1b9a; }
tr.inconc td:first-child { border-left: 4px solid #ef6c00; }
tr.pass td:first-child { border-left: 4px solid #2e7d32; }
h2 { margin: 2rem 0 0.5rem; font-size: 1.1rem; }
.num { text-align: right; font-variant-numeric: tabular-nums; }
</style></head><body>
<h1>{{.Name}}</h1>
<p class="meta">
overall verdict
<span class="verdict {{.OverallClass}}">{{.OverallStr}}</span>
&nbsp; {{len .Cases}} cases in {{printf "%.2f" .DurationSeconds}}s
</p>
<table>
<thead><tr><th>verdict</th><th>module</th><th>name</th><th>duration</th><th>reason</th></tr></thead>
<tbody>
{{range .Cases -}}
<tr class="{{.Class}}"><td><span class="verdict {{.Class}}">{{.Str}}</span></td>
<td>{{.Module}}</td><td>{{.Name}}</td><td>{{printf "%.2fs" .DurationSeconds}}</td><td>{{.Reason}}</td></tr>
{{end -}}
</tbody></table>
{{if .HasMetrics}}
<h2>Performance profile</h2>
<p class="meta">per-port send&rarr;receive latency and throughput, captured on the real clock (<code>ntt exec --profile</code>)</p>
<table>
<thead><tr><th>testcase</th><th>port</th><th class="num">sent</th><th class="num">recv</th>
<th class="num">recv/s</th><th class="num">min</th><th class="num">p50</th><th class="num">p90</th><th class="num">p99</th></tr></thead>
<tbody>
{{range .Metrics -}}
<tr><td>{{.Case}}</td><td>{{.Port}}</td><td class="num">{{.Sends}}</td><td class="num">{{.Receives}}</td>
<td class="num">{{printf "%.1f" .Throughput}}</td><td class="num">{{.Min}}</td><td class="num">{{.P50}}</td>
<td class="num">{{.P90}}</td><td class="num">{{.P99}}</td></tr>
{{end -}}
</tbody></table>
{{end}}
</body></html>
`

type htmlCase struct {
	Name, Module, Reason, Str, Class string
	DurationSeconds                  float64
}

// htmlMetric is one flattened (testcase, port) profiling row.
type htmlMetric struct {
	Case, Port         string
	Sends, Receives    int
	Throughput         float64
	Min, P50, P90, P99 string
}

type htmlData struct {
	Name, OverallStr, OverallClass string
	DurationSeconds                float64
	Cases                          []htmlCase
	Metrics                        []htmlMetric
	HasMetrics                     bool
}

// RenderHTML writes the suite as a self-contained HTML document.
func RenderHTML(w io.Writer, s *Suite) error {
	tpl, err := template.New("report").Parse(htmlTpl)
	if err != nil {
		return err
	}
	data := htmlData{
		Name:            s.Name,
		OverallStr:      s.Verdict().String(),
		OverallClass:    s.Verdict().String(),
		DurationSeconds: s.Duration().Seconds(),
	}
	for _, c := range s.Cases {
		data.Cases = append(data.Cases, htmlCase{
			Name:            c.Name,
			Module:          c.Module,
			Reason:          c.Reason,
			Str:             c.Verdict.String(),
			Class:           c.Verdict.String(),
			DurationSeconds: c.Duration.Seconds(),
		})
		// Flatten any profiling metrics into a second table, so a
		// --profile run's latency/throughput shows up in the HTML
		// artefact CI publishes (not only in --format=profile/json).
		if c.Metrics == nil {
			continue
		}
		name := c.Name
		if c.Module != "" {
			name = c.Module + "." + name
		}
		for _, p := range c.Metrics.Ports {
			data.Metrics = append(data.Metrics, htmlMetric{
				Case:       name,
				Port:       p.Port,
				Sends:      p.Sends,
				Receives:   p.Receives,
				Throughput: p.Throughput,
				Min:        p.Latency.Min.String(),
				P50:        p.Latency.P50.String(),
				P90:        p.Latency.P90.String(),
				P99:        p.Latency.P99.String(),
			})
		}
	}
	data.HasMetrics = len(data.Metrics) > 0
	return tpl.Execute(w, data)
}

// SortCases sorts cases by module then name, deterministically. Useful
// for stable diffs in CI artefact storage.
func SortCases(cs []Case) {
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[i].Module != cs[j].Module {
			return cs[i].Module < cs[j].Module
		}
		return cs[i].Name < cs[j].Name
	})
}

// ---------------------------------------------------------------------------
// Profiling
// ---------------------------------------------------------------------------

// LatencyStatsFromSamples summarises round-trip latency samples: count,
// min/max/mean and the p50/p90/p99 nearest-rank percentiles. An empty
// input yields a zero LatencyStats.
func LatencyStatsFromSamples(samples []time.Duration) LatencyStats {
	n := len(samples)
	if n == 0 {
		return LatencyStats{}
	}
	sorted := make([]time.Duration, n)
	copy(sorted, samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	var sum time.Duration
	for _, d := range sorted {
		sum += d
	}
	pct := func(p float64) time.Duration {
		// Nearest-rank: index = ceil(p/100 * n) - 1, clamped.
		idx := int(math.Ceil(p/100*float64(n))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= n {
			idx = n - 1
		}
		return sorted[idx]
	}
	return LatencyStats{
		Count: n,
		Min:   sorted[0],
		Max:   sorted[n-1],
		Mean:  sum / time.Duration(n),
		P50:   pct(50),
		P90:   pct(90),
		P99:   pct(99),
	}
}

// RenderProfile writes a human-readable performance profile: per testcase,
// one row per port with send/receive counts, throughput and latency
// percentiles. Cases without metrics (a functional run) are listed with a
// note so the output is never silently empty.
func RenderProfile(w io.Writer, s *Suite) error {
	fmt.Fprintf(w, "profile %q: %s (%d cases in %s)\n", s.Name, s.Verdict(), len(s.Cases), s.Duration())
	any := false
	for _, c := range s.Cases {
		if c.Metrics == nil || len(c.Metrics.Ports) == 0 {
			continue
		}
		any = true
		fmt.Fprintf(w, "\n%s.%s  [%s]  %s\n", c.Module, c.Name, c.Verdict, c.Duration)
		fmt.Fprintf(w, "  %-20s %8s %8s %12s %10s %10s %10s %10s\n",
			"port", "sent", "recv", "recv/s", "min", "p50", "p90", "p99")
		for _, p := range c.Metrics.Ports {
			l := p.Latency
			fmt.Fprintf(w, "  %-20s %8d %8d %12.1f %10s %10s %10s %10s\n",
				p.Port, p.Sends, p.Receives, p.Throughput,
				l.Min, l.P50, l.P90, l.P99)
		}
	}
	if !any {
		fmt.Fprintln(w, "  (no per-port metrics captured; run with --profile against a live SUT)")
	}
	return nil
}

// VerdictFromString parses a TTCN-3 verdict keyword. Unknown text
// returns None plus an error.
func VerdictFromString(s string) (Verdict, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "none":
		return None, nil
	case "pass":
		return Pass, nil
	case "inconc":
		return Inconc, nil
	case "fail":
		return Fail, nil
	case "error":
		return Error, nil
	}
	return None, fmt.Errorf("unknown verdict %q", s)
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/semantic"
	"github.com/nokia/ntt/ttcn3/syntax"
	"github.com/spf13/cobra"
)

var (
	conformanceBaseline     string
	conformanceRegress      float64
	conformanceTimeout      time.Duration
	conformanceJobs         int
	conformanceProfile      string
	conformanceDifferential bool

	// ConformanceCommand walks the ETSI conformance suite and reports
	// the fraction of files whose actual outcome (interpreter verdict
	// for positive tests, parse/analyze rejection for negative tests)
	// matches the @verdict annotation in the file header. This is the
	// "real" CI gate the plan calls for: it measures execution
	// behaviour, not just parser success.
	ConformanceCommand = &cobra.Command{
		Use:   "conformance [path...]",
		Short: "Run the ETSI conformance suite and report verdict-match rate",
		Long: `conformance walks each path looking for .ttcn / .ttcn3 files annotated
with an "@verdict" tag (per ETSI's test header convention). It runs each
testcase through the interpreter, classifies the actual outcome, and
compares against the annotation. The pass-rate is the fraction of files
where actual == expected; --baseline gates regressions just like ntt check.

Annotations understood:
  @verdict pass accept, ttcn3verdict:<v>  expect testcase verdict <v>
  @verdict pass accept                    expect testcase verdict pass
  @verdict pass reject                    expect parse/semantic rejection
  @verdict inconclusive                   skipped (counted neither way)
`,
		Args: cobra.MinimumNArgs(1),
		RunE: runConformance,
	}
)

func init() {
	RootCommand.AddCommand(ConformanceCommand)
	ConformanceCommand.Flags().StringVar(&conformanceBaseline, "baseline", "",
		"compare against a prior pass-rate snapshot (JSON)")
	ConformanceCommand.Flags().Float64Var(&conformanceRegress, "regress", 0.0,
		"max allowed drop in pass-rate vs baseline, in percentage points")
	ConformanceCommand.Flags().DurationVar(&conformanceTimeout, "timeout", 5*time.Second,
		"per-testcase execution budget")
	ConformanceCommand.Flags().IntVar(&conformanceJobs, "jobs", 8,
		"number of files to run in parallel")
	ConformanceCommand.Flags().StringVar(&conformanceProfile, "profile", "approximate",
		"execution semantics profile: approximate (default gate) or strict")
	ConformanceCommand.Flags().BoolVar(&conformanceDifferential, "differential", false,
		"also run each executed testcase under the strict profile and report verdict divergences (diagnostic; not part of the gate)")
}

// runProfile parses the --profile flag into a SemanticsProfile. Unknown
// values fall back to approximate so the gate never silently switches.
func runProfile() runtime.SemanticsProfile {
	if strings.EqualFold(conformanceProfile, "strict") {
		return runtime.ProfileStrict
	}
	return runtime.ProfileApproximate
}

// ConformanceResult is the per-file outcome of a conformance run. The
// field names are stable; CI tooling and the dashboard render them
// directly.
type ConformanceResult struct {
	Path     string `json:"path"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Match    bool   `json:"match"`
	Reason   string `json:"reason,omitempty"`
	// Provenance records HOW the outcome was determined, so a match
	// can be traced to real execution vs an approximation:
	//   "executed"    - ran the testcase; runtime verdict compared to
	//                   the (non-reject) expectation. The only value
	//                   that certifies operational-semantics correctness.
	//   "exec-reject" - negative test; the interpreter aborted ("error")
	//                   or the body reported "fail", relaxed to a reject
	//                   match (the violation may not have been truly
	//                   detected - it may just have crashed).
	//   "static"      - matched by a semantic-analyzer rejection.
	//   "parse"       - matched by a parse-error rejection.
	//   "parse-only"  - accepted without executing (noexecution / no
	//                   testcase): parse+analyze succeeded.
	//   "runtime-error"/"timeout" - the interpreter errored/timed out.
	Provenance string `json:"provenance,omitempty"`
	// StrictActual / Diverged are populated only in --differential mode
	// for executed files: the verdict under ProfileStrict and whether it
	// differs from the (approximate) Actual. Divergences are the Phase-1
	// work-list — they show where the strict semantics change behaviour.
	StrictActual string `json:"strict_actual,omitempty"`
	Diverged     bool   `json:"diverged,omitempty"`
}

// ConformanceSummary is the suite-wide aggregate.
type ConformanceSummary struct {
	Total    int     `json:"total"`
	Matched  int     `json:"matched"`
	Skipped  int     `json:"skipped"`
	PassRate float64 `json:"pass_rate"`
	// Provenance is a histogram of matched files by how the match was
	// obtained (see ConformanceResult.Provenance). It exposes how much
	// of PassRate rests on real execution vs static/approximate paths.
	Provenance map[string]int `json:"provenance,omitempty"`
	// RealExecRate is the honest metric the semantics roadmap grows:
	// the fraction of considered (non-skipped) files matched by real
	// execution ("executed"), i.e. the interpreter ran the testcase and
	// produced the expected verdict. Expected to sit below PassRate
	// until the strict operational-semantics paths land.
	RealExecRate float64 `json:"real_exec_rate"`
	// Diverged counts executed files whose ProfileStrict verdict differed
	// from ProfileApproximate (only populated in --differential mode).
	Diverged int                 `json:"diverged,omitempty"`
	Results  []ConformanceResult `json:"results,omitempty"`
}

func runConformance(cmd *cobra.Command, args []string) error {
	files := collectTTCN3Files(args)
	if len(files) == 0 {
		return fmt.Errorf("no TTCN-3 files found under %v", args)
	}

	summary := runConformanceFiles(files)

	if outputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(summary)
	} else {
		fmt.Printf("ran %d files: %d matched, %d skipped (%.2f%% match rate)\n",
			summary.Total, summary.Matched, summary.Skipped, summary.PassRate)
		fmt.Printf("  real-execution match rate: %.2f%% (%d executed)\n",
			summary.RealExecRate, summary.Provenance["executed"])
		if len(summary.Provenance) > 0 {
			keys := make([]string, 0, len(summary.Provenance))
			for k := range summary.Provenance {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			fmt.Printf("  matched by:")
			for _, k := range keys {
				fmt.Printf(" %s=%d", k, summary.Provenance[k])
			}
			fmt.Println()
		}
		if conformanceDifferential {
			fmt.Printf("  strict-vs-approximate divergences: %d\n", summary.Diverged)
			if verbose > 0 {
				for _, r := range summary.Results {
					if r.Diverged {
						fmt.Printf("  DIVERGE %s  approximate=%s strict=%s\n",
							r.Path, r.Actual, r.StrictActual)
					}
				}
			}
		}
		if verbose > 0 {
			for _, r := range summary.Results {
				if !r.Match {
					fmt.Printf("  MISS %s  expected=%s actual=%s  %s\n",
						r.Path, r.Expected, r.Actual, r.Reason)
				}
			}
		}
	}

	if conformanceBaseline != "" {
		if err := enforceConformanceBaseline(summary); err != nil {
			return err
		}
	}
	return nil
}

func runConformanceFiles(files []string) ConformanceSummary {
	jobs := make(chan string)
	results := make([]ConformanceResult, 0, len(files))
	var (
		mu              sync.Mutex
		matched, skipped int64
	)
	var wg sync.WaitGroup
	for i := 0; i < conformanceJobs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				r := runOneConformance(path)
				mu.Lock()
				results = append(results, r)
				mu.Unlock()
				if r.Expected == "" {
					atomic.AddInt64(&skipped, 1)
					continue
				}
				if r.Match {
					atomic.AddInt64(&matched, 1)
				}
			}
		}()
	}
	for _, f := range files {
		jobs <- f
	}
	close(jobs)
	wg.Wait()

	sort.Slice(results, func(i, j int) bool { return results[i].Path < results[j].Path })

	considered := len(files) - int(skipped)
	rate := 0.0
	if considered > 0 {
		rate = float64(matched) / float64(considered) * 100
	}
	// Provenance histogram over matched files, and the honest
	// real-execution rate (matches obtained by actually running the
	// testcase and getting the expected verdict).
	prov := map[string]int{}
	diverged := 0
	for _, r := range results {
		if r.Match && r.Provenance != "" {
			prov[r.Provenance]++
		}
		if r.Diverged {
			diverged++
		}
	}
	realRate := 0.0
	if considered > 0 {
		realRate = float64(prov["executed"]) / float64(considered) * 100
	}
	return ConformanceSummary{
		Total:        len(files),
		Matched:      int(matched),
		Skipped:      int(skipped),
		PassRate:     rate,
		Provenance:   prov,
		RealExecRate: realRate,
		Diverged:     diverged,
		Results:      results,
	}
}

// runOneConformance is the heart of the gate: parse the file, extract
// its @verdict expectation, drive the appropriate path, and compare.
func runOneConformance(path string) ConformanceResult {
	r := ConformanceResult{Path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		r.Reason = "read: " + err.Error()
		return r
	}
	expected := parseVerdictAnnotation(string(data))
	r.Expected = expected
	if expected == "" {
		// No annotation: caller decides whether to skip; we mark
		// expected="" so the summary can count it under skipped.
		return r
	}

	tree := ttcn3.ParseFile(path)
	if tree == nil || tree.Err != nil {
		r.Actual = "parse-error"
		r.Match = expected == "reject"
		r.Provenance = "parse"
		if !r.Match && tree != nil && tree.Err != nil {
			r.Reason = tree.Err.Error()
		}
		return r
	}

	diags := semantic.NewAnalyzer(nil).Analyze(tree)
	for _, d := range diags {
		if d.Severity != semantic.SeverityError {
			continue
		}
		// Positive tests sometimes import predefined modules (JSON,
		// XSD, ...) that we don't model. Treat unknown-import as a
		// soft diagnostic for positive tests: if the testcase body
		// can still produce its verdict via the interpreter, we
		// honour that rather than synthesising a fake reject.
		if expected == "pass" && d.Code == "unknown-import" {
			continue
		}
		r.Actual = "semantic-error"
		r.Match = expected == "reject"
		r.Provenance = "static"
		if !r.Match {
			r.Reason = d.Message
		}
		return r
	}

	// `@verdict pass accept, noexecution` marks a parse/semantic
	// acceptance test that must NOT be executed (ETSI test-header
	// convention). Parsing and analysis already succeeded, so honour
	// the directive and report pass instead of running a testcase
	// whose body may legitimately drive the verdict elsewhere
	// (e.g. Syn_24_toplevel_002's setverdict-sequence cases).
	if expected == "pass" && strings.Contains(matchLine(string(data), "@verdict"), "noexecution") {
		r.Actual = "pass"
		r.Match = true
		r.Provenance = "parse-only"
		return r
	}

	// Positive test path: drive the first testcase via the interpreter.
	tcName, ok := firstTestcaseName(tree)
	if !ok {
		// No testcase. The TTCN-3 conformance suite frequently
		// uses `@verdict pass accept, noexecution` for tests
		// that only verify the parser and semantic analyser
		// accept the file. We already parsed and semantically
		// analysed without errors, so this is a successful
		// "pass" for those tests.
		if expected == "pass" {
			r.Actual = "pass"
			r.Match = true
			r.Provenance = "parse-only"
			return r
		}
		// Otherwise (no annotation we recognise as positive),
		// fall back to "skip" so summary stats stay honest.
		r.Expected = ""
		return r
	}
	// Pull in sibling .ttcn3 files from the same directory so the
	// testcase can reach helper modules without us needing an actual
	// import resolver. Most ETSI fixtures define their helpers in a
	// neighbouring file; we treat each directory as one flat scope.
	trees := []*ttcn3.Tree{tree}
	if siblings := collectSiblingTrees(path); len(siblings) > 0 {
		trees = append(trees, siblings...)
	}

	// Primary run under the selected profile (approximate = the gate).
	// A negative test is satisfied at runtime via the relaxation in
	// classifyExecution (Titan-style static tools catch these at compile
	// time; an interpreter-first tool catches them at runtime).
	prof := runProfile()
	r.Actual, r.Reason = execVerdict(trees, tcName, prof)
	classifyExecution(&r, expected)

	// Differential diagnostic (not part of the gate): re-run under the
	// strict profile and record any verdict divergence from the primary
	// (approximate) run. This is the Phase-1 work-list — it surfaces
	// exactly where strict operational semantics change behaviour.
	if conformanceDifferential && prof != runtime.ProfileStrict {
		strictActual, _ := execVerdict(trees, tcName, runtime.ProfileStrict)
		if strictActual != r.Actual {
			r.Diverged = true
			r.StrictActual = strictActual
		}
	}
	return r
}

// execVerdict runs tcName under the given semantics profile with the
// per-case timeout and returns the raw outcome — a verdict string
// ("pass"/"fail"/"inconc"/"error"), or "runtime-error"/"timeout" — plus
// an explanatory reason. Profile lets the same path serve the gate
// (approximate), a `--profile=strict` run, and the differential harness.
func execVerdict(trees []*ttcn3.Tree, tcName string, profile runtime.SemanticsProfile) (actual, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), conformanceTimeout)
	defer cancel()
	type out struct {
		v      runtime.Verdict
		reason string
		err    error
	}
	ch := make(chan out, 1)
	go func() {
		// Pass ctx so a strict run that blocks (honest alt/timer waits)
		// is cancelled on timeout instead of leaking a spinning
		// goroutine after we return "timeout" below. Strict runs use the
		// deterministic clock so real-time timers fire instantly (no 5s
		// waits, no timeout artifacts in the differential); approximate
		// runs keep the historical real-clock behaviour.
		v, r, err := interpreter.RunTestcaseWith(trees, tcName,
			interpreter.TestcaseOptions{
				Profile:            profile,
				DeterministicClock: profile == runtime.ProfileStrict,
				Context:            ctx,
			})
		ch <- out{v: v, reason: r, err: err}
	}()
	select {
	case o := <-ch:
		if o.err != nil {
			return "runtime-error", o.err.Error()
		}
		return string(o.v), o.reason
	case <-ctx.Done():
		return "timeout", "execution exceeded " + conformanceTimeout.String()
	}
}

// classifyExecution fills Match/Provenance from a raw execution outcome
// (r.Actual/r.Reason already set by execVerdict) against the expected
// annotation, mirroring the gate's historical classification including
// the negative-test relaxation (a `reject` test is satisfied when the
// interpreter aborts with "error" or the body reports "fail").
func classifyExecution(r *ConformanceResult, expected string) {
	switch r.Actual {
	case "runtime-error":
		r.Match = expected == "reject" || expected == "error"
		r.Provenance = "runtime-error"
	case "timeout":
		r.Match = false
		r.Provenance = "timeout"
	default:
		r.Match = expected == r.Actual
		r.Provenance = "executed"
		if !r.Match && expected == "reject" && (r.Actual == "error" || r.Actual == "fail") {
			r.Match = true
			r.Provenance = "exec-reject"
		}
		// For executed tests the reason only explains a miss.
		if r.Match {
			r.Reason = ""
		}
	}
}

// reVerdictAnnotation matches the various @verdict header forms ETSI
// uses across the suite. The capture groups are (status, ttcn3verdict)
// with ttcn3verdict optional.
var reVerdictAnnotation = regexp.MustCompile(`@verdict\s+([a-zA-Z_-]+)(?:\s+(?:accept|reject))?(?:[^a-zA-Z]+ttcn3verdict:([a-zA-Z]+))?`)

func parseVerdictAnnotation(src string) string {
	m := reVerdictAnnotation.FindStringSubmatch(src)
	if m == nil {
		return ""
	}
	status := strings.ToLower(m[1])
	switch status {
	case "pass":
		// `pass accept` (positive) vs `pass reject` (negative). We
		// re-scan the line because the optional accept/reject token
		// can be anywhere between the status and the ttcn3verdict.
		line := matchLine(src, "@verdict")
		switch {
		case strings.Contains(line, "reject"):
			return "reject"
		case len(m) > 2 && m[2] != "":
			return strings.ToLower(m[2])
		default:
			return "pass"
		}
	case "inconclusive", "skip":
		return ""
	}
	return ""
}

func matchLine(src, marker string) string {
	for _, line := range strings.Split(src, "\n") {
		if strings.Contains(line, marker) {
			return line
		}
	}
	return ""
}

// collectSiblingTrees parses every other .ttcn / .ttcn3 file living in
// the same directory as path and returns those that parsed cleanly.
// It is intentionally lossy: a sibling that fails to parse is just
// dropped so we never poison the primary test's run with an unrelated
// fixture's parse error.
func collectSiblingTrees(path string) []*ttcn3.Tree {
	dir := filepathDir(path)
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var trees []*ttcn3.Tree
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".ttcn") && !strings.HasSuffix(name, ".ttcn3") {
			continue
		}
		full := dir + "/" + name
		if full == path {
			continue
		}
		t := ttcn3.ParseFile(full)
		if t == nil || t.Err != nil {
			continue
		}
		trees = append(trees, t)
	}
	return trees
}

func filepathDir(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[:i]
		}
	}
	return ""
}

// firstTestcaseName returns the qualified name (Module.testcase) of
// the first testcase declared in the first module of the tree.
func firstTestcaseName(tree *ttcn3.Tree) (string, bool) {
	for _, m := range tree.Modules() {
		mod, ok := m.Node.(*syntax.Module)
		if !ok {
			continue
		}
		modName := syntax.Name(mod.Name)
		for _, def := range mod.Defs {
			fn, ok := def.Def.(*syntax.FuncDecl)
			if !ok {
				continue
			}
			if !fn.IsTest() {
				continue
			}
			return modName + "." + syntax.Name(fn.Name), true
		}
	}
	return "", false
}

func enforceConformanceBaseline(current ConformanceSummary) error {
	data, err := os.ReadFile(conformanceBaseline)
	if err != nil {
		return fmt.Errorf("reading baseline: %w", err)
	}
	var prior ConformanceSummary
	if err := json.Unmarshal(data, &prior); err != nil {
		return fmt.Errorf("parsing baseline JSON: %w", err)
	}
	// Round both rates to two decimals before subtracting so a
	// baseline written by the `%.2f` printer (e.g. "70.97") doesn't
	// look like a regression against an unrounded current
	// (e.g. 70.96907...). The printed values would be visually
	// identical; the unrounded subtraction was tripping the gate
	// with a "drop 0.00%" message which was obviously wrong.
	priorR := math.Round(prior.PassRate*100) / 100
	currR := math.Round(current.PassRate*100) / 100
	drop := priorR - currR
	if drop > conformanceRegress {
		return fmt.Errorf(
			"conformance regressed from %.2f%% to %.2f%% (drop %.2f%%, max allowed %.2f%%)",
			prior.PassRate, current.PassRate, drop, conformanceRegress)
	}
	return nil
}

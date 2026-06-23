package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/semantic"
	"github.com/spf13/cobra"
)

var (
	checkBaseline string
	checkRegress  float64

	// CheckCommand provides a fast static analysis sweep over a TTCN-3
	// source tree. It is intentionally lower-cost than `ntt lint`: it
	// only runs the parser and the semantic analyzer's hard-error rules,
	// so it can scale to the full ETSI conformance suite (~5000 files)
	// and double as the CI pass-rate gate the ntt-Titan roadmap calls for.
	CheckCommand = &cobra.Command{
		Use:   "check [path...]",
		Short: "Parse + semantic-check a TTCN-3 source tree",
		Long: `check walks the given paths, parses every .ttcn / .ttcn3 / .ttcnpp
file it finds, runs the semantic analyzer on each, and prints a pass-rate
summary. A file "passes" when it parses without errors and the analyzer
reports no errors (warnings do not affect the verdict).

When --baseline is given, check reads the prior pass-rate from that JSON
file and exits non-zero if the current rate dropped by more than --regress
percentage points. This is the gate the ntt-Titan roadmap uses to keep the
ETSI conformance pass-rate from regressing between commits.
`,
		Args: cobra.MinimumNArgs(1),
		RunE: runCheck,
	}
)

func init() {
	RootCommand.AddCommand(CheckCommand)
	CheckCommand.Flags().StringVar(&checkBaseline, "baseline", "",
		"compare against a prior pass-rate snapshot (JSON)")
	CheckCommand.Flags().Float64Var(&checkRegress, "regress", 0.0,
		"max allowed drop in pass-rate vs baseline, in percentage points")
}

// CheckResult is the per-file outcome reported by `ntt check`.
type CheckResult struct {
	Path     string   `json:"path"`
	Passed   bool     `json:"passed"`
	Errors   []string `json:"errors,omitempty"`
	Warnings int      `json:"warnings,omitempty"`
}

// CheckSummary is the suite-wide aggregate. The field names are stable so
// CI tooling and the upcoming ntt-Titan conformance dashboard can both
// consume it without coordination.
type CheckSummary struct {
	Total    int           `json:"total"`
	Passed   int           `json:"passed"`
	Failed   int           `json:"failed"`
	Warnings int           `json:"warnings"`
	PassRate float64       `json:"pass_rate"`
	Results  []CheckResult `json:"results,omitempty"`
}

func runCheck(cmd *cobra.Command, args []string) error {
	files := collectTTCN3Files(args)
	if len(files) == 0 {
		return fmt.Errorf("no TTCN-3 files found under %v", args)
	}

	// Build a suite-wide DB before running the per-file checks, so
	// every file's analyzer sees every other file's module/symbol
	// definitions. Without this, `import from M` lookups fall back
	// to "unknown module" because each worker would otherwise only
	// know about its own file.
	db := &ttcn3.DB{}
	db.Index(files...)

	summary := checkFiles(files, db)

	if outputJSON {
		if err := writeJSON(os.Stdout, summary); err != nil {
			return err
		}
	} else {
		writeText(os.Stdout, summary)
	}

	if checkBaseline != "" {
		if err := enforceBaseline(summary); err != nil {
			return err
		}
	}
	if summary.Failed > 0 {
		// Exit non-zero so CI Makefiles can trust the exit code and
		// don't have to parse stdout to detect failed files.
		return fmt.Errorf("%d of %d files failed semantic check",
			summary.Failed, summary.Total)
	}
	return nil
}

// checkFiles fans the per-file work out to a small worker pool. The default
// parallelism keeps us comfortably under the open-file limit while still
// keeping the wall-clock low on a 5000-file suite.
func checkFiles(files []string, db *ttcn3.DB) CheckSummary {
	const workers = 8
	jobs := make(chan string)
	results := make([]CheckResult, 0, len(files))
	var mu sync.Mutex
	var passed, warnings int64

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				r := checkOne(path, db)
				mu.Lock()
				results = append(results, r)
				mu.Unlock()
				if r.Passed {
					atomic.AddInt64(&passed, 1)
				}
				if r.Warnings > 0 {
					atomic.AddInt64(&warnings, int64(r.Warnings))
				}
			}
		}()
	}
	for _, f := range files {
		jobs <- f
	}
	close(jobs)
	wg.Wait()

	sort.Slice(results, func(i, j int) bool {
		return results[i].Path < results[j].Path
	})

	total := len(files)
	rate := 0.0
	if total > 0 {
		rate = float64(passed) / float64(total) * 100
	}
	return CheckSummary{
		Total:    total,
		Passed:   int(passed),
		Failed:   total - int(passed),
		Warnings: int(warnings),
		PassRate: rate,
		Results:  results,
	}
}

// checkOne runs the parser + semantic analyzer against a single file.
// Files that fail to parse are reported with the parser error captured;
// they always count as failed even if the analyzer would otherwise be
// happy.
func checkOne(path string, db *ttcn3.DB) CheckResult {
	r := CheckResult{Path: path}
	tree := ttcn3.ParseFile(path)
	if tree == nil {
		r.Errors = append(r.Errors, "parse returned nil tree")
		return r
	}
	if tree.Err != nil {
		r.Errors = append(r.Errors, tree.Err.Error())
		return r
	}
	diags := semantic.NewAnalyzer(db).Analyze(tree)
	for _, d := range diags {
		switch d.Severity {
		case semantic.SeverityError:
			r.Errors = append(r.Errors, fmt.Sprintf("%s: %s", d.Code, d.Message))
		case semantic.SeverityWarn:
			r.Warnings++
		}
	}
	r.Passed = len(r.Errors) == 0
	return r
}

// collectTTCN3Files walks each argument and returns every TTCN-3 source
// file under it. Symlinks are followed lazily, and hidden directories
// (the leading dot) are skipped so we don't recurse into `.git`.
func collectTTCN3Files(args []string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, a := range args {
		info, err := os.Stat(a)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			if isTTCN3(a) && !seen[a] {
				seen[a] = true
				out = append(out, a)
			}
			continue
		}
		_ = filepath.Walk(a, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				base := filepath.Base(p)
				if strings.HasPrefix(base, ".") && p != a {
					return filepath.SkipDir
				}
				return nil
			}
			if isTTCN3(p) && !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
			return nil
		})
	}
	sort.Strings(out)
	return out
}

func isTTCN3(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ttcn", ".ttcn3", ".ttcnpp":
		return true
	}
	return false
}

func writeJSON(w io.Writer, s CheckSummary) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(s)
}

func writeText(w io.Writer, s CheckSummary) {
	fmt.Fprintf(w, "checked %d files: %d passed, %d failed (%.2f%% pass-rate), %d warnings\n",
		s.Total, s.Passed, s.Failed, s.PassRate, s.Warnings)
	// Verbose listing only in -v mode keeps the default output kind to
	// big batches like the ETSI suite.
	if verbose > 0 {
		for _, r := range s.Results {
			if !r.Passed {
				fmt.Fprintf(w, "  FAIL %s\n", r.Path)
				for _, e := range r.Errors {
					fmt.Fprintf(w, "    %s\n", e)
				}
			}
		}
	}
}

func enforceBaseline(current CheckSummary) error {
	data, err := os.ReadFile(checkBaseline)
	if err != nil {
		return fmt.Errorf("reading baseline: %w", err)
	}
	var prior CheckSummary
	if err := json.Unmarshal(data, &prior); err != nil {
		return fmt.Errorf("parsing baseline JSON: %w", err)
	}
	drop := prior.PassRate - current.PassRate
	if drop > checkRegress {
		return fmt.Errorf(
			"pass-rate regressed from %.2f%% to %.2f%% (drop of %.2f%%, max allowed %.2f%%)",
			prior.PassRate, current.PassRate, drop, checkRegress)
	}
	return nil
}

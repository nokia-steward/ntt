package results

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nokia/ntt/internal/fs"
)

// File name of the test results file
var Filename = "test_results.json"

func Latest() (*DB, error) {
	b, err := fs.Open(Filename).Bytes()
	if err != nil {
		if os.IsNotExist(err) {
			return &DB{}, nil
		}
		return nil, err
	}
	var db DB
	return &db, json.Unmarshal(b, &db)
}

type DB struct {
	Version  string
	Sessions []Session
}

// Runs returns a slice with all runs from all sessions
func (db *DB) Runs() []Run {
	var runs []Run
	for _, s := range db.Sessions {
		for i := range s.Runs {
			s.Runs[i].ExpectedVerdict = s.ExpectedVerdict
		}
		runs = append(runs, s.Runs...)
	}
	return runs
}

func (db *DB) MaxLoad() int {
	if len(db.Sessions) == 0 {
		return 0
	}
	maxload := db.Sessions[0].MaxLoad
	for _, s := range db.Sessions {
		if s.MaxLoad > maxload {
			maxload = s.MaxLoad
		}
	}
	return maxload
}

func (db *DB) MaxJobs() int {
	if len(db.Sessions) == 0 {
		return 0
	}
	maxjobs := db.Sessions[0].MaxJobs
	for _, s := range db.Sessions {
		if s.MaxJobs > maxjobs {
			maxjobs = s.MaxJobs
		}
	}
	return maxjobs
}

type Session struct {
	Id              string
	MaxJobs         int    `json:"max_jobs,omitempty"`
	MaxLoad         int    `json:"max_load,omitempty"`
	ExpectedVerdict string `json:"expected_verdict,omitempty"`
	Runs            []Run  `json:"runs,omitempty"`
}

// A Run describes the execution of a single test case.
type Run struct {
	Name     string `json:"name"`               // Full qualified test name
	Instance int    `json:"instance,omitempty"` // Test instance
	Verdict  string `json:"verdict,omitempty"`  // the test verdict (pass, fail, none, ...)
	Reason   string `json:"reason,omitempty"`   // Optional reason for verdicts

	Begin Timestamp `json:"begin"` // When the test was started
	End   Timestamp `json:"end"`   // When the test ended

	WorkingDir string  `json:"working_dir,omitempty"` // Working Directory of the test
	Load       float64 `json:"load,omitempty"`        // the system load when the test was started
	MaxMem     int     `json:"max_mem,omitempty"`     // the maximum memory used when the test ended

	RunnerID        string `json:"runner_id,omitempty"`
	ExpectedVerdict string `json:"expected_verdict,omitempty"`
}

// A unique identifier of the run. Usually something like "testname-2"
func (r Run) ID() string {
	return r.Name + "-" + fmt.Sprintf("%d", r.Instance)
}

// A unique identifier of the job.
func (r Run) JobID() string {
	return r.Name
}

// Duration of an individual run
func (r Run) Duration() time.Duration {
	return r.End.Sub(r.Begin.Time)
}

// String returns a printable and simplified representation of Run
func (r Run) String() string {
	return fmt.Sprintf("%s	%s	%s", r.Verdict, r.ID(), r.Duration())
}

// Timestamp is a Unix timestamp in milliseconds.
type Timestamp struct {
	time.Time
}

// MarshalJSON is used to convert the timestamp to JSON
func (t Timestamp) MarshalJSON() ([]byte, error) {
	ms := t.UnixNano() / int64(time.Millisecond)
	return []byte(strconv.FormatInt(ms, 10)), nil
}

// UnmarshalJSON is used to convert the timestamp from JSON
func (t *Timestamp) UnmarshalJSON(s []byte) (err error) {
	r := string(s)
	q, err := strconv.ParseInt(r, 10, 64)
	if err != nil {
		return err
	}
	secs := q / 1000
	ms := q % 1000
	t.Time = time.Unix(secs, ms*int64(time.Millisecond))
	return nil
}

// Total returns total time spent if all runs would have been executed sequentially.
func Total(runs []Run) time.Duration {
	var sum time.Duration
	for _, r := range runs {
		sum += r.Duration()
	}
	return sum
}

// Duration returns the time spent between the first and the last test run.
func Duration(runs []Run) time.Duration {
	if len(runs) == 0 {
		return 0
	}
	return Last(runs).End.Sub(First(runs).Begin.Time)
}

// First returns the first test run
func First(runs []Run) Run {
	if len(runs) == 0 {
		return Run{}
	}

	first := runs[0]
	for _, r := range runs {
		if r.Begin.Before(first.Begin.Time) {
			first = r
		}
	}
	return first
}

// Last returns the last test run
func Last(runs []Run) Run {
	if len(runs) == 0 {
		return Run{}
	}

	last := runs[0]
	for _, r := range runs {
		if r.End.After(last.End.Time) {
			last = r
		}
	}
	return last
}

// Shortest returns the shortest test run
func Shortest(runs []Run) Run {
	if len(runs) == 0 {
		return Run{}
	}

	shortest := runs[0]
	for _, r := range runs {
		if r.Duration() < shortest.Duration() {
			shortest = r
		}
	}
	return shortest
}

// Longest returns the longest test run
func Longest(runs []Run) Run {
	if len(runs) == 0 {
		return Run{}
	}

	longest := runs[0]
	for _, r := range runs {
		if r.Duration() > longest.Duration() {
			longest = r
		}
	}
	return longest
}

// Duration returns a slice of test run durations
func Durations(runs []Run) []time.Duration {
	ret := make([]time.Duration, len(runs))
	for i := range runs {
		ret[i] = runs[i].Duration()
	}
	return ret
}

// Loads returns a slice of test run durations
func Loads(runs []Run) []float64 {
	ret := make([]float64, len(runs))
	for i := range runs {
		ret[i] = runs[i].Load
	}
	return ret
}

// FinalVerdicts folds multiple runs instances into one with a final verdict.
//
// * A test is considered "pass", if all runs had verdict "pass".
// * A test is considered "unstable", if only some runs had verdict "pass"
// * Runs with worse or equal verdict will overwrite previous runs.
func FinalVerdicts(runs []Run) []Run {

	severity := func(verdict string) int {
		switch strings.TrimSpace(strings.ToLower(verdict)) {
		case "skipped":
			return -1
		case "pass":
			return 0
		case "none", "":
			return 1
		case "inconc":
			return 2
		case "fail":
			return 3
		case "error":
			return 4
		default:
			return 5
		}
	}

	var (
		tests    = make(map[string]Run)
		jobs     = make([]string, 0, len(runs))
		passed   = make(map[string]bool)
		unstable = make(map[string]bool)
	)

	for _, run := range runs {
		last, ok := tests[run.JobID()]
		if !ok {
			jobs = append(jobs, run.JobID())
			last = run
		}

		// A test is a success if it passes at least once.
		if run.Verdict == "pass" {
			passed[run.JobID()] = true
		}

		// A test is unstable if it has its verdicts change
		if run.Verdict != last.Verdict {
			unstable[run.JobID()] = true
		}

		// Remember worst and latest test run for post analysis
		if severity(run.Verdict) >= severity(last.Verdict) {
			tests[run.JobID()] = run
		}
	}

	ret := make([]Run, len(jobs))
	for i, job := range jobs {
		t := tests[job]
		if passed[job] {
			t.Verdict = "pass"
			if unstable[job] {
				t.Verdict = "unstable"
			}
		}

		ret[i] = t
	}
	return ret
}

// Average returns the average test duration (median)
func Average(slice []time.Duration) time.Duration {
	if len(slice) == 0 {
		return 0
	}

	sort.Sort(durationSlice(slice))

	n := len(slice) / 2

	// odd slice length
	if len(slice)&1 == 1 {
		return slice[n]
	}

	// even slice length
	return (slice[n-1] + slice[n]) / 2

}

// Mean returns arithmetic mean
func Mean(slice []time.Duration) time.Duration {
	var sum int
	for _, d := range slice {
		sum += int(d)
	}

	return time.Duration(sum / len(slice))
}

// Deviation returns the standard deviation of duration
func Deviation(slice []time.Duration) time.Duration {
	if len(slice) == 0 {
		return 0
	}

	mean := Mean(slice)

	v := 0.0
	for _, d := range slice {
		v += math.Pow(float64(d), 2) / float64(mean)
	}

	return time.Duration(math.Sqrt(v))
}

type durationSlice []time.Duration

func (a durationSlice) Len() int           { return len(a) }
func (a durationSlice) Less(i, j int) bool { return a[i] < a[j] }
func (a durationSlice) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

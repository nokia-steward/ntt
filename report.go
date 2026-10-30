package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"text/template"
	"time"

	"github.com/nokia/ntt/internal/env"
	"github.com/nokia/ntt/internal/results"
	"github.com/nokia/ntt/project"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

var (
	ReportCommand = &cobra.Command{
		Use:   "report",
		Short: "Show report about latest test run",
		Long: `show information about latest test run",

The report command shows a summary of the latest test run. The summary includes
information such as a list of tests which did not pass, average run times, CPU
load, etc.
Command line options '--json' and '--junit' show similar output, but with JSON
or JUNIT formatting.

Use environment variable 'NTT_COLORS=never' to disable colors.

Templating
----------

ntt uses the Go templates format which you can use to specify custom output templates.
Example:

	ntt report --template "{{.Name}} took {{.Tests.Duration}}"


Available Objects

  .Report is a collection of test runs
  .Report.Cores:      number of CPU cores
  .Report.Environ:    list of environment variable
  .Report.Getenv:     value of an environment variable
  .Report.LineCount:  number of TTCN-3 source code lines
  .Report.MaxJobs:    maximum number of parallel test jobs
  .Report.MaxLoad:    maximum allowed CPU load
  .Report.Modules:    a list of collection sorted by module
  .Report.Name:       name of the collection
  .Report.Runs:       list of test runs
  .Report.Tests:      list of tests (with final verdict)
  .Report.FixedTests: list of tests where unstable tests are changed to pass or fail (based on ExpectedVerdict)

  .RunSlice is a list of test runs
  .RunSlice.Load:      Return systemload slice for every run
  .RunSlice.Average:   Average duration of runs (median)
  .RunSlice.Deviation: Standard deviation
  .RunSlice.Duration:  Timespan of first and last test run
  .RunSlice.Failed:    A slice of failed test runs (inconc, none, error, fail, ...)
  .RunSlice.Skipped:   A slice of skipped test
  .RunSlice.First:     First test run
  .RunSlice.Last:      Last test run
  .RunSlice.Longest:   Longest test run
  .RunSlice.NotPassed: A slice of tests without 'pass' verdict
  .RunSlice.Result:    Final result (PASSED, FAILED, UNSTABLE, NOEXEC)
  .RunSlice.Shortest:  Shortest test run
  .RunSlice.Total:     Sum of all test run durations
  .RunSlice.Unstable:  List of unstable test runs

  .Run is a individual test run
  .Run.ID:          test run ID (e.g. test.Stable_A-2)
  .Run.Name:        full qualified test name (test.Stable_A)
  .Run.Instance:    test instance (e.g. 2)
  .Run.Module:      module name (test)
  .Run.Testcase:    testcase name (e.g. Stable_A)
  .Run.Verdict:     the test verdict (pass, fail, none, ...)
  .Run.Begin:       when the test was started (time.Time Go object)
  .Run.End:         when the test ended (time.Time Go object)
  .Run.Duration:    a time.Duration Go object
  .Run.Load:        the system load when the test was started
  .Run.MaxMem:      the maximum memory used when the test ended
  .Run.Reason:      optional reason for verdicts
  .Run.ReasonFiles: content of *.reason files
  .Run.RunnerID:    the ID of the runner exeuting the run
  .Run.WorkingDir:  working Directory of the test

  .File is a (reason) file
  .File.Name:    path to file
  .File.Content: content of file


Additional commands

  green:    output ANSI sequences for color green
  red:      output ANSI sequences for color red
  orange:   output ANSI sequences for color orange
  bold:     output ANSI sequences for bold text
  off:      output ANSI sequences to reset attributes
  colorize: colorize output
  join:     join input with a separator
  json:     encode input using JSON format
  min:      returns the minimum of a float slice
  max:      returns the maximum of a float slice
  median:   returns the median of a float slice



Summary template:
` + SummaryTemplate + `


JUnit template:
` + JUnitTemplate + `


JSON template:
` + JSONTemplate + `


`,
		RunE: report,
	}

	useJSON  = false
	useJUnit = false

	templateText = ""
)

const (
	SummaryTemplate = `{{bold}}==================================  Summary  =================================={{off}}
{{range .Tests.NotPassed}}{{ printf "%-10s %s" .Verdict .Name  | colorize }}
{{else}}{{if eq (len .Tests) 0}}{{orange}}{{bold}}WARNING: No matching test cases found!{{off}}
{{else}}{{green}}all tests have passed{{off}}
{{end}}{{end}}
{{len .Tests}} test cases took {{bold}}{{.Tests.Duration}}{{off}} to execute (total runs: {{len .Runs}}
{{- with .Tests.Failed}}, {{red}}not passed: {{len .}}{{off}}{{end}}
{{- with .Tests.Unstable}}, {{orange}}unstable: {{len .}}{{off}}{{end}})
{{bold}}==============================================================================={{off}}

{{ printf "%s (±%s)" .Tests.Average .Tests.Deviation | printf "Average  : %-30s CPU cores      : " }}{{printf "%d" .Cores}}
{{ printf "Shortest : %-30s Parallel tests : %d" .Tests.Shortest.Duration .MaxJobs }}
{{ printf "Longest  : %-30s Load limit     : %d" .Tests.Longest.Duration .MaxLoad}}
{{ printf "Total    : %-30s Load average   : %.2f" .Tests.Total (median .Tests.Load)}}

{{bold}}==============================================================================={{off}}
{{bold}}Final Result: {{.Tests.Result | colorize}}{{off}}
{{bold}}==============================================================================={{off}}
`

	JUnitTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<testsuites>{{range .Modules}}

<testsuite name="{{.Name}}" tests="{{len .FixedTests}}" failures="{{len .FixedTests.Failed}}" skipped="{{len .FixedTests.Skipped}}" errors="" time="{{.FixedTests.Total.Seconds}}">
{{range .FixedTests}}<testcase name="{{.Testcase}}" time="{{.Duration.Seconds}}">
{{if eq .Verdict "skipped"}}  <skipped type="skipped" message="Verdict: skipped">{{with .Reason}}{{. | html }}{{end}}</skipped>
{{else if and (ne .Verdict "unstable") (ne .Verdict "pass")}}  <failure>Verdict: {{.Verdict}} {{with .Reason}}({{. | html }}){{end}}
{{range .ReasonFiles}}{{.Name}}: {{.Content}}{{end}}
  </failure>
{{end}}</testcase>

{{end}}</testsuite>
{{end}}</testsuites>
`

	JSONTemplate = `{
  "name"          : "{{.Name}}",
  "timestamp"     : {{.Runs.First.Begin.Unix}},
  "cores"         : {{.Cores}},
  "parallel_jobs" : {{.MaxJobs}},
  "max_load"      : {{.MaxLoad}},
  "suite": {
    "linecount": {{.LineCount}}
  },
  "load": {
    "min" : {{min .Tests.Load}},
    "max" : {{max .Tests.Load}},
    "avg" : {{median .Tests.Load}}
  },
  "tests": {
    "result"   : "{{ .Tests.Result }}",
    "tests"    : {{len .Tests }},
    "failed"   : {{len .Tests.Failed}},
    "unstable" : {{.Tests.Unstable | json}},
    "duration" : {
      "real"  : {{.Tests.Duration.Milliseconds}},
      "total" : {{.Tests.Total.Milliseconds}},
      "min"   : {{.Tests.Shortest.Duration.Milliseconds}},
      "max"   : {{.Tests.Longest.Duration.Milliseconds}},
      "avg"   : {{.Tests.Average.Milliseconds}},
      "dev"   : {{.Tests.Deviation.Milliseconds}}
    }
  },
  "env": {{ .Environ | json }}
}
`
)

func report(cmd *cobra.Command, args []string) error {
	switch {
	case useJSON:
		templateText = JSONTemplate
	case useJUnit:
		templateText = JUnitTemplate
	}

	if templateText == "" {
		templateText = SummaryTemplate
	}

	return ReportTemplate(os.Stdout, templateText)
}

func ReportTemplate(w io.Writer, text string) error {
	report, err := NewReport(Project)
	if err != nil {
		return err
	}

	tmpl, err := template.New("ntt-report-template").Funcs(funcMap).Parse(text)
	if err != nil {
		return err
	}

	return tmpl.Execute(w, report)
}

var funcMap = template.FuncMap{
	"green": func() string {
		if env := os.Getenv("NTT_COLORS"); env == "never" {
			return ""
		}
		return "\x1b[32;1m"
	},
	"red": func() string {
		if env := os.Getenv("NTT_COLORS"); env == "never" {
			return ""
		}

		return "\x1b[31;1m"
	},
	"orange": func() string {
		if env := os.Getenv("NTT_COLORS"); env == "never" {
			return ""
		}
		return "\x1b[38;5;208;1m"
	},
	"bold": func() string {
		if env := os.Getenv("NTT_COLORS"); env == "never" {
			return ""
		}
		return "\x1b[1m"
	},
	"off": func() string {
		if env := os.Getenv("NTT_COLORS"); env == "never" {
			return ""
		}
		return "\x1b[0m"
	},

	"colorize": func(s string) string {
		if env := os.Getenv("NTT_COLORS"); env == "never" {
			return s
		}
		pass := regexp.MustCompile(`(?i)pass`)
		fail := regexp.MustCompile(`(?i)fail|error`)
		unstable := regexp.MustCompile(`(?i)unstable`)
		none := regexp.MustCompile(`(?i)\bnone`)
		inconc := regexp.MustCompile(`(?i)\binconc`)

		re := regexp.MustCompile(`(?i)\b(NORUN|pass\w*|fail\w*|\w*error\w*|unstable|none|inconc\w*)\b`)
		return re.ReplaceAllStringFunc(s, func(s string) string {
			switch {
			case s == "NORUN":
				return "\x1b[38;5;208;1m" + s + "\x1b[0m"
			case pass.MatchString(s):
				return "\x1b[32;1m" + s + "\x1b[0m"
			case fail.MatchString(s):
				return "\x1b[31;1m" + s + "\x1b[0m"
			case unstable.MatchString(s):
				return "\x1b[38;5;208;1m" + s + "\x1b[0m"
			case inconc.MatchString(s) || none.MatchString(s):
				return "\x1b[38;5;201;1m" + s + "\x1b[0m"
			default:
				return s
			}
		})
	},
	"join": func(sep string, v interface{}) string {
		return strings.Join(v.([]string), sep)
	},
	"json": func(v interface{}) (string, error) {
		b, err := json.Marshal(v)
		return string(b), err
	},
	"min": func(v []float64) float64 {
		if len(v) == 0 {
			return 0
		}

		x := v[0]
		for _, r := range v {
			if r < x {
				x = r
			}
		}
		return x
	},
	"max": func(v []float64) float64 {
		if len(v) == 0 {
			return 0
		}

		x := v[0]
		for _, r := range v {
			if r > x {
				x = r
			}
		}
		return x
	},
	"median": func(v []float64) float64 {
		if len(v) == 0 {
			return 0
		}

		sort.Float64s(v)

		n := len(v) / 2

		// odd v length
		if len(v)&1 == 1 {
			return v[n]
		}

		// even v length
		return (v[n-1] + v[n]) / 2
	},
}

func init() {
	ReportCommand.PersistentFlags().BoolVarP(&useJSON, "json", "", false, "output report in JSON format")
	ReportCommand.PersistentFlags().BoolVarP(&useJUnit, "junit", "", false, "output report in Junit format")
	ReportCommand.PersistentFlags().StringVarP(&templateText, "template", "t", "", "output report with custom template")
}

type Report struct {
	Collection
	suite *project.Config
	db    results.DB
	Cores int
}

func NewReport(suite *project.Config) (*Report, error) {
	db, err := results.Latest()
	if err != nil {
		return nil, err
	}

	r := Report{
		suite: suite,
		Cores: runtime.NumCPU(),
	}

	r.Name = suite.Name

	if db != nil {
		r.db = *db
		r.Collection = *NewCollection(r.Name, db.Runs()...)
	}

	return &r, nil
}

func (r *Report) Getenv(s string) string {
	if s, ok := env.LookupEnv(s); ok {
		return s
	}

	return r.suite.Variables[s]
}

func (r *Report) Environ() []string {
	env := make(map[string]string)
	for k, v := range r.suite.Variables {
		env[k] = v
	}
	for _, e := range os.Environ() {
		kv := strings.SplitN(e, "=", 2)
		if len(kv) == 2 {
			env[kv[0]] = kv[1]
		}
	}

	var list []string
	for k, v := range env {
		list = append(list, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(list)
	return list
}
func (r *Report) LineCount() (int, error) {
	files, err := project.Files(r.suite)
	if err != nil {
		return -1, err
	}

	var sum int32
	g := new(errgroup.Group)
	for _, file := range files {
		file := file // https://golang.org/doc/faq#closures_and_goroutines
		g.Go(func() error {
			var err error

			buf := make([]byte, 32*1024)
			lineSep := []byte{'\n'}
			r, err := os.Open(file)
			if err != nil {
				return err
			}
			for {
				c, err := r.Read(buf)
				atomic.AddInt32(&sum, int32(bytes.Count(buf[:c], lineSep)))

				if err != nil {
					if err == io.EOF {
						err = nil
					}
					return err
				}
			}
		})
	}

	return int(sum), g.Wait()
}

func (r *Report) MaxJobs() int {
	return r.db.MaxJobs()
}

func (r *Report) MaxLoad() int {
	return r.db.MaxLoad()
}

type Collection struct {
	Name string
	runs RunSlice
}

func NewCollection(name string, runs ...results.Run) *Collection {
	c := &Collection{
		Name: name,
		runs: make(RunSlice, len(runs)),
	}

	for i, r := range runs {
		c.runs[i] = Run{r}
	}
	return c
}

func (c Collection) Runs() RunSlice {
	return c.runs
}

func (c Collection) Tests() RunSlice {
	return NewRunSlice(results.FinalVerdicts(c.runs.asResultsRun()))
}

func (c Collection) FixedTests() RunSlice {
	runs := NewRunSlice(results.FinalVerdicts(c.runs.asResultsRun()))

	for i := range runs {
		if runs[i].Verdict == "unstable" {
			if runs[i].ExpectedVerdict == "fail" {
				runs[i].Verdict = "fail"
			} else {
				runs[i].Verdict = "pass"
			}
		}
	}
	return runs
}

func (c Collection) Modules() []*Collection {
	suites := make(map[string]*Collection)
	for _, r := range c.runs {
		c, ok := suites[r.Module()]
		if !ok {
			c = NewCollection(r.Module())
		}
		c.runs = append(c.runs, r)
		suites[r.Module()] = c
	}

	ret := make([]*Collection, 0, len(suites))
	for _, v := range suites {
		ret = append(ret, v)
	}
	return ret
}

type RunSlice []Run

func NewRunSlice(runs []results.Run) RunSlice {
	ret := make(RunSlice, len(runs))
	for i, r := range runs {
		ret[i] = Run{r}
	}
	return ret
}

func (rs RunSlice) Total() time.Duration {
	return results.Total(rs.asResultsRun())
}

func (rs RunSlice) Duration() time.Duration {
	return results.Duration(rs.asResultsRun())
}

func (rs RunSlice) First() Run {
	return Run{results.First(rs.asResultsRun())}
}

func (rs RunSlice) Last() Run {
	return Run{results.Last(rs.asResultsRun())}
}

func (rs RunSlice) Shortest() Run {
	return Run{results.Shortest(rs.asResultsRun())}
}

func (rs RunSlice) Longest() Run {
	return Run{results.Longest(rs.asResultsRun())}
}

func (rs RunSlice) Load() []float64 {
	return results.Loads(rs.asResultsRun())
}

func (rs RunSlice) Average() time.Duration {
	return results.Average(results.Durations(rs.asResultsRun()))
}

func (rs RunSlice) Deviation() time.Duration {
	return results.Deviation(results.Durations(rs.asResultsRun()))
}

func (rs RunSlice) NotPassed() []Run {
	return rs.filter(func(s string) bool { return s != "pass" })
}

func (rs RunSlice) Skipped() []Run {
	return rs.filter(func(s string) bool { return s == "skipped" })
}

func (rs RunSlice) Failed() []Run {
	return rs.filter(func(s string) bool { return s != "pass" && s != "unstable" })
}

func (rs RunSlice) Unstable() []Run {
	return rs.filter(func(s string) bool { return s == "unstable" })
}

func (rs RunSlice) Result() string {
	switch {
	case len(rs) == 0:
		return "NORUN"

	case len(rs.Failed()) != 0:
		return "FAILED"

	case len(rs.Unstable()) != 0:
		return "UNSTABLE"

	case len(rs.Failed()) == 0 && len(rs.Unstable()) == 0:
		return "PASSED"

	default:
		return "FAILED"
	}
}

func (rs RunSlice) filter(f func(s string) bool) []Run {
	ret := make([]Run, 0, len(rs))
	for _, r := range rs {
		if f(r.Verdict) {
			ret = append(ret, r)
		}
	}
	return ret
}

func (rs RunSlice) asResultsRun() []results.Run {
	ret := make([]results.Run, len(rs))
	for i, r := range rs {
		ret[i] = r.Run
	}
	return ret
}

type Run struct{ results.Run }

func (r Run) Module() string {
	if f := strings.Split(r.Name, "."); len(f) == 2 {
		return f[0]
	}
	return ""
}

func (r Run) Testcase() string {
	f := strings.Split(r.Name, ".")
	return f[len(f)-1]
}

func (r Run) ReasonFiles() ([]File, error) {
	paths, err := filepath.Glob(filepath.Join(r.WorkingDir, "*.reason"))
	if err != nil {
		return nil, err
	}

	var files []File
	for _, p := range paths {
		b, err := ioutil.ReadFile(p)
		if err != nil {
			return nil, err
		}
		files = append(files, File{p, string(b)})
	}
	return files, err
}

type File struct {
	Name    string
	Content string
}

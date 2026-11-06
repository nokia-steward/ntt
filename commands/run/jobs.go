package run

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nokia/ntt/internal/basket"
	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/k3log"
	"github.com/nokia/ntt/internal/verify"

	"github.com/nokia/ntt/project"
	"github.com/nokia/ntt/ttcn3/doc"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// plan is what the project's own sources declare for a run. Sources are
// visited in manifest order; files in a directory and declarations in a file
// are visited in source order. Imported sources are excluded.
type plan struct {
	// Controls are the control parts, as jobs.
	Controls []Job

	// Tests are the testcases, as jobs.
	Tests []Job

	// Modules are the module ids declared by the suite's own sources.
	Modules []string

	// Outputs are the @output expectations of the suite's own sources.
	Outputs []Output

	// Errors are the @error expectations of the suite's own sources.
	Errors []ExpectedError

	// fileModule maps a source file's base name to the module it declares.
	fileModule map[string]string

	rejectModule bool
}

// Jobs returns the jobs to run: every control part, or every testcase when
// the suite declares no control part. The result is homogeneous.
func (p plan) Jobs() []Job {
	if len(p.Controls) > 0 {
		return slices.Clone(p.Controls)
	}
	return slices.Clone(p.Tests)
}

// selected returns the jobs to run. With no name, tag, or basket filter it is
// Jobs: every control part, or every testcase when the suite has none. An
// active basket runs the matching testcases in source order and skips control
// parts. A basket that matches nothing returns no jobs.
func (p plan) selected(b basket.Basket) []Job {
	if !basketActive(b) {
		return p.Jobs()
	}
	var jobs []Job
	for _, tc := range p.Tests {
		if b.Match(tc.Name, tc.tags) {
			jobs = append(jobs, tc)
		}
	}
	return jobs
}

// basketActive reports whether b constrains the run. Sub-baskets loaded from
// K3_LIST_BASKETS count, as do the regex and tag flags.
func basketActive(b basket.Basket) bool {
	return len(b.NameRegex) > 0 || len(b.NameExclude) > 0 ||
		len(b.TagsRegex) > 0 || len(b.TagsExclude) > 0 ||
		len(b.Baskets) > 0
}

// Test returns the testcase with the given fully qualified name. It finds
// testcases a control part calls, too.
func (p plan) Test(name string) (Job, bool) {
	for _, tc := range p.Tests {
		if tc.Name == name {
			return tc, true
		}
	}
	return Job{}, false
}

// ExpectsRejection reports whether every testcase is expected to make the
// compiler or runtime reject its module.
func (p plan) ExpectsRejection() bool {
	if !p.rejectModule {
		return false
	}
	for _, tc := range p.Tests {
		if tc.Expect.Kind != verify.ExpectReject {
			return false
		}
	}
	return true
}

// scan lists control parts and testcases from the project's own sources.
func scan(conf *project.Config) (plan, error) {
	var p plan
	files, err := fs.TTCN3Files(conf.Sources...)
	if err != nil {
		return plan{}, err
	}

	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			return plan{}, err
		}

		root := syntax.Parse(src, syntax.WithFilename(file))
		if err := root.Err(); err != nil {
			return plan{}, err
		}

		var module string
		var moduleExpect verify.Expectation
		var testcases []span
		var scanErr error
		root.Inspect(func(n syntax.Node) bool {
			if scanErr != nil {
				return false
			}
			switch n := n.(type) {
			case *syntax.Module:
				module = syntax.Name(n)
				if module != "" {
					p.Modules = append(p.Modules, module)
					if p.fileModule == nil {
						p.fileModule = map[string]string{}
					}
					p.fileModule[filepath.Base(file)] = module
				}
				expect, ok, err := verify.ParsePIXIT(moduleTags(n))
				if err != nil {
					scanErr = fmt.Errorf("%s: %w", sourceLoc(n), err)
					return false
				}
				if ok {
					moduleExpect = expect
					if expect.Kind == verify.ExpectReject {
						p.rejectModule = true
					}
				}
			case *syntax.ControlPart:
				if module != "" {
					p.Controls = append(p.Controls, Job{Name: module + ".control", Loc: sourceLoc(n)})
				}
			case *syntax.FuncDecl:
				if module == "" || !n.IsTest() {
					break
				}
				name := syntax.Name(n.Name)
				if name == "" {
					break
				}
				tags := doc.FindAllTags(syntax.Doc(n))
				job := Job{Name: module + "." + name, Loc: sourceLoc(n), Expect: moduleExpect, tags: tags}
				if v, ok, err := verify.ParseExpectedVerdict(tags); err != nil {
					scanErr = fmt.Errorf("%s: %w", sourceLoc(n), err)
					return false
				} else if ok {
					job.Expect = verify.Expectation{Kind: verify.ExpectExact, Verdict: v}
				}
				if r, ok, err := verify.ParseExpectedReason(tags); err != nil {
					scanErr = fmt.Errorf("%s: %w", sourceLoc(n), err)
					return false
				} else if ok {
					job.ExpectReason = r
				}
				p.Tests = append(p.Tests, job)
				testcases = append(testcases, span{name: job.Name, begin: syntax.Begin(n).Line, end: syntax.End(n).Line})
			}
			return true
		})
		if scanErr != nil {
			return plan{}, scanErr
		}
		outputs, errs, err := scanLineTags(root, file, testcases)
		if err != nil {
			return plan{}, err
		}
		p.Outputs = append(p.Outputs, outputs...)
		p.Errors = append(p.Errors, errs...)
	}
	return p, nil
}

// span is the first and last line of a named declaration.
type span struct {
	name       string
	begin, end int
}

func (s span) contains(line int) bool { return s.begin <= line && line <= s.end }

// scanLineTags returns the `// @output:` and `// @error` expectations of one
// parsed file, in source order. The comment must end a line that holds code,
// usually a log statement or the declaration that fails. A line comment runs
// to the end of its line, so a line has at most one expectation.
func scanLineTags(root *syntax.Root, file string, testcases []span) ([]Output, []ExpectedError, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return nil, nil, err
	}
	abs = filepath.Clean(abs)
	var outputs []Output
	var errs []ExpectedError
	for tok := root.FirstTok(); tok != nil; tok = tok.NextTok() {
		if tok.Kind() != syntax.COMMENT {
			continue
		}
		line := root.Position(tok.Pos()).Line
		loc := fmt.Sprintf("%s:%d", filepath.Base(file), line)
		text, isOutput, err := verify.ParseExpectedOutput(tok.String())
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", loc, err)
		}
		category, isError, err := verify.ParseExpectedError(tok.String())
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", loc, err)
		}
		tag := "@output"
		switch {
		case isError:
			tag = "@error"
		case !isOutput:
			continue
		}
		prev := tok.PrevTok()
		for prev != nil && prev.Kind() == syntax.COMMENT {
			prev = prev.PrevTok()
		}
		if prev == nil || root.Position(prev.End()).Line != line {
			return nil, nil, fmt.Errorf("%s: %s must follow code on the same line", loc, tag)
		}
		var testcase string
		for _, tc := range testcases {
			if tc.contains(line) {
				testcase = tc.name
				break
			}
		}
		if isError {
			errs = append(errs, ExpectedError{File: abs, Line: line, Category: category, Loc: loc, Testcase: testcase})
		} else {
			outputs = append(outputs, Output{File: abs, Line: line, Text: text, Loc: loc, Testcase: testcase})
		}
	}
	return outputs, errs, nil
}

// eventModule returns the module that produced event, using the source file
// named in the event. It is empty when the file is not one of the suite's.
func (p plan) eventModule(event k3log.Event) string {
	src := event.Source()
	if src == "" || p.fileModule == nil {
		return ""
	}
	file := src
	if i := strings.LastIndex(src, ":"); i >= 0 {
		file = src[:i]
	}
	return p.fileModule[filepath.Base(file)]
}

// moduleTags returns all documentation tags from the comment block that
// immediately precedes n. Unlike syntax.Doc it permits blank lines inside a
// multi-line module banner.
func moduleTags(n syntax.Node) [][]string {
	var comments []string
	for tok := n.FirstTok().PrevTok(); tok != nil; tok = tok.PrevTok() {
		if tok.Kind() != syntax.COMMENT {
			break
		}
		comments = append(comments, tok.String())
	}
	slices.Reverse(comments)
	return doc.FindAllTags(strings.Join(comments, "\n"))
}

// sourceLoc returns the file:line of n, or "" when the node has no position.
func sourceLoc(n syntax.Node) string {
	file := syntax.Filename(n)
	line := syntax.Begin(n).Line
	if file == "" || line <= 0 {
		return ""
	}
	return fmt.Sprintf("%s:%d", filepath.Base(file), line)
}

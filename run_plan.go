package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/project"
	"github.com/nokia/ntt/ttcn3/doc"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// expectKind is how a testcase's final verdict is judged.
type expectKind int

const (
	// The zero expectation fails a run for fail or error.
	_ expectKind = iota
	// expectExact requires the verdict exactly.
	expectExact
	// expectNotError accepts every verdict except error.
	expectNotError
	// expectReject requires error after a successful build.
	expectReject
)

type expectation struct {
	kind    expectKind
	verdict string
}

// runJob is a testcase or a control part ntt run executes.
type runJob struct {
	// name is the qualified name: "Module.Testcase" or "Module.control".
	name string
	// loc is the file:line of the declaration, or "".
	loc    string
	expect expectation
	// tags are a testcase's documentation tags, which baskets match.
	tags [][]string
}

// runPlan is what a suite's own sources declare for a run, in source order:
// imported sources are not part of it.
type runPlan struct {
	controls []runJob
	tests    []runJob
	// modules are the ids of the modules the sources declare.
	modules []string
	// fileModule maps a source file's base name to its module.
	fileModule map[string]string
	// rejectModule is set when a module expects to be rejected.
	rejectModule bool
	// files are the suite's own source files.
	files []string
}

// jobs is every control part, or every testcase when the suite declares no
// control part.
func (p runPlan) jobs() []runJob {
	if len(p.controls) > 0 {
		return slices.Clone(p.controls)
	}
	return slices.Clone(p.tests)
}

// selected is the jobs to run: with no filter jobs; with an active basket the
// testcases it matches, in source order, and no control part.
func (p runPlan) selected(b Basket) []runJob {
	if !basketActive(b) {
		return p.jobs()
	}
	var jobs []runJob
	for _, tc := range p.tests {
		if b.Match(tc.name, tc.tags) {
			jobs = append(jobs, tc)
		}
	}
	return jobs
}

// named is the jobs for the test ids given after "--", in their order: a
// testcase or control part of the suite, or of a module the suite imports.
func (p runPlan) named(ids []string) []runJob {
	jobs := make([]runJob, 0, len(ids))
	for _, id := range ids {
		if tc, ok := p.test(id); ok {
			jobs = append(jobs, tc)
			continue
		}
		jobs = append(jobs, runJob{name: id})
	}
	return jobs
}

// basketActive reports whether b constrains the run: a name or tag filter,
// or baskets loaded from the environment.
func basketActive(b Basket) bool {
	return len(b.NameRegex) > 0 || len(b.NameExclude) > 0 ||
		len(b.TagsRegex) > 0 || len(b.TagsExclude) > 0 ||
		len(b.Baskets) > 0
}

// test returns the testcase of the qualified name.
func (p runPlan) test(name string) (runJob, bool) {
	for _, tc := range p.tests {
		if tc.name == name {
			return tc, true
		}
	}
	return runJob{}, false
}

// expectsRejection reports whether every testcase expects its module to be
// rejected.
func (p runPlan) expectsRejection() bool {
	if !p.rejectModule {
		return false
	}
	for _, tc := range p.tests {
		if tc.expect.kind != expectReject {
			return false
		}
	}
	return true
}

// eventModule is the module of the suite a log line's location is in, or "".
func (p runPlan) eventModule(line string) string {
	fields := strings.SplitN(line, "|", 4)
	if len(fields) < 3 {
		return ""
	}
	_, src, ok := strings.Cut(fields[2], "=")
	if !ok {
		return ""
	}
	if i := strings.LastIndex(src, ":"); i >= 0 {
		src = src[:i]
	}
	return p.fileModule[filepath.Base(src)]
}

// scanRunPlan lists the control parts and testcases of the suite's own
// sources and what verdicts they expect.
func scanRunPlan(conf *project.Config) (runPlan, error) {
	var p runPlan
	files, err := fs.TTCN3Files(conf.Sources...)
	if err != nil {
		return runPlan{}, err
	}
	p.files = files
	p.fileModule = map[string]string{}
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			return runPlan{}, err
		}
		root := syntax.Parse(src, syntax.WithFilename(file))
		if err := root.Err(); err != nil {
			return runPlan{}, err
		}
		var module string
		var moduleExpect expectation
		var scanErr error
		root.Inspect(func(n syntax.Node) bool {
			if scanErr != nil {
				return false
			}
			switch n := n.(type) {
			case *syntax.Module:
				module = syntax.Name(n)
				if module != "" {
					p.modules = append(p.modules, module)
					p.fileModule[filepath.Base(file)] = module
				}
				e, ok, err := parsePIXIT(moduleTags(n))
				if err != nil {
					scanErr = fmt.Errorf("%s: %w", sourceLoc(n), err)
					return false
				}
				// A module's tag is its own: the next module of the
				// file expects what its own tag says, if any.
				moduleExpect = expectation{}
				if ok {
					moduleExpect = e
					if e.kind == expectReject {
						p.rejectModule = true
					}
				}
			case *syntax.ControlPart:
				if module != "" {
					p.controls = append(p.controls, runJob{name: module + ".control", loc: sourceLoc(n)})
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
				job := runJob{name: module + "." + name, loc: sourceLoc(n), expect: moduleExpect, tags: tags}
				if v, ok, err := parseExpectedVerdict(tags); err != nil {
					scanErr = fmt.Errorf("%s: %w", sourceLoc(n), err)
					return false
				} else if ok {
					job.expect = expectation{kind: expectExact, verdict: v}
				}
				p.tests = append(p.tests, job)
			}
			return true
		})
		if scanErr != nil {
			return runPlan{}, scanErr
		}
	}
	return p, nil
}

// moduleTags returns the documentation tags of the comment block right
// before n. Blank lines may separate the comments of a module's banner.
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

// sourceLoc is the file:line of n, or "".
func sourceLoc(n syntax.Node) string {
	file := syntax.Filename(n)
	line := syntax.Begin(n).Line
	if file == "" || line <= 0 {
		return ""
	}
	return fmt.Sprintf("%s:%d", filepath.Base(file), line)
}

// directiveError reports a malformed verdict directive.
type directiveError struct {
	tag, value string
}

func (e *directiveError) Error() string {
	return fmt.Sprintf("invalid %s directive: %q", e.tag, e.value)
}

// parseExpectedVerdict returns the verdict a testcase's documentation tags
// expect, if any. Three styles are understood:
//
//	// @verdict: <verdict>
//	// @k3:verdict <verdict>
//	// @verdict ..., ttcn3verdict:<verdict>     (ETSI)
//
// The first wins; a malformed one is an error, wherever it is.
func parseExpectedVerdict(tags [][]string) (string, bool, error) {
	var expected string
	var found bool
	for _, tag := range tags {
		if len(tag) < 2 {
			continue
		}
		key, value := tag[0], tag[1]
		switch key {
		case "@k3":
			fields := strings.Fields(value)
			if len(fields) == 0 || fields[0] != "verdict" {
				continue
			}
			if len(fields) != 2 || !isVerdictName(fields[1]) {
				return "", false, &directiveError{tag: key + ":verdict", value: value}
			}
			if !found {
				expected, found = fields[1], true
			}
		case "@verdict":
			if v, hasTTCN3, err := etsiTTCN3Verdict(value); hasTTCN3 {
				if err != nil {
					return "", false, err
				}
				if !found {
					expected, found = v, true
				}
				continue
			}
			if v := strings.TrimSpace(value); isVerdictName(v) {
				if !found {
					expected, found = v, true
				}
				continue
			}
			return "", false, &directiveError{tag: key, value: value}
		}
	}
	return expected, found, nil
}

// parsePIXIT returns the expectation a module's ETSI-style @verdict tag
// declares: "pass accept" any verdict but error, "pass accept,
// ttcn3verdict:<v>" that verdict, "pass reject" a rejected module.
func parsePIXIT(tags [][]string) (expectation, bool, error) {
	var expected expectation
	var found bool
	for _, tag := range tags {
		if len(tag) < 2 || tag[0] != "@verdict" {
			continue
		}
		fields := strings.FieldsFunc(tag[1], func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t'
		})
		if len(fields) < 2 || fields[0] != "pass" {
			return expectation{}, false, &directiveError{tag: "@verdict", value: tag[1]}
		}
		switch fields[1] {
		case "reject":
			if _, hasTTCN3, err := etsiTTCN3Verdict(tag[1]); err != nil {
				return expectation{}, false, err
			} else if hasTTCN3 {
				return expectation{}, false, &directiveError{tag: "@verdict", value: tag[1]}
			}
			if !found {
				expected, found = expectation{kind: expectReject}, true
			}
		case "accept":
			v, hasTTCN3, err := etsiTTCN3Verdict(tag[1])
			if err != nil {
				return expectation{}, false, err
			}
			if hasTTCN3 {
				if !found {
					expected, found = expectation{kind: expectExact, verdict: v}, true
				}
				continue
			}
			if !found {
				expected, found = expectation{kind: expectNotError}, true
			}
		default:
			return expectation{}, false, &directiveError{tag: "@verdict", value: tag[1]}
		}
	}
	return expected, found, nil
}

// etsiTTCN3Verdict extracts the ttcn3verdict:<verdict> token of an ETSI
// @verdict specification; found tells no token from an invalid one.
func etsiTTCN3Verdict(spec string) (verdict string, found bool, err error) {
	for _, field := range strings.FieldsFunc(spec, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	}) {
		if field != "ttcn3verdict" && !strings.HasPrefix(field, "ttcn3verdict:") {
			continue
		}
		v, ok := strings.CutPrefix(field, "ttcn3verdict:")
		if !ok || !isVerdictName(v) {
			return "", true, &directiveError{tag: "@verdict", value: spec}
		}
		return v, true, nil
	}
	return "", false, nil
}

func isVerdictName(s string) bool {
	switch s {
	case "none", "pass", "inconc", "fail", "error":
		return true
	}
	return false
}

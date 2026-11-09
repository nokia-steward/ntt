package run

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nokia/ntt/internal/k3log"
)

// Output is one `// @output: <text>` expectation. Every ulog event logged
// from File at Line must have the text Text.
type Output struct {
	// File is the absolute, cleaned path of the source file.
	File string

	// Line is the line of the comment, and of the statements it checks.
	Line int

	// Text is the expected ulog text, compared verbatim.
	Text string

	// Loc is the base name of File and Line, as file:line.
	Loc string

	// Testcase is the fully qualified name of the testcase whose declaration
	// contains Line, or "" when the line is outside any testcase.
	Testcase string
}

// outputFailure is an @output expectation that was not met. actual is the
// ulog text that differed, and "" when the line logged nothing.
type outputFailure struct {
	loc      string
	expected string
	actual   string
}

// outputMismatchLine is the k3log error for an @output expectation that is
// not met:
//
//	timestamp|OMIS->Output mismatch|k3=file:line|expected|actual
//
// actual is empty when the line logged nothing. A '|' in either text is
// written as \u007c, so the line keeps one field per text.
func outputMismatchLine(stamp, loc, expected, actual string) string {
	return errorLine(k3log.ErrOMIS, stamp, loc, escapeBar(expected), escapeBar(actual))
}

// outputCheck compares ulog events with the @output expectations and records
// every expectation that is not met.
type outputCheck struct {
	outputs []Output
	scope   runScope
	seen    []bool
	failed  []outputFailure
}

// newOutputCheck returns a check for a run that starts jobs. A run that is
// not complete starts testcases only.
func newOutputCheck(outputs []Output, complete bool, jobs []Job) *outputCheck {
	return &outputCheck{
		outputs: outputs,
		scope:   newRunScope(complete, jobs),
		seen:    make([]bool, len(outputs)),
	}
}

// runScope tells which source line expectations a run must meet.
type runScope struct {
	// complete reports whether the run executes every default job, so that
	// every expectation must be met. Otherwise only the expectations inside
	// a started testcase are.
	complete bool
	started  map[string]bool
}

func newRunScope(complete bool, jobs []Job) runScope {
	started := make(map[string]bool, len(jobs))
	for _, job := range jobs {
		started[job.Name] = true
	}
	return runScope{complete: complete, started: started}
}

// covers reports whether the run must meet an expectation inside testcase,
// which is "" for a line outside any testcase.
func (s runScope) covers(testcase string) bool {
	return s.complete || (testcase != "" && s.started[testcase])
}

// observe compares a ulog event with the expectations for its source line and
// returns an OMIS error line for each one its text differs from. The text is
// every field after the location, joined with '|', because k3r does not
// escape a '|' in logged values. A ulog without a location matches no
// expectation.
func (c *outputCheck) observe(event k3log.Event) []string {
	if event.Category != k3log.ULOG {
		return nil
	}
	file, line, ok := sourceLine(event.Source())
	if !ok {
		return nil
	}
	text := tail(event, 3)
	var lines []string
	for i, o := range c.outputs {
		if o.Line != line || !sameSource(o.File, file) {
			continue
		}
		c.seen[i] = true
		if text != o.Text {
			c.failed = append(c.failed, outputFailure{loc: o.Loc, expected: o.Text, actual: text})
			lines = append(lines, outputMismatchLine(event.Field(0), o.Loc, o.Text, text))
		}
	}
	return lines
}

// finish returns an OMIS error line, with an empty actual text, for each
// expectation whose line logged nothing during the run. When the run is not
// complete, an expectation outside the started testcases is not reported,
// because the code that would log it may not have run.
func (c *outputCheck) finish(stamp string) []string {
	var lines []string
	for i, o := range c.outputs {
		if c.seen[i] || !c.scope.covers(o.Testcase) {
			continue
		}
		c.failed = append(c.failed, outputFailure{loc: o.Loc, expected: o.Text})
		lines = append(lines, outputMismatchLine(stamp, o.Loc, o.Text, ""))
	}
	return lines
}

// sourceLine splits an event location file:line.
func sourceLine(src string) (file string, line int, ok bool) {
	i := strings.LastIndex(src, ":")
	if i <= 0 {
		return "", 0, false
	}
	line, err := strconv.Atoi(src[i+1:])
	if err != nil {
		return "", 0, false
	}
	return src[:i], line, true
}

// sameSource reports whether the file named by an event is the source file
// path. An absolute event file must equal path. A relative one must name path
// from the working directory, as the compiler was given it, or be a trailing
// part of it, for example a base name written with --strip-filenames. Paths
// compare in slash form, so that on Windows `\` and `/` name the same file,
// and a path rooted without a drive counts as absolute.
func sameSource(path, file string) bool {
	path = filepath.ToSlash(path)
	clean := filepath.ToSlash(filepath.Clean(file))
	if filepath.IsAbs(file) || strings.HasPrefix(clean, "/") {
		return clean == path
	}
	if abs, err := filepath.Abs(file); err == nil && filepath.ToSlash(abs) == path {
		return true
	}
	return strings.HasSuffix(path, "/"+clean)
}

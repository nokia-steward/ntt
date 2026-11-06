package run

import (
	"errors"
	"os/exec"

	"github.com/nokia/ntt/internal/k3log"
)

// ExpectedError is one `// @error` or `// @error: <category>` expectation.
// An error event must be reported from File at Line, and every error event
// from there must have the category.
type ExpectedError struct {
	// File is the absolute, cleaned path of the source file.
	File string

	// Line is the line of the comment, and of the code that fails.
	Line int

	// Category is the expected k3log error category, for example CONV, or ""
	// when any error category is expected.
	Category string

	// Loc is the base name of File and Line, as file:line.
	Loc string

	// Testcase is the fully qualified name of the testcase whose declaration
	// contains Line, or "" when the line is outside any testcase.
	Testcase string
}

// errorEvents is the k3r event filter that selects every error event.
const errorEvents = "^[A-Z0-9]{4}$"

// loadFailure is the exit status of k3r when an error ends it, for example
// a module that fails to load.
const loadFailure = 2

// errorFailure is an @error expectation that was not met. actual is the
// category of an error reported from the line, and "" when the line reported
// none.
type errorFailure struct {
	loc      string
	expected string
	actual   string
}

// errorMismatchLine is the k3log error for an @error expectation that is not
// met:
//
//	timestamp|EMIS->Error mismatch|k3=file:line|expected|actual
//
// expected is empty for a bare @error, actual when the line reported no
// error.
func errorMismatchLine(stamp, loc, expected, actual string) string {
	return errorLine(k3log.ErrEMIS, stamp, loc, expected, actual)
}

// errorCheck compares k3r error events with the @error expectations. It
// records every expectation that is not met, and counts the error events
// that an expectation accepts and those none does.
type errorCheck struct {
	errors []ExpectedError
	scope  runScope
	seen   []bool
	failed []errorFailure

	expected   int
	unexpected int
}

// newErrorCheck returns a check for a run that starts jobs. A run that is
// not complete starts testcases only.
func newErrorCheck(errs []ExpectedError, complete bool, jobs []Job) *errorCheck {
	return &errorCheck{
		errors: errs,
		scope:  newRunScope(complete, jobs),
		seen:   make([]bool, len(errs)),
	}
}

// observe compares an error event with the expectations for its source line
// and returns an EMIS error line for each one whose category differs. The
// event is expected when an expectation on its line accepts its category.
// An error without a location matches no expectation.
func (c *errorCheck) observe(event k3log.Event) []string {
	if !event.Category.IsError() {
		return nil
	}
	accepted := false
	var lines []string
	if file, line, ok := sourceLine(event.Source()); ok {
		for i, e := range c.errors {
			if e.Line != line || !sameSource(e.File, file) {
				continue
			}
			c.seen[i] = true
			if e.Category == "" || e.Category == event.ID() {
				accepted = true
				continue
			}
			c.failed = append(c.failed, errorFailure{loc: e.Loc, expected: e.Category, actual: event.ID()})
			lines = append(lines, errorMismatchLine(event.Field(0), e.Loc, e.Category, event.ID()))
		}
	}
	if accepted {
		c.expected++
	} else {
		c.unexpected++
	}
	return lines
}

// finish returns an EMIS error line, with an empty actual category, for each
// expectation whose line reported no error during the run. When the run is
// not complete, an expectation outside the started testcases is not
// reported, because the code that would fail may not have run.
func (c *errorCheck) finish(stamp string) []string {
	var lines []string
	for i, e := range c.errors {
		if c.seen[i] || !c.scope.covers(e.Testcase) {
			continue
		}
		c.failed = append(c.failed, errorFailure{loc: e.Loc, expected: e.Category})
		lines = append(lines, errorMismatchLine(stamp, e.Loc, e.Category, ""))
	}
	return lines
}

// accepts reports whether err, the exit error of k3r, is the expected end of
// the run: k3r stopped with an error, for example while loading the module,
// and every error event it reported was expected.
func (c *errorCheck) accepts(err error) bool {
	var exit *exec.ExitError
	return errors.As(err, &exit) && exit.ExitCode() == loadFailure && c.expected > 0 && c.unexpected == 0
}

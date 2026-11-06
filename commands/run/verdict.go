package run

import (
	"fmt"
	"strings"

	"github.com/nokia/ntt/internal/k3log"
	"github.com/nokia/ntt/internal/verify"
)

// testcaseVerdict is the name, verdict, and reason of one finished testcase
// that fails the run. expected is set when the testcase's @verdict tag did not
// match, expectedReason when its @k3:reason tag did not match.
type testcaseVerdict struct {
	name           string
	verdict        string
	expected       string
	reason         string
	expectedReason string
}

// verdictFailure is returned when one or more testcases have a verdict that
// fails the run, or an @output or @error expectation is not met. It has no
// message: the k3log already reported them, and the process exits 1 because
// this is an ordinary command error.
type verdictFailure struct {
	tests   []testcaseVerdict
	outputs []outputFailure
	errors  []errorFailure
}

func (e *verdictFailure) Error() string { return "" }

// mismatchLine is the k3log error for a testcase whose final verdict differs
// from its @verdict tag:
//
//	timestamp|VMIS->Verdict mismatch|k3[=file:line]|testcase|expected|actual
func mismatchLine(stamp, loc, name, expected, actual string) string {
	return errorLine(k3log.ErrVMIS, stamp, loc, name, expected, actual)
}

// reasonMismatchLine is the k3log error for a testcase whose final verdict
// reason differs from its @k3:reason tag:
//
//	timestamp|RMIS->Reason mismatch|k3[=file:line]|testcase|expected|actual
//
// A '|' in either reason is written as \u007c, as k3r does inside a reason
// argument, so the line keeps one field per reason.
func reasonMismatchLine(stamp, loc, name, expected, actual string) string {
	return errorLine(k3log.ErrRMIS, stamp, loc, name, escapeBar(expected), escapeBar(actual))
}

func errorLine(c k3log.Category, stamp, loc string, fields ...string) string {
	id := "k3"
	if loc != "" {
		id += "=" + loc
	}
	parts := strings.SplitN(string(c), "|", 3)
	return fmt.Sprintf("%s|%s->%s|%s|%s", stamp, parts[0], parts[1], id, strings.Join(fields, "|"))
}

func escapeBar(s string) string {
	return strings.ReplaceAll(s, "|", `\u007c`)
}

// failsRun reports whether a testcase verdict fails the k3 process. fail and
// error do. none, pass, and inconc do not.
func failsRun(verdict string) bool {
	switch verdict {
	case "fail", "error":
		return true
	default:
		return false
	}
}

// judgeVerdict reports whether verdict fails the run under expect. expected
// is the verdict expect requires when verdict does not meet it, and "" when
// there is nothing to report as a mismatch.
func judgeVerdict(expect verify.Expectation, verdict string) (fails bool, expected string) {
	switch expect.Kind {
	case verify.ExpectExact:
		if verdict == expect.Verdict {
			return false, ""
		}
		return true, expect.Verdict
	case verify.ExpectNotError:
		return verdict == "error", ""
	case verify.ExpectReject:
		if verdict == "error" {
			return false, ""
		}
		return true, "error"
	default:
		return failsRun(verdict), ""
	}
}

// verdictCheck records every testcase whose verdict fails the run, including
// one a control part calls, and every tagged testcase that never finished.
type verdictCheck struct {
	plan   plan
	scope  runScope
	ran    map[string]bool
	failed []testcaseVerdict
}

// newVerdictCheck returns a check for a run that starts jobs. A run that is
// not complete starts testcases only.
func newVerdictCheck(p plan, complete bool, jobs []Job) *verdictCheck {
	return &verdictCheck{plan: p, scope: newRunScope(complete, jobs)}
}

// observe judges the verdict and reason of a tcfi event and returns the k3log
// error lines to write after the event: a VMIS line for an @verdict mismatch,
// then an RMIS line for an @k3:reason mismatch.
//
// The verdict and the reason are judged independently. A @k3:reason tag alone
// says nothing about the verdict, which is still judged by the testcase's
// verdict expectation, by default failing the run for fail or error; tag such
// a testcase with @k3:verdict too. A tcfi event without a reason does not
// match any @k3:reason tag.
func (c *verdictCheck) observe(event k3log.Event) []string {
	if event.Category != k3log.TCFI {
		return nil
	}
	if c.ran == nil {
		c.ran = map[string]bool{}
	}
	c.ran[ranTestcase(event.Field(3))] = true
	name := eventJobName(event.Field(3))
	result := testcaseVerdict{name: name, verdict: event.Field(4)}
	tc, _ := c.plan.Test(name)

	var lines []string
	fails, expected := judgeVerdict(tc.Expect, result.verdict)
	if expected != "" {
		result.expected = expected
		lines = append(lines, mismatchLine(event.Field(0), tc.Loc, name, expected, result.verdict))
	}
	if tc.ExpectReason != "" {
		if reason := tail(event, 5); reason != tc.ExpectReason {
			fails = true
			result.reason, result.expectedReason = reason, tc.ExpectReason
			lines = append(lines, reasonMismatchLine(event.Field(0), tc.Loc, name, tc.ExpectReason, reason))
		}
	}
	if fails {
		c.failed = append(c.failed, result)
	}
	return lines
}

// finish returns the error lines for each testcase with an exact verdict or
// a reason expectation that finished nowhere during the run: a VMIS line
// with an empty actual verdict for the verdict, then an RMIS line with an
// empty actual reason for the reason. One tcfi is enough. When the run is
// not complete, a testcase it did not start is not reported.
func (c *verdictCheck) finish(stamp string) []string {
	var lines []string
	for _, tc := range c.plan.Tests {
		exact := tc.Expect.Kind == verify.ExpectExact
		if (!exact && tc.ExpectReason == "") || c.ran[tc.Name] || !c.scope.covers(tc.Name) {
			continue
		}
		result := testcaseVerdict{name: tc.Name}
		if exact {
			result.expected = tc.Expect.Verdict
			lines = append(lines, mismatchLine(stamp, tc.Loc, tc.Name, tc.Expect.Verdict, ""))
		}
		if tc.ExpectReason != "" {
			result.expectedReason = tc.ExpectReason
			lines = append(lines, reasonMismatchLine(stamp, tc.Loc, tc.Name, tc.ExpectReason, ""))
		}
		c.failed = append(c.failed, result)
	}
	return lines
}

// ranTestcase returns the testcase a tcfi event finishes. An instance of a
// generic testcase, logged as "module.T<S>", is a run of "module.T".
func ranTestcase(eventName string) string {
	if i := strings.IndexAny(eventName, "(<["); i >= 0 {
		return eventName[:i]
	}
	return eventName
}

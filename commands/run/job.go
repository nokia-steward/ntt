package run

import (
	"strings"
	"time"

	"github.com/nokia/ntt/internal/k3log"
	"github.com/nokia/ntt/internal/verify"
)

// Job is one test case or one control part that k3 run executes.
type Job struct {
	// Name is the fully qualified name, for example "module.Testcase" or
	// "module.control".
	Name string

	// Expect is how the testcase's final verdict is judged.
	Expect verify.Expectation

	// ExpectReason is the reason from the testcase's @k3:reason tag, or ""
	// when the reason is not checked. It must equal Reason exactly.
	ExpectReason string

	// Loc is the file:line of the declaration, or "" when it is unknown.
	Loc string

	// tags are the documentation tags of a testcase. A control part has none.
	// An active basket uses them to choose which testcases run.
	tags [][]string

	// Start is the timestamp of the job's start event (tcst or cpen).
	Start time.Time

	// End is the timestamp of the job's finish event (tcfi or cplv).
	End time.Time

	// Verdict is the verdict k3r reports on the testcase's tcfi event: none,
	// pass, inconc, fail, or error. A control part has no tcfi event, so
	// Verdict stays empty.
	Verdict string

	// Reason is the text that follows the verdict on that tcfi event. It is
	// empty when the event carries none, and for a control part.
	Reason string
}

// Duration reports how long the job ran. It is zero until the job has both a
// start and an end time.
func (j Job) Duration() time.Duration {
	if j.Start.IsZero() || j.End.IsZero() {
		return 0
	}
	return j.End.Sub(j.Start)
}

// tracker records when each job started and finished and, for a testcase, the
// verdict and reason k3r reports on tcfi. Jobs finish in the order they were
// started. A test case runs from tcst to tcfi, a control part from cpen to
// cplv. Events for test cases a control part calls in between do not belong
// to their own job.
type tracker struct {
	jobs  []Job
	index int
}

// observe updates the current job from one k3log event.
func (t *tracker) observe(event k3log.Event) {
	if t.index >= len(t.jobs) || !namesJob(event.Field(3), t.jobs[t.index].Name) {
		return
	}
	stamp, err := event.Stamp()
	if err != nil {
		return
	}
	switch event.Category {
	case k3log.TCST, k3log.CPEN:
		t.jobs[t.index].Start = stamp
	case k3log.TCFI, k3log.CPLV:
		if t.jobs[t.index].Start.IsZero() {
			return
		}
		t.jobs[t.index].End = stamp
		if event.Category == k3log.TCFI {
			t.jobs[t.index].Verdict = event.Field(4)
			t.jobs[t.index].Reason = tail(event, 5)
		}
		t.index++
	}
}

// tail returns event fields from n onward, joined with '|'. A reason can
// itself contain '|'.
func tail(event k3log.Event, n int) string {
	fields := strings.Split(event.String(), "|")
	if n >= len(fields) {
		return ""
	}
	return strings.Join(fields[n:], "|")
}

// namesJob reports whether an event name refers to the job. Control parts are
// logged as "module.control[0]" and some logs append "()".
func namesJob(eventName, jobName string) bool {
	return eventJobName(eventName) == jobName
}

// eventJobName returns the testcase or control name carried by an event.
func eventJobName(eventName string) string {
	if i := strings.IndexAny(eventName, "(["); i >= 0 {
		return eventName[:i]
	}
	return eventName
}

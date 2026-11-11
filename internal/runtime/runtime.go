package runtime

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Flags int

const (
	Local Flags = 1 << iota
	Testcase
	Out
	Parameter
	Inout
	Optional
	Omit
	Present
	// Member marks a variable declared in a component type body, and the
	// function that declares them. Each component holds its own copy.
	Member
	Template
	Altstep
)

// Default is an activated altstep with its arguments.
type Default struct {
	Fn     *Function
	Args   []any
	Active bool
}

func (d *Default) String() string {
	if d.Active {
		return "active"
	}
	return "inactive"
}

type Function struct {
	Flags
	Name string
	Signature
	Body any
	Attributes
	NLocals int
}

type Component struct {
	Name    string
	mu      sync.Mutex
	verdict Verdict
	done    chan struct{}
	stopped atomic.Bool
	started atomic.Bool

	// Timers lists the timers this component has used, and Defaults its
	// active defaults in activation order. Only the component's own
	// behaviour touches them.
	Timers   []*Timer
	Defaults []*Default
}

// String formats c as k3r logs a component reference, e.g. "MTC(running)".
func (c *Component) String() string {
	switch {
	case c.Done():
		return c.Name + "(stopped)"
	case c.Started():
		return c.Name + "(running)"
	}
	return c.Name + "(inactive)"
}

// Null is the null component reference. k3r logs it as "nullomit".
type Null struct{}

func (Null) String() string { return "nullomit" }

type Var struct {
	Flags
	Name string
	Type Type
	Attributes
	Value any
	Index int
}

// Timer is a timer instance. Default is the declared duration; it is not a
// float64 when the timer was declared without one. Expiry is computed from
// the clock when asked, so a timer needs no goroutine.
type Timer struct {
	Name     string
	Default  any
	started  time.Time
	deadline time.Time
	active   bool
}

// Start (re)starts t for d seconds. A negative d is an error.
func (t *Timer) Start(d float64) error {
	if d < 0 || math.IsNaN(d) {
		return fmt.Errorf("negative timer duration %v", d)
	}
	t.started = time.Now()
	t.deadline = t.started.Add(time.Duration(d * float64(time.Second)))
	t.active = true
	return nil
}

// Stop makes t inactive and reports whether it was running.
func (t *Timer) Stop() bool {
	running := t.Running()
	t.active = false
	return running
}

// Running reports whether t is started and has not expired.
func (t *Timer) Running() bool { return t.RunningAt(time.Now()) }

// RunningAt reports whether t is started and has not expired at now.
func (t *Timer) RunningAt(now time.Time) bool { return t.active && now.Before(t.deadline) }

// Read returns the seconds elapsed since t started, or 0 when not running.
func (t *Timer) Read() float64 {
	if !t.Running() {
		return 0
	}
	return time.Since(t.started).Seconds()
}

// Timeout reports whether t has expired, and consumes the expiry. Otherwise
// running reports whether t may still expire, at deadline.
func (t *Timer) Timeout() (ready, running bool, deadline time.Time) {
	return t.TimeoutAt(time.Now())
}

// TimeoutAt is Timeout at now.
func (t *Timer) TimeoutAt(now time.Time) (ready, running bool, deadline time.Time) {
	if !t.active {
		return false, false, time.Time{}
	}
	if t.RunningAt(now) {
		return false, true, t.deadline
	}
	t.active = false
	return true, false, time.Time{}
}

// String formats t as k3r logs a timer value, e.g. "{inactive,0.5}".
func (t *Timer) String() string {
	state := "inactive"
	if t.Running() {
		state = "running"
	} else if t.active {
		state = "expired"
	}
	d := "-"
	if f, ok := t.Default.(float64); ok {
		d = FormatFloat(f)
	}
	return "{" + state + "," + d + "}"
}

// FormatFloat formats x as k3r does: fixed notation with 21 significant
// digits, trailing zeros dropped, and at least one decimal.
func FormatFloat(x float64) string {
	switch {
	case math.IsInf(x, 1):
		return "infinity"
	case math.IsInf(x, -1):
		return "-infinity"
	case math.IsNaN(x):
		return "not_a_number"
	case x == 0:
		return "0.0"
	}
	exp := int(math.Floor(math.Log10(math.Abs(x))))
	s := strconv.FormatFloat(x, 'f', max(20-exp, 1), 64)
	s = strings.TrimRight(s, "0")
	if strings.HasSuffix(s, ".") {
		s += "0"
	}
	return s
}

// LengthOf returns the number of elements of a string, record of, set of,
// or array value, or of a template whose length is fixed.
func LengthOf(v any) (int, error) {
	switch x := v.(type) {
	case string:
		return len(x), nil
	case Binary:
		return x.Len(), nil
	case ModifiedTemplate:
		return LengthOf(x.Val)
	case []any:
		for _, e := range x {
			if e == AnyOrNone {
				return 0, fmt.Errorf("lengthof: template length is not fixed")
			}
		}
		return len(x), nil
	}
	return 0, fmt.Errorf("lengthof: unsupported %T", v)
}

// Verdict is a TTCN-3 verdicttype value.
type Verdict string

const (
	VerdictNone   Verdict = "none"
	VerdictPass   Verdict = "pass"
	VerdictFail   Verdict = "fail"
	VerdictInconc Verdict = "inconc"
	VerdictError  Verdict = "error"
)

// NewComponent returns a component named name. Its verdict is none and Done
// is false until Finish.
func NewComponent(name string) *Component {
	return &Component{Name: name, verdict: VerdictNone, done: make(chan struct{})}
}

// SetVerdict stores v when it is strictly stronger than the current verdict
// and returns the previous verdict. The order is
// none < pass < inconc < fail < error. A zero Verdict ranks as none.
func (c *Component) SetVerdict(v Verdict) (prev Verdict) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prev = c.verdict
	if verdictRank(v) > verdictRank(prev) {
		c.verdict = v
	}
	return prev
}

// Verdict returns the component's current verdict.
func (c *Component) Verdict() Verdict {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.verdict
}

// Finish closes the done channel. A second call does nothing.
func (c *Component) Finish() {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-c.done:
	default:
		close(c.done)
	}
}

// Done reports whether Finish has been called.
func (c *Component) Done() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

// Stop marks the component stopped.
func (c *Component) Stop() { c.stopped.Store(true) }

// Stopped reports whether Stop has been called.
func (c *Component) Stopped() bool { return c.stopped.Load() }

// Start marks the component as running a behaviour.
func (c *Component) Start() { c.started.Store(true) }

// Started reports whether Start has been called.
func (c *Component) Started() bool { return c.started.Load() }

// verdictRank orders verdicts for overwriting. The zero Verdict and none
// share the lowest rank; anything unrecognized does too.
func verdictRank(v Verdict) int {
	switch v {
	case VerdictPass:
		return 1
	case VerdictInconc:
		return 2
	case VerdictFail:
		return 3
	case VerdictError:
		return 4
	default:
		return 0
	}
}

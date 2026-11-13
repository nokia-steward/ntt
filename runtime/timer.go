package runtime

import "time"

// TimerBusyWaitPolls is how many times a timer may be polled with
// `.running`, at one instant of the virtual clock, before the poller is
// taken to be busy-waiting on it (ETSI ES 201 873-1 23.6): on the virtual
// clock computing takes no time, so the timer would never expire.
const TimerBusyWaitPolls = 1000

// TimerNow is the time timer operations are performed at: the wall clock,
// the virtual clock of the testcase or control part when there is one,
// and which of the two timers run on.
type TimerNow struct {
	Wall time.Time
	// Virtual is the virtual clock's reading; HasVirtual reports whether
	// there is a virtual clock at all.
	Virtual    float64
	HasVirtual bool
	// OnVirtual reports whether expiry and the elapsed time are measured
	// on the virtual clock rather than the wall clock.
	OnVirtual bool
}

// Start starts t at now for duration seconds (ETSI 23.2). A timer started
// again starts anew.
func (t *TimerHandle) Start(now TimerNow, duration float64) {
	t.Running = true
	t.Ticks = 0
	t.MaxTicks = TimerBusyWaitPolls
	t.StartedAt = now.Wall
	t.StartedAtVirtual = 0
	if now.HasVirtual {
		t.StartedAtVirtual = now.Virtual
	}
	t.Duration = duration
}

// Stop stops t (ETSI 23.3); a stopped timer is inactive.
func (t *TimerHandle) Stop() {
	t.Running = false
	t.StartedAt = time.Time{}
}

// Fire makes the expiry of t take effect, as a timeout operation that
// matched does (ETSI 23.7): t is inactive, and a poll of it no longer
// counts as busy-waiting.
func (t *TimerHandle) Fire() {
	t.Ticks = t.MaxTicks
	t.Running = false
}

// VirtualDeadline is the virtual time t expires at.
func (t *TimerHandle) VirtualDeadline() float64 {
	return t.StartedAtVirtual + t.Duration
}

// Expired reports whether t, running, has reached its deadline at now. A
// timer of no duration expires at once; one never started does not.
func (t *TimerHandle) Expired(now TimerNow) bool {
	if t == nil || !t.Running {
		return false
	}
	if t.Duration <= 0 {
		return true
	}
	if now.OnVirtual {
		return now.Virtual >= t.VirtualDeadline()
	}
	if t.StartedAt.IsZero() {
		return false
	}
	deadline := t.StartedAt.Add(time.Duration(t.Duration * float64(time.Second)))
	return !now.Wall.Before(deadline)
}

// Poll is t.running at now (ETSI 23.6): whether t is started and has not
// expired. On the virtual clock it is false once t has been polled more
// than TimerBusyWaitPolls times at one instant, so a loop that busy-waits
// on it ends.
func (t *TimerHandle) Poll(now TimerNow) bool {
	if t == nil || !t.Running {
		return false
	}
	if t.Duration <= 0 || t.Expired(now) {
		return false
	}
	if !now.OnVirtual {
		return true
	}
	if now.Virtual != t.PolledAtVirtual {
		t.PolledAtVirtual, t.Ticks = now.Virtual, 0
	}
	t.Ticks++
	return t.Ticks <= TimerBusyWaitPolls
}

// Elapsed is t.read at now (ETSI 23.4): the seconds since t started, at
// most its duration, and 0 when t is inactive or has expired.
func (t *TimerHandle) Elapsed(now TimerNow) float64 {
	if t == nil || !t.Running || t.Expired(now) {
		return 0
	}
	var elapsed float64
	switch {
	case !now.OnVirtual:
		if t.StartedAt.IsZero() {
			return 0
		}
		elapsed = now.Wall.Sub(t.StartedAt).Seconds()
	case now.HasVirtual:
		elapsed = now.Virtual - t.StartedAtVirtual
	default:
		return 0
	}
	if elapsed < 0 {
		elapsed = 0
	}
	if t.Duration > 0 && elapsed > t.Duration {
		elapsed = t.Duration
	}
	return elapsed
}

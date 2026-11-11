package runtime

import (
	"fmt"
	"os"
)

// ComponentState is what the alive, running, done and killed operations
// answer of a test component (ETSI ES 201 873-1 21.3.5-21.3.8).
type ComponentState struct {
	Alive, Running, Done, Killed bool
}

// ComponentState is the state of r as by, the component asking, sees it;
// t may be nil, outside a testcase. A nil r is done and killed. A
// component no behaviour was started on is inactive, not running
// (21.3.6), except the MTC, which runs from the testcase's start, and a
// component asking about itself, which runs the behaviour it was given,
// by call rather than by start included.
func (t *TestcaseExec) ComponentState(r, by *ComponentRef) ComponentState {
	if r == nil {
		return ComponentState{Done: true, Killed: true}
	}
	alive, done := r.IsAlive(), r.IsDone()
	s := ComponentState{
		Alive:   alive,
		Running: alive && !done,
		Done:    done || !alive,
		Killed:  !alive,
	}
	if s.Running && !r.IsStarted() {
		s.Running = t != nil && (r.ID == t.MTCID() || by != nil && by.ID == r.ID)
	}
	return s
}

// Terminate records the end of r's behaviour by a stop, or by a kill
// (ETSI 21.3.3, 21.3.4): r is done, and no longer alive when it was
// killed or was not created alive.
func (r *ComponentRef) Terminate(kill bool) {
	r.SetDone(true)
	if kill || !r.AliveModifier {
		r.SetAlive(false)
	}
}

// StopComponent stops r, or kills it (ETSI 21.3.3, 21.3.4): its behaviour
// ends (Terminate) and is signalled to unwind, without waiting for it;
// the ports it mapped are unmapped, so another can map them; and stopping
// the MTC stops the testcase.
func (t *TestcaseExec) StopComponent(r *ComponentRef, kill bool) {
	r.Terminate(kill)
	t.StopPTC(r.ID)
	t.UnmapComponentPorts(r.ID)
	if r.ID == t.MTCID() {
		t.Stop()
	}
}

// SetVerdictOn sets the verdict v, with reason, on the component c that
// sets it, and on the testcase (ETSI 24.1); c is nil when the testcase
// itself sets it.
func (t *TestcaseExec) SetVerdictOn(c *ComponentRef, v Verdict, reason string) {
	t.SetVerdict(v, reason)
	if c != nil {
		c.MergeVerdict(v)
	}
}

// UnmapComponentPorts unmaps, through their port drivers, the ports the
// component compID mapped. Unmapping them again does nothing.
func (t *TestcaseExec) UnmapComponentPorts(compID int64) {
	t.unmapPorts(fmt.Sprintf("UnmapComponentPorts(%d)", compID), t.DrainPortMaps(compID))
}

// UnmapAllPorts unmaps, through their port drivers, every port any
// component of the testcase mapped, so the next testcase can map them.
func (t *TestcaseExec) UnmapAllPorts() {
	t.unmapPorts("UnmapAllPorts", t.DrainAllPortMaps())
}

// unmapPorts unmaps entries through their ports' drivers; a port without
// a driver has nothing to release. NTT_PORT_DEBUG traces it.
func (t *TestcaseExec) unmapPorts(what string, entries []PortMapEntry) {
	debug := os.Getenv("NTT_PORT_DEBUG") != ""
	if debug {
		fmt.Fprintf(os.Stderr, "[ntt] %s: %d entries\n", what, len(entries))
	}
	for _, ent := range entries {
		drv := t.PortDriver(ent.Local)
		if drv == nil {
			continue
		}
		if debug {
			fmt.Fprintf(os.Stderr, "[ntt] %s: Unmap(%q, %q)\n", what, ent.Local, ent.Remote)
		}
		_ = drv.Unmap(ent.Local, ent.Remote)
	}
}

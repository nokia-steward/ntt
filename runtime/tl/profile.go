package tl

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// Profiling a run from its log: the per-port measures `ntt exec --profile`
// takes while a run is live, computed afterwards from the events'
// timestamps, for any logged run and either clock.
//
// The model is the live profiler's: a port's sends and receives are its
// tliMSend and tliMReceive events, and each send answered by a receive on
// the same port, before the next send, is one latency sample, the time
// between the two. Procedure-based operations are not counted, as the live
// profiler does not count them.

// PortProfile is the traffic of one component's port in a testcase.
type PortProfile struct {
	Component string // the component, by its key (see Run)
	Port      string // the port, with its index for a port array element
	Sends     int
	Receives  int
	Latencies []time.Duration // one per send answered by a receive
}

// TestcaseProfile is one testcase's profile. Its duration is the time from
// its tliTcStart to its tliTcTerminated, zero when the log has no end for
// it.
type TestcaseProfile struct {
	Testcase string // as Run names it: Module.testcase, #n from its second run
	Module   string
	Name     string
	Verdict  string // as tliTcTerminated gives it; "" when the log has no end
	Reason   string
	Duration time.Duration
	Ports    []PortProfile // ordered by component, then port
}

// Profile computes the profile of every testcase in the log, in order.
// Events outside any testcase are not part of any profile.
func (l *Log) Profile() []TestcaseProfile {
	var out []TestcaseProfile
	for _, r := range l.Runs() {
		if r.Testcase == Outside {
			continue
		}
		out = append(out, r.profile())
	}
	return out
}

func (r *Run) profile() TestcaseProfile {
	p := TestcaseProfile{Testcase: r.Testcase}
	var start, end int64
	var comps []string
	for c := range r.Components {
		comps = append(comps, c)
	}
	sort.Strings(comps)
	for _, c := range comps {
		ports := map[string]*PortProfile{}
		pending := map[string]int64{} // port -> time of its unanswered send
		for _, e := range r.Components[c] {
			ts, _ := strconv.ParseInt(e.Attr("ts"), 10, 64)
			switch {
			case e.Tag == "tliTcStart":
				start = ts
				if tc := e.Kid("tcId"); tc != nil {
					if n := tc.Kid("name"); n != nil {
						p.Module, p.Name = n.Attr("moduleName"), n.Attr("baseName")
					}
				}
			case e.Tag == "tliTcTerminated":
				end = ts
				if v := e.Kid("verdict"); v != nil {
					if val := v.Kid("value"); val != nil {
						p.Verdict = val.Text
					}
				}
				if rs := e.Kid("reason"); rs != nil {
					p.Reason = rs.Text
				}
			case strings.HasPrefix(e.Tag, "tliMSend_"):
				port := portProfile(ports, c, e)
				port.Sends++
				pending[port.Port] = ts
			case strings.HasPrefix(e.Tag, "tliMReceive_"):
				port := portProfile(ports, c, e)
				port.Receives++
				if sent, ok := pending[port.Port]; ok {
					port.Latencies = append(port.Latencies, time.Duration(ts-sent)*time.Microsecond)
					delete(pending, port.Port)
				}
			}
		}
		var names []string
		for n := range ports {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			p.Ports = append(p.Ports, *ports[n])
		}
	}
	if end > start && start > 0 {
		p.Duration = time.Duration(end-start) * time.Microsecond
	}
	return p
}

// portProfile returns the profile of the port event e was performed at.
func portProfile(ports map[string]*PortProfile, comp string, e *Node) *PortProfile {
	name := ""
	if at := e.Kid("at"); at != nil {
		if pt := at.Kid("port"); pt != nil {
			name = portName(pt)
		}
	}
	p := ports[name]
	if p == nil {
		p = &PortProfile{Component: comp, Port: name}
		ports[name] = p
	}
	return p
}

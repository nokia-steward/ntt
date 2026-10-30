package tl

// lines.go writes the test log one event per line, its fields separated
// by `|`:
//
//	timestamp|event|identity=file:line|field|field...
//	20261008T091305.653242|setv|MTC=test.ttcn3:50|none|pass|"done"
//
// The timestamp is local time, to the microsecond. The event is a
// four-letter code, in lower case for an event and in upper case, followed
// by `->` and what it means, for an error. The identity is the component
// that produced it — MTC, SYSTEM, a PTC by its name, `$` and its number
// for one created without a name, ntt for the test system itself — with
// the source file and line when there is one. The fields are the event's
// own, values and templates written compactly (see linevalue.go):
// `{1,"a",omit}`, `?`, `T.f(x=1,y=-)`.
//
// It is the same stream of events as the TCI-TL log (ES 201 873-6), in a
// form to read in a pager and to split with awk, in the layout and with
// the event codes of a widely used structured line log of TTCN-3 runtimes,
// so that the tools reading those read it too. Events with no line of
// their own (an alt's snapshot that found nothing, say) are left out; the
// XML or JSONL log has them all. Fields are not escaped: a value never
// holds a newline, which char(...) writes, but may hold a `|`.

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LinesWriter writes events as lines. It is safe for concurrent use.
type LinesWriter struct {
	mu     sync.Mutex
	w      *bufio.Writer
	err    error
	closed bool
	loc    *time.Location

	verdicts map[string]string // per component, its verdict so far
	names    map[string]string // per component id, its name
	states   map[string]string // per component, inactive, running, stopped or killed
	frames   map[string][]frame
	timers   map[string]int64    // per component and timer, when it expires (µs)
	queues   map[string][]queued // per port, the messages detected in it
	sent     map[string]string   // per component, where it last sent
	guard    string              // the duration execute() gave the testcase starting
	execAt   string              // where execute() was called
	tcStart  int64               // when the testcase started (µs)
}

// frame is a behaviour a component is in: its qualified name, which its
// local variables are named after, its name without the module, the actual
// parameters it was entered with, and where it was called from.
type frame struct {
	scope, leaf, params, at string
}

// queued is a message in a port's queue, as a receive matching it shows it.
type queued struct {
	item string
}

// NewLinesWriter starts a log of lines, its timestamps in local time.
func NewLinesWriter(w io.Writer) *LinesWriter {
	return &LinesWriter{
		w: bufio.NewWriter(w), loc: time.Local,
		verdicts: map[string]string{}, names: map[string]string{}, states: map[string]string{},
		frames: map[string][]frame{}, timers: map[string]int64{}, queues: map[string][]queued{},
		sent: map[string]string{},
	}
}

// In sets the time zone the timestamps are written in.
func (lw *LinesWriter) In(loc *time.Location) *LinesWriter {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	lw.loc = loc
	return lw
}

// Log writes e as a line, or nothing for an event that has none.
func (lw *LinesWriter) Log(e *Event) {
	n := e.node()
	lw.mu.Lock()
	defer lw.mu.Unlock()
	if lw.closed || lw.err != nil {
		return
	}
	for _, l := range lw.lines(e, n) {
		_, lw.err = lw.w.WriteString(l + "\n")
	}
	if lw.err == nil {
		lw.err = lw.w.Flush()
	}
}

// Close flushes the log and returns the first write error. It does not
// close the underlying writer.
func (lw *LinesWriter) Close() error {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	if !lw.closed {
		lw.closed = true
		if lw.err == nil {
			lw.err = lw.w.Flush()
		}
	}
	return lw.err
}

// errorDescs is what an error event means, written after its code.
var errorDescs = map[string]string{
	"TSTP": "Test stopped by the testcase.stop operation",
	"DEAD": "Deadlock: no alternative available",
	"TIME": "Testcase execution time limit exceeded",
}

// lines renders e: usually one line, none for an event with no line of its
// own, more for one that stands for several.
func (lw *LinesWriter) lines(e *Event, n *Node) []string {
	arg := func(name string) *Node { return n.Kid(name) }
	val := func(name string) string { return lw.lineValue(arg(name)) }
	has := func(name string) bool { return n.Kid(name) != nil }
	who := lw.identity(e.C)
	here := lw.place(who, e.Src, e.Line)
	if who == "ntt" {
		// The test system's own events have no place in the source but
		// those of the behaviours it runs.
		here = "ntt"
	}
	line := func(id, at string, fields ...string) string {
		head := id
		if d, ok := errorDescs[id]; ok {
			head += "->" + d
		}
		parts := append([]string{lw.stamp(e.Ts), head, at}, fields...)
		for i := range parts {
			parts[i] = oneLine(parts[i])
		}
		return strings.Join(parts, "|")
	}
	one := func(id string, fields ...string) []string { return []string{line(id, here, fields...)} }

	switch op := e.Op; op {
	case "tliTcExecute":
		lw.guard = plain(arg("dur"))
		lw.execAt = srcLine(e.Src, e.Line)
		return nil
	case "tliTcStart":
		guard := lw.guard
		if has("dur") {
			guard = plain(arg("dur"))
		}
		lw.guard = ""
		if guard == "" {
			guard = "unlimited"
		} else {
			guard = lineFloat(guard)
		}
		// A testcase starts afresh: its components, ports and timers
		// are its own, its component ids too. The control part
		// running it is still where it was.
		control := lw.frames["ntt"]
		lw.verdicts, lw.names, lw.states = map[string]string{}, map[string]string{}, map[string]string{}
		lw.frames, lw.timers, lw.queues, lw.sent = map[string][]frame{"ntt": control}, map[string]int64{}, map[string][]queued{}, map[string]string{}
		lw.tcStart = e.Ts
		lw.verdicts["MTC"] = "none"
		lw.states["MTC"] = "running"
		tc := testcaseName(n)
		return []string{
			line("tcst", "ntt", tc, guard),
			line("cocr", "ntt", "MTC", "once"),
			line("cost", lw.place("MTC", e.Src, e.Line), tc),
		}
	case "tliTcTerminated":
		v := plain(arg("verdict"))
		lw.states["MTC"] = "stopped"
		return []string{line("cofi", "MTC", v), line("tcfi", "ntt", testcaseName(n), v)}
	case "tliTcStop":
		return one("TSTP", plain(arg("reason")))
	case "tliSEnter", "tliSLeave":
		return lw.behaviour(e, n, who, here, line)
	case "tliVar":
		if e.Path != "" && len(e.Parts) == 1 {
			// The element or field assigned, not its variable.
			el := e.Parts[0].node()
			return one("vach", lineTypeName(el), lw.valueName(e, who, arg("name"))+e.Path, lw.lineValue(el))
		}
		return one("vach", lineTypeName(arg("val")), lw.valueName(e, who, arg("name")), val("val"))
	case "tliModulePar":
		return []string{line("mpar", "ntt", qualified(arg("name")), val("val"))}
	case "tliSetVerdict":
		prev := lw.verdicts[who]
		if prev == "" {
			prev = "none"
		}
		v := plain(arg("verdict"))
		lw.verdicts[who] = worse(prev, v)
		var out []string
		if v == "inconc" || v == "fail" {
			if fs := lw.frames[who]; len(fs) > 0 {
				var trace []string
				for i := len(fs) - 1; i >= 0; i-- {
					trace = append(trace, fs[i].at+":"+fs[i].leaf+"("+fs[i].params+")")
				}
				out = append(out, line("bctr", here, trace...))
			}
		}
		fields := []string{prev, v}
		if reason := lw.concat(e, arg("reason")); reason != "" {
			fields = append(fields, reason)
		}
		return append(out, line("setv", here, fields...))
	case "tliGetVerdict":
		return one("getv", plain(arg("verdict")))
	case "tliLog":
		return one("ulog", lw.concat(e, arg("log")))
	case "tliAction":
		return one("uact", lw.concat(e, arg("action")))
	case "tliInfo":
		return one("dbg1", plain(arg("info")))
	case "tliMatch", "tliMatchMismatch":
		mismatch := ""
		if op == "tliMatchMismatch" {
			mismatch = "value:" + val("expr") + "~" + val("tmpl")
		}
		return one("matc", val("expr"), val("tmpl"), mismatch)
	case "tliEncode":
		return one("enco", lineTypeName(arg("val")), plain(arg("codec")), "", "")
	case "tliDecode":
		return one("deco", lineTypeName(arg("val")), plain(arg("codec")), "", "")

	// Components.
	case "tliCCreate":
		id, name := compID(arg("comp"))
		name = lw.compName(id, name)
		if name == "MTC" {
			return nil // with the testcase's start
		}
		lw.states[name] = "inactive"
		alive := "once"
		if plain(arg("alive")) == "true" {
			alive = "alive"
		}
		return one("cocr", name, alive)
	case "tliCStart":
		id, name := compID(arg("comp"))
		name = lw.compName(id, name)
		lw.states[name] = "running"
		lw.verdicts[name] = "none"
		return []string{line("cost", lw.place(name, e.Site.Src, e.Site.Line), qualified(arg("name")))}
	case "tliCTerminated":
		if lw.states[who] != "killed" {
			lw.states[who] = "stopped"
		}
		return []string{line("cofi", who, plain(arg("verdict")))}
	case "tliCStop":
		return one("cosp", lw.compRef(arg("comp")))
	case "tliCKill":
		name := lw.compRef(arg("comp"))
		lw.states[name] = "killed"
		return one("coki", name)
	case "tliCDone", "tliCDoneMismatch", "tliCKilled", "tliCKilledMismatch":
		id := pick(strings.HasPrefix(op, "tliCDone"), "codo", "cokd")
		name := lw.compOf(n)
		outcome := "ready"
		if strings.HasSuffix(op, "Mismatch") {
			outcome = "wait"
			if lw.states[name] == "inactive" {
				outcome = "never"
			}
		}
		return one(id, name, outcome)
	case "tliCRunning":
		return one("coru", lw.compRef(arg("comp")), status(plain(arg("status")), "running", "stopped"))
	case "tliCAlive":
		// Stopped and inactive components are alive: only killed is not.
		alive := "alive"
		if st := plain(arg("status")); st == ComponentKilled || st == "false" {
			alive = "killed"
		}
		return one("coal", lw.compRef(arg("comp")), alive)

	// Ports.
	case "tliPConnect":
		return one("ptcn", lw.portRef(arg("port1")), lw.portRef(arg("port2")))
	case "tliPDisconnect":
		return one("ptdi", lw.portRef(arg("port1")), lw.portRef(arg("port2")))
	case "tliPMap", "tliPMapParam":
		return one("ptmp", lw.portRef(arg("port1")), lw.portRef(arg("port2")), "")
	case "tliPUnmap", "tliPUnmapParam":
		return one("ptun", lw.portRef(arg("port1")), lw.portRef(arg("port2")))
	case "tliPClear":
		p := lw.portRef(arg("port"))
		delete(lw.queues, p)
		return one("ptcl", p)
	case "tliPStart":
		return one("ptst", lw.portRef(arg("port")))
	case "tliPStop":
		return one("ptsp", lw.portRef(arg("port")))
	case "tliPHalt":
		return one("ptha", lw.portRef(arg("port")))
	}

	// Communication, by the family of the operation.
	op := e.Op
	base := op
	if i := strings.IndexByte(op, '_'); i > 0 {
		base = op[:i]
	}
	switch base {
	case "tliMSend":
		lw.sent[who] = here
		return one("ptsd", lw.portRef(arg("at")), lw.portRef(arg("to")), lineTypeName(arg("msgValue")), val("msgValue"))
	case "tliPrCall":
		lw.sent[who] = here
		return one("ptsd", lw.portRef(arg("at")), lw.portRef(arg("to")), "call", lw.signature(n, true))
	case "tliPrReply":
		lw.sent[who] = here
		return one("ptsd", lw.portRef(arg("at")), lw.portRef(arg("to")), "reply", lw.signature(n, false)+lw.result(n, "replValue"))
	case "tliPrRaise":
		lw.sent[who] = here
		return one("ptsd", lw.portRef(arg("at")), lw.portRef(arg("to")), "exception", qualified(arg("signature"))+" "+val("excValue"))
	case "tliMDetected":
		return lw.detected(e, n, "message(value="+lineTypeName(arg("msgValue"))+":"+val("msgValue"))
	case "tliPrGetCallDetected":
		return lw.detected(e, n, "call("+lw.signature(n, true))
	case "tliPrGetReplyDetected":
		return lw.detected(e, n, "reply("+lw.signature(n, false)+lw.result(n, "replValue"))
	case "tliPrCatchDetected":
		return lw.detected(e, n, "exception(value="+qualified(arg("signature"))+":"+val("excValue"))
	case "tliMReceive", "tliMMismatch":
		return one("ptrx", lw.portRef(arg("at")), lw.description(arg("msgTmpl")), lw.outcome(n, base == "tliMReceive", "consume", "msgValue", "msgTmpl"))
	case "tliMChecked", "tliCheckedAny", "tliCheckAnyMismatch":
		return one("ptck", lw.portRef(arg("at")), lw.description(arg("msgTmpl")), lw.outcome(n, base != "tliCheckAnyMismatch", "check", "msgValue", "msgTmpl"))
	case "tliPrGetCall", "tliPrGetCallMismatch":
		return one("ptrx", lw.portRef(arg("at")), "call="+lw.signatureTmpl(n), lw.outcome(n, base == "tliPrGetCall", "consume", "", ""))
	case "tliPrGetReply", "tliPrGetReplyMismatch", "tliPrGetReplyChecked":
		return one("ptrx", lw.portRef(arg("at")), "reply="+lw.signatureTmpl(n), lw.outcome(n, base != "tliPrGetReplyMismatch", "consume", "", ""))
	case "tliPrCatch", "tliPrCatchMismatch", "tliPrCatchChecked":
		return one("ptrx", lw.portRef(arg("at")), "exception="+qualified(arg("signature"))+":"+val("excTmpl"),
			lw.outcome(n, base != "tliPrCatchMismatch", "consume", "", ""))
	case "tliPrCatchTimeout":
		return one("tmto", qualified(arg("signature")), "ready")
	}

	// Timers.
	timer := ""
	if t := arg("timer"); t != nil {
		timer = lw.timerName(e, who, t)
	}
	switch op {
	case "tliTStart":
		d := plain(arg("dur"))
		if f, err := strconv.ParseFloat(d, 64); err == nil {
			lw.timers[who+"\x00"+timer] = e.Ts + int64(f*1e6)
		}
		return one("tmst", timer, lineFloat(d)+"s")
	case "tliTStop":
		delete(lw.timers, who+"\x00"+timer)
		return one("tmsp", timer)
	case "tliTRead":
		return one("tmrd", timer, lineFloat(plain(arg("elapsed")))+"s")
	case "tliTRunning":
		return one("tmru", timer, strconv.FormatBool(plain(arg("status")) == TimerRunning))
	case "tliTTimeout":
		delete(lw.timers, who+"\x00"+timer)
		return one("tmto", timer, "ready")
	case "tliTTimeoutMismatch":
		outcome := "never(inactive)"
		if at, ok := lw.timers[who+"\x00"+timer]; ok {
			outcome = "wait(" + lw.stamp(at) + ")"
		}
		return one("tmto", timer, outcome)

	// Alternatives and defaults.
	case "tliAEnter":
		return one("alen")
	case "tliALeave":
		return one("allv")
	case "tliARepeat":
		return one("alrp")
	case "tliAWait":
		return one("alwt", lw.until(who))
	case "tliAActivate":
		return one("dtac", lw.call(n, true))
	case "tliADeactivate":
		return one("dtde", val("ref"))
	}
	return nil
}

// behaviour writes the entry into or exit from a function, altstep,
// testcase or control part, and keeps the stack of the behaviours each
// component is in.
func (lw *LinesWriter) behaviour(e *Event, n *Node, who, here string, line func(id, at string, fields ...string) string) []string {
	kind := plain(n.Kid("kind"))
	enter := e.Op == "tliSEnter"
	name := qualified(n.Kid("name"))
	if kind == "control" {
		if enter {
			lw.push(who, frame{scope: name, leaf: leaf(name)})
		} else {
			lw.pop(who)
		}
		return []string{line(pick(enter, "cpen", "cplv"), "ntt", name+"()")}
	}
	call := name + "(" + lw.params(n.Kid("tciPars"), enter) + ")"
	if kind == "testcase" {
		if enter {
			lw.push(who, frame{scope: name, leaf: leaf(name), params: lw.params(n.Kid("tciPars"), true), at: lw.execAt})
		} else {
			lw.pop(who)
		}
		return []string{line(pick(enter, "tcen", "tclv"), who, call)}
	}
	id := pick(enter, "fnen", "fnlv")
	switch {
	case strings.Contains(kind, "altstep"):
		id = pick(enter, "asen", "aslv")
	case strings.Contains(kind, "external"):
		id = pick(enter, "fxen", "fxlv")
	}
	var at string
	if enter {
		// Where the behaviour was called from, as the line of its
		// leaving is too.
		at = srcLine(e.Site.Src, e.Site.Line)
		if at == "" {
			at = srcLine(e.Src, e.Line)
		}
		lw.push(who, frame{scope: name, leaf: leaf(name), params: lw.params(n.Kid("tciPars"), true), at: at})
	} else {
		at = lw.pop(who).at
		if rv := n.Kid("returnValue"); rv != nil {
			call += "->" + lw.lineValue(rv)
		}
	}
	place := who
	if at != "" {
		place += "=" + at
	}
	return []string{line(id, place, call)}
}

func (lw *LinesWriter) push(who string, f frame) {
	lw.frames[who] = append(lw.frames[who], f)
}

func (lw *LinesWriter) pop(who string) frame {
	fs := lw.frames[who]
	if len(fs) == 0 {
		return frame{}
	}
	lw.frames[who] = fs[:len(fs)-1]
	return fs[len(fs)-1]
}

// detected writes a message, call, reply or exception arriving in a port's
// queue, as the component that sent it — from where it sent it — and
// keeps it for the receive that takes it.
func (lw *LinesWriter) detected(e *Event, n *Node, item string) []string {
	at := lw.portRef(n.Kid("at"))
	sender, senderType := "", ""
	if from := n.Kid("from"); from != nil {
		if c := from.Kid("comp"); c != nil {
			id, name := compID(c)
			sender = lw.compName(id, name)
			senderType = compType(c)
		}
	}
	stamp := lw.since(e.Ts)
	if sender != "" {
		item += ",sender=" + senderType + ":" + sender + "(" + lw.state(sender) + ")"
	} else {
		item += ",sender=:null"
	}
	item += ",timestamp=" + stamp + ")"
	lw.queues[at] = append(lw.queues[at], queued{item: item})
	who, here := sender, lw.sent[sender]
	if who == "" {
		who, here = "ntt", "ntt"
	}
	if here == "" {
		here = who
	}
	parts := []string{lw.stamp(e.Ts), "ptqu", here, at, item}
	for i := range parts {
		parts[i] = oneLine(parts[i])
	}
	return []string{strings.Join(parts, "|")}
}

// outcome is how a receive, check, getcall, getreply or catch went: the
// message it took, with how it took it, or the mismatch.
func (lw *LinesWriter) outcome(n *Node, ok bool, how, valueArg, tmplArg string) string {
	at := lw.portRef(n.Kid("at"))
	if !ok {
		if valueArg != "" && n.Kid(valueArg) != nil {
			return "never(mismatch=value:" + lw.lineValue(n.Kid(valueArg)) + "~" + lw.lineValue(n.Kid(tmplArg)) + ")"
		}
		return "never(mismatch)"
	}
	q := lw.queues[at]
	item := ""
	if len(q) > 0 {
		item = q[0].item
		if how == "consume" {
			lw.queues[at] = q[1:]
		}
	} else if valueArg != "" && n.Kid(valueArg) != nil {
		item = "message(value=" + lineTypeName(n.Kid(valueArg)) + ":" + lw.lineValue(n.Kid(valueArg)) + ")"
	}
	return "ready(match=" + item + ")+" + how
}

// description is the template a receive or check matches its message
// against, `value=` its type and itself, nothing for a receive of any.
func (lw *LinesWriter) description(t *Node) string {
	if t == nil {
		return ""
	}
	return "value=" + lineTypeName(t) + ":" + lw.lineValue(t)
}

// until is when the alt component who waits in can go on at the latest:
// the soonest expiry of its running timers, or maximum.
func (lw *LinesWriter) until(who string) string {
	var soonest int64
	found := false
	keys := make([]string, 0, len(lw.timers))
	for k := range lw.timers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if strings.HasPrefix(k, who+"\x00") && (!found || lw.timers[k] < soonest) {
			soonest, found = lw.timers[k], true
		}
	}
	if !found {
		return "maximum"
	}
	return lw.stamp(soonest)
}

// stamp writes microseconds since the epoch as 20060102T150405.000000.
func (lw *LinesWriter) stamp(us int64) string {
	return time.UnixMicro(us).In(lw.loc).Format("20060102T150405.000000")
}

// since writes the time since the testcase started as seconds, a point
// and six digits of microseconds.
func (lw *LinesWriter) since(us int64) string {
	d := us - lw.tcStart
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%d.%06d", d/1e6, d%1e6)
}

// identity names the component c: MTC, SYSTEM, a PTC by its name, `$` and
// its number for one without, ntt for the test system.
func (lw *LinesWriter) identity(c ComponentID) string {
	if c.Null || c.Name == "" && c.ID == "" || c.ID == "control" {
		return "ntt"
	}
	return lw.compName(c.ID, c.Name)
}

// compName is the name of the component with id and name, as identity has
// it, remembered for references to it by its id.
func (lw *LinesWriter) compName(id, name string) string {
	switch {
	case strings.EqualFold(name, "mtc"):
		name = "MTC"
	case strings.EqualFold(name, "system"):
		name = "SYSTEM"
	case name == "" || name == id:
		if known := lw.names[id]; known != "" {
			return known
		}
		name = "$" + id
	}
	if id != "" {
		lw.names[id] = name
	}
	return name
}

// compRef names the component a parameter names.
func (lw *LinesWriter) compRef(n *Node) string {
	if n == nil {
		return "-"
	}
	id, name := compID(n)
	if id == "" && name == "" {
		return lw.lineValue(n)
	}
	return lw.compName(id, name)
}

// componentValue writes a component reference: its component's name and
// state, worker(running).
func (lw *LinesWriter) componentValue(n *Node) string {
	id := ""
	if v := n.Kid("value"); v != nil {
		id = v.Text
	}
	name := lw.names[id]
	if name == "" {
		name = "$" + id
	}
	return name + "(" + lw.state(name) + ")"
}

func (lw *LinesWriter) state(name string) string {
	if s := lw.states[name]; s != "" {
		return s
	}
	return "running"
}

// compOf is the component of a done or killed: the one it names, or the
// template it was asked of.
func (lw *LinesWriter) compOf(n *Node) string {
	if k := n.Kid("comp"); k != nil {
		return lw.compRef(k)
	}
	if k := n.Kid("compTmpl"); k != nil {
		switch {
		case k.Kid("any") != nil:
			return "(any)"
		case k.Kid("all") != nil:
			return "(all)"
		}
		// A component value names its component by id.
		if len(k.Kids) == 1 && k.Kids[0].Tag == "component" {
			if v := k.Kids[0].Kid("value"); v != nil {
				if name := lw.names[v.Text]; name != "" {
					return name
				}
			}
		}
		return lw.lineValue(k)
	}
	return "-"
}

// portRef names a port as its component's name, a point and its own:
// MTC.p, worker.q[2].
func (lw *LinesWriter) portRef(n *Node) string {
	if n == nil {
		return ""
	}
	comp, port := n.Kid("comp"), n.Kid("port")
	if port == nil {
		return lw.lineValue(n)
	}
	name := portName(port)
	if comp == nil {
		return name
	}
	id, cname := compID(comp)
	return lw.compName(id, cname) + "." + name
}

// timerName names a timer as its owner and its own name: the component's,
// MTC.t, or the behaviour's it is local to, M.f.t.
func (lw *LinesWriter) timerName(e *Event, who string, t *Node) string {
	name := ""
	if id := t.Kid("id"); id != nil {
		if nm := id.Kid("name"); nm != nil {
			name = nm.Text
		}
	}
	return lw.scoped(e, who, name)
}

// valueName names the variable an assignment is to, as timerName names a
// timer.
func (lw *LinesWriter) valueName(e *Event, who string, n *Node) string {
	name := ""
	if n != nil {
		name = n.Attr("baseName")
	}
	return lw.scoped(e, who, name)
}

func (lw *LinesWriter) scoped(e *Event, who, name string) string {
	if !e.Member {
		if fs := lw.frames[who]; len(fs) > 0 {
			return fs[len(fs)-1].scope + "." + name
		}
	}
	return who + "." + name
}

// params writes actual parameters as name=value, separated by commas: on
// entry those a behaviour is given, `-` for an out parameter; on leaving
// those it gives back, `-` for an in parameter.
func (lw *LinesWriter) params(p *Node, enter bool) string {
	if p == nil {
		return ""
	}
	var ps []string
	for _, par := range p.Kids {
		mode := par.Attr("mode")
		v := "-"
		if val := par.Kid("val"); val != nil && (enter && mode != "out" || !enter && mode != "in" && mode != "") {
			v = lw.lineValue(val)
		}
		if name := par.Attr("name"); name != "" {
			ps = append(ps, name+"="+v)
		} else {
			ps = append(ps, v)
		}
	}
	return strings.Join(ps, ",")
}

// call is a behaviour named with its actual parameters.
func (lw *LinesWriter) call(n *Node, enter bool) string {
	return qualified(n.Kid("name")) + "(" + lw.params(n.Kid("tciPars"), enter) + ")"
}

// signature is a procedure call's signature and parameters.
func (lw *LinesWriter) signature(n *Node, enter bool) string {
	return qualified(n.Kid("signature")) + "(" + lw.params(n.Kid("tciPars"), enter) + ")"
}

// signatureTmpl is a signature with the template its parameters match.
func (lw *LinesWriter) signatureTmpl(n *Node) string {
	if t := n.Kid("parsTmpl"); t != nil {
		return qualified(n.Kid("signature")) + ":" + lw.lineValue(t)
	}
	return lw.signature(n, true)
}

// result appends a reply's value, if any.
func (lw *LinesWriter) result(n *Node, name string) string {
	if k := n.Kid(name); k != nil {
		return "->" + lw.lineValue(k)
	}
	return ""
}

// concat is what a log, an action or a verdict's reason writes: the values
// it was made of one after the other, or failing those its text.
func (lw *LinesWriter) concat(e *Event, fallback *Node) string {
	if len(e.Parts) == 0 {
		if fallback == nil {
			return ""
		}
		return fallback.Text
	}
	var b strings.Builder
	for _, v := range e.Parts {
		b.WriteString(lw.lineValue(v.node()))
	}
	return b.String()
}

// place is the identity field: the component, and where in the source.
func (lw *LinesWriter) place(who, src string, line int) string {
	if at := srcLine(src, line); at != "" {
		return who + "=" + at
	}
	return who
}

// srcLine is a source position as file:line, the file by its name.
func srcLine(src string, line int) string {
	if src == "" {
		return ""
	}
	return filepath.Base(src) + ":" + strconv.Itoa(line)
}

// qualified writes a qualified name, Module.name.
func qualified(n *Node) string {
	if n == nil {
		return "-"
	}
	if n.Attr("baseName") == "" {
		if nm := n.Kid("name"); nm != nil {
			return qualified(nm)
		}
		return brief(n)
	}
	if m := n.Attr("moduleName"); m != "" {
		return m + "." + n.Attr("baseName")
	}
	return n.Attr("baseName")
}

// leaf is a qualified name without its module.
func leaf(name string) string {
	if i := strings.IndexByte(name, '.'); i >= 0 {
		return name[i+1:]
	}
	return name
}

// compType is the type of a component as Module.Type.
func compType(c *Node) string {
	if id := c.Kid("id"); id != nil {
		if t := id.Kid("type"); t != nil {
			return t.Text
		}
	}
	return ""
}

// plain is the text of a parameter holding a simple value.
func plain(n *Node) string {
	if n == nil {
		return ""
	}
	if n.Text != "" || len(n.Kids) == 0 {
		return n.Text
	}
	if len(n.Kids) == 1 {
		return plain(n.Kids[0])
	}
	return brief(n)
}

func pick(enter bool, in, out string) string {
	if enter {
		return in
	}
	return out
}

// worse is the verdict a component has after it sets v, having had prev
// (ETSI 24.1: a verdict only gets worse).
func worse(prev, v string) string {
	rank := map[string]int{"none": 0, "pass": 1, "inconc": 2, "fail": 3, "error": 4}
	if rank[v] > rank[prev] {
		return v
	}
	return prev
}

func status(s, yes, no string) string {
	switch s {
	case ComponentRunning, "true":
		return yes
	case ComponentKilled, ComponentStopped, ComponentInactive, "false":
		return no
	}
	if strings.Contains(s, "alive") {
		return yes
	}
	return s
}

// oneLine keeps a field from breaking its line: a newline or carriage
// return from free text is written as a space.
func oneLine(s string) string {
	if !strings.ContainsAny(s, "\n\r") {
		return s
	}
	return strings.NewReplacer("\n", " ", "\r", " ").Replace(s)
}

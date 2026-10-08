package tl

// lines.go writes the test log one event per line, its fields separated
// by `|`:
//
//	timestamp|event|component=file:line|field|field...
//	20261008T091305.653242|setv|mtc=test.ttcn3:50|none|pass|done waiting
//
// The timestamp is UTC, to the microsecond; the event a four-letter code,
// in lower case for an event and upper case for an error; the component
// the one that produced it, with the source file's name and line when
// there is one (ntt for the test system itself). The fields are the event's own, each
// written as TTCN-3 writes values and templates: `{n := 1, s := "a"}`,
// `?`, `T.f(x=1,y=2)`. A `|`, a newline or a backslash in a field is
// escaped with a backslash.
//
// It is the same stream of events as the TCI-TL log (ES 201 873-6), in a
// form to read in a pager and to split with awk. Events with no line of
// their own (an alt's snapshot that found nothing, say) are left out; the
// XML or JSONL log has them all.

import (
	"bufio"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LinesWriter writes events as lines. It is safe for concurrent use.
type LinesWriter struct {
	mu       sync.Mutex
	w        *bufio.Writer
	err      error
	closed   bool
	verdicts map[string]string // per component, its verdict so far (setv)
	names    map[string]string // per component id, its name (cocr)
	guard    string            // the duration of the testcase execute() started
}

// NewLinesWriter starts a log of lines.
func NewLinesWriter(w io.Writer) *LinesWriter {
	return &LinesWriter{w: bufio.NewWriter(w), verdicts: map[string]string{}, names: map[string]string{}}
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

// lines renders e: usually one line, none for an event with no line of
// its own, two for a testcase's end (its MTC's and the testcase's).
func (lw *LinesWriter) lines(e *Event, n *Node) []string {
	arg := func(name string) string {
		if k := n.Kid(name); k != nil {
			return field(k)
		}
		return "-"
	}
	has := func(name string) bool { return n.Kid(name) != nil }
	who := e.C.Name
	if e.C.Null || who == "" {
		who = "ntt"
	}
	line := func(id, comp string, fields ...string) string {
		parts := []string{stamp(e.Ts), id, place(comp, e.Src, e.Line)}
		for _, f := range fields {
			parts = append(parts, escapeField(f))
		}
		return strings.Join(parts, "|")
	}
	one := func(id string, fields ...string) []string { return []string{line(id, who, fields...)} }
	outcome := func(ok bool) string {
		if ok {
			return "match"
		}
		return "mismatch"
	}
	switch op := e.Op; op {
	case "tliCtrlStart", "tliCtrlStartWithParameters":
		return one("cpen", "control")
	case "tliCtrlTerminated", "tliCtrlTerminatedWithResult", "tliCtrlStop":
		return one("cplv", "control")
	case "tliTcExecute":
		lw.guard = arg("dur")
		return nil
	case "tliTcStart":
		guard := lw.guard
		if has("dur") {
			guard = arg("dur")
		}
		lw.guard = ""
		return []string{line("tcst", "ntt", arg("tcId"), guard)}
	case "tliTcTerminated":
		v := arg("verdict")
		return []string{line("cofi", "mtc", v), line("tcfi", "ntt", arg("tcId"), v)}
	case "tliTcStop":
		return one("TSTP", arg("reason"))
	case "tliSEnter", "tliSLeave":
		kind := arg("kind")
		call := behaviour(n)
		if op == "tliSLeave" && has("returnValue") {
			call += "->" + arg("returnValue")
		}
		enter := op == "tliSEnter"
		switch {
		case kind == "testcase":
			return one(pick(enter, "tcen", "tclv"), call)
		case kind == "control":
			return nil // cpen / cplv
		case strings.Contains(kind, "altstep"):
			return one(pick(enter, "asen", "aslv"), call)
		case strings.Contains(kind, "external"):
			return one(pick(enter, "fxen", "fxlv"), call)
		}
		return one(pick(enter, "fnen", "fnlv"), call)
	case "tliVar":
		return one("vach", "-", arg("name"), arg("val"))
	case "tliModulePar":
		return one("mpar", arg("name"), arg("val"))
	case "tliSetVerdict":
		prev := lw.verdicts[who]
		if prev == "" {
			prev = "none"
		}
		v := arg("verdict")
		lw.verdicts[who] = worse(prev, v)
		reason := ""
		if has("reason") {
			reason = arg("reason")
		}
		return one("setv", prev, v, reason)
	case "tliGetVerdict":
		return one("getv", arg("verdict"))
	case "tliLog":
		return one("ulog", arg("log"))
	case "tliAction":
		return one("uact", arg("action"))
	case "tliInfo":
		return one("dbg1", arg("info"))
	case "tliMatch", "tliMatchMismatch":
		diffs := ""
		if has("diffs") {
			diffs = arg("diffs")
		}
		return one("matc", arg("expr"), arg("tmpl"), diffs)
	case "tliEncode":
		return one("enco", kindOf(n.Kid("val")), codec(n), "-", "-")
	case "tliDecode":
		return one("deco", kindOf(n.Kid("val")), codec(n), "-", "-")

	// Components.
	case "tliCCreate":
		if c := n.Kid("comp"); c != nil {
			if id, name := compID(c); id != "" && name != "" {
				lw.names[id] = name
			}
		}
		alive := "once"
		if arg("alive") == "true" {
			alive = "alive"
		}
		return one("cocr", arg("comp"), alive)
	case "tliCStart":
		return []string{line("cost", arg("comp"), behaviour(n))}
	case "tliCTerminated":
		return one("cofi", arg("verdict"))
	case "tliCStop":
		return one("cosp", arg("comp"))
	case "tliCKill":
		return one("coki", arg("comp"))
	case "tliCDone", "tliCDoneMismatch":
		return one("codo", lw.compOf(n), outcome(op == "tliCDone"))
	case "tliCKilled", "tliCKilledMismatch":
		return one("cokd", lw.compOf(n), outcome(op == "tliCKilled"))
	case "tliCRunning":
		return one("coru", arg("comp"), status(arg("status"), "running", "stopped"))
	case "tliCAlive":
		return one("coal", arg("comp"), status(arg("status"), "alive", "killed"))

	// Ports.
	case "tliPConnect":
		return one("ptcn", arg("port1"), arg("port2"))
	case "tliPDisconnect":
		return one("ptdi", arg("port1"), arg("port2"))
	case "tliPMap", "tliPMapParam":
		return one("ptmp", arg("port1"), arg("port2"))
	case "tliPUnmap", "tliPUnmapParam":
		return one("ptun", arg("port1"), arg("port2"))
	case "tliPClear":
		return one("ptcl", arg("port"))
	case "tliPStart":
		return one("ptst", arg("port"))
	case "tliPStop":
		return one("ptsp", arg("port"))
	case "tliPHalt":
		return one("ptha", arg("port"))
	}

	// Communication, by the family of the operation.
	op := e.Op
	base := op
	if i := strings.IndexByte(op, '_'); i > 0 {
		base = op[:i]
	}
	switch base {
	case "tliMSend":
		return one("ptsd", arg("at"), arg("to"), kindOf(n.Kid("msgValue")), arg("msgValue"))
	case "tliPrCall":
		return one("ptsd", arg("at"), arg("to"), "call", signature(n))
	case "tliPrReply":
		return one("ptsd", arg("at"), arg("to"), "reply", signature(n)+result(n, "replValue"))
	case "tliPrRaise":
		return one("ptsd", arg("at"), arg("to"), "exception", arg("signature")+" "+arg("excValue"))
	case "tliMDetected":
		return one("ptqu", arg("at"), "message "+arg("msgValue"))
	case "tliPrGetCallDetected":
		return one("ptqu", arg("at"), "call "+signature(n))
	case "tliPrGetReplyDetected":
		return one("ptqu", arg("at"), "reply "+signature(n)+result(n, "replValue"))
	case "tliPrCatchDetected":
		return one("ptqu", arg("at"), "exception "+arg("signature")+" "+arg("excValue"))
	case "tliMReceive", "tliMMismatch":
		return one("ptrx", arg("at"), arg("msgTmpl"), outcome(base == "tliMReceive"))
	case "tliMChecked", "tliCheckedAny", "tliCheckAnyMismatch":
		return one("ptck", arg("at"), arg("msgTmpl"), outcome(base != "tliCheckAnyMismatch"))
	case "tliPrGetCall", "tliPrGetCallMismatch":
		return one("ptrx", arg("at"), "call "+signatureTmpl(n), outcome(base == "tliPrGetCall"))
	case "tliPrGetReply", "tliPrGetReplyMismatch", "tliPrGetReplyChecked":
		return one("ptrx", arg("at"), "reply "+signatureTmpl(n), outcome(base != "tliPrGetReplyMismatch"))
	case "tliPrCatch", "tliPrCatchMismatch", "tliPrCatchChecked":
		return one("ptrx", arg("at"), "exception "+arg("signature")+" "+arg("excTmpl"), outcome(base != "tliPrCatchMismatch"))
	case "tliPrCatchTimeout":
		return one("tmto", arg("signature"), "match")
	}

	// Timers.
	switch op {
	case "tliTStart":
		return one("tmst", arg("timer"), arg("dur"))
	case "tliTStop":
		return one("tmsp", arg("timer"))
	case "tliTRead":
		return one("tmrd", arg("timer"), arg("elapsed"))
	case "tliTRunning":
		return one("tmru", arg("timer"), strconv.FormatBool(arg("status") == TimerRunning))
	case "tliTTimeout", "tliTTimeoutMismatch":
		return one("tmto", arg("timer"), outcome(op == "tliTTimeout"))

	// Alternatives and defaults.
	case "tliAEnter":
		return one("alen")
	case "tliALeave":
		return one("allv")
	case "tliARepeat":
		return one("alrp")
	case "tliAWait":
		return one("alwt", "-")
	case "tliAActivate":
		return one("dtac", behaviour(n))
	case "tliADeactivate":
		return one("dtde", arg("ref"))
	}
	return nil
}

// stamp writes microseconds since the epoch as 20060102T150405.000000, UTC.
func stamp(us int64) string {
	return time.UnixMicro(us).UTC().Format("20060102T150405.000000")
}

// place is the component field: the component, and where in the source —
// the file by its name, the XML and JSONL logs having its path.
func place(comp, src string, line int) string {
	if src == "" {
		return comp
	}
	return comp + "=" + filepath.Base(src) + ":" + strconv.Itoa(line)
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

// field writes a parameter's content as TTCN-3 writes it.
func field(n *Node) string { return brief(n) }

// behaviour is a function, altstep or testcase named with its actual
// parameters: `T.f(x=1,y=2)`.
func behaviour(n *Node) string {
	name := "-"
	if k := n.Kid("name"); k != nil {
		name = field(k)
	}
	if k := n.Kid("tcId"); k != nil {
		name = field(k)
	}
	return name + params(n.Kid("tciPars"))
}

// params writes actual parameters as `(name=value,...)`, `()` for none.
func params(p *Node) string {
	if p == nil {
		return "()"
	}
	var ps []string
	for _, par := range p.Kids {
		v := "-"
		if val := par.Kid("val"); val != nil {
			v = brief(val)
		}
		if name := par.Attr("name"); name != "" {
			ps = append(ps, name+"="+v)
		} else {
			ps = append(ps, v)
		}
	}
	return "(" + strings.Join(ps, ",") + ")"
}

// signature is a procedure call's signature and parameters.
func signature(n *Node) string {
	s := "-"
	if k := n.Kid("signature"); k != nil {
		s = field(k)
	}
	return s + params(n.Kid("tciPars"))
}

// signatureTmpl is a signature with the template its parameters match.
func signatureTmpl(n *Node) string {
	s := "-"
	if k := n.Kid("signature"); k != nil {
		s = field(k)
	}
	if t := n.Kid("parsTmpl"); t != nil {
		return s + " " + field(t)
	}
	return s + params(n.Kid("tciPars"))
}

// result appends a reply's value, if any.
func result(n *Node, name string) string {
	if k := n.Kid(name); k != nil {
		return " value " + field(k)
	}
	return ""
}

// compOf is the component of a done or killed: the one it names, or the
// template it was asked of.
func (lw *LinesWriter) compOf(n *Node) string {
	if k := n.Kid("comp"); k != nil {
		return field(k)
	}
	if k := n.Kid("compTmpl"); k != nil {
		// A component value names its component by id.
		if len(k.Kids) == 1 && k.Kids[0].Tag == "component" {
			if v := k.Kids[0].Kid("value"); v != nil {
				if name := lw.names[v.Text]; name != "" {
					return name
				}
			}
		}
		return field(k)
	}
	return "-"
}

// kindOf names a value's type: its type's name when it has one, else its
// kind (integer, record, ...).
func kindOf(n *Node) string {
	if n == nil || len(n.Kids) == 0 {
		return "-"
	}
	v := n.Kids[0]
	if t := v.Attr("type"); t != "" {
		return t
	}
	return v.Tag
}

func codec(n *Node) string {
	if k := n.Kid("codec"); k != nil {
		return field(k)
	}
	return "-"
}

// escapeField escapes what would break a line or its fields.
func escapeField(s string) string {
	if !strings.ContainsAny(s, "|\\\n\r") {
		return s
	}
	r := strings.NewReplacer(`\`, `\\`, "|", `\|`, "\n", `\n`, "\r", `\r`)
	return r.Replace(s)
}

package interpreter

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/nokia/ntt/builtins"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/runtime/tl"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// Test logging: the interpreter reports the TTCN-3 operations it performs
// as TCI-TL events (ETSI ES 201 873-6 clause 7.3.4.1; see runtime/tl). With
// no logger attached every helper here returns at its first check, so the
// cost of logging being off is one lookup per operation.

// tlExec returns the running testcase when test logging is on, else nil.
func tlExec(env runtime.Scope) *runtime.TestcaseExec {
	exec := runtime.FindTestcaseExec(env)
	if exec == nil || exec.TestLogger() == nil {
		return nil
	}
	return exec
}

// tlPos locates n in the test specification for an event's src and line.
// A nil node of a concrete type — a *syntax.CallExpr that is nil, passed as
// a syntax.Node — is not a nil interface, so it is checked for explicitly.
func tlPos(n syntax.Node) (string, int) {
	if n == nil {
		return "", 0
	}
	if v := reflect.ValueOf(n); v.Kind() == reflect.Ptr && v.IsNil() {
		return "", 0
	}
	src, _ := nodeFilename(n)
	line, _ := nodeLine(n)
	return src, line
}

// tlEmit logs op from the current component at n's position — unless it
// repeats, in a guard scan after waiting, what an earlier scan of the same
// alt round logged (see runtime.TLScanRepeats). Arrivals and mismatches
// have their own rules, and are passed on.
func tlEmit(exec *runtime.TestcaseExec, n syntax.Node, op string, args ...tl.Arg) {
	defer tlRecover(exec)
	src, line := tlPos(n)
	if !tl.ArrivalTiming(op) && exec.TLScanRepeats(currentCompID(exec), &tl.Event{Op: op, Src: src, Line: line, Args: args}) {
		return
	}
	exec.TLog(op, src, line, args...)
}

// tlEmitLazy is tlEmit for an event whose parameters are built by args,
// inside the protection tlRecover gives: building them evaluates nothing
// of the test, but converting a value can still fail.
func tlEmitLazy(exec *runtime.TestcaseExec, n syntax.Node, op string, args func() []tl.Arg) {
	defer tlRecover(exec)
	tlEmit(exec, n, op, args()...)
}

// tlTestcaseStarted records the testcase's identity and logs its start
// (tliTcStart, then tliTcStarted once it runs).
func tlTestcaseStarted(exec *runtime.TestcaseExec, tc *syntax.FuncDecl, module, name string, env runtime.Scope) {
	defer tlRecover(exec)
	tcID := tlArg("tcId", tl.TestcaseID(module, name))
	pars := tlArg("tciPars", tlTestcaseParams(tc, env))
	exec.SetTLTestcase(tcID, pars)
	tlEmit(exec, tc, "tliTcStart", tcID, pars)
	tlEmit(exec, tc, "tliTcStarted", tcID, pars)
}

// tlRecover keeps a failure of the logging itself from changing the test:
// a panic while building or writing an event is recorded in the log, as a
// tliInfo saying what failed, and the test goes on as if logging were off.
// Every logging entry point defers it.
func tlRecover(exec *runtime.TestcaseExec) {
	if r := recover(); r != nil {
		// Recording the failure goes through the same logger, which may
		// be what failed; that must not escape either.
		defer func() { _ = recover() }()
		exec.TLog("tliInfo", "", 0,
			tlArg("level", tl.Integer(0)),
			tlArg("info", tl.String(fmt.Sprintf("test logging failed: %v", r))))
	}
}

// tlArg builds an event parameter.
func tlArg(name string, c tl.Content) tl.Arg { return tl.Arg{Name: name, Val: c} }

// tlOwnPort identifies a port of the current component.
func tlOwnPort(exec *runtime.TestcaseExec, port string) tl.PortID {
	return exec.TLPort(currentCompID(exec), port)
}

// tlQualified returns the module and base name a behaviour call refers to:
// `f(...)` in the current module, or `M.f(...)`.
func tlQualified(body syntax.Node, env runtime.Scope) (string, string) {
	module := moduleNameFromEnv(env)
	ce, ok := body.(*syntax.CallExpr)
	if !ok {
		return module, syntax.Name(body)
	}
	switch f := ce.Fun.(type) {
	case *syntax.SelectorExpr:
		return syntax.Name(f.X), syntax.Name(f.Sel)
	default:
		return module, syntax.Name(ce.Fun)
	}
}

// tlCompTemplate is the component template of a done/killed operation:
// `any component`, `all component`, or the component itself.
func tlCompTemplate(exec *runtime.TestcaseExec, kind string, ref *runtime.ComponentRef) tl.Content {
	switch kind {
	case "any component":
		return tl.AnyTemplate()
	case "all component":
		return tl.AllTemplate()
	}
	return tl.ValueTemplate(runtime.TLValue(ref))
}

// tlTimerTemplate is the timer template of a timeout operation.
func tlTimerTemplate(kind string, th *runtime.TimerHandle) tl.Content {
	switch kind {
	case "any timer":
		return tl.AnyTemplate()
	case "all timer":
		return tl.AllTemplate()
	}
	return tl.ValueTemplate(runtime.TLValue(th))
}

// tlTimer identifies a timer.
func tlTimer(th *runtime.TimerHandle) tl.Content {
	name := ""
	if th != nil {
		name = th.Name
	}
	return tl.TimerID(name, "", "")
}

// tlTestcaseParams lists a testcase's formal parameters with the values
// they are bound to in env.
func tlTestcaseParams(tc *syntax.FuncDecl, env runtime.Scope) tl.Content {
	var ps []tl.Param
	if tc != nil && tc.Params != nil {
		for _, fp := range tc.Params.List {
			if fp == nil || fp.Name == nil {
				continue
			}
			mode := "in"
			if fp.Direction != nil {
				mode = fp.Direction.String()
			}
			v, _ := env.Get(fp.Name.String())
			ps = append(ps, tl.Param{Name: fp.Name.String(), Mode: mode, Val: runtime.TLValue(v)})
		}
	}
	return tl.Params(ps...)
}

// tlPortConfig logs a connect / disconnect / map / unmap between two port
// endpoints of the connection graph.
func tlPortConfig(exec *runtime.TestcaseExec, n syntax.Node, op string, a, b runtime.PortEndpoint) {
	defer tlRecover(exec)
	if exec.TestLogger() == nil {
		return
	}
	tlEmit(exec, n, op,
		tlArg("port1", exec.TLPort(a.Comp, a.Port).Content()),
		tlArg("port2", exec.TLPort(b.Comp, b.Port).Content()))
}

// tlTimerEvent logs a timer operation from its result: start, stop, read,
// running, and a timeout that happened (a timeout guard that did not
// match is not logged).
func tlTimerEvent(exec *runtime.TestcaseExec, n syntax.Node, th *runtime.TimerHandle, op string, res runtime.Object) {
	defer tlRecover(exec)
	timer := tlArg("timer", tlTimer(th))
	switch op {
	case "start":
		tlEmit(exec, n, "tliTStart", timer, tlArg("dur", tl.Duration(th.Duration)))
	case "stop":
		tlEmit(exec, n, "tliTStop", timer, tlArg("dur", tl.Duration(th.Duration)))
	case "read":
		if f, ok := res.(runtime.Float); ok {
			tlEmit(exec, n, "tliTRead", timer, tlArg("elapsed", tl.Duration(float64(f))))
		}
	case "running":
		status := tl.TimerInactive
		if b, ok := res.(runtime.Bool); ok && bool(b) {
			status = tl.TimerRunning
		} else if th != nil && th.Running {
			// Started, past its deadline, timeout not yet consumed.
			status = tl.TimerExpired
		}
		tlEmit(exec, n, "tliTRunning", timer, tlArg("status", tl.String(status)))
	case "timeout":
		if b, ok := res.(runtime.Bool); ok && !bool(b) {
			// A timeout guard that did not match: once per alt round
			// (see TLAltEpoch), not on every re-check of a waiting alt.
			if exec.TLMismatchIsNew(fmt.Sprintf("%d\x00%p", currentCompID(exec), n), exec.TLAltEpoch(currentCompID(exec))) {
				tlEmit(exec, n, "tliTTimeoutMismatch", timer, tlArg("timerTmpl", tlTimerTemplate("", th)))
			}
			return
		}
		// A blocking timeout that returned because the testcase or this
		// component was stopped did not time out.
		if exec.Stopped() || componentStopRequested(exec) {
			return
		}
		// The executor finds an expired timer when it looks, so the
		// detection and the timeout are logged together.
		tlEmit(exec, n, "tliTTimeoutDetected", timer)
		tlEmit(exec, n, "tliTTimeout", timer, tlArg("timerTmpl", tlTimerTemplate("", th)))
	}
}

// aliveCreateCtx is set while the operand of an `alive` modifier is
// evaluated, so the component it creates is logged as alive.
var aliveCreateCtx altContext

// createComponent creates a PTC for a create operation and logs it
// (tliCCreate).
func createComponent(typeName, name string, n syntax.Node, env runtime.Scope) *runtime.ComponentRef {
	ref := newComponentRef(typeName, name, env)
	if exec := tlExec(env); exec != nil {
		tlEmit(exec, n, "tliCCreate",
			tlArg("comp", exec.TLComponent(ref).Content()),
			tlArg("name", tl.String(name)),
			tlArg("alive", tl.Boolean(aliveCreateCtx.active())))
	}
	return ref
}

// tlTerminated logs, on behalf of a PTC, that its behaviour has ended.
func tlTerminated(exec *runtime.TestcaseExec, ref *runtime.ComponentRef) {
	defer tlRecover(exec)
	if exec == nil || ref == nil {
		return
	}
	if l := exec.TestLogger(); l != nil {
		exec.TLogFrom(l, exec.TLComponent(ref), "tliCTerminated", "", 0,
			tlArg("verdict", tl.Verdict(tlVerdict(ref.GetVerdict()))))
	}
}

// tlStopKill logs a stop or kill operation on ref.
func tlStopKill(env runtime.Scope, n syntax.Node, op string, ref *runtime.ComponentRef) {
	if exec := tlExec(env); exec != nil {
		tlOp := "tliCStop"
		if op == "kill" {
			tlOp = "tliCKill"
		}
		tlEmit(exec, n, tlOp, tlArg("comp", exec.TLComponent(ref).Content()))
	}
}

// tlDoneKilled logs a done or killed operation that matched: on ref, or
// with kind "any component" / "all component".
func tlDoneKilled(env runtime.Scope, n syntax.Node, op, kind string, ref *runtime.ComponentRef) {
	exec := tlExec(env)
	if exec == nil {
		return
	}
	tlOp := "tliCDone"
	if op == "killed" {
		tlOp = "tliCKilled"
	}
	tlEmit(exec, n, tlOp, tlArg("compTmpl", tlCompTemplate(exec, kind, ref)))
}

// tlSend logs a send (tliMSend_m, or tliMSend_c and its multicast and
// broadcast forms) as the engine routes it: to the system when the port is
// mapped, otherwise to the connected peers the `to` clause addresses, or to
// every connected peer when there is none. dest is the value the engine
// already evaluated the `to` clause to — never evaluated again here, since a
// log must not change what the test does.
func tlSend(exec *runtime.TestcaseExec, n syntax.Node, port string, dest, payload runtime.Object) {
	defer tlRecover(exec)
	self := currentCompID(exec)
	at := tlArg("at", exec.TLPort(self, port).Content())
	msg := tlArg("msgValue", runtime.TLValue(payload).AsValue())
	if exec.TLMapped(self, port) {
		args := []tl.Arg{at, tlArg("to", exec.TLPort(-2, port).Content()), msg}
		if l, ok := dest.(*runtime.List); ok && l.ListType == runtime.VALUE_LIST {
			// `to (a1, a2)`: a multicast to the listed addresses.
			addrs := tl.Value{Kind: "record"}
			for _, a := range l.Elements {
				addrs.Elems = append(addrs.Elems, runtime.TLValue(a))
			}
			tlEmit(exec, n, "tliMSend_m_MC", append(args, tlArg("addrValues", addrs.AsTyped()))...)
			return
		}
		if dest != nil {
			args = append(args, tlArg("addrValue", runtime.TLValue(dest).AsValue()))
		}
		tlEmit(exec, n, "tliMSend_m", args...)
		return
	}
	peers := tlConnectedPeers(exec, port)
	if ids := tlTargetIDs(dest); ids != nil {
		var to []tl.PortID
		for _, p := range peers {
			if ids[p.Comp] {
				to = append(to, exec.TLPort(p.Comp, p.Port))
			}
		}
		switch {
		case len(to) == 1:
			tlEmit(exec, n, "tliMSend_c", at, tlArg("to", to[0].Content()), msg)
			return
		case len(to) > 1:
			tlEmit(exec, n, "tliMSend_c_MC", at, tlArg("to", tl.PortIDList(to...)), msg)
			return
		}
	}
	switch len(peers) {
	case 0:
		tlEmit(exec, n, "tliMSend_c", at, msg)
	case 1:
		tlEmit(exec, n, "tliMSend_c", at, tlArg("to", exec.TLPort(peers[0].Comp, peers[0].Port).Content()), msg)
	default:
		var to []tl.PortID
		for _, p := range peers {
			to = append(to, exec.TLPort(p.Comp, p.Port))
		}
		tlEmit(exec, n, "tliMSend_c_BC", at, tlArg("to", tl.PortIDList(to...)), msg)
	}
}

// sortPortIDs orders ports by component id, then name and index, so that
// two runs log a set of destinations alike.
func sortPortIDs(ps []tl.PortID) {
	num := func(id string) int64 { n, _ := strconv.ParseInt(id, 10, 64); return n }
	sort.Slice(ps, func(i, j int) bool {
		a, b := ps[i], ps[j]
		if a.Comp.ID != b.Comp.ID {
			return num(a.Comp.ID) < num(b.Comp.ID)
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Index < b.Index
	})
}

// tlConnectedPeers returns the endpoints port of the current component is
// connected to, the way the engine routes to them: an element of a port
// array is connected under the array's name, and its peer is the same
// element of the peer's array.
func tlConnectedPeers(exec *runtime.TestcaseExec, port string) []runtime.PortEndpoint {
	cur := int64(-1)
	if c := exec.CurrentComponent(); c != nil {
		cur = c.ID
	}
	peers := exec.ConnectedPeers(runtime.PortEndpoint{Comp: cur, Port: port})
	if len(peers) == 0 {
		base, suffix := splitPortIndex(port)
		if suffix == "" {
			return nil
		}
		peers = exec.ConnectedPeers(runtime.PortEndpoint{Comp: cur, Port: base})
		for i := range peers {
			peers[i].Port += suffix
		}
	}
	// The connection graph is a map: list the peers in one order, so that
	// two runs log a broadcast alike.
	sort.Slice(peers, func(i, j int) bool {
		if peers[i].Comp != peers[j].Comp {
			return peers[i].Comp < peers[j].Comp
		}
		return peers[i].Port < peers[j].Port
	})
	return peers
}

// tlTargetIDs is the set of components a `to` value addresses: one
// component, or a list of them. Nil when it addresses none.
func tlTargetIDs(dest runtime.Object) map[int64]bool {
	var refs []runtime.Object
	switch v := dest.(type) {
	case *runtime.ComponentRef:
		refs = []runtime.Object{v}
	case *runtime.List:
		refs = v.Elements
	default:
		return nil
	}
	ids := map[int64]bool{}
	for _, r := range refs {
		if c, ok := r.(*runtime.ComponentRef); ok && c != nil {
			ids[c.ID] = true
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}

// tlReceive logs a message receiving operation on port against the head
// of its queue: a receive or check that matched (tliMReceive_c / _m,
// tliMChecked_c / _m), or a head that did not match (tliMMismatch_c /
// _m). The _m forms apply to a port mapped to the system, whose sender is
// an address rather than a component. tmpl and fromTmpl are the templates
// the engine matched against; payloadOk says the value matched and only
// the `from` clause did not. A mismatch is logged once per message and
// receiving operation when dedupe is set: an alt re-checking its guards
// against an unchanged queue head is not a new event, but each head a
// trigger discards is.
func tlReceive(exec *runtime.TestcaseExec, call syntax.Node, port string, head runtime.PortMessage, tmpl, fromTmpl runtime.Object, ev string, payloadOk, dedupe bool) {
	defer tlRecover(exec)
	self := currentCompID(exec)
	if ev == "mismatch" && dedupe {
		key := fmt.Sprintf("%d\x00%s\x00%p", self, port, call)
		if !exec.TLMismatchIsNew(key, head.Seq) {
			return
		}
	}
	suffix := "_c"
	if exec.TLMapped(self, port) {
		suffix = "_m"
	}
	if tmpl == nil {
		tmpl = runtime.Any
	}
	checkAny := ev != "receive" && tlIsCheckAny(call, ev)
	args := []tl.Arg{tlArg("at", exec.TLPort(self, port).Content())}
	if !checkAny {
		args = append(args,
			tlArg("msgValue", runtime.TLValue(head.Payload).AsValue()),
			tlArg("msgTmpl", runtime.TLValue(tmpl).AsTemplate()))
	}
	if ev == "mismatch" && !checkAny {
		ds := tlDiffs(head.Payload, tmpl)
		if payloadOk {
			ds = []tl.Diff{{Val: ".", Tmpl: ".", Desc: "sender does not match the from clause"}}
		}
		args = append(args, tlArg("diffs", tl.Diffs(ds...)))
	}
	if suffix == "_c" {
		if s, ok := head.Sender.(*runtime.ComponentRef); ok && s != nil {
			args = append(args, tlArg("from", exec.TLComponent(s).Content()))
		}
		if fromTmpl != nil {
			args = append(args, tlArg("fromTmpl", tl.ValueTemplate(runtime.TLValue(fromTmpl))))
		}
	} else {
		if head.Sender != nil {
			args = append(args, tlArg("addrValue", runtime.TLValue(head.Sender).AsValue()))
		}
		if fromTmpl != nil {
			args = append(args, tlArg("addressTmpl", runtime.TLValue(fromTmpl).AsTemplate()))
		}
	}
	op := map[string]string{"receive": "tliMReceive", "check": "tliMChecked", "mismatch": "tliMMismatch"}[ev] + suffix
	if checkAny {
		// A check with no receiving operation (`p.check`,
		// `p.check(from c)`) checks for anything at the head of the
		// queue, message or not.
		op = map[string]string{"check": "tliCheckedAny", "mismatch": "tliCheckAnyMismatch"}[ev] + suffix
	}
	if orig, ok := checkAnyCalls.Load(call); ok {
		// Locate the check itself, not the call standing in for it.
		call = orig.(syntax.Node)
	}
	tlEmit(exec, call, op, args...)
}

// checkAnyCalls maps each check with no receiving operation to the
// nameless call the engine matches it by, and back: one per check in the
// source, so that a mismatch of it is recognised as the same one on the
// next pass of an alt, and logged where the check is.
var checkAnyCalls sync.Map

// checkAnyCall returns the nameless call standing in for check n: with
// test logging off, a new one, as only the log tells the calls apart.
func checkAnyCall(n *syntax.CallExpr, env runtime.Scope) *syntax.CallExpr {
	if tlExec(env) == nil {
		return &syntax.CallExpr{Fun: &syntax.Ident{}}
	}
	if c, ok := checkAnyCalls.Load(n); ok {
		return c.(*syntax.CallExpr)
	}
	c, loaded := checkAnyCalls.LoadOrStore(n, &syntax.CallExpr{Fun: &syntax.Ident{}})
	if !loaded {
		checkAnyCalls.Store(c, n)
	}
	return c.(*syntax.CallExpr)
}

// isCheckAnyStandIn reports whether call is the nameless call standing in
// for a check with no receiving operation (see checkAnyCall).
func isCheckAnyStandIn(call *syntax.CallExpr) bool {
	if call == nil {
		return false
	}
	id, ok := call.Fun.(*syntax.Ident)
	return ok && id != nil && id.Tok == nil
}

// tlIsCheckAny reports whether the receiving operation logged as call is a
// check with no receiving operation of its own: a bare `p.check` guard
// (logged with no call), `p.check` or `p.check()`, or `p.check(from c)`,
// for which the engine synthesises a call with no name.
func tlIsCheckAny(call syntax.Node, ev string) bool {
	c, ok := call.(*syntax.CallExpr)
	if !ok || c == nil {
		return ev == "check"
	}
	switch f := c.Fun.(type) {
	case *syntax.Ident:
		return f == nil || f.Tok == nil || f.String() == "check"
	case *syntax.SelectorExpr:
		return syntax.Name(f.Sel) == "check"
	}
	return false
}

// tlDiffs locates where a value fails a record template: one difference
// per field the template constrains and the value does not satisfy, as an
// XPath relative to the logged value and template. Other values differ as
// a whole.
func tlDiffs(val, tmpl runtime.Object) []tl.Diff {
	vf, tf := tlFields(val), tlFields(tmpl)
	if vf == nil || tf == nil {
		return []tl.Diff{{Val: ".", Tmpl: ".", Desc: "value does not match template"}}
	}
	var out []tl.Diff
	for name, t := range tf {
		v, ok := vf[name]
		if !ok {
			v = runtime.Omit
		}
		if b, ok := builtins.Match(v, t).(runtime.Bool); ok && bool(b) {
			continue
		}
		path := "*[@name='" + name + "']"
		out = append(out, tl.Diff{Val: path, Tmpl: path, Desc: "field " + name})
	}
	if len(out) == 0 {
		return []tl.Diff{{Val: ".", Tmpl: ".", Desc: "value does not match template"}}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Val < out[j].Val })
	return out
}

func tlFields(o runtime.Object) map[string]runtime.Object {
	switch v := o.(type) {
	case *runtime.Record:
		return v.Fields
	case *runtime.List:
		if len(v.FieldNames) == len(v.Elements) && len(v.FieldNames) > 0 {
			m := make(map[string]runtime.Object, len(v.Elements))
			for i, n := range v.FieldNames {
				m[n] = v.Elements[i]
			}
			return m
		}
	}
	return nil
}

// tlVerdict names a component's local verdict; one never set is none
// (ETSI 22.4.1).
func tlVerdict(v runtime.Verdict) string {
	if v == "" {
		return string(runtime.NoneVerdict)
	}
	return string(v)
}

// tlTestcaseTerminated logs the end of a testcase with its final verdict
// (tliTcTerminated). No-op when logging is off or the testcase never
// started.
func tlTestcaseTerminated(exec *runtime.TestcaseExec, v runtime.Verdict, reason string) {
	defer tlRecover(exec)
	if exec == nil || exec.TestLogger() == nil || exec.TLTestcase() == nil {
		return
	}
	args := append(append([]tl.Arg(nil), exec.TLTestcase()...), tlArg("verdict", tl.Verdict(tlVerdict(v))))
	if reason != "" {
		args = append(args, tlArg("reason", tl.String(reason)))
	}
	// Mark the end first: a component still running must not log after
	// tliTcTerminated as if the testcase were going on.
	exec.TLEnd()
	exec.TLogFrom(exec.TestLogger(), exec.TLComponentByID(exec.MTCID()), "tliTcTerminated", "", 0, args...)
}

// tlTestcaseStopped logs that a testcase was stopped from outside, e.g. by
// its execute() timeout (tliTcStop).
func tlTestcaseStopped(exec *runtime.TestcaseExec, reason string) {
	defer tlRecover(exec)
	if exec == nil || exec.TestLogger() == nil || exec.TLTestcase() == nil {
		return
	}
	exec.TLogFrom(exec.TestLogger(), exec.TLComponentByID(exec.MTCID()), "tliTcStop", "", 0,
		tlArg("reason", tl.String(reason)))
}

// tlProcSend logs a call, reply or raise (tliPrCall, tliPrReply,
// tliPrRaise) as the engine delivered it: through a port driver to the
// system (_m), or to the queues in keys (_c, with _MC when the operation
// addressed several components and _BC when it reached several without
// addressing them). A delivery only to the sending port's own queue is a
// loopback with no peer to name.
func tlProcSend(exec *runtime.TestcaseExec, n syntax.Node, op, port string, msg runtime.PortMessage, keys []string, addressed, viaDriver bool) {
	defer tlRecover(exec)
	self := currentCompID(exec)
	name := map[string]string{"call": "tliPrCall", "reply": "tliPrReply", "raise": "tliPrRaise"}[op]
	at := tlArg("at", exec.TLPort(self, port).Content())
	body := []tl.Arg{tlArg("signature", tl.Signature(msg.Signature))}
	switch op {
	case "call":
		body = append(body, tlArg("tciPars", runtime.TLParams(msg.Payload)))
	case "reply":
		body = append(body, tlArg("tciPars", runtime.TLParams(msg.Payload)))
		if msg.RetValue != nil {
			body = append(body, tlArg("replValue", runtime.TLValue(msg.RetValue).AsValue()))
		}
	case "raise":
		if msg.RetValue != nil {
			body = append(body, tlArg("excValue", runtime.TLValue(msg.RetValue).AsValue()))
		}
	}
	// A port mapped to the system addresses the system, whether or not a
	// driver carries the operation — as a message send on it does.
	if viaDriver || exec.TLMapped(self, port) {
		args := append([]tl.Arg{at, tlArg("to", exec.TLPort(-2, port).Content())}, body...)
		tlEmit(exec, n, name+"_m", args...)
		return
	}
	// The port's own queue is a destination only when it is connected to
	// itself; on an unconnected port it is the loopback, with no peer.
	connected := len(tlConnectedPeers(exec, port)) > 0
	var to []tl.PortID
	own := exec.PortKey(port)
	for _, k := range keys {
		if k == own && !connected {
			continue
		}
		to = append(to, exec.TLPortForKey(k))
	}
	sortPortIDs(to)
	switch {
	case len(to) == 0:
		tlEmit(exec, n, name+"_c", append([]tl.Arg{at}, body...)...)
	case len(to) == 1:
		tlEmit(exec, n, name+"_c", append([]tl.Arg{at, tlArg("to", to[0].Content())}, body...)...)
	case addressed:
		tlEmit(exec, n, name+"_c_MC", append([]tl.Arg{at, tlArg("to", tl.PortIDList(to...))}, body...)...)
	default:
		tlEmit(exec, n, name+"_c_BC", append([]tl.Arg{at, tlArg("to", tl.PortIDList(to...))}, body...)...)
	}
}

// tlProcReceive logs a procedure receiving operation against the head of a
// port queue: a getcall, getreply or catch that matched (tliPrGetCall,
// tliPrGetReply, tliPrCatch), a check that did (the Checked forms), or a
// head that did not (the Mismatch forms), _c or _m. pars and val are the
// parameter and value templates the engine matched with, fromTmpl its
// `from` template; desc, when set, explains a mismatch the templates do
// not, such as a reply to another signature.
func tlProcReceive(exec *runtime.TestcaseExec, call syntax.Node, port string, head runtime.PortMessage, pars, val, fromTmpl runtime.Object, ev, desc string, dedupe bool) {
	defer tlRecover(exec)
	self := currentCompID(exec)
	if ev == "mismatch" && dedupe {
		key := fmt.Sprintf("%d\x00%s\x00%p", self, port, call)
		if !exec.TLMismatchIsNew(key, head.Seq) {
			return
		}
	}
	base := map[runtime.PortMsgKind]string{
		runtime.MsgCall:      "tliPrGetCall",
		runtime.MsgReply:     "tliPrGetReply",
		runtime.MsgException: "tliPrCatch",
	}[head.Kind]
	if base == "" {
		return
	}
	op := base + map[string]string{"receive": "", "check": "Checked", "mismatch": "Mismatch"}[ev]
	suffix := "_c"
	if exec.TLMapped(self, port) {
		suffix = "_m"
	}
	args := []tl.Arg{
		tlArg("at", exec.TLPort(self, port).Content()),
		tlArg("signature", tl.Signature(head.Signature)),
	}
	switch head.Kind {
	case runtime.MsgCall, runtime.MsgReply:
		args = append(args, tlArg("tciPars", runtime.TLParams(head.Payload)))
		if pars != nil {
			args = append(args, tlArg("parsTmpl", runtime.TLValue(pars).AsTemplate()))
		}
		if head.Kind == runtime.MsgReply {
			if head.RetValue != nil {
				args = append(args, tlArg("replValue", runtime.TLValue(head.RetValue).AsValue()))
			}
			if val != nil {
				args = append(args, tlArg("replTmpl", runtime.TLValue(val).AsTemplate()))
			}
		}
	case runtime.MsgException:
		if head.RetValue != nil {
			args = append(args, tlArg("excValue", runtime.TLValue(head.RetValue).AsValue()))
		}
		if val != nil {
			args = append(args, tlArg("excTmpl", runtime.TLValue(val).AsTemplate()))
		}
	}
	if ev == "mismatch" {
		var ds []tl.Diff
		switch {
		case desc != "":
			ds = []tl.Diff{{Val: ".", Tmpl: ".", Desc: desc}}
		case pars != nil && !matchProcObj(pars, head.Payload):
			ds = tlDiffs(head.Payload, pars)
		case val != nil:
			ds = tlDiffs(head.RetValue, val)
		}
		args = append(args, tlArg("diffs", tl.Diffs(ds...)))
	}
	if suffix == "_c" {
		if s, ok := head.Sender.(*runtime.ComponentRef); ok && s != nil {
			args = append(args, tlArg("from", exec.TLComponent(s).Content()))
		}
		if fromTmpl != nil {
			args = append(args, tlArg("fromTmpl", tl.ValueTemplate(runtime.TLValue(fromTmpl))))
		}
	} else if fromTmpl != nil {
		args = append(args, tlArg("addressTmpl", runtime.TLValue(fromTmpl).AsTemplate()))
	}
	tlEmit(exec, call, op+suffix, args...)
}

// tlCatchTimeout logs the timeout of a blocking call, caught by its
// `catch(timeout)` guard (tliPrCatchTimeout).
func tlCatchTimeout(exec *runtime.TestcaseExec, g syntax.Node, env runtime.Scope) {
	defer tlRecover(exec)
	es, ok := g.(*syntax.ExprStmt)
	if !ok {
		return
	}
	info := extractCommOp(es.Expr)
	if info.call == nil {
		return
	}
	sel, ok := info.call.Fun.(*syntax.SelectorExpr)
	if !ok {
		return
	}
	port, ok := portExprName(sel.X, env)
	if !ok {
		port = syntax.Name(sel.X)
	}
	at := tlArg("at", tlOwnPort(exec, port).Content())
	sig := tlArg("signature", tl.Signature(currentCallSignature(env)))
	// As with a timer's timeout, the executor finds the call timer
	// expired when it looks: the detection and the catch are logged
	// together.
	tlEmit(exec, g, "tliPrCatchTimeoutDetected", at, sig)
	tlEmit(exec, g, "tliPrCatchTimeout", at, sig)
}

// tlControlComponent stands for the control part, which runs outside any
// test component, as the producer of its own events.
var tlControlComponent = tl.ComponentID{Name: "control", ID: "control"}

// tlControl logs a control-part event (tliCtrlStart, tliCtrlTerminated).
// A control part has no testcase execution, so it logs directly.
func tlControl(l tl.Logger, n syntax.Node, op string, args ...tl.Arg) {
	if l == nil {
		return
	}
	defer tlControlRecover(l)
	src, line := tlPos(n)
	l.Log(&tl.Event{Op: op, Ts: tlNow(l), Src: src, Line: line, C: tlControlComponent, Args: args})
}

// tlExecute logs an execute() of the control part (tliTcExecute): the
// testcase, its actual parameters, and the timeout when one is given.
func tlExecute(l tl.Logger, n syntax.Node, mod *syntax.Module, module, tcName string, args []runtime.Object, timeout float64, hasTimeout bool) {
	if l == nil {
		return
	}
	defer tlControlRecover(l)
	var ps []tl.Param
	if tc := findTestcase(mod, tcName); tc != nil && tc.Params != nil {
		for i, fp := range tc.Params.List {
			if fp == nil || fp.Name == nil || i >= len(args) || args[i] == nil {
				continue
			}
			ps = append(ps, tl.Param{Name: fp.Name.String(), Val: runtime.TLValue(args[i])})
		}
	}
	a := []tl.Arg{tlArg("tcId", tl.TestcaseID(module, tcName)), tlArg("tciPars", tl.Params(ps...))}
	if hasTimeout {
		a = append(a, tlArg("dur", tl.Duration(timeout)))
	}
	tlControl(l, n, "tliTcExecute", a...)
}

// tlControlRecover is tlRecover for the control part's events.
func tlControlRecover(l tl.Logger) {
	if r := recover(); r != nil {
		defer func() { _ = recover() }()
		l.Log(&tl.Event{Op: "tliInfo", Ts: tlNow(l), C: tlControlComponent, Args: []tl.Arg{
			tlArg("level", tl.Integer(0)),
			tlArg("info", tl.String(fmt.Sprintf("test logging failed: %v", r))),
		}})
	}
}

// tlNow is the timestamp for an event not timed by a testcase: the log's
// monotonic clock when it keeps one, the wall clock otherwise.
func tlNow(l tl.Logger) int64 {
	if c, ok := l.(interface{ Now() int64 }); ok {
		return c.Now()
	}
	return time.Now().UnixMicro()
}

// tlScope logs entering or leaving a scope (tliSEnter, tliSLeave): a
// function, altstep or testcase, with its parameters as bound in env and,
// on leaving, the value it returns.
func tlScope(exec *runtime.TestcaseExec, n syntax.Node, op, module, name, kind string, params *syntax.FormalPars, env runtime.Scope, ret runtime.Object) {
	defer tlRecover(exec)
	args := []tl.Arg{tlArg("name", tl.QualifiedName(module, name))}
	if params != nil && len(params.List) > 0 {
		var ps []tl.Param
		for _, fp := range params.List {
			if fp == nil || fp.Name == nil {
				continue
			}
			mode := "in"
			if fp.Direction != nil {
				mode = fp.Direction.String()
			}
			v, _ := env.Get(fp.Name.String())
			if t, ok := v.(*runtime.LazyThunk); ok {
				// A @lazy or @fuzzy parameter: its value once
				// evaluated; logging must not evaluate it.
				v = runtime.Undefined
				if t.Once && !t.Fuzzy {
					v = t.Cached
				}
			}
			ps = append(ps, tl.Param{Name: fp.Name.String(), Mode: mode, Val: runtime.TLValue(v)})
		}
		args = append(args, tlArg("tciPars", tl.Params(ps...)))
	}
	if op == "tliSLeave" {
		if rv, ok := ret.(*runtime.ReturnValue); ok && !rv.Stopped && rv.Value != nil && rv.Value != runtime.Undefined {
			args = append(args, tlArg("returnValue", runtime.TLValue(rv.Value).AsValue()))
		}
	}
	args = append(args, tlArg("kind", tl.String(kind)))
	tlEmit(exec, n, op, args...)
}

// tlCompStatus is a component's state as TCI-TL's ComponentStatusType
// names it, consistent with the answer the operation op gave: running
// (tliCRunning) or alive (tliCAlive).
func tlCompStatus(ref *runtime.ComponentRef, env runtime.Scope, op string, answer bool) string {
	switch {
	case ref == nil:
		return tl.ComponentNull
	case op == "tliCRunning" && answer:
		return tl.ComponentRunning
	case op == "tliCAlive" && !answer:
		return tl.ComponentKilled
	case op == "tliCRunning" && compKilled(ref, env):
		return tl.ComponentKilled
	case op == "tliCAlive" && compRunning(ref, env):
		return tl.ComponentRunning
	case ref.Started:
		return tl.ComponentStopped
	}
	return tl.ComponentInactive
}

// tlCompQuery logs a running or alive operation on ref (tliCRunning,
// tliCAlive) with the component's status, given the answer it gave.
func tlCompQuery(env runtime.Scope, n syntax.Node, op string, ref *runtime.ComponentRef, answer bool) {
	exec := tlExec(env)
	if exec == nil || ref == nil {
		return
	}
	tlEmitLazy(exec, n, op, func() []tl.Arg {
		return []tl.Arg{tlArg("comp", exec.TLComponent(ref).Content()), tlArg("status", tl.String(tlCompStatus(ref, env, op, answer)))}
	})
}

// tlDoneKilledMismatch logs a done or killed guard that did not match
// (tliCDoneMismatch, tliCKilledMismatch), once per alt round.
func tlDoneKilledMismatch(env runtime.Scope, n syntax.Node, op string, ref *runtime.ComponentRef) {
	exec := tlExec(env)
	if exec == nil || ref == nil {
		return
	}
	if !exec.TLMismatchIsNew(fmt.Sprintf("%d\x00%p", currentCompID(exec), n), exec.TLAltEpoch(currentCompID(exec))) {
		return
	}
	tlOp := "tliCDoneMismatch"
	if op == "killed" {
		tlOp = "tliCKilledMismatch"
	}
	tlEmitLazy(exec, n, tlOp, func() []tl.Arg {
		return []tl.Arg{tlArg("comp", exec.TLComponent(ref).Content()), tlArg("compTmpl", tlCompTemplate(exec, "", ref))}
	})
}

// tlPortStatus is the state of the current component's port as TCI-TL's
// PortStatusType names it; a port no operation has stopped or halted is
// started.
func tlPortStatus(exec *runtime.TestcaseExec, port string) string {
	s, _ := exec.PortLifecycle(runtime.PortEndpoint{Comp: currentComponentID(exec), Port: port})
	switch s {
	case "stopped":
		return tl.PortStopped
	case "halted":
		return tl.PortHalted
	}
	return tl.PortStarted
}

// tlEncode logs an encvalue (tliEncode): the value and what it was
// encoded to. The executor's encoder does not fail, and which codec it
// used is not known to it, so neither is given.
func tlEncode(exec *runtime.TestcaseExec, n syntax.Node, val, res runtime.Object) {
	defer tlRecover(exec)
	msg, ok := runtime.TLEncoded(res)
	if !ok {
		return
	}
	tlEmit(exec, n, "tliEncode", tlArg("val", runtime.TLValue(val).AsValue()), tlArg("msg", msg))
}

// tlDecode logs a decvalue (tliDecode): the encoded value it was given
// and either the decoded value — the variable it was decoded into, as it
// is after the call — or that it failed to decode. Annex B has one or the
// other.
func tlDecode(exec *runtime.TestcaseExec, n *syntax.CallExpr, enc, res runtime.Object, env runtime.Scope) {
	defer tlRecover(exec)
	msg, ok := runtime.TLEncoded(enc)
	if !ok {
		return
	}
	if r, ok := res.(runtime.Int); !ok || r.Int.Sign() != 0 {
		tlEmit(exec, n, "tliDecode", tlArg("msg", msg), tlArg("decoder-failure", tl.String(tl.TciError)))
		return
	}
	var val runtime.Object = runtime.Undefined
	if id, ok := n.Args.List[1].(*syntax.Ident); ok {
		if v, ok := env.Get(id.String()); ok {
			val = v
		}
	}
	tlEmit(exec, n, "tliDecode", tlArg("msg", msg), tlArg("val", runtime.TLValue(val).AsValue()))
}

// tlRnd logs an rnd (tliRnd): the number drawn and the seed it was given,
// null when it was given none.
func tlRnd(exec *runtime.TestcaseExec, n syntax.Node, args []runtime.Object, res runtime.Object) {
	defer tlRecover(exec)
	f, ok := res.(runtime.Float)
	if !ok {
		return
	}
	from := tl.Value{Kind: "float", Null: true}
	if len(args) > 0 {
		if seed, ok := args[0].(runtime.Float); ok {
			from = runtime.TLValue(seed)
		}
	}
	tlEmit(exec, n, "tliRnd", tlArg("val", runtime.TLValue(f).AsTyped()), tlArg("from", from.AsTyped()))
}

// tlMatch logs a match (tliMatch, tliMatchMismatch) with the value, the
// template and, for a mismatch, where they differ.
func tlMatch(exec *runtime.TestcaseExec, n syntax.Node, val, tmpl, res runtime.Object) {
	b, ok := res.(runtime.Bool)
	if !ok {
		return
	}
	defer tlRecover(exec)
	expr := tlArg("expr", runtime.TLValue(val).AsValue())
	t := tlArg("tmpl", runtime.TLValue(tmpl).AsTemplate())
	if bool(b) {
		tlEmit(exec, n, "tliMatch", expr, t)
		return
	}
	tlEmit(exec, n, "tliMatchMismatch", expr, t, tlArg("diffs", tl.Diffs(tlDiffs(val, tmpl)...)))
}

// tlVar logs an assignment (tliVar): the variable assigned, and its whole
// value after the assignment — for `v.f[1] := x`, the new value of v.
func tlVar(exec *runtime.TestcaseExec, lhs syntax.Expr, env runtime.Scope) {
	defer tlRecover(exec)
	root := lhs
	for {
		switch x := root.(type) {
		case *syntax.IndexExpr:
			root = x.X
			continue
		case *syntax.SelectorExpr:
			root = x.X
			continue
		}
		break
	}
	id, ok := root.(*syntax.Ident)
	if !ok {
		return
	}
	name := id.String()
	v, ok := env.Get(name)
	if !ok {
		return
	}
	module := moduleNameFromEnv(env)
	if e, ok := env.(*runtime.Env); ok {
		if owner := e.Owner(name); owner != nil {
			module = moduleNameFromEnv(owner)
		}
	}
	tlEmit(exec, lhs, "tliVar", tlArg("name", tl.QualifiedName(module, name)), tlArg("val", runtime.TLValue(v).AsValue()))
}

// tlModulePar logs a read of a module parameter (tliModulePar) with its
// value: a read of a name whose binding is marked as one, with the module
// that declares it (see annotateDef).
func tlModulePar(env runtime.Scope, n syntax.Node, name string, val runtime.Object) {
	exec := tlExec(env)
	if exec == nil {
		return
	}
	defer tlRecover(exec)
	e, ok := env.(*runtime.Env)
	if !ok {
		return
	}
	owner := e.Owner(name)
	if owner == nil {
		return
	}
	mark, _ := owner.Get(moduleParKey(name))
	module, ok := mark.(*runtime.String)
	if !ok || !owner.Binds(moduleParKey(name)) {
		return
	}
	tlEmit(exec, n, "tliModulePar", tlArg("name", tl.QualifiedName(string(module.Value), name)), tlArg("val", runtime.TLValue(val).AsValue()))
}

// tlEvaluate logs the evaluation of a @lazy or @fuzzy parameter
// (tliEvaluate): a @lazy one's first read, each read of a @fuzzy one.
func tlEvaluate(exec *runtime.TestcaseExec, n syntax.Node, t *runtime.LazyThunk, val runtime.Object) {
	defer tlRecover(exec)
	tlEmit(exec, n, "tliEvaluate", tlArg("name", tl.QualifiedName(t.Module, t.Name)),
		tlArg("evalResult", runtime.TLValue(val).AsValue()))
}

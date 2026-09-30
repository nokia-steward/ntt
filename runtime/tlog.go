package runtime

import (
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nokia/ntt/runtime/tl"
)

// Test logging (TCI-TL, ETSI ES 201 873-6 clause 7.3.4.1). The executor
// reports each TTCN-3 operation it performs to the logger attached here;
// with none attached, every call below returns at the nil check.

// tlEverActive is set once any testcase in the process has had a logger.
// The hottest interpreter paths — reading an identifier, assigning — test
// it before looking for their testcase, so logging costs them one atomic
// load when it is off.
var tlEverActive atomic.Bool

// TLActive reports whether test logging may be on for some testcase.
func TLActive() bool { return tlEverActive.Load() }

// SetTestLogger attaches l to the testcase. Call before the testcase runs,
// before any of its components start: the logger is read without a lock.
func (t *TestcaseExec) SetTestLogger(l tl.Logger) {
	if l != nil {
		tlEverActive.Store(true)
	}
	t.tlog = l
	t.tlStart = time.Now()
	// Start no earlier than the log's latest event (see tl.Monotonic).
	if c, ok := l.(interface{ Now() int64 }); ok {
		t.tlStart = time.UnixMicro(c.Now())
	}
}

// TestLogger returns the attached logger, or nil.
func (t *TestcaseExec) TestLogger() tl.Logger {
	if t == nil {
		return nil
	}
	return t.tlog
}

// SetTLTestcase records the testcase identity (tcId and tciPars) that its
// start and termination events carry.
func (t *TestcaseExec) SetTLTestcase(args ...tl.Arg) { t.tlTc = args }

// TLTestcase returns what SetTLTestcase recorded, or nil.
func (t *TestcaseExec) TLTestcase() []tl.Arg { return t.tlTc }

// TLMismatchIsNew reports whether a mismatch identified by key — the
// component, port and receiving operation — has not yet been logged
// against the message numbered seq, and records it. An alt that keeps
// re-checking its guards against an unchanged queue head logs each
// mismatch once; a different message at the head logs again.
func (t *TestcaseExec) TLMismatchIsNew(key string, seq uint64) bool {
	t.tlMu.Lock()
	defer t.tlMu.Unlock()
	if t.tlSeen == nil {
		t.tlSeen = map[string]uint64{}
	}
	if prev, ok := t.tlSeen[key]; ok && prev == seq {
		return false
	}
	t.tlSeen[key] = seq
	return true
}

// TLAltEpoch numbers component comp's current alt round: one that begins
// afresh, on entering an alt or after one matched. A guard that does not
// match — a done, killed or timeout — is logged once per round, keyed on
// it (see TLMismatchIsNew), where a waiting alt re-checks its guards many
// times within one round.
func (t *TestcaseExec) TLAltEpoch(comp int64) uint64 {
	t.tlMu.Lock()
	defer t.tlMu.Unlock()
	return t.tlAlt[comp]
}

// TLAltBump begins a new alt round for component comp.
func (t *TestcaseExec) TLAltBump(comp int64) {
	t.tlMu.Lock()
	defer t.tlMu.Unlock()
	if t.tlAlt == nil {
		t.tlAlt = map[int64]uint64{}
	}
	t.tlAlt[comp]++
	if s := t.tlScans[comp]; s != nil {
		s.seen = nil
	}
}

// An alt's guard scan: the evaluation of its guards and of the defaults
// after them. An alt that finds nothing waits and scans again, on the real
// clock every few milliseconds, and each scan performs the operations of
// the last one again — the same module parameter read, the same default
// altstep entered. The log records such an operation the first time in an
// alt round (see TLAltEpoch); a scan after waiting logs what is new: a
// receive that now matches, a component whose state has changed.
type tlScan struct {
	active, again bool
	seen          map[string]bool
}

// TLScanBegin begins a guard scan of component comp's alt; again is set
// for a scan after waiting.
func (t *TestcaseExec) TLScanBegin(comp int64, again bool) {
	t.tlMu.Lock()
	defer t.tlMu.Unlock()
	if t.tlScans == nil {
		t.tlScans = map[int64]*tlScan{}
	}
	s := t.tlScans[comp]
	if s == nil {
		s = &tlScan{}
		t.tlScans[comp] = s
	}
	s.active, s.again = true, again
}

// TLScanEnd ends component comp's guard scan: an alternative matched and
// its body runs, or the alt waits.
func (t *TestcaseExec) TLScanEnd(comp int64) {
	t.tlMu.Lock()
	defer t.tlMu.Unlock()
	if s := t.tlScans[comp]; s != nil {
		s.active = false
	}
}

// TLScanRepeats reports whether e, logged by component comp during a guard
// scan after waiting, repeats an operation logged in the same alt round,
// and records it.
func (t *TestcaseExec) TLScanRepeats(comp int64, e *tl.Event) bool {
	t.tlMu.Lock()
	s := t.tlScans[comp]
	active := s != nil && s.active
	t.tlMu.Unlock()
	if !active {
		return false
	}
	key := e.Key()
	t.tlMu.Lock()
	defer t.tlMu.Unlock()
	if s.seen == nil {
		s.seen = map[string]bool{}
	}
	if s.seen[key] {
		return s.again
	}
	s.seen[key] = true
	return false
}

// TLTime is the timestamp of an event logged now, in microseconds since
// the Unix epoch: the wall clock, or under the virtual clock the
// testcase's start plus the virtual time elapsed, so a log reflects the
// clock the testcase ran on.
func (t *TestcaseExec) TLTime() int64 {
	t.mu.Lock()
	start, det := t.tlStart, t.deterministicClock
	t.mu.Unlock()
	if det || t.SchedulerActive() {
		return start.UnixMicro() + int64(t.VirtualClock()*1e6)
	}
	return time.Now().UnixMicro()
}

// TLog logs one event produced by the current component. src and line
// locate the operation in the test specification when known.
func (t *TestcaseExec) TLog(op, src string, line int, args ...tl.Arg) {
	l := t.TestLogger()
	if l == nil {
		return
	}
	t.TLogFrom(l, t.TLCurrent(), op, src, line, args...)
}

// TLogFrom logs one event produced by component c.
//
// Once the testcase's end has been logged, a component still acting — one
// whose behaviour outlived its testcase — must not appear to act in
// whatever testcase the log is in by then. Its event is logged as a
// tliInfo naming the operation, the component and the testcase it belongs
// to.
func (t *TestcaseExec) TLogFrom(l tl.Logger, c tl.ComponentID, op, src string, line int, args ...tl.Arg) {
	e := &tl.Event{Op: op, Ts: t.TLTime(), Src: src, Line: line, C: c, Args: args}
	// Held across the check and the logging, so no event checked before
	// the end is logged after it (see TLEnd).
	t.tlEndMu.RLock()
	defer t.tlEndMu.RUnlock()
	if t.tlEnded.Load() && op != "tliTcTerminated" {
		e = &tl.Event{Op: "tliInfo", Ts: e.Ts, Src: src, Line: line,
			C: tl.ComponentID{Name: c.Name, ID: tl.LateID, Type: c.Type},
			Args: []tl.Arg{
				{Name: "level", Val: tl.Integer(1)},
				{Name: "info", Val: tl.String(fmt.Sprintf("after testcase %s ended: %s", t.Name, e.Summary()))},
			}}
	}
	l.Log(e)
}

// TLEnd records that the testcase's end has been logged (see TLogFrom).
// Events being logged when it is called are logged first.
func (t *TestcaseExec) TLEnd() {
	t.tlEndMu.Lock()
	defer t.tlEndMu.Unlock()
	t.tlEnded.Store(true)
}

// TLCurrent identifies the component running on the calling goroutine.
func (t *TestcaseExec) TLCurrent() tl.ComponentID {
	if cur := t.CurrentComponent(); cur != nil {
		return t.TLComponent(cur)
	}
	return t.TLComponentByID(t.MTCID())
}

// TLComponent identifies ref: its name, its id, and its component type.
// The MTC is named "mtc", an unnamed PTC by its id.
func (t *TestcaseExec) TLComponent(ref *ComponentRef) tl.ComponentID {
	if ref == nil {
		return tl.ComponentID{Null: true}
	}
	id := strconv.FormatInt(ref.ID, 10)
	name := ref.Name
	if ref.ID == t.MTCID() {
		name = "mtc"
	} else if name == "" {
		name = id
	}
	return tl.ComponentID{Name: name, ID: id, Type: ref.TypeName}
}

// TLComponentByID identifies a component by its id, including the
// endpoint ids the connection graph uses for the MTC (0) and the system
// (-2).
func (t *TestcaseExec) TLComponentByID(id int64) tl.ComponentID {
	switch id {
	case -2:
		return tl.ComponentID{Name: "system", ID: "system"}
	case 0:
		id = t.MTCID()
	}
	for _, r := range t.AllComponents() {
		if r != nil && r.ID == id {
			return t.TLComponent(r)
		}
	}
	return tl.ComponentID{Name: strconv.FormatInt(id, 10), ID: strconv.FormatInt(id, 10)}
}

// TLPort identifies the port instance name of component compID. A port
// array element keeps its index.
func (t *TestcaseExec) TLPort(compID int64, name string) tl.PortID {
	name = barePortName(name)
	index := -1
	if i := strings.IndexByte(name, '['); i > 0 && strings.HasSuffix(name, "]") {
		if n, err := strconv.Atoi(name[i+1 : len(name)-1]); err == nil {
			index = n
			name = name[:i]
		}
	}
	return tl.PortID{Comp: t.TLComponentByID(compID), Name: name, Index: index}
}

// portOwner returns the component id a port queue key belongs to: the id
// a qualified key carries, or the MTC for a bare name.
func (t *TestcaseExec) portOwner(key string) int64 {
	if strings.HasPrefix(key, portQualPrefix) {
		rest := key[len(portQualPrefix):]
		if i := strings.IndexByte(rest, '/'); i > 0 {
			if id, err := strconv.ParseInt(rest[:i], 10, 64); err == nil {
				return id
			}
		}
	}
	return t.MTCID()
}

// TLMapped reports whether a component's port is mapped to the system,
// which makes its message operations the _m variants of TCI-TL rather
// than the _c ones.
func (t *TestcaseExec) TLMapped(compID int64, port string) bool {
	port = barePortName(port)
	if t.IsMapped(PortEndpoint{Comp: compID, Port: port}) {
		return true
	}
	if compID == t.MTCID() && t.IsMapped(PortEndpoint{Comp: 0, Port: port}) {
		return true
	}
	// Only a driver already bound: looking one up may create it.
	return t.boundPortDriver(t.PortKeyFor(compID, port))
}

// boundPortDriver reports whether a port driver is bound to instance,
// without the lookup PortDriver does, which can create one.
func (t *TestcaseExec) boundPortDriver(instance string) bool {
	t.driverMu.Lock()
	defer t.driverMu.Unlock()
	d, ok := t.boundPorts[instance]
	return ok && d != nil
}

// TLPortForKey identifies the port a queue key belongs to: its owner and
// its name.
func (t *TestcaseExec) TLPortForKey(key string) tl.PortID {
	return t.TLPort(t.portOwner(key), key)
}

// TLParams is a procedure's parameter record as a TCI parameter list: one
// parameter per field, in field order. The runtime does not keep the
// parameters' passing modes, so none is given.
func TLParams(rec Object) tl.Content {
	v := TLValue(rec)
	if v.Kind != "record" && v.Kind != "set" {
		return tl.Params()
	}
	ps := make([]tl.Param, 0, len(v.Elems))
	for _, e := range v.Elems {
		name := e.Name
		e.Name = ""
		ps = append(ps, tl.Param{Name: name, Val: e})
	}
	return tl.Params(ps...)
}

// tlDetected logs the arrival of a message, call, reply or exception in a
// port queue (tliMDetected, tliPrGetCallDetected, tliPrGetReplyDetected,
// tliPrCatchDetected, each _c or _m), on behalf of the receiving
// component.
func (t *TestcaseExec) tlDetected(key string, msg PortMessage) {
	l := t.TestLogger()
	if l == nil {
		return
	}
	// A failure of the logging must not change the test: record it in
	// the log instead (see the interpreter's tlRecover).
	defer func() {
		if r := recover(); r != nil {
			defer func() { _ = recover() }()
			t.TLog("tliInfo", "", 0,
				tl.Arg{Name: "level", Val: tl.Integer(0)},
				tl.Arg{Name: "info", Val: tl.String(fmt.Sprintf("test logging failed: %v", r))})
		}
	}()
	if msg.Kind != MsgMessage {
		t.tlProcDetected(l, key, msg)
		return
	}
	owner := t.portOwner(key)
	at := t.TLPort(owner, key)
	recv := at.Comp
	if t.TLMapped(owner, key) {
		// Annex B gives tliMDetected_m the encoded message (msgValue,
		// Types:TriMessageType), not the decoded value, which arrives with
		// the receive. The executor holds decoded values, so the octets
		// are known only when the payload is itself an octetstring.
		from := tl.PortID{Comp: t.TLComponentByID(-2), Name: at.Name, Index: -1}
		octets := ""
		if b, ok := msg.Payload.(*Binarystring); ok && b.Unit == Octet {
			octets = TLValue(b).Text
		}
		t.TLogFrom(l, recv, "tliMDetected_m", "", 0,
			tl.Arg{Name: "at", Val: at.Content()},
			tl.Arg{Name: "from", Val: from.Content()},
			tl.Arg{Name: "msgValue", Val: tl.Message(octets)})
		return
	}
	from := at
	if s, ok := msg.Sender.(*ComponentRef); ok && s != nil {
		from = tl.PortID{Comp: t.TLComponent(s), Name: at.Name, Index: -1}
		for _, p := range t.ConnectedPeers(PortEndpoint{Comp: t.connID(owner), Port: at.Name}) {
			if p.Comp == s.ID || (p.Comp == 0 && s.ID == t.MTCID()) {
				from = t.TLPort(s.ID, p.Port)
				break
			}
		}
	}
	t.TLogFrom(l, recv, "tliMDetected_c", "", 0,
		tl.Arg{Name: "at", Val: at.Content()},
		tl.Arg{Name: "from", Val: from.Content()},
		tl.Arg{Name: "msgValue", Val: TLValue(msg.Payload).AsValue()})
}

// connID is the id the connection graph records for a component: the MTC
// is 0 there.
func (t *TestcaseExec) connID(compID int64) int64 {
	if compID == t.MTCID() {
		return 0
	}
	return compID
}

// TLValue converts a runtime value or template to its TCI-TL form. The
// runtime does not always know a value's declared type, so the element is
// inferred from the value: a string of ASCII characters is a charstring,
// any other a universal_charstring; a value with field names a record, an
// ordered list a record_of and an unordered one a set_of. Record fields
// held in a map are written in name order, the runtime having no other.
// A value of no recognisable kind is written as a charstring holding its
// TTCN-3 text.
func TLValue(o Object) tl.Value {
	switch v := o.(type) {
	case nil:
		return tl.Value{Kind: "charstring", Null: true}
	case Int:
		return tl.Value{Kind: "integer", Text: v.Int.String()}
	case Float:
		return tl.Value{Kind: "float", Text: ttcnFloat(float64(v))}
	case Bool:
		return tl.Value{Kind: "boolean", Text: strconv.FormatBool(bool(v))}
	case Verdict:
		return tl.Value{Kind: "verdicttype", Text: string(v)}
	case *String:
		kind := "charstring"
		for _, r := range v.Value {
			if r > 127 {
				kind = "universal_charstring"
				break
			}
		}
		if v.IsPattern {
			return tl.Value{Kind: kind, Match: &tl.Matching{Symbol: "pattern", Pattern: string(v.Value)}}
		}
		return tl.Value{Kind: kind, Text: string(v.Value)}
	case *Binarystring:
		return tlBinary(v)
	case *EnumValue:
		return tl.Value{Kind: "enumerated", Text: v.Key()}
	case *Record:
		names := make([]string, 0, len(v.Fields))
		for n := range v.Fields {
			names = append(names, n)
		}
		sort.Strings(names)
		out := tl.Value{Kind: "record"}
		for _, n := range names {
			f := TLValue(v.Fields[n])
			f.Name = n
			out.Elems = append(out.Elems, f)
		}
		return out
	case *List:
		if len(v.FieldNames) == len(v.Elements) && len(v.FieldNames) > 0 {
			out := tl.Value{Kind: "record"}
			for i, e := range v.Elements {
				f := TLValue(e)
				f.Name = v.FieldNames[i]
				out.Elems = append(out.Elems, f)
			}
			return out
		}
		out := tl.Value{Kind: "record_of"}
		if !v.IsOrdered() {
			out.Kind = "set_of"
		}
		for _, e := range v.Elements {
			out.Elems = append(out.Elems, TLValue(e))
		}
		return out
	case *Map:
		// TCI-TL has no map value; a map is its key/value pairs, in key
		// order, as a record of records.
		pairs := v.Pairs()
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].Key.Inspect() < pairs[j].Key.Inspect() })
		out := tl.Value{Kind: "record_of", Type: "map"}
		for _, p := range pairs {
			k, val := TLValue(p.Key), TLValue(p.Value)
			k.Name, val.Name = "key", "value"
			out.Elems = append(out.Elems, tl.Value{Kind: "record", Elems: []tl.Value{k, val}})
		}
		return out
	case *ComponentRef:
		if v == nil {
			return tl.Value{Kind: "component", Null: true}
		}
		return tl.Value{Kind: "component", Text: strconv.FormatInt(v.ID, 10), Type: v.TypeName}
	case *TimerHandle:
		return tl.Value{Kind: "timer", Text: v.Name}
	case *Range:
		m := &tl.Matching{Symbol: "range", ExclLower: v.LowerExcl, ExclUpper: v.UpperExcl}
		kind := "integer"
		if v.Lower != nil {
			lo := TLValue(v.Lower)
			kind = lo.Kind
			m.Lower = &lo
		}
		if v.Upper != nil {
			hi := TLValue(v.Upper)
			kind = hi.Kind
			m.Upper = &hi
		}
		return tl.Value{Kind: kind, Match: m}
	case *IfPresent:
		inner := TLValue(v.Inner)
		inner.IfPresent = true
		return inner
	case *LengthRestricted:
		inner := TLValue(v.Inner)
		if inner.Kind == "" {
			// A bare `? length(n)` has no type of its own; the length
			// attribute needs a scalar or list element to sit on.
			inner.Kind = "charstring"
		}
		l := &tl.Length{Lower: v.Min}
		if v.Max >= 0 {
			max := v.Max
			l.Upper = &max
		}
		inner.Length = l
		return inner
	}
	switch o {
	case Any:
		return tl.Value{Match: &tl.Matching{Symbol: "any_value"}}
	case AnyOrNone:
		return tl.Value{Match: &tl.Matching{Symbol: "any_value_or_none"}}
	case Omit:
		return tl.Value{Omit: true}
	case Undefined:
		// Unbound: the schema's "null", used "if no value is given".
		return tl.Value{Kind: "charstring", Null: true}
	}
	return tl.Value{Kind: "charstring", Text: o.Inspect()}
}

// TLEncoded is an encoded value — what encvalue returns, or decvalue is
// given — as a Types:TriMessageType: a bitstring's bits left-aligned in
// octets, a hexstring's digits two to an octet, a character string's
// UTF-8 octets. Ok is false for anything else.
func TLEncoded(o Object) (tl.Content, bool) {
	switch v := o.(type) {
	case *Binarystring:
		if v.Value == nil || v.Value.Sign() < 0 || v.Length < 0 {
			return tl.Content{}, false
		}
		bits := v.Length * int(v.Unit)
		octets := (bits + 7) / 8
		if octets == 0 {
			return tl.EncodedMessage("", 0), true
		}
		pad := octets*8 - bits
		aligned := new(big.Int).Lsh(v.Value, uint(pad))
		return tl.EncodedMessage(padLeft(strings.ToUpper(aligned.Text(16)), octets*2), pad), true
	case *String:
		return tl.EncodedMessage(strings.ToUpper(hex.EncodeToString([]byte(string(v.Value)))), 0), true
	}
	return tl.Content{}, false
}

// ttcnFloat is f in TTCN-3 notation, the special values included.
func ttcnFloat(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "infinity"
	case math.IsInf(f, -1):
		return "-infinity"
	case math.IsNaN(f):
		return "not_a_number"
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

func tlBinary(b *Binarystring) tl.Value {
	kind := map[Unit]string{Bit: "bitstring", Octet: "octetstring"}[b.Unit]
	if kind == "" {
		kind = "hexstring"
	}
	if b.Value == nil || b.Value.Sign() < 0 {
		// A template literal: its source digits, wildcards included, in
		// the form a value's digits take — no layout, upper case.
		s := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(b.String, "B"), "O"), "H")
		s = strings.Join(strings.Fields(strings.Trim(s, "'")), "")
		return tl.Value{Kind: kind, Text: strings.ToUpper(s)}
	}
	var s string
	switch b.Unit {
	case Bit:
		s = padLeft(b.Value.Text(2), b.Length)
	case Octet:
		s = padLeft(strings.ToUpper(b.Value.Text(16)), b.Length*2)
	default:
		s = padLeft(strings.ToUpper(b.Value.Text(16)), b.Length)
	}
	return tl.Value{Kind: kind, Text: s}
}

func padLeft(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return strings.Repeat("0", n-len(s)) + s
}

// tlProcDetected logs the arrival of a procedure envelope.
func (t *TestcaseExec) tlProcDetected(l tl.Logger, key string, msg PortMessage) {
	owner := t.portOwner(key)
	at := t.TLPort(owner, key)
	mapped := t.TLMapped(owner, key)
	op := map[PortMsgKind]string{
		MsgCall:      "tliPrGetCallDetected",
		MsgReply:     "tliPrGetReplyDetected",
		MsgException: "tliPrCatchDetected",
	}[msg.Kind]
	if op == "" {
		return
	}
	args := []tl.Arg{{Name: "at", Val: at.Content()}}
	if mapped {
		op += "_m"
		args = append(args,
			tl.Arg{Name: "from", Val: tl.PortID{Comp: t.TLComponentByID(-2), Name: at.Name, Index: -1}.Content()},
			tl.Arg{Name: "signature", Val: tl.Signature(msg.Signature)})
		// The _m forms carry encoded parameters (TriParameterList), which
		// the executor does not hold; the decoded ones come with the
		// receive.
		t.TLogFrom(l, at.Comp, op, "", 0, args...)
		return
	}
	op += "_c"
	if s, ok := msg.Sender.(*ComponentRef); ok && s != nil {
		from := tl.PortID{Comp: t.TLComponent(s), Name: at.Name, Index: -1}
		for _, p := range t.ConnectedPeers(PortEndpoint{Comp: t.connID(owner), Port: at.Name}) {
			if p.Comp == s.ID || (p.Comp == 0 && s.ID == t.MTCID()) {
				from = t.TLPort(s.ID, p.Port)
				break
			}
		}
		args = append(args, tl.Arg{Name: "from", Val: from.Content()})
	}
	args = append(args, tl.Arg{Name: "signature", Val: tl.Signature(msg.Signature)})
	switch msg.Kind {
	case MsgCall:
		args = append(args, tl.Arg{Name: "tciPars", Val: TLParams(msg.Payload)})
	case MsgReply:
		args = append(args, tl.Arg{Name: "tciPars", Val: TLParams(msg.Payload)})
		if msg.RetValue != nil {
			args = append(args, tl.Arg{Name: "replValue", Val: TLValue(msg.RetValue).AsValue()})
		}
	case MsgException:
		if msg.RetValue != nil {
			args = append(args, tl.Arg{Name: "excValue", Val: TLValue(msg.RetValue).AsValue()})
		}
	}
	t.TLogFrom(l, at.Comp, op, "", 0, args...)
}

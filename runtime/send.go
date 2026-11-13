package runtime

import "strings"

// The endpoint ids of the components that are not a PTC.
const (
	MTCEndpointID    int64 = 0  // the MTC, the testcase's main thread
	SystemEndpointID int64 = -2 // the system, the far side of a map
)

// SystemComponent is the reference of the system component. It is the
// sender of what a mapped port receives when no port driver stands for
// the system.
var SystemComponent = &ComponentRef{ID: SystemEndpointID, Name: "system"}

// CurrentComponentID is the endpoint id of the running component, the
// MTC's outside a PTC.
func (t *TestcaseExec) CurrentComponentID() int64 {
	if t != nil {
		if c := t.CurrentComponent(); c != nil {
			return c.ID
		}
	}
	return MTCEndpointID
}

// Ended reports whether r is a component that is no more: killed, or one
// not alive-type whose behaviour is done (ETSI ES 201 873-1 21.3.3,
// 21.3.4). Its connections are released. The MTC and the system never
// end this way.
func (r *ComponentRef) Ended() bool {
	if r == nil || r.ID == MTCEndpointID || r.ID == SystemEndpointID {
		return false
	}
	return !r.IsAlive() || r.IsDone() && !r.AliveModifier
}

// PortPeers returns the endpoints port of component comp is connected to.
// An element of a port array is connected as itself; one whose array was
// connected as a whole reaches the same element of the peer's array.
func (t *TestcaseExec) PortPeers(comp int64, port string) []PortEndpoint {
	if peers := t.ConnectedPeers(PortEndpoint{Comp: comp, Port: port}); len(peers) > 0 {
		return peers
	}
	i := strings.IndexByte(port, '[')
	if i < 0 {
		return nil
	}
	base, suffix := port[:i], port[i:]
	peers := t.ConnectedPeers(PortEndpoint{Comp: comp, Port: base})
	for i := range peers {
		peers[i].Port += suffix
	}
	return peers
}

// LivePeers returns what endpoint e is connected to (mapped false) or
// mapped to (mapped true), but the ports of components that have ended.
func (t *TestcaseExec) LivePeers(e PortEndpoint, mapped bool) []PortEndpoint {
	peers := t.ConnectedPeers(e)
	if mapped {
		peers = t.MappedPeers(e)
	}
	ended := map[int64]bool{}
	for _, ref := range t.AllComponents() {
		if ref.Ended() {
			ended[ref.ID] = true
		}
	}
	if len(ended) == 0 {
		return peers
	}
	out := peers[:0:0]
	for _, p := range peers {
		if !ended[p.Comp] {
			out = append(out, p)
		}
	}
	return out
}

// PeerKeys returns the queue keys of the ports port of the running
// component is connected to, where a send, call, reply or raise on it is
// delivered. It is nil for a port connected to none, and when no
// component runs: what is sent there goes on the port's own queue. port
// is the instance name the connect graph knows, not qualified by the
// component.
func (t *TestcaseExec) PeerKeys(port string) []string {
	cur := t.CurrentComponent()
	if cur == nil {
		return nil
	}
	peers := t.PortPeers(cur.ID, port)
	if len(peers) == 0 {
		return nil
	}
	keys := make([]string, 0, len(peers))
	for _, peer := range peers {
		keys = append(keys, t.PortKeyFor(peer.Comp, peer.Port))
	}
	return keys
}

// AddressedPeerKeys returns the queue keys of the ports port of the
// running component is connected to that belong to the components ids
// names, those a sending operation with a to clause addresses.
func (t *TestcaseExec) AddressedPeerKeys(port string, ids map[int64]bool) []string {
	cur := int64(-1)
	if c := t.CurrentComponent(); c != nil {
		cur = c.ID
	}
	var keys []string
	for _, peer := range t.PortPeers(cur, port) {
		if ids[peer.Comp] {
			keys = append(keys, t.PortKeyFor(peer.Comp, peer.Port))
		}
	}
	return keys
}

// Route returns the queue keys what the running component sends on port
// is delivered to, routing and delivery being apart so that a delivery
// is logged before it happens. ids is the set of components a to clause
// addresses, nil when there is none or it is not known. The keys are
// those of the addressed components' ports port is connected to, if any,
// with addressed set; else of all the ports it is connected to; else of
// port's own queue.
func (t *TestcaseExec) Route(port string, ids map[int64]bool) (keys []string, addressed bool) {
	if ids != nil {
		if keys := t.AddressedPeerKeys(port, ids); len(keys) > 0 {
			return keys, true
		}
	}
	if keys := t.PeerKeys(port); keys != nil {
		return keys, false
	}
	return []string{t.PortKey(port)}, false
}

// DeliverEnvelope enqueues msg on each of keys.
func (t *TestcaseExec) DeliverEnvelope(keys []string, msg PortMessage) {
	for _, key := range keys {
		t.EnqueueEnvelope(key, msg)
	}
}

// TargetComponentIDs is the set of components the value of a to clause
// addresses: one component, or a list of them. It is nil when it
// addresses none.
func TargetComponentIDs(to Object) map[int64]bool {
	var refs []Object
	switch v := to.(type) {
	case *ComponentRef:
		refs = []Object{v}
	case *List:
		refs = v.Elements
	default:
		return nil
	}
	ids := map[int64]bool{}
	for _, r := range refs {
		if c, ok := r.(*ComponentRef); ok && c != nil {
			ids[c.ID] = true
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}

// SendError is the error of the sending operation op on port, which is
// stopped or halted (ETSI ES 201 873-1 22.5.3, 22.5.4), or nil.
func (t *TestcaseExec) SendError(op, port string) Object {
	switch t.PortStateOf(t.PortKey(port)) {
	case PortStopped:
		return Errorf("%s on port %s, which is stopped (ETSI 22.5.3)", op, port)
	case PortHalted:
		return Errorf("%s on port %s, which is halted (ETSI 22.5.4)", op, port)
	}
	return nil
}

// ReceiverError is the error of the sending operation op on port of the
// running component, without a to clause unless hasTo: a port connected
// to several others is sent on with one (ETSI ES 201 873-1 22.2.1 d,
// 22.3.1 e, 22.3.3 d, 22.3.5 c). A port neither connected nor mapped is
// let be: the MTC's ports are the test system interface of a testcase
// without a system clause (16.3), whose loopback stands in for the SUT.
func (t *TestcaseExec) ReceiverError(op, port string, hasTo bool) Object {
	ep := PortEndpoint{Comp: t.CurrentComponentID(), Port: port}
	if peers := t.LivePeers(ep, false); !hasTo && len(peers) > 1 {
		return Errorf("%s on port %s, connected to %d ports, without a to clause (ETSI 22.2.1 d)", op, port, len(peers))
	}
	return nil
}

// SendMessage is the send operation of the running component on port
// (ETSI ES 201 873-1 22.2.1), its operands evaluated: payload, and to,
// the value of its to clause, or nil when it has none or its value is
// not known; hasTo says it has one. It returns the operation's error, or
// nil. The caller checks SendError first and logs the send.
//
// A port bound to a driver hands the message to the driver. A connected
// port delivers it to the ports it is connected to, only to those of the
// components a to clause addresses, if it addresses any of them. Any
// other port takes it on its own queue: a mapped port's from the system,
// as the SUT would answer it, the others' from the running component.
func (t *TestcaseExec) SendMessage(port string, payload, to Object, hasTo bool) Object {
	key := t.PortKey(port)
	sender := to
	if sender == nil {
		if cur := t.CurrentComponent(); cur != nil {
			sender = cur
		}
	}
	// Profiling: count the send and stamp it so the next receive on this
	// port can sample the round-trip latency (no-op unless profiling is on).
	t.RecordSend(key)
	if drv := t.PortDriver(key); drv != nil {
		if err := drv.Send(payload, sender); err != nil {
			return Errorf("port %q: driver send failed: %v", key, err)
		}
		// Release the start barrier's send phase: a daemon PTC binds its
		// listen socket on its first external send, so the parent's
		// start can be sure the bind has been issued. Idempotent.
		if cur := t.CurrentComponent(); cur != nil {
			if exit := t.PTCExit(cur.ID); exit != nil {
				exit.SignalSend()
			}
		}
		return nil
	}
	if err := t.ReceiverError("send", port, hasTo); err != nil {
		return err
	}
	if targets := t.PeerKeys(port); targets != nil {
		from := t.CurrentComponent()
		if hasTo {
			if ids := TargetComponentIDs(sender); ids != nil {
				if keys := t.AddressedPeerKeys(port, ids); len(keys) > 0 {
					targets = keys
				}
			}
		}
		for _, k := range targets {
			t.EnqueueMessageFrom(k, payload, from)
		}
		return nil
	}
	if t.IsMapped(PortEndpoint{Comp: t.CurrentComponentID(), Port: port}) {
		sender = SystemComponent
	}
	t.EnqueueMessageFrom(key, payload, sender)
	return nil
}

package runtime

// ReceiveResult is what a receiving operation found on its port.
type ReceiveResult uint8

const (
	// ReceiveEmpty: the queue holds nothing the operation looks at.
	ReceiveEmpty ReceiveResult = iota
	// ReceiveMismatch: the message at the head of the queue does not
	// match; it stays queued.
	ReceiveMismatch
	// ReceiveMatched: the message at the head of the queue matches; a
	// receiving operation that consumes removed it.
	ReceiveMatched
)

// A ReceiveOp is a receiving operation on one port of the running
// component (ETSI ES 201 873-1 22.2.2-22.2.4, 22.3.2, 22.3.4, 22.3.6,
// 22.4), its operands evaluated: Match tells whether a message at the
// head of the queue matches its template and its from clause.
type ReceiveOp struct {
	Port string      // the port's queue key
	Kind PortMsgKind // the envelopes the operation takes
	// AnyKind is a check with no receiving operation, which looks at
	// whatever is at the head of the queue, a call, reply or exception
	// included (22.4).
	AnyKind bool
	// Limit is the number of entries the queue held when the alt's
	// snapshot was taken, the entries the operation can see; -1 sees all.
	Limit int
	// Consume removes the message matched: receive, trigger, getcall,
	// getreply and catch do; check does not.
	Consume bool
	// Trigger removes a message at the head of the queue that does not
	// match, and looks at the next (22.2.3).
	Trigger bool
	// Match reports whether the message at the head of the queue matches.
	// It is called with the receive section locked, once per head.
	Match func(PortMessage) bool
}

// Receive performs op: it matches the message at the head of the queue,
// removes it if it matches and op consumes, or if it does not and op is
// a trigger, which then looks at the next. It returns the message matched
// (the one removed, when op consumes) and what it found.
//
// The peek, match and removal happen in one critical section, so two
// receivers sharing a queue never take the same message: what one matched
// is what it removes.
func (t *TestcaseExec) Receive(op ReceiveOp) (PortMessage, ReceiveResult) {
	t.ReceiveLock()
	defer t.ReceiveUnlock()
	limit := op.Limit
	for {
		head, ok := t.PeekKindLimited(op.Port, op.Kind, limit)
		if op.AnyKind {
			head, ok = t.PeekHeadLimited(op.Port, limit)
		}
		if !ok {
			return PortMessage{}, ReceiveEmpty
		}
		if !op.Match(head) {
			if !op.Trigger {
				return head, ReceiveMismatch
			}
			// One entry of the snapshot is gone.
			_, _ = t.DequeueKindLimited(op.Port, op.Kind, limit)
			if limit > 0 {
				limit--
			}
			continue
		}
		if op.Consume {
			if deq, ok := t.DequeueKindLimited(op.Port, op.Kind, limit); ok {
				head = deq
			}
			// Profiling: a message receive samples the round-trip latency
			// against the last send on its port (no-op unless profiling is
			// on). Procedure receives are not: the model pairs a message
			// sent with its answer.
			if op.Kind == MsgMessage {
				t.RecordReceive(op.Port)
			}
		}
		return head, ReceiveMatched
	}
}

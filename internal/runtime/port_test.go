package runtime

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testPort(owner *Component, name string, typ Type) *Port {
	return &Port{Owner: owner, Name: name, Type: typ}
}

func TestPortSetBindings(t *testing.T) {
	owner := NewComponent("MTC")
	typ := PortType{InTypes: []Type{IntegerType}, OutTypes: []Type{IntegerType}}
	p0 := testPort(owner, "p[0]", typ)
	p1 := testPort(owner, "p[1]", typ)
	value := []any{p0, p1}
	ports := NewPortSet(LoopbackAdapter)
	ports.Register(owner, "p", value)

	got, ok := ports.Binding(owner, "p")
	require.True(t, ok)
	assert.Equal(t, value, got)
	assert.Equal(t, []*Port{p0, p1}, ports.Ports(owner))
}

func TestPortLoopbackSnapshotAndCopy(t *testing.T) {
	owner := NewComponent("MTC")
	elem := RecordOfType{ElementType: IntegerType}
	typ := PortType{InTypes: []Type{elem}, OutTypes: []Type{elem}}
	port := testPort(owner, "p", typ)
	ports := NewPortSet(LoopbackAdapter)
	ports.Register(owner, "p", port)

	value := []any{1, 2}
	deliveries, err := ports.Send(port, Message{Value: value, Type: elem}, nil, false)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	assert.True(t, deliveries[0].Queued)
	value[0] = 9

	ports.TakeSnapshot(owner)
	message, ok := ports.Snapshot(port)
	require.True(t, ok)
	assert.Equal(t, []any{1, 2}, message.Value)
	assert.Equal(t, owner, message.Sender)
	assert.True(t, ports.Consume(port))
	ports.TakeSnapshot(owner)
	_, ok = ports.Snapshot(port)
	assert.False(t, ok)
}

func TestPortConnectionsAndState(t *testing.T) {
	a := NewComponent("A")
	b := NewComponent("B")
	typ := PortType{InTypes: []Type{IntegerType}, OutTypes: []Type{IntegerType}}
	pa := testPort(a, "p", typ)
	pb := testPort(b, "p", typ)
	ports := NewPortSet(LoopbackAdapter)
	ports.Register(a, "p", pa)
	ports.Register(b, "p", pb)

	ports.Connect(pa, pb, false)
	connected, err := ports.CheckState([]*Port{pa}, "Connected", false)
	require.NoError(t, err)
	assert.True(t, connected)

	deliveries, err := ports.Send(pa, Message{Value: 7, Type: IntegerType}, nil, false)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	assert.Equal(t, pb, deliveries[0].Port)
	ports.TakeSnapshot(b)
	message, ok := ports.Snapshot(pb)
	require.True(t, ok)
	assert.Equal(t, 7, message.Value)
	assert.Equal(t, a, message.Sender)

	ports.Disconnect(pa, pb, false)
	connected, err = ports.CheckState([]*Port{pa}, "Connected", false)
	require.NoError(t, err)
	assert.False(t, connected)
}

func TestPortLifecycle(t *testing.T) {
	owner := NewComponent("MTC")
	typ := PortType{InTypes: []Type{IntegerType}, OutTypes: []Type{IntegerType}}
	port := testPort(owner, "p", typ)
	ports := NewPortSet(LoopbackAdapter)
	ports.Register(owner, "p", port)

	_, err := ports.Send(port, Message{Value: 1}, nil, false)
	require.NoError(t, err)
	ports.Halt(port)
	assert.Equal(t, PortHalted, ports.Status(port))

	ports.TakeSnapshot(owner)
	message, ok := ports.Snapshot(port)
	require.True(t, ok)
	assert.Equal(t, 1, message.Value)
	assert.True(t, ports.Consume(port))
	ports.TakeSnapshot(owner)
	_, ok = ports.Snapshot(port)
	assert.False(t, ok)
	assert.Equal(t, PortStopped, ports.Status(port))

	ports.Start(port)
	assert.Equal(t, PortStarted, ports.Status(port))
	assert.Empty(t, port.queue)
	ports.Stop(port)
	assert.Equal(t, PortStopped, ports.Status(port))
	_, err = ports.Send(port, Message{Value: 2}, nil, false)
	assert.Error(t, err)
	ports.Clear(port)
	assert.Empty(t, port.queue)
}

func TestBroadcastAdapter(t *testing.T) {
	owner := NewComponent("MTC")
	typ := PortType{InTypes: []Type{IntegerType}, OutTypes: []Type{IntegerType}}
	p1 := testPort(owner, "p1", typ)
	p2 := testPort(owner, "p2", typ)
	ports := NewPortSet(BroadcastAdapter)
	ports.Register(owner, "p1", p1)
	ports.Register(owner, "p2", p2)

	deliveries, err := ports.Send(p1, Message{Value: 1}, nil, false)
	require.NoError(t, err)
	assert.Len(t, deliveries, 2)
	ports.TakeSnapshot(owner)
	_, p1Ready := ports.Snapshot(p1)
	_, p2Ready := ports.Snapshot(p2)
	assert.True(t, p1Ready)
	assert.True(t, p2Ready)
}

func TestPortInvalidStateWithoutPorts(t *testing.T) {
	ports := NewPortSet(LoopbackAdapter)
	_, err := ports.CheckState(nil, "started", true)
	assert.Error(t, err)
}

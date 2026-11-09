package runtime

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

// PortStatus is the input state of a message port.
type PortStatus uint8

const (
	PortStarted PortStatus = iota
	PortHalted
	PortStopped
)

func (s PortStatus) String() string {
	switch s {
	case PortStarted:
		return "Started"
	case PortHalted:
		return "Halted"
	case PortStopped:
		return "Stopped"
	default:
		return "Unknown"
	}
}

// PortAdapter selects the in-memory test-system adapter.
type PortAdapter uint8

const (
	LoopbackAdapter PortAdapter = iota
	BroadcastAdapter
)

// Message is one item in a message-port input queue.
type Message struct {
	Value     any
	Type      Type
	Sender    any
	Timestamp time.Time

	halt bool
}

// Port is one live component port.
type Port struct {
	Owner *Component
	Name  string
	Type  Type

	status   PortStatus
	queue    []Message
	snapshot Message
	snapped  bool
	links    []PortLink
}

func NewPort(owner *Component, name string, typ Type) *Port {
	return &Port{Owner: owner, Name: name, Type: typ, status: PortStarted}
}

func (p *Port) String() string {
	if p.Owner == nil {
		return p.Name
	}
	return p.Owner.Name + "." + p.Name
}

// PortLink is either a component connection or a system mapping.
type PortLink struct {
	Port   *Port
	Mapped bool
}

// Delivery reports the result of sending one message to one input port.
type Delivery struct {
	Port    *Port
	Message Message
	Queued  bool
	Reason  string
}

type portBinding struct {
	owner *Component
	name  string
	value any
}

// PortSet is the ALL-PORT-STATES of one testcase. Its lock makes taking a
// snapshot atomic with sends from parallel component behaviours.
type PortSet struct {
	mu       sync.Mutex
	adapter  PortAdapter
	bindings []portBinding
	ports    []*Port
}

func NewPortSet(adapter PortAdapter) *PortSet {
	return &PortSet{adapter: adapter}
}

// Register records the value of one component port declaration. value is a
// Port or a nested []any for a port array.
func (s *PortSet) Register(owner *Component, name string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.bindings {
		if s.bindings[i].owner == owner && s.bindings[i].name == name {
			s.bindings[i].value = value
			return
		}
	}
	s.bindings = append(s.bindings, portBinding{owner: owner, name: name, value: value})
	collectPorts(value, &s.ports)
}

func collectPorts(value any, ports *[]*Port) {
	switch value := value.(type) {
	case *Port:
		*ports = append(*ports, value)
	case []any:
		for _, value := range value {
			collectPorts(value, ports)
		}
	}
}

// Binding resolves the name part of component:name.
func (s *PortSet) Binding(owner *Component, name string) (any, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, binding := range s.bindings {
		if binding.owner == owner && binding.name == name {
			return binding.value, true
		}
	}
	return nil, false
}

// Ports returns the component's ports in declaration and array-index order.
func (s *PortSet) Ports(owner *Component) []*Port {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*Port(nil), s.owned(owner)...)
}

func (s *PortSet) owned(owner *Component) []*Port {
	ports := make([]*Port, 0)
	for _, port := range s.ports {
		if port.Owner == owner {
			ports = append(ports, port)
		}
	}
	return ports
}

// Connect creates an idempotent, symmetric connection or mapping.
func (s *PortSet) Connect(a, b *Port, mapped bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	addLink(a, b, mapped)
	addLink(b, a, mapped)
}

func addLink(from, to *Port, mapped bool) {
	if slices.ContainsFunc(from.links, func(link PortLink) bool {
		return link.Port == to && link.Mapped == mapped
	}) {
		return
	}
	from.links = append(from.links, PortLink{Port: to, Mapped: mapped})
}

// Disconnect removes one specific connection or mapping.
func (s *PortSet) Disconnect(a, b *Port, mapped bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delLink(a, b, mapped)
	delLink(b, a, mapped)
}

func delLink(from, to *Port, mapped bool) {
	from.links = slices.DeleteFunc(from.links, func(link PortLink) bool {
		return link.Port == to && link.Mapped == mapped
	})
}

// DisconnectPort removes every link of kind mapped from port.
func (s *PortSet) DisconnectPort(port *Port, mapped bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disconnectPorts([]*Port{port}, mapped)
}

// DisconnectComponent removes every link of kind mapped involving owner.
func (s *PortSet) DisconnectComponent(owner *Component, mapped bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disconnectPorts(s.owned(owner), mapped)
}

// DisconnectAll removes every link of kind mapped.
func (s *PortSet) DisconnectAll(mapped bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disconnectPorts(s.ports, mapped)
}

func (s *PortSet) disconnectPorts(ports []*Port, mapped bool) {
	selected := make(map[*Port]bool, len(ports))
	for _, port := range ports {
		selected[port] = true
	}
	for _, port := range s.ports {
		port.links = slices.DeleteFunc(port.links, func(link PortLink) bool {
			return link.Mapped == mapped && (selected[port] || selected[link.Port])
		})
	}
}

// Links returns a copy of port's links.
func (s *PortSet) Links(port *Port) []PortLink {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]PortLink(nil), port.links...)
}

// TakeSnapshot copies the first visible item of every port owned by owner.
func (s *PortSet) TakeSnapshot(owner *Component) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, port := range s.owned(owner) {
		port.snapped = false
		if port.status == PortStopped || len(port.queue) == 0 {
			continue
		}
		if port.status == PortHalted && port.queue[0].halt {
			port.queue = port.queue[1:]
			port.status = PortStopped
			continue
		}
		port.snapshot = port.queue[0]
		port.snapped = true
	}
}

// Snapshot returns the item captured by the latest TakeSnapshot.
func (s *PortSet) Snapshot(port *Port) (Message, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return port.snapshot, port.snapped
}

// Snapshots returns all captured port heads owned by owner.
func (s *PortSet) Snapshots(owner *Component) []struct {
	Port    *Port
	Message Message
} {
	s.mu.Lock()
	defer s.mu.Unlock()
	var snapshots []struct {
		Port    *Port
		Message Message
	}
	for _, port := range s.owned(owner) {
		if port.snapped {
			snapshots = append(snapshots, struct {
				Port    *Port
				Message Message
			}{port, port.snapshot})
		}
	}
	return snapshots
}

// Consume removes the queue head captured in the latest snapshot.
func (s *PortSet) Consume(port *Port) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !port.snapped || len(port.queue) == 0 {
		return false
	}
	port.queue = port.queue[1:]
	port.snapped = false
	if port.status == PortHalted && len(port.queue) > 0 && port.queue[0].halt {
		port.queue = port.queue[1:]
		port.status = PortStopped
	}
	return true
}

// Clear removes queued input without changing port status.
func (s *PortSet) Clear(port *Port) {
	s.mu.Lock()
	defer s.mu.Unlock()
	port.queue = nil
	port.snapped = false
}

// Start clears and starts port.
func (s *PortSet) Start(port *Port) {
	s.mu.Lock()
	defer s.mu.Unlock()
	port.queue = nil
	port.snapped = false
	port.status = PortStarted
}

// Stop makes queued input invisible without clearing it.
func (s *PortSet) Stop(port *Port) {
	s.mu.Lock()
	defer s.mu.Unlock()
	port.snapped = false
	port.status = PortStopped
}

// Halt stops accepting input and places a marker after already queued items.
func (s *PortSet) Halt(port *Port) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if port.status == PortHalted {
		return
	}
	port.status = PortHalted
	port.queue = append(port.queue, Message{halt: true})
}

func (s *PortSet) Status(port *Port) PortStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return port.status
}

// CheckState implements the single, any, and all forms of port.checkstate.
func (s *PortSet) CheckState(ports []*Port, state string, anyPort bool) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch state {
	case "Started", "Halted", "Stopped", "Connected", "Mapped", "Linked":
	default:
		return false, fmt.Errorf("invalid port state %q", state)
	}
	if len(ports) == 0 {
		return false, nil
	}
	matches := func(port *Port) (bool, error) {
		switch state {
		case "Started":
			return port.status == PortStarted, nil
		case "Halted":
			return port.status == PortHalted, nil
		case "Stopped":
			return port.status == PortStopped, nil
		case "Connected", "Mapped", "Linked":
			return slices.ContainsFunc(port.links, func(link PortLink) bool {
				return state == "Linked" || state == "Mapped" && link.Mapped || state == "Connected" && !link.Mapped
			}), nil
		}
		return false, nil
	}
	result := !anyPort
	for _, port := range ports {
		ok, err := matches(port)
		if err != nil {
			return false, err
		}
		if anyPort && ok {
			return true, nil
		}
		if !anyPort && !ok {
			return false, nil
		}
	}
	return result, nil
}

// Send routes and queues msg. destinations is nil for an unaddressed send;
// all selects the "to all component" form.
func (s *PortSet) Send(source *Port, msg Message, destinations []any, all bool) ([]Delivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if source.status != PortStarted {
		return nil, fmt.Errorf("%s is %s", source, source.status)
	}
	if !accepts(source, msg.Value, false) {
		return nil, fmt.Errorf("%s cannot send %T", source, msg.Value)
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	msg.Value = CloneValue(msg.Value)
	msg.Sender = source.Owner

	type target struct {
		port     *Port
		sender   any
		external bool
	}
	var targets []target
	direct := func(destination any) (*Port, bool) {
		var found *Port
		for _, link := range source.links {
			if link.Mapped {
				continue
			}
			component, ok := destination.(*Component)
			if !ok || link.Port.Owner != component {
				continue
			}
			if found != nil && found != link.Port {
				return nil, false
			}
			found = link.Port
		}
		return found, found != nil
	}
	external := func(sender any) {
		ports := []*Port{source}
		if s.adapter == BroadcastAdapter {
			ports = ports[:0]
			for _, port := range s.ports {
				if port.Owner != nil && port.Owner.Name != "SYSTEM" {
					ports = append(ports, port)
				}
			}
		}
		for _, port := range ports {
			targets = append(targets, target{port: port, sender: sender, external: true})
		}
	}

	switch {
	case all:
		for _, link := range source.links {
			if !link.Mapped {
				targets = append(targets, target{port: link.Port, sender: source.Owner})
			}
		}
		if len(targets) == 0 || slices.ContainsFunc(source.links, func(link PortLink) bool { return link.Mapped }) {
			external(source.Owner)
		}
	case destinations != nil:
		for _, destination := range destinations {
			if port, ok := direct(destination); ok {
				targets = append(targets, target{port: port, sender: source.Owner})
				continue
			}
			if _, isComponent := destination.(*Component); isComponent {
				return nil, fmt.Errorf("%s has no connection to %v", source, destination)
			}
			if _, null := destination.(Null); null || !acceptsAddress(source, destination) {
				return nil, fmt.Errorf("%s cannot address %v", source, destination)
			}
			external(destination)
		}
	default:
		var connected []*Port
		mapped := false
		for _, link := range source.links {
			if link.Mapped {
				mapped = true
			} else {
				connected = append(connected, link.Port)
			}
		}
		switch {
		case len(connected) == 1 && !mapped:
			targets = append(targets, target{port: connected[0], sender: source.Owner})
		case len(connected) > 1:
			return nil, fmt.Errorf("%s has multiple destinations", source)
		default:
			external(source.Owner)
		}
	}

	seen := make(map[*Port]bool)
	deliveries := make([]Delivery, 0, len(targets))
	for _, target := range targets {
		if seen[target.port] {
			continue
		}
		seen[target.port] = true
		item := msg
		item.Sender = CloneValue(target.sender)
		delivery := Delivery{Port: target.port, Message: item}
		switch {
		case target.port.status != PortStarted:
			delivery.Reason = "port " + target.port.status.String()
		case !accepts(target.port, item.Value, true):
			if target.external {
				continue
			}
			return nil, fmt.Errorf("%s cannot receive %T", target.port, item.Value)
		default:
			target.port.queue = append(target.port.queue, item)
			delivery.Queued = true
		}
		deliveries = append(deliveries, delivery)
	}
	return deliveries, nil
}

func accepts(port *Port, value any, incoming bool) bool {
	base := BaseType(port.Type)
	typ, ok := base.(PortType)
	if !ok {
		if ptr, ok := base.(*PortType); ok {
			typ = *ptr
		} else {
			return false
		}
	}
	types := typ.OutTypes
	if incoming {
		types = typ.InTypes
	}
	for _, typ := range types {
		if named, ok := typ.(*NamedType); ok && (named.Name == "anytype" || strings.HasSuffix(named.Name, ".anytype")) {
			return true
		}
		if IsCompatible(value, typ) == nil {
			return true
		}
	}
	return false
}

func acceptsAddress(port *Port, value any) bool {
	base := BaseType(port.Type)
	var typ PortType
	switch portType := base.(type) {
	case PortType:
		typ = portType
	case *PortType:
		typ = *portType
	default:
		return false
	}
	return typ.Address != nil && IsCompatible(value, typ.Address) == nil
}

// CloneValue makes the value copy required by TTCN-3 assignment and
// communication while retaining immutable type and component references.
func CloneValue(value any) any {
	switch value := value.(type) {
	case []any:
		clone := make([]any, len(value))
		for i, item := range value {
			clone[i] = CloneValue(item)
		}
		return clone
	case map[any]any:
		clone := make(map[any]any, len(value))
		for key, item := range value {
			clone[CloneValue(key)] = CloneValue(item)
		}
		return clone
	case map[string]any:
		clone := make(map[string]any, len(value))
		for key, item := range value {
			clone[key] = CloneValue(item)
		}
		return clone
	case ValueList:
		clone := make(ValueList, len(value))
		for i, item := range value {
			clone[i] = CloneValue(item)
		}
		return clone
	case TemplateList:
		return TemplateList{Kind: value.Kind, Items: CloneValue(value.Items).([]any)}
	case ModifiedTemplate:
		return ModifiedTemplate{Val: CloneValue(value.Val), Length: CloneValue(value.Length), IfPresent: value.IfPresent}
	default:
		return value
	}
}

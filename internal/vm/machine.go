package vm

import (
	"cmp"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/nokia/ntt/internal/k3log"
	"github.com/nokia/ntt/internal/runtime"
	"github.com/nokia/ntt/internal/t3xf"
	"github.com/nokia/ntt/internal/t3xf/opcode"
	"io"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	StackSize = 1024

	ErrOverflow = fmt.Errorf("overflow")

	// errFault aborts a testcase or PTC behaviour with an error verdict and
	// lets the control part continue.
	errFault = errors.New("ttcn3 fault")

	// localsMu guards Image.locals. VAR publishes a descriptor once.
	localsMu sync.Mutex
	// eventMu keeps k3log lines from different goroutines from interleaving.
	eventMu sync.Mutex
)

type Frame struct {
	PC int
	SP int
	BP int
	Fn *runtime.Function

	outBack []outParamBack
	// leave is logged when the frame returns, "" for none.
	leave k3log.Category
}

type outParamBack struct {
	target  any
	written bool
}

// slotRef is a local variable or parameter of a calling frame, passed as an
// inout argument. Slot is its absolute index in Stack.
type slotRef struct {
	*runtime.Var
	Slot int
}

// resolve makes the l-value lv independent of the current frame: a local
// variable or parameter becomes its stack slot, and indices are evaluated.
func (m *Machine) resolve(lv any) any {
	switch x := lv.(type) {
	case *runtime.Var:
		switch {
		case x.Flags&runtime.Inout != 0:
			return m.Stack[m.BP+x.Index]
		case x.Flags&(runtime.Local|runtime.Parameter) != 0:
			return &slotRef{Var: x, Slot: m.BP + x.Index}
		}
	case *ElementRef:
		return &ElementRef{Container: m.resolve(x.Container), Index: m.deref(x.Index)}
	}
	return lv
}

func assignable(value any) bool {
	switch value := value.(type) {
	case *runtime.Var, *slotRef:
		return true
	case *ElementRef:
		return assignable(value.Container)
	default:
		return false
	}
}

// ElementRef represents a reference to an element within a container (array, record-of, etc.)
// It stores both the container reference and the index/field needed to access the element.
type ElementRef struct {
	Container any // *runtime.Var or *ElementRef
	Index     any // The index, field name, or any TTCN-3 value used as a key
}

// logCollection holds variadic log() arguments built by mark/collect.
type logCollection []any

type commContext struct {
	port      *runtime.Port
	message   runtime.Message
	copyValue bool
}

type Machine struct {
	*Image
	Frame
	Frames  []Frame
	Stack   []any
	Globals []any

	// Out receives k3log lines. Defaults to os.Stdout when nil.
	Out io.Writer

	marks []int // open MARK positions, innermost last

	// self is the component this machine runs as. tc is non-nil only for
	// the MTC and PTCs of one testcase; those machines share it.
	self *runtime.Component
	tc   *testcase
	// chosen is the alt step body to run. Non-nil means a step matched,
	// including when that body is empty.
	chosen *Block
	// callPC is the execute or startc that invoked this behaviour. -1 when
	// this machine was not started that way.
	callPC int
	// altWait is set when the current snapshot has a guard that may become
	// ready. altNever is set when a guard cannot.
	altWait  bool
	altNever bool
	// wake is the earliest deadline of a timer the current snapshot waits
	// for. Zero means none.
	wake time.Time
	// taken holds the steps of the current interleave that have matched,
	// and is nil in an alt. open counts the steps evaluated in a snapshot.
	// chosenPC is the step that set chosen.
	taken    map[int]bool
	open     int
	chosenPC int
	// snap is what the alternatives of the current alt pass are evaluated
	// against, and nil outside their evaluation (see altSnapshot).
	snap *altSnapshot
	// asDefault makes the next call log dten/dtlv.
	asDefault bool
	// probing lets isbound and friends read missing elements.
	probing bool

	// creator reports the events of m while it initialises its component.
	creator *Machine

	// valueTarget holds the target variable or element reference during
	// execution of a compound assignment block (VALUE opcode).
	valueTarget any
	comm        *commContext
}

// testcase is the shared state of one execute. n names unnamed components
// "$1", "$2", …. wg counts behaviours started with startc. notify closes ch
// so every waiter wakes, then replaces it.
type testcase struct {
	mu     sync.Mutex
	mtc    *runtime.Component
	system *runtime.Component
	ptcs   map[*runtime.Component]*Machine
	ports  *runtime.PortSet
	n      int
	ch     chan struct{}
	wg     sync.WaitGroup
}

func (t *testcase) notify() {
	t.mu.Lock()
	defer t.mu.Unlock()
	close(t.ch)
	t.ch = make(chan struct{})
}

func (t *testcase) wait() <-chan struct{} {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ch
}

// create returns the machine of a new PTC of type typ, created by m. An
// unnamed one takes the next "$n", starting at 1. A name passed in is used
// as-is and does not consume a number.
func (t *testcase) create(m *Machine, typ runtime.Type, name string, unnamed bool) (*Machine, error) {
	t.mu.Lock()
	if unnamed {
		t.n++
		name = "$" + strconv.Itoa(t.n)
	}
	ptc := m.spawn(runtime.NewComponent(name))
	t.ptcs[ptc.self] = ptc
	t.mu.Unlock()
	ptc.tc = t
	return ptc, ptc.initComponent(m, typ)
}

// altSnapshot is the state an alt or interleave pass evaluates all its
// alternatives against (ETSI ES 201 873-1 20.1): the port queues
// (PortSet.TakeSnapshot), whether each component of the testcase is done,
// and the time timers are read at. A component that ends or a timer that
// expires during the pass is seen by the next one, by every alternative
// alike.
type altSnapshot struct {
	done map[*runtime.Component]bool
	now  time.Time
}

// takeSnapshot is the snapshot of a pass of m's alt.
func (m *Machine) takeSnapshot() *altSnapshot {
	var done map[*runtime.Component]bool
	if m.tc != nil {
		done = m.tc.componentsDone()
	}
	return &altSnapshot{done: done, now: time.Now()}
}

// componentsDone is whether each component of the testcase is done.
func (t *testcase) componentsDone() map[*runtime.Component]bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	done := make(map[*runtime.Component]bool, len(t.ptcs)+1)
	for c := range t.ptcs {
		done[c] = c.Done()
	}
	if t.mtc != nil {
		done[t.mtc] = t.mtc.Done()
	}
	return done
}

// componentDone reports whether comp is done, as the current alt pass sees
// it, or now outside one.
func (m *Machine) componentDone(comp *runtime.Component) bool {
	if m.snap != nil {
		if done, ok := m.snap.done[comp]; ok {
			return done
		}
	}
	return comp.Done()
}

// now is the time timers are read at: that of the current alt pass, or
// the present outside one.
func (m *Machine) now() time.Time {
	if m.snap != nil {
		return m.snap.now
	}
	return time.Now()
}

func (t *testcase) machine(c *runtime.Component) *Machine {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ptcs[c]
}

func (t *testcase) stopAll() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for c := range t.ptcs {
		c.Stop()
	}
}

func New(img *Image) *Machine {
	m := &Machine{
		Image:  img,
		Stack:  make([]any, StackSize),
		callPC: -1,
		self:   runtime.NewComponent(""),
	}
	return m
}

func Run(img *Image, out io.Writer) error {
	m := New(img)
	m.Out = out
	return m.Run()
}

// Run initialises the module and runs every control part.
func (m *Machine) Run() error {
	if err := m.Init(); err != nil {
		return err
	}
	for _, control := range m.Controls {
		if err := m.RunControl(control); err != nil {
			return err
		}
	}
	return nil
}

// Init initialises the module variables, constants and module parameters. It
// must be called once before RunControl or RunTestcase.
func (m *Machine) Init() error {
	for i, tag := range m.Tags {
		v, ok := tag.(*runtime.Var)
		if !ok {
			continue
		}
		switch _, op, _ := t3xf.Decode(m.Code[i<<2:]); op {
		case opcode.VAR, opcode.CONST, opcode.CONSTW, opcode.MPAR, opcode.MPARD:
		default:
			continue
		}
		for len(m.Globals) <= v.Index {
			m.Globals = append(m.Globals, runtime.Undefined{})
		}
		switch val := v.Value.(type) {
		case nil:
		case Block:
			if _, endOp, _ := t3xf.Decode(m.Code[val.End:]); endOp == opcode.RETURN {
				if err := m.Execute(&runtime.Function{Body: val}); err != nil {
					return err
				}
				m.store(v, m.popValue())
			} else {
				m.store(v, val)
			}
		default:
			m.Globals[v.Index] = m.deref(val)
		}
	}
	return nil
}

// RunControl runs the control part fn, framed by its cpen and cplv events.
func (m *Machine) RunControl(fn *runtime.Function) error {
	if err := m.event(k3log.CPEN, "k3r", k3log.NoPos, fn.Name+"()"); err != nil {
		return err
	}
	if err := m.Execute(fn); err != nil {
		return err
	}
	return m.event(k3log.CPLV, "k3r", k3log.NoPos, fn.Name+"()")
}

// RunTestcase runs the parameterless testcase tc on a new MTC, as if a
// control part executed it without a guard, and returns its final verdict.
func (m *Machine) RunTestcase(tc *runtime.Function) (runtime.Verdict, error) {
	return m.execute(tc, nil, "unlimited", -1)
}

func (m *Machine) Push(v any) {
	m.Stack[m.SP] = v
	m.SP++
}

func (m *Machine) Pop() any {
	m.SP--
	v := m.Stack[m.SP]
	return v
}

func (m *Machine) fnen(fn *runtime.Function) {
	body := fn.Body.(Block)
	base := m.SP - len(fn.Parameters)
	outBack := make([]outParamBack, len(fn.Parameters))
	for i, p := range fn.Parameters {
		arg := m.Stack[base+i]
		switch {
		case p.Flags&runtime.Inout != 0:
			m.Stack[base+i] = m.resolve(arg)
		case p.Flags&runtime.Out != 0:
			outBack[i] = outParamBack{target: arg}
			m.Stack[base+i] = runtime.Undefined{}
		default:
			m.Stack[base+i] = m.deref(arg)
		}
	}
	args := m.Stack[base:m.SP]
	enter, leave := k3log.FNEN, k3log.FNLV
	switch {
	case fn.Flags&runtime.Template != 0:
		enter, leave = "", ""
	case m.asDefault:
		m.asDefault = false
		enter, leave = k3log.DTEN, k3log.DTLV
	case fn.Flags&runtime.Altstep != 0:
		enter, leave = k3log.ASEN, k3log.ASLV
	}
	if enter != "" {
		who, _ := m.origin()
		if err := m.event(enter, who, m.PC, m.signature(fn, args, false)); err != nil {
			panic(&Error{PC: m.PC, Err: err})
		}
	}

	// An empty body is a single ISCAN instruction without RETURN.
	if body.Begin == body.End {
		if leave != "" {
			who, _ := m.origin()
			if err := m.event(leave, who, m.PC, m.signature(fn, args, true)); err != nil {
				panic(&Error{PC: m.PC, Err: err})
			}
		}
		m.SP = base
		return
	}

	caller := m.Frame
	caller.SP = base
	m.Frames = append(m.Frames, caller)
	m.Frame = Frame{
		PC:      body.Begin + 4, // after ISCAN
		SP:      m.SP + fn.NLocals,
		BP:      base,
		Fn:      fn,
		outBack: outBack,
		leave:   leave,
	}

	if m.SP*100/len(m.Stack) > 90 {
		s := make([]any, len(m.Stack)*2)
		copy(s, m.Stack)
		m.Stack = s
	}
}

func (m *Machine) fnlv() {
	fn := m.Fn
	hasReturn := fn.ReturnType != nil
	var ret any
	if hasReturn {
		if m.SP <= m.BP+len(fn.Parameters)+fn.NLocals {
			panic(m.fault(k3log.ErrOSUF))
		}
		ret = m.Pop()
	}

	leaving, args := m.popFrame()
	for i, back := range leaving.outBack {
		if back.target == nil || !back.written {
			continue
		}
		m.store(back.target, args[i])
	}

	if leaving.leave != "" {
		sig := m.signature(fn, args, true)
		if hasReturn {
			sig += "->" + formatValue(m.deref(ret), fn.ReturnType)
		}
		who, _ := m.origin()
		if err := m.event(leaving.leave, who, m.PC, sig); err != nil {
			panic(&Error{PC: m.PC, Err: err})
		}
	}
	if hasReturn {
		m.Push(ret)
	}
}

// popFrame returns to the caller and returns the frame left and its
// parameter slots.
func (m *Machine) popFrame() (Frame, []any) {
	leaving := m.Frame
	for _, value := range m.Stack[leaving.BP : leaving.BP+len(leaving.Fn.Parameters)+leaving.Fn.NLocals] {
		if timer, ok := value.(*runtime.Timer); ok {
			m.removeTimer(timer)
		}
	}
	m.Frame = m.Frames[len(m.Frames)-1]
	m.Frames = m.Frames[:len(m.Frames)-1]
	return leaving, m.Stack[leaving.BP : leaving.BP+len(leaving.Fn.Parameters)]
}

// signature formats "name(p=v,...)" for fnen, fnlv, tcen, and tclv. args
// are the parameter slots. On leaving, in parameters print as "-".
func (m *Machine) signature(fn *runtime.Function, args []any, leaving bool) string {
	parts := make([]string, len(fn.Parameters))
	for i, p := range fn.Parameters {
		v := "-"
		if !leaving || p.Flags&(runtime.Out|runtime.Inout) != 0 {
			v = formatValue(m.deref(args[i]), p.Type)
		}
		parts[i] = p.Name[strings.LastIndex(p.Name, ".")+1:] + "=" + v
	}
	return fn.Name + "(" + strings.Join(parts, ",") + ")"
}

// log raises a k3log event at the origin of m. A write error panics, and
// run returns it.
func (m *Machine) log(cat k3log.Category, fields ...string) {
	who, pc := m.origin()
	if err := m.event(cat, who, pc, fields...); err != nil {
		panic(&Error{PC: m.PC, Err: err})
	}
}

func (m *Machine) load(v *runtime.Var) any {
	if v.Index < 0 {
		return v.Value
	}
	if v.Flags&runtime.Parameter != 0 {
		val := m.Stack[m.BP+v.Index]
		if v.Flags&runtime.Inout != 0 {
			return m.deref(val)
		}
		return val
	}
	if v.Flags&runtime.Local != 0 {
		return m.Stack[m.BP+v.Index]
	}
	if v.Index >= len(m.Globals) {
		return runtime.Undefined{}
	}
	return m.Globals[v.Index]
}

func (m *Machine) setSliceIndex(slice []any, intIdx int, value any) []any {
	for len(slice) <= intIdx {
		slice = append(slice, runtime.Undefined{})
	}
	slice[intIdx] = value
	return slice
}

func countSliceRefs(val any, target []any) int {
	s, ok := val.([]any)
	if !ok {
		return 0
	}
	n := 0
	if len(s) > 0 && &s[0] == &target[0] {
		n++
	}
	for _, elem := range s {
		n += countSliceRefs(elem, target)
	}
	return n
}

func (m *Machine) unshareSlice(slice []any) []any {
	if len(slice) == 0 {
		return slice
	}
	n := 0
	for _, slot := range m.Globals {
		n += countSliceRefs(slot, slice)
	}
	for i := 0; i < m.SP; i++ {
		n += countSliceRefs(m.Stack[i], slice)
	}
	if n <= 1 {
		return slice
	}
	return append([]any(nil), slice...)
}

func (m *Machine) unshareMap(mp map[any]any) map[any]any {
	if mp == nil {
		return make(map[any]any)
	}
	cp := make(map[any]any, len(mp))
	for k, v := range mp {
		cp[k] = v
	}
	return cp
}

func fieldByIndex(fields []*runtime.Var, idx any) *runtime.Var {
	i, ok := idx.(int)
	if !ok {
		return nil
	}
	for _, f := range fields {
		if f.Index == i {
			return f
		}
	}
	if i >= 0 && i < len(fields) {
		return fields[i]
	}
	return nil
}

func (m *Machine) exprType(operand any) runtime.Type {
	switch x := operand.(type) {
	case *runtime.Var:
		return x.Type
	case *slotRef:
		return x.Type
	case *ElementRef:
		base := runtime.BaseType(m.exprType(x.Container))
		if fields := runtime.RecordFields(base); fields != nil {
			if f := fieldByIndex(fields, m.deref(x.Index)); f != nil {
				return f.Type
			}
		} else if fields := runtime.UnionFields(base); fields != nil {
			if f := fieldByIndex(fields, m.deref(x.Index)); f != nil {
				return f.Type
			}
		} else if et, ok := runtime.ElemType(base); ok {
			return et
		} else if vt, ok := runtime.MapValueType(base); ok {
			return vt
		}
	}
	return nil
}

func (m *Machine) arrayBounds(base runtime.Type) (int, int) {
	var lenSpec any
	switch a := runtime.BaseType(base).(type) {
	case runtime.ArrayType:
		lenSpec = a.Length
	case *runtime.ArrayType:
		lenSpec = a.Length
	default:
		return 0, 0
	}
	if blk, ok := lenSpec.(Block); ok && blk.Begin < blk.End {
		if err := m.runBlock(blk); err == nil {
			lenSpec = m.popValue()
		}
	}
	switch v := lenSpec.(type) {
	case int:
		return 0, v
	case runtime.Range:
		lo, ok1 := v.Lo.(int)
		hi, ok2 := v.Hi.(int)
		if ok1 && ok2 && hi >= lo {
			return lo, hi - lo + 1
		}
	}
	return 0, 0
}

func (m *Machine) arrayLen(base runtime.Type) int {
	_, n := m.arrayBounds(base)
	return n
}

func (m *Machine) store(target any, value any) {
	if blk, ok := value.(Block); ok {
		if blk.Begin >= blk.End {
			m.store(target, runtime.Undefined{})
			return
		}
		// k3r keeps the fields and elements the block does not assign.
		if _, ok := m.deref(target).([]any); !ok {
			m.store(target, []any{})
		}
		prev := m.valueTarget
		m.valueTarget = target
		err := m.runBlock(blk)
		m.valueTarget = prev
		if err != nil {
			panic(err)
		}
		return
	}
	switch t := target.(type) {
	case *slotRef:
		if err := runtime.IsCompatible(value, t.Type); err != nil {
			panic(fmt.Errorf("type error assigning to variable %q: %w", t.Name, err))
		}
		m.replacingTimer(m.Stack[t.Slot], value)
		m.Stack[t.Slot] = value
	case *runtime.Var:
		// Type check before assignment
		if err := runtime.IsCompatible(value, t.Type); err != nil {
			panic(fmt.Errorf("type error assigning to variable %q: %w", t.Name, err))
		}
		if t.Index < 0 {
			t.Value = value
			return
		}
		if t.Flags&runtime.Parameter != 0 {
			if t.Flags&runtime.Inout != 0 {
				m.store(m.Stack[m.BP+t.Index], value)
				return
			}
			m.replacingTimer(m.Stack[m.BP+t.Index], value)
			m.Stack[m.BP+t.Index] = value
			if t.Flags&runtime.Out != 0 {
				for i, p := range m.Fn.Parameters {
					if p == t && i < len(m.Frame.outBack) {
						m.Frame.outBack[i].written = true
						break
					}
				}
			}
			return
		}
		if t.Flags&runtime.Local != 0 {
			m.replacingTimer(m.Stack[m.BP+t.Index], value)
			m.Stack[m.BP+t.Index] = value
			return
		}
		for len(m.Globals) <= t.Index {
			m.Globals = append(m.Globals, runtime.Undefined{})
		}
		m.replacingTimer(m.Globals[t.Index], value)
		m.Globals[t.Index] = value
	case *ElementRef:
		idx := m.deref(t.Index)
		base := m.exprType(t.Container)
		if _, ok := runtime.MapValueType(base); ok {
			key := idx
			var mp map[any]any
			switch c := m.deref(t.Container).(type) {
			case map[any]any:
				mp = m.unshareMap(c)
			case nil, runtime.Undefined:
				mp = make(map[any]any)
			default:
				panic(fmt.Errorf("unsupported container type for map assignment: %T", c))
			}
			mp[key] = value
			m.store(t.Container, mp)
			return
		}
		intIdx, ok := idx.(int)
		if !ok {
			panic(fmt.Errorf("array index must be int, got %T", idx))
		}
		if intIdx < 0 {
			panic(fmt.Errorf("array index cannot be negative: %d", intIdx))
		}
		fields := runtime.RecordFields(base)
		var slice []any
		switch c := m.deref(t.Container).(type) {
		case nil, runtime.Undefined:
			n := len(fields)
			if n == 0 {
				n = m.arrayLen(base)
			}
			if n > 0 {
				slice = make([]any, n)
				for i := range slice {
					slice[i] = runtime.Undefined{}
				}
			}
		case runtime.TemplateLiteral:
			switch {
			case len(fields) > 0:
				slice = make([]any, len(fields))
				for i, f := range fields {
					if f.Flags&runtime.Optional != 0 {
						slice[i] = runtime.AnyOrNone
					} else {
						slice[i] = runtime.Any
					}
				}
			case m.arrayLen(base) > 0:
				n := max(m.arrayLen(base), intIdx+1)
				slice = make([]any, n)
				for i := range slice {
					slice[i] = runtime.Any
				}
			default:
				slice = make([]any, intIdx+2)
				for i := 0; i < intIdx; i++ {
					slice[i] = runtime.Any
				}
				slice[intIdx+1] = runtime.AnyOrNone
			}
		case []any:
			slice = m.unshareSlice(c)
			for len(slice) < len(fields) {
				slice = append(slice, runtime.Undefined{}) //nolint:makezero // pads the elements c has

			}
		case string, runtime.Binary:
			s, err := runtime.SetElement(c, intIdx, value)
			if err != nil {
				panic(err)
			}
			m.store(t.Container, s)
			return
		default:
			panic(fmt.Errorf("unsupported container type for element assignment: %T", c))
		}
		if runtime.UnionFields(base) != nil {
			slice = nil
		}
		m.store(t.Container, m.setSliceIndex(slice, intIdx, value))
	default:
		panic(fmt.Errorf("invalid store target: %T", target))
	}
}

// operand returns x, or the operand block x leaves on the stack. A run
// error panics, and run returns it.
func (m *Machine) operand(x any) any {
	blk, ok := x.(Block)
	if !ok {
		return x
	}
	if err := m.runBlock(blk); err != nil {
		panic(err)
	}
	return m.Pop()
}

// popValue pops the top of the stack and dereferences variable references.
func (m *Machine) popValue() any { return m.deref(m.Pop()) }

// rndSeedLiteral reports whether rnd(seed) was compiled as an ieee754dp
// literal immediately before this RND, and returns that literal.
func (m *Machine) rndSeedLiteral() (float64, bool) {
	const ieee754Size = 12
	if m.PC < ieee754Size {
		return 0, false
	}
	n, op, arg := t3xf.Decode(m.Code[m.PC-ieee754Size:])
	if op != opcode.IEEE754DP || n != ieee754Size {
		return 0, false
	}
	if m.SP == 0 {
		return 0, false
	}
	seed, ok := m.Stack[m.SP-1].(float64)
	if !ok || seed != arg.(float64) {
		return 0, false
	}
	return seed, true
}

// deref returns the value of an operand, auto-loading if it's a variable reference or element reference.
func (m *Machine) deref(operand any) any {
	switch v := operand.(type) {
	case *runtime.Var:
		return m.load(v)
	case *slotRef:
		return m.Stack[v.Slot]
	case logCollection:
		items := make(runtime.ValueList, len(v))
		for i, item := range v {
			items[i] = m.deref(item)
		}
		return items
	case *ElementRef:
		idx := m.deref(v.Index)
		switch container := m.deref(v.Container).(type) {
		case nil, runtime.Undefined:
			return runtime.Undefined{}
		case *runtime.Component:
			name, ok := idx.(string)
			if !ok || m.tc == nil {
				return runtime.Undefined{}
			}
			value, ok := m.tc.ports.Binding(container, name)
			if !ok {
				return runtime.Undefined{}
			}
			return value
		case runtime.TemplateLiteral:
			if container == runtime.Any {
				if intIdx, ok := idx.(int); ok {
					if f := fieldByIndex(runtime.RecordFields(m.exprType(v.Container)), intIdx); f != nil && f.Flags&runtime.Optional != 0 {
						return runtime.AnyOrNone
					}
				}
				return runtime.Any
			}
			return container
		case []any:
			intIdx, ok := idx.(int)
			if !ok {
				panic(fmt.Errorf("array index must be int, got %T", idx))
			}
			if intIdx < 0 || intIdx >= len(container) || container[intIdx] == nil {
				return runtime.Undefined{}
			}
			return container[intIdx]
		case string, runtime.Binary:
			if e, ok := runtime.Element(container, idx.(int)); ok {
				return e
			}
			return runtime.Undefined{}
		case map[any]any:
			if v, ok := container[m.deref(v.Index)]; ok {
				return v
			}
			return runtime.Undefined{}
		default:
			panic(fmt.Errorf("unsupported container type for element access: %T", container))
		}
	}
	return operand
}

func (m *Machine) Execute(fn *runtime.Function, args ...any) error {
	if len(fn.Parameters) != len(args) {
		return fmt.Errorf("invalid number of arguments: %d != %d", len(fn.Parameters), len(args))
	}

	body := fn.Body.(Block)
	// SetVerdict cannot lower a verdict, so a top-level Execute starts over
	// on a fresh component. The MTC keeps the component execute() installed.
	if m.tc == nil {
		m.self = runtime.NewComponent("")
	}
	if body.Begin == body.End {
		return nil
	}
	copy(m.Stack, args)
	m.Frame = Frame{
		PC: body.Begin + 4, // execute after ISCAN
		SP: fn.NLocals + len(fn.Parameters),
		BP: 0,
		Fn: fn,
	}
	return m.run(-1)
}

func (m *Machine) Verdict() runtime.Verdict {
	if m.self == nil {
		return runtime.VerdictNone
	}
	return m.self.Verdict()
}

func (m *Machine) run(stopPC int) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(*Error); ok {
				err = e
				return
			}
			if e, ok := r.(error); ok {
				err = &Error{PC: m.PC, Err: e}
				return
			}
			// Re-panic for non-error panics
			panic(r)
		}
	}()

	// stopPC is a byte offset in this activation. A call may jump to a
	// body past that offset and must be allowed to return.
	base := len(m.Frames)
	for {
		if m.stopped() || len(m.Frames) < base {
			return nil
		}
		if stopPC >= 0 && len(m.Frames) == base && m.PC >= stopPC {
			return nil
		}

		n, op, arg := t3xf.Decode(m.Code[m.PC:])
		depth := len(m.Frames)

		switch op {

		case opcode.VAR, opcode.VARDUP, opcode.CONST:
			name := m.Pop().(string)
			typ := m.Pop().(runtime.Type)
			var init any = runtime.Undefined{}
			switch op {
			case opcode.VARDUP:
				m.Pop()
			case opcode.CONST:
				if err := m.runBlock(m.Pop().(Block)); err != nil {
					return err
				}
				init = m.popValue()
			}
			v := m.localVar(name, typ, arg.(int))
			m.store(v, init)
			if runtime.BaseType(typ) == runtime.TimerType {
				m.timer(v)
			}
			if v.Flags&runtime.Member != 0 && m.tc != nil {
				if value, ok := m.newPorts(name, typ); ok {
					m.store(v, value)
					m.tc.ports.Register(m.self, name, value)
				}
			}

		case opcode.TEMPLATE:
			name := m.Pop().(string)
			bblk := m.Pop().(Block)
			pblk := m.Pop().(Block)
			retType := m.Pop().(runtime.Type)
			localsMu.Lock()
			if m.Tags[m.PC>>2] == nil {
				l := &loader{Image: m.Image}
				nLocals, _ := arg.(int)
				m.Tags[m.PC>>2] = &runtime.Function{
					Flags:     runtime.Template,
					Name:      name,
					Body:      bblk,
					Signature: runtime.Signature{Parameters: loadObjs[*runtime.Var](l, pblk), ReturnType: retType},
					NLocals:   nLocals,
				}
			}
			localsMu.Unlock()

		case opcode.REF:
			addr := int(arg.(t3xf.Reference))
			// The loader skips bodies, so a VAR inside a function has no Tags
			// entry. The runtime registers that descriptor under its address.
			obj := m.Tags[addr>>2]
			if obj == nil {
				localsMu.Lock()
				obj = m.locals[addr]
				localsMu.Unlock()
			}
			switch obj := obj.(type) {
			case *runtime.Function:
				m.fnen(obj)
				if len(m.Frames) > depth {
					n = 0
				}
			case *runtime.Var:
				// Push reference to the variable (can be used for both reading and assignment)
				m.Push(obj)
			case runtime.Type:
				m.Push(obj)
			default:
				panic(fmt.Errorf("unknown reference type: [0x%04x] %T", m.PC, obj))
			}

		case opcode.ASSIGN, opcode.MOVE:
			target := m.Pop()
			src := m.Pop()
			value := m.deref(src)
			if _, undef := value.(runtime.Undefined); undef {
				return m.fault(k3log.ErrUNDF, m.operandName(src))
			}
			m.store(target, value)
			// Assignments inside a compound value are reported as one.
			if m.valueTarget == nil && src != target {
				m.log(k3log.VACH, m.typeName(target), m.operandName(target), formatValue(m.deref(target), m.exprType(target)))
			}

		case opcode.ASSIGND:
			top := m.Pop()
			if typ, ok := top.(runtime.Type); ok {
				v := &runtime.Var{Name: "(anon)", Type: typ, Index: -1}
				m.store(v, m.popValue())
				m.Push(v)
				break
			}
			// Modified template: <modification> <base args> @base assignd
			base := m.deref(top)
			if s, ok := base.([]any); ok {
				base = append([]any(nil), s...)
			}
			mod := m.popValue()
			v := &runtime.Var{Name: "(anon)", Index: -1}
			m.store(v, base)
			m.store(v, mod)
			m.Push(v)

		case opcode.VALUE:
			if m.valueTarget != nil {
				m.Push(m.valueTarget)
				break
			}
			if m.comm != nil {
				value := m.comm.message.Value
				if m.comm.copyValue {
					value = runtime.CloneValue(value)
				}
				m.Push(value)
				break
			}
			m.Push(nil)

		case opcode.SENDER:
			if m.comm == nil {
				return m.fault(k3log.ErrINOP, "sender outside a communication operation")
			}
			m.Push(runtime.CloneValue(m.comm.message.Sender))

		case opcode.TIMESTAMP:
			if m.comm == nil {
				return m.fault(k3log.ErrINOP, "timestamp outside a communication operation")
			}
			if m.SP > 0 {
				if marker, ok := m.Stack[m.SP-1].(string); ok {
					switch {
					case marker == "__port_index":
						m.SP--
						m.Push(portIndex(m.comm.port.Name))
					case strings.HasPrefix(marker, "__port_from:"):
						m.SP--
						name := strings.TrimPrefix(marker, "__port_from:")
						m.Push(m.comm.port.Name == name || strings.HasPrefix(m.comm.port.Name, name+"["))
					}
					if marker == "__port_index" || strings.HasPrefix(marker, "__port_from:") {
						break
					}
				}
			}
			m.Push(float64(m.comm.message.Timestamp.UnixNano()) / float64(time.Second))

		case opcode.RETURN:
			if len(m.Frames) == 0 {
				return nil
			}
			// An altstep runs the body of the step it chose and returns
			// whether it chose one.
			if m.Fn.Flags&runtime.Altstep != 0 {
				chosen := m.chosen
				m.chosen = nil
				if chosen != nil {
					if err := m.runBlock(*chosen); err != nil {
						return err
					}
				}
				m.fnlv()
				m.Push(chosen != nil)
				break
			}
			if m.Fn.ReturnType != nil && m.SP > 0 {
				if v, ok := m.Stack[m.SP-1].(*runtime.Var); !ok || v.Index >= 0 {
					m.Stack[m.SP-1] = m.deref(m.Stack[m.SP-1])
				}
			}
			m.fnlv()

		case opcode.GOTO:
			m.PC, n = int(arg.(t3xf.Reference)), 0

		case opcode.JT:
			if m.popValue().(bool) {
				n += arg.(int)
			}

		case opcode.JNT:
			if !m.popValue().(bool) {
				n += arg.(int)
			}

		case opcode.IF:
			blk := m.Pop().(Block)
			cond := m.popValue().(bool)
			if cond && blk.Begin < blk.End {
				m.PC, n = blk.Begin+4, 0
			}

		case opcode.IFELSE:
			blkElse := m.Pop().(Block)
			blkThen := m.Pop().(Block)
			cond := m.popValue().(bool)

			var blk Block
			if cond {
				blk = blkThen
			} else {
				blk = blkElse
			}

			if blk.Begin < blk.End {
				m.PC, n = blk.Begin+4, 0
			}

		case opcode.NOP, opcode.LINE:

		case opcode.DROP:
			m.SP--

		case opcode.DUP:
			m.Stack[m.SP] = m.Stack[m.SP-1]
			m.SP++

		case opcode.DEF:
			idx := m.Pop()
			container := m.Pop()
			m.Push(&ElementRef{
				Container: container,
				Index:     idx,
			})

		case opcode.IDEF:
			container := m.Pop()
			m.Push(&ElementRef{
				Container: container,
				Index:     arg.(int),
			})

		case opcode.GET, opcode.IGET:
			idx := arg
			if op == opcode.GET {
				idx = m.popValue()
			}
			ref := &ElementRef{Container: m.Pop(), Index: idx}
			if !m.probing {
				if err := m.checkElement(ref); err != nil {
					return err
				}
			}
			m.Push(ref)

		case opcode.ISCAN:
			blk := Block{Begin: m.PC, End: int(arg.(t3xf.Reference))}
			m.Push(blk)
			m.PC, n = blk.End+4, 0

		case opcode.NAME:
			m.Push(arg.(string))

		case opcode.UTF8:
			m.Push(arg.(string))

		case opcode.BITS, opcode.NIBBLES, opcode.OCTETS:
			unit := byte('B')
			if op == opcode.NIBBLES {
				unit = 'H'
			} else if op == opcode.OCTETS {
				unit = 'O'
			}
			m.Push(binaryLiteral(unit, arg.(*t3xf.String)))

		case opcode.PATTERN:
			m.Push(runtime.Pattern(m.popValue().(string)))

		case opcode.OMIT:
			m.Push(runtime.OmitVal)

		case opcode.ANY:
			m.Push(runtime.Any)

		case opcode.ANYN:
			m.Push(runtime.AnyOrNone)

		case opcode.INFINITYP:
			m.Push(runtime.PosInfinity)

		case opcode.INFINITYN:
			m.Push(runtime.NegInfinity)

		case opcode.IEEE754DP:
			m.Push(arg.(float64))

		case opcode.TRUE:
			m.Push(true)

		case opcode.FALSE:
			m.Push(false)

		case opcode.NOT:
			switch x := m.popValue().(type) {
			case bool:
				m.Push(!x)
			default:
				v, _ := runtime.Bitwise(x.(runtime.Binary), runtime.Binary{}, func(a, _ uint64) uint64 { return ^a })
				m.Push(v)
			}

		case opcode.AND, opcode.OR:
			y := m.Pop()
			switch x := m.popValue().(type) {
			case bool:
				if op == opcode.AND && !x || op == opcode.OR && x {
					m.Push(x)
					break
				}
				m.Push(m.deref(m.operand(y)).(bool))
			default:
				f := func(a, b uint64) uint64 { return a & b }
				if op == opcode.OR {
					f = func(a, b uint64) uint64 { return a | b }
				}
				v, err := runtime.Bitwise(x.(runtime.Binary), m.deref(y).(runtime.Binary), f)
				if err != nil {
					return &Error{PC: m.PC, Err: err}
				}
				m.Push(v)
			}

		case opcode.ISBOUND, opcode.ISPRESENT, opcode.ISCHOSEN, opcode.ISVALUE:
			probing := m.probing
			m.probing = true
			opnd := m.operand(m.Pop())
			m.probing = probing
			v := m.deref(opnd)
			_, undef := v.(runtime.Undefined)
			switch op {
			case opcode.ISPRESENT:
				m.Push(!undef && v != runtime.OmitVal && v != runtime.AnyOrNone)
			case opcode.ISVALUE:
				cat, _ := m.checkDefined(v, m.exprType(opnd), "", false, false)
				m.Push(cat == "")
			default:
				m.Push(!undef)
			}

		case opcode.NATLONG:
			m.Push(arg.(int))

		case opcode.CLONE:
			v := m.popValue()
			if s, ok := v.([]any); ok {
				v = append([]any(nil), s...)
			}
			m.Push(v)

		case opcode.NEG:
			switch x := m.popValue().(type) {
			case float64:
				m.Push(-x)
			default:
				xi := x.(int)
				z := -xi
				if z > math.MaxInt32 || z < math.MinInt32 {
					return &Error{PC: m.PC, Err: fmt.Errorf("%w (%d)", ErrOverflow, xi)}
				}
				m.Push(z)
			}

		case opcode.MARK:
			m.marks = append(m.marks, m.SP)

		case opcode.COLLECT, opcode.VLIST:
			if len(m.marks) == 0 {
				return &Error{PC: m.PC, Err: fmt.Errorf("collect without mark")}
			}
			mark := m.marks[len(m.marks)-1]
			m.marks = m.marks[:len(m.marks)-1]
			objs := make([]any, m.SP-mark)
			if op == opcode.VLIST {
				for i, item := range m.Stack[mark:m.SP] {
					if blk, ok := item.(Block); ok && blk.Begin >= blk.End {
						objs[i] = runtime.Undefined{}
					} else {
						objs[i] = m.deref(item)
					}
				}
				m.SP = mark
				m.Push(objs)
			} else {
				copy(objs, m.Stack[mark:m.SP])
				m.SP = mark
				m.Push(logCollection(objs))
			}

		case opcode.COMPLEMENT, opcode.SUBSET, opcode.SUPERSET, opcode.PERMUTATION:
			var items []any
			switch v := m.popValue().(type) {
			case runtime.ValueList:
				items = []any(v)
			case []any:
				items = v
			default:
				items = []any{v}
			}
			m.Push(runtime.TemplateList{Kind: op.String(), Items: items})

		case opcode.LENGTH:
			length := m.popValue()
			obj := m.popValue()
			m.Push(runtime.ApplyLength(obj, length))

		case opcode.IFPRESENT:
			m.Push(runtime.ApplyIfPresent(m.popValue()))

		case opcode.VALUEOF:
			opnd := m.Pop()
			val := m.deref(opnd)
			if cat, name := m.checkDefined(val, m.exprType(opnd), m.operandName(opnd), false, false); cat != "" {
				return m.fault(cat, name)
			}
			m.Push(val)

		case opcode.MATCH, opcode.SMATCH:
			res, err := m.execMatch(op == opcode.MATCH)
			if err != nil {
				return err
			}
			m.Push(res)

		case opcode.ENUMERATED:
			m.Push(m.loadEnum(m.Pop().(Block)))

		case opcode.FROM, opcode.TO:
			setofType := m.Pop().(runtime.Type)
			mapVal := m.deref(m.Pop())
			var mp map[any]any
			if m, ok := mapVal.(map[any]any); ok {
				mp = m
			}
			elemType, _ := runtime.ElemType(setofType)
			if op == opcode.FROM {
				m.Push(runtime.MapKeys(mp, elemType))
			} else {
				m.Push(runtime.MapValues(mp, elemType))
			}

		case opcode.LOG:
			m.log(k3log.ULOG, m.logText(m.Pop()))

		case opcode.INT2STR, opcode.INT2CHAR, opcode.CHAR2INT, opcode.INT2FLOAT, opcode.FLOAT2INT,
			opcode.STR2INT, opcode.STR2FLOAT, opcode.BIT2INT, opcode.HEX2INT, opcode.OCT2INT,
			opcode.BIT2HEX, opcode.OCT2HEX, opcode.BIT2OCT, opcode.HEX2OCT, opcode.HEX2BIT, opcode.OCT2BIT,
			opcode.BIT2STR, opcode.HEX2STR, opcode.OCT2STR, opcode.STR2HEX, opcode.STR2OCT,
			opcode.CHAR2OCT, opcode.OCT2CHR, opcode.ENUM2INT,
			opcode.INT2BIT, opcode.INT2HEX, opcode.INT2OCT, opcode.SUBSTR, opcode.REPLACE:
			n := map[opcode.Opcode]int{opcode.INT2BIT: 2, opcode.INT2HEX: 2, opcode.INT2OCT: 2, opcode.SUBSTR: 3, opcode.REPLACE: 4}[op]
			args := make([]any, max(n, 1))
			for i := len(args) - 1; i >= 0; i-- {
				args[i] = m.popValue()
			}
			v, err := runtime.Convert(op.String(), args...)
			if err != nil {
				return &Error{PC: m.PC, Err: err}
			}
			m.Push(v)

		case opcode.REGEXP:
			group := m.popValue().(int)
			p := m.popValue()
			if s, ok := p.(string); ok {
				p = runtime.Pattern(s)
			}
			s := m.popValue().(string)
			re, err := p.(runtime.Pattern).Compile(m.patternRef)
			if err != nil {
				return &Error{PC: m.PC, Err: err}
			}
			res := ""
			if sub := re.FindStringSubmatch(s); group+1 < len(sub) {
				res = sub[group+1]
			}
			m.Push(res)

		case opcode.LENGTHOF:
			n, err := runtime.LengthOf(m.popValue())
			if err != nil {
				return &Error{PC: m.PC, Err: err}
			}
			m.Push(n)

		case opcode.SIZEOF:
			n, err := runtime.SizeOf(m.popValue())
			if err != nil {
				return &Error{PC: m.PC, Err: err}
			}
			m.Push(n)

		case opcode.ALLFROM, opcode.ALLFROMP:
			v, err := runtime.AllFrom(m.popValue(), op == opcode.ALLFROMP)
			if err != nil {
				return &Error{PC: m.PC, Err: err}
			}
			m.Push(v)

		case opcode.RND:
			if seed, ok := m.rndSeedLiteral(); ok {
				runtime.SeedRnd(seed)
			}
			m.Push(runtime.Rnd())

		case opcode.ENCVALUE:
			v := m.popValue()
			bits, err := runtime.EncValue(v, nil)
			if err != nil {
				return &Error{PC: m.PC, Err: err}
			}
			m.Push(bits)

		case opcode.DECVALUE:
			decRef := m.Pop()
			encRef := m.Pop()
			encVal := m.deref(encRef)
			enc, ok := encVal.(runtime.Binary)
			if !ok {
				return &Error{PC: m.PC, Err: fmt.Errorf("decvalue: expected bitstring, got %T", encVal)}
			}
			var target any
			code, rest := runtime.DecValue(enc, m.exprType(decRef), &target)
			if code == 0 {
				m.store(decRef, target)
				if assignable(encRef) {
					m.store(encRef, rest)
				}
			}
			m.Push(code)

		case opcode.INT2ENUM:
			target := m.Pop()
			n := m.popValue().(int)
			typ := m.exprType(target)
			if v, ok := target.(*runtime.Var); ok && v.Type != nil {
				typ = v.Type
			}
			ev, err := runtime.Int2Enum(n, typ)
			if err != nil {
				return m.fault(k3log.ErrDOME, strconv.Itoa(n))
			}
			m.store(target, ev)

		case opcode.CAT:
			y := m.popValue()
			v, err := runtime.Cat(m.popValue(), y)
			if err != nil {
				return &Error{PC: m.PC, Err: err}
			}
			m.Push(v)

		case opcode.SHL, opcode.SHR, opcode.ROL, opcode.ROR:
			n := m.popValue().(int)
			v, err := runtime.Shift(m.popValue(), n, op == opcode.SHL || op == opcode.ROL, op == opcode.ROL || op == opcode.ROR)
			if err != nil {
				return &Error{PC: m.PC, Err: err}
			}
			m.Push(v)

		case opcode.XOR:
			y := m.popValue()
			switch x := m.popValue().(type) {
			case bool:
				m.Push(x != y.(bool))
			default:
				v, err := runtime.Bitwise(x.(runtime.Binary), y.(runtime.Binary), func(a, b uint64) uint64 { return a ^ b })
				if err != nil {
					return &Error{PC: m.PC, Err: err}
				}
				m.Push(v)
			}

		case opcode.ADD, opcode.SUB, opcode.MUL:
			y := m.popValue()
			x := m.popValue()
			if xf, ok := x.(float64); ok {
				yf := y.(float64)
				m.Push(map[opcode.Opcode]float64{opcode.ADD: xf + yf, opcode.SUB: xf - yf, opcode.MUL: xf * yf}[op])
				break
			}
			xi, yi := int64(x.(int)), int64(y.(int))
			z := map[opcode.Opcode]int64{opcode.ADD: xi + yi, opcode.SUB: xi - yi, opcode.MUL: xi * yi}[op]
			if z > math.MaxInt32 || z < math.MinInt32 {
				return &Error{PC: m.PC, Err: fmt.Errorf("%w (%d %s %d)", ErrOverflow, xi, op, yi)}
			}
			m.Push(int(z))

		case opcode.DIV, opcode.REM, opcode.MOD:
			yv := m.popValue()
			xv := m.popValue()
			if xf, ok := xv.(float64); ok {
				if yv.(float64) == 0 {
					return m.fault(k3log.DIV0, "(anon)")
				}
				m.Push(xf / yv.(float64))
				break
			}
			x, y := xv.(int), yv.(int)
			if y == 0 {
				return m.fault(k3log.DIV0, "(anon)")
			}
			z := x / y
			switch op {
			case opcode.DIV:
				if z > math.MaxInt32 || z < math.MinInt32 {
					return &Error{PC: m.PC, Err: fmt.Errorf("%w (%d / %d)", ErrOverflow, x, y)}
				}
			case opcode.REM:
				z = x % y
			default:
				z = x % y
				if z < 0 {
					ay := y
					if ay < 0 {
						ay = -ay
					}
					z += ay
				}
			}
			m.Push(z)

		case opcode.RANGE:
			hi := m.popValue()
			lo := m.popValue()
			m.Push(runtime.Range{Lo: lo, Hi: hi})

		case opcode.LT, opcode.GT, opcode.LE, opcode.GE:
			y, _ := toFloat(m.popValue())
			x, _ := toFloat(m.popValue())
			m.Push(map[opcode.Opcode]bool{opcode.LT: x < y, opcode.GT: x > y, opcode.LE: x <= y, opcode.GE: x >= y}[op])

		case opcode.EQ, opcode.NE:
			y := m.popValue()
			x := m.popValue()
			m.Push(equal(x, y) == (op == opcode.EQ))

		case opcode.INTEGER:
			m.Push(runtime.IntegerType)
		case opcode.BOOLEAN:
			m.Push(runtime.BooleanType)
		case opcode.BITSTRING:
			m.Push(runtime.BitstringType)
		case opcode.HEXSTRING:
			m.Push(runtime.HexstringType)
		case opcode.OCTETSTRING:
			m.Push(runtime.OctetstringType)
		case opcode.CHARSTRING:
			m.Push(runtime.CharstringType)
		case opcode.CHARSTRINGU:
			m.Push(runtime.UniversalCharstringType)
		case opcode.FLOAT:
			m.Push(runtime.FloatType)
		case opcode.TIMER:
			m.Push(runtime.TimerType)
		case opcode.VERDICTTYPE:
			m.Push(runtime.VerdictType)
		case opcode.DEFAULT:
			m.Push(runtime.DefaultType)
		case opcode.CLOSURETYPE:
			m.Push(runtime.Closure)

		case opcode.PERMITO, opcode.PERMITP, opcode.PERMITT:
			var flags runtime.Flags
			if op == opcode.PERMITO {
				flags = runtime.Omit
			} else if op == opcode.PERMITP {
				flags = runtime.Present
			}
			m.Push(runtime.TemplateType{
				BaseType: m.Pop().(runtime.Type),
				Flags:    flags,
			})

		case opcode.ARRAY:
			length := m.Pop()
			elem := m.Pop().(runtime.Type)
			m.Push(runtime.ArrayType{ElementType: elem, Length: length})

		case opcode.PASS:
			m.Push(runtime.VerdictPass)
		case opcode.FAIL:
			m.Push(runtime.VerdictFail)
		case opcode.NONE:
			m.Push(runtime.VerdictNone)
		case opcode.INCONC:
			m.Push(runtime.VerdictInconc)
		case opcode.ERROR:
			m.Push(runtime.VerdictError)

		case opcode.SETVERDICT:
			v, reason := m.verdictArg(m.Pop())
			prev := m.self.SetVerdict(v)
			comp, pc := m.origin()
			fields := []string{string(prev), string(v)}
			if reason != "" {
				fields = append(fields, reason)
			}
			if err := m.event(k3log.SETV, comp, pc, fields...); err != nil {
				return &Error{PC: m.PC, Err: err}
			}

		case opcode.CREATE, opcode.CREATEN:
			if m.tc == nil {
				panic(fmt.Errorf("create outside a testcase"))
			}
			typ := m.Pop().(runtime.Type)
			name := ""
			unnamed := true
			if op == opcode.CREATEN {
				name = m.popValue().(string)
				unnamed = false
			}
			ptc, err := m.tc.create(m, typ, name, unnamed)
			if err != nil {
				return err
			}
			m.log(k3log.COCR, ptc.self.Name, "once")
			m.Push(ptc.self)

		case opcode.STARTC:
			callPC := m.PC
			nextPC := m.PC + n
			comp := m.popValue().(*runtime.Component)
			blk := m.Pop().(Block)
			fn, args := m.callee(blk)
			m.PC, n = nextPC, 0
			m.startBehaviour(comp, fn, args, callPC)

		case opcode.DONE:
			comp := m.popValue().(*runtime.Component)
			outcome := "wait"
			switch {
			case m.componentDone(comp):
				outcome = "ready"
			case !comp.Started():
				outcome = "never"
				m.altNever = true
			default:
				m.altWait = true
			}
			who, pc := m.origin()
			if err := m.event(k3log.CODO, who, pc, comp.Name, outcome); err != nil {
				return &Error{PC: m.PC, Err: err}
			}
			m.Push(outcome == "ready")

		case opcode.SPECPLC:
			blk := m.Pop().(Block)
			if err := m.runBlock(blk); err != nil {
				return err
			}
			if m.stopped() {
				return nil
			}

		case opcode.STEP:
			sblk := m.Pop().(Block)
			cblk := m.Pop().(Block)
			if m.chosen != nil || m.taken[m.PC] {
				break
			}
			m.open++
			if err := m.runBlock(cblk); err != nil {
				return err
			}
			// A stop inside the guard returns before the bool is pushed.
			if m.stopped() {
				return nil
			}
			if m.popValue().(bool) {
				m.chosen, m.chosenPC = &sblk, m.PC
			}

		case opcode.ELSE:
			blk := m.Pop().(Block)
			if m.chosen == nil {
				m.chosen = &blk
				m.log(k3log.ELSE, "ready")
			}

		case opcode.ALT, opcode.ALTND, opcode.INTERLEAVE:
			if err := m.runAlt(m.Pop().(Block), op); err != nil {
				return err
			}

		case opcode.REPEAT:
			m.log(k3log.ALRP)
			return errRepeat

		case opcode.BREAK:
			return errBreak

		case opcode.ACTIVATE:
			fn, args := m.callee(m.Pop().(Block))
			d := &runtime.Default{Fn: fn, Args: args, Active: true}
			m.self.Defaults = append(m.self.Defaults, d)
			m.log(k3log.DTAC, m.signature(fn, args, false))
			m.Push(d)

		case opcode.DEACTIVATE, opcode.DEACTIVATEA:
			ds := m.self.Defaults
			if op == opcode.DEACTIVATE {
				d, ok := m.popValue().(*runtime.Default)
				if !ok {
					break
				}
				ds = []*runtime.Default{d}
			}
			for _, d := range ds {
				if d.Active {
					d.Active = false
					m.self.Defaults = slices.DeleteFunc(m.self.Defaults, func(x *runtime.Default) bool { return x == d })
					m.log(k3log.DTDE, m.signature(d.Fn, d.Args, false))
				}
			}

		case opcode.CONNECT, opcode.DISCONNECT:
			if m.tc == nil {
				return m.fault(k3log.ErrINOP, op.String()+" outside a testcase")
			}
			p2, err := m.port(m.Pop())
			if err != nil {
				return err
			}
			p1, err := m.port(m.Pop())
			if err != nil {
				return err
			}
			if op == opcode.CONNECT {
				m.tc.ports.Connect(p1, p2, false)
				m.log(k3log.PTCN, p1.String(), p2.String())
			} else {
				m.tc.ports.Disconnect(p1, p2, false)
				m.log(k3log.PTDI, p1.String(), p2.String())
			}

		case opcode.MAP, opcode.UNMAP:
			if m.tc == nil {
				return m.fault(k3log.ErrINOP, op.String()+" outside a testcase")
			}
			m.Pop() // map/unmap actual parameter list
			p2, err := m.port(m.Pop())
			if err != nil {
				return err
			}
			p1, err := m.port(m.Pop())
			if err != nil {
				return err
			}
			if op == opcode.MAP {
				m.tc.ports.Connect(p1, p2, true)
				m.log(k3log.PTMP, p1.String(), p2.String())
			} else {
				m.tc.ports.Disconnect(p1, p2, true)
				m.log(k3log.PTUN, p1.String(), p2.String())
			}

		case opcode.DISCONNECTA, opcode.UNMAPA:
			if m.tc == nil {
				return m.fault(k3log.ErrINOP, op.String()+" outside a testcase")
			}
			if op == opcode.UNMAPA {
				m.Pop() // unmap actual parameter list
			}
			opnd := m.Pop()
			var name string
			switch value := m.deref(opnd).(type) {
			case *runtime.Port:
				name = value.String()
				m.tc.ports.DisconnectPort(value, op == opcode.UNMAPA)
			case *runtime.Component:
				name = value.Name
				m.tc.ports.DisconnectComponent(value, op == opcode.UNMAPA)
			default:
				return m.fault(k3log.ErrINOP, fmt.Sprintf("%s on %T", op, value))
			}
			if op == opcode.UNMAPA {
				m.log(k3log.PTUN, name)
			} else {
				m.log(k3log.PTDI, name)
			}

		case opcode.DISCONNECTAA, opcode.UNMAPAA:
			if m.tc == nil {
				return m.fault(k3log.ErrINOP, op.String()+" outside a testcase")
			}
			m.tc.ports.DisconnectAll(op == opcode.UNMAPAA)
			if op == opcode.UNMAPAA {
				m.log(k3log.PTUN, "all")
			} else {
				m.log(k3log.PTDI, "all")
			}

		case opcode.START, opcode.STARTD:
			opnd := m.Pop()
			if op == opcode.START {
				if port, ok := m.deref(opnd).(*runtime.Port); ok {
					m.tc.ports.Start(port)
					m.log(k3log.PTST, port.String())
					break
				}
			}
			t := m.timer(opnd)
			d, ok := t.Default.(float64)
			if op == opcode.STARTD {
				d, ok = m.popValue().(float64), true
			}
			if !ok {
				return m.fault(k3log.ErrUNDF, t.Name)
			}
			if t.Start(d) != nil {
				return m.fault(k3log.ErrCONV, "timer", "timer", "timeout value", formatValue(d, nil))
			}
			m.log(k3log.TMST, t.Name, runtime.FormatFloat(d)+"s")

		case opcode.STOP:
			opnd := m.Pop()
			if c, ok := m.deref(opnd).(*runtime.Component); ok {
				c.Stop()
				if m.tc != nil {
					m.tc.notify()
				}
				break
			}
			if port, ok := m.deref(opnd).(*runtime.Port); ok {
				m.tc.ports.Stop(port)
				m.log(k3log.PTSP, port.String())
				break
			}
			if t := m.timer(opnd); t.Stop() {
				m.log(k3log.TMSP, t.Name)
			}

		case opcode.STARTAP, opcode.STOPAP:
			ports, err := m.ports()
			if err != nil {
				return err
			}
			for _, port := range ports {
				if op == opcode.STARTAP {
					m.tc.ports.Start(port)
				} else {
					m.tc.ports.Stop(port)
				}
			}
			if op == opcode.STARTAP {
				m.log(k3log.PTST, "(all)")
			} else {
				m.log(k3log.PTSP, "(all)")
			}

		case opcode.STOPAT:
			for _, t := range m.self.Timers {
				t.Stop()
			}
			m.log(k3log.TMSP, "(all)")

		case opcode.READ:
			t := m.timer(m.Pop())
			r := t.Read()
			m.log(k3log.TMRD, t.Name, runtime.FormatFloat(r)+"s")
			m.Push(r)

		case opcode.RUNNING:
			opnd := m.Pop()
			if c, ok := m.deref(opnd).(*runtime.Component); ok {
				m.Push(c.Started() && !m.componentDone(c))
				break
			}
			t := m.timer(opnd)
			r := t.RunningAt(m.now())
			m.log(k3log.TMRU, t.Name, formatValue(r, nil))
			m.Push(r)

		case opcode.RUNNING1T:
			now := m.now()
			r := slices.ContainsFunc(m.self.Timers, func(t *runtime.Timer) bool { return t.RunningAt(now) })
			m.log(k3log.TMRU, "(any)", formatValue(r, nil))
			m.Push(r)

		case opcode.CLEAR, opcode.HALT:
			port, err := m.port(m.Pop())
			if err != nil {
				return err
			}
			if op == opcode.CLEAR {
				m.tc.ports.Clear(port)
				m.log(k3log.PTCL, port.String())
			} else {
				m.tc.ports.Halt(port)
				m.log(k3log.PTHA, port.String())
			}

		case opcode.CLEARA, opcode.HALTA:
			ports, err := m.ports()
			if err != nil {
				return err
			}
			for _, port := range ports {
				if op == opcode.CLEARA {
					m.tc.ports.Clear(port)
				} else {
					m.tc.ports.Halt(port)
				}
			}
			if op == opcode.CLEARA {
				m.log(k3log.PTCL, "(all)")
			} else {
				m.log(k3log.PTHA, "(all)")
			}

		case opcode.CHECKSTATE, opcode.CHECKSTATEAL, opcode.CHECKSTATEAN:
			state, ok := m.popValue().(string)
			if !ok {
				return m.fault(k3log.ErrCONV, "port", "charstring", "state", formatValue(state, nil))
			}
			var ports []*runtime.Port
			if op == opcode.CHECKSTATE {
				port, err := m.port(m.Pop())
				if err != nil {
					return err
				}
				ports = []*runtime.Port{port}
			} else {
				var err error
				ports, err = m.ports()
				if err != nil {
					return err
				}
			}
			result, err := m.tc.ports.CheckState(ports, state, op == opcode.CHECKSTATEAN)
			if err != nil {
				return m.fault(k3log.ErrCONV, "port", "charstring", "state", runtime.Quote(state))
			}
			m.Push(result)

		case opcode.SEND, opcode.SEND1, opcode.SENDN, opcode.SENDA:
			port, err := m.port(m.Pop())
			if err != nil {
				return err
			}
			messageOperand := m.Pop()
			message := m.deref(messageOperand)
			typ := m.exprType(messageOperand)
			if cat, name := m.checkDefined(message, typ, m.operandName(messageOperand), false, false); cat != "" {
				return m.fault(cat, name)
			}
			var destinations []any
			all := op == opcode.SENDA
			switch op {
			case opcode.SEND1:
				destinations = []any{m.popValue()}
			case opcode.SENDN:
				switch values := m.popValue().(type) {
				case runtime.ValueList:
					destinations = []any(values)
				case []any:
					destinations = values
				default:
					destinations = []any{values}
				}
			}
			item := runtime.Message{Value: message, Type: typ}
			deliveries, sendErr := m.tc.ports.Send(port, item, destinations, all)
			if sendErr != nil {
				return m.fault(k3log.ErrCONE, port.String(), "-", sendErr.Error())
			}
			typeName := m.typeName(messageOperand)
			if typeName == "" {
				typeName = valueTypeName(message)
			}
			m.log(k3log.PTSD, port.String(), "-", typeName, formatValue(message, typ))
			queued := false
			for _, delivery := range deliveries {
				if delivery.Queued {
					queued = true
					m.log(k3log.PTQU, delivery.Port.String(), "message "+formatValue(delivery.Message.Value, delivery.Message.Type))
				} else {
					m.log(k3log.PTDS, delivery.Port.String(), "message "+formatValue(delivery.Message.Value, delivery.Message.Type), delivery.Reason)
				}
			}
			if queued {
				m.tc.notify()
			}

		case opcode.RECEIVE, opcode.RECEIVEC, opcode.RECEIVE1, opcode.RECEIVEC1,
			opcode.TRIGGER, opcode.TRIGGER1, opcode.CHECK, opcode.CHECK1:
			if err := m.portComm(op); err != nil {
				return err
			}

		case opcode.TIMEOUT:
			t := m.timer(m.Pop())
			ready, running, deadline := t.TimeoutAt(m.now())
			outcome := ""
			if ready {
				outcome = "ready"
			}
			m.Push(m.timeout(t.Name, outcome, running, deadline))

		case opcode.TIMEOUT1:
			var outcome string
			var running bool
			var deadline time.Time
			if len(m.self.Timers) == 0 {
				m.log(k3log.TMTO, "(any)", "never(none)")
				m.Push(false)
				break
			}
			now := m.now()
			for _, t := range m.self.Timers {
				ready, r, d := t.TimeoutAt(now)
				if ready {
					outcome = "ready(" + t.Name + ")"
					break
				}
				if r && (!running || d.Before(deadline)) {
					running, deadline = true, d
				}
			}
			m.Push(m.timeout("(any)", outcome, running, deadline))

		case opcode.NULL:
			m.Push(runtime.Null{})

		case opcode.SELF:
			m.Push(m.self)

		case opcode.MTC, opcode.SYSTEM:
			if m.tc == nil {
				return m.fault(k3log.ErrINOP, op.String()+" outside a testcase")
			}
			c := m.tc.mtc
			if op == opcode.SYSTEM {
				c = cmp.Or(m.tc.system, m.tc.mtc)
			}
			m.Push(c)

		case opcode.GETVERDICT:
			v := m.self.Verdict()
			m.log(k3log.GETV, string(v))
			m.Push(v)

		case opcode.ACTION:
			m.log(k3log.UACT, m.logText(m.Pop()))

		case opcode.STOPI:
			m.self.Stop()
			if m.tc != nil {
				m.tc.notify()
			}
			return nil

		case opcode.TCSTOP:
			return m.fault(k3log.ErrTSTP, m.logText(m.Pop()))

		case opcode.EXECUTE, opcode.EXECUTEL:
			callPC := m.PC
			blk := m.Pop().(Block)
			guard := "unlimited"
			if op == opcode.EXECUTEL {
				guard = formatValue(m.popValue().(float64), nil)
			}
			nextPC := m.PC + n
			fn, args := m.callee(blk)
			v, err := m.execute(fn, args, guard, callPC)
			if err != nil {
				return err
			}
			m.Push(v)
			m.PC, n = nextPC, 0

		default:
			return &Error{PC: m.PC, Err: fmt.Errorf("%w (%s)", ErrInvalidInstruction, op.String())}

		}
		// A RETURN in a nested block has already moved PC into the caller.
		if len(m.Frames) < depth && op != opcode.RETURN {
			continue
		}
		m.PC += n
	}
}

// localVar registers the declaration at m.PC once. Two machines running the
// same instruction share the descriptor; the value lives on each machine's
// stack, or in its Globals for a component variable.
func (m *Machine) localVar(name string, typ runtime.Type, index int) *runtime.Var {
	localsMu.Lock()
	defer localsMu.Unlock()
	if m.locals == nil {
		m.locals = make(map[int]*runtime.Var)
	}
	if v, ok := m.locals[m.PC]; ok {
		return v
	}
	flags := runtime.Member
	if m.Fn.Flags&runtime.Member == 0 {
		flags, index = runtime.Local, len(m.Fn.Parameters)+index
	}
	v := &runtime.Var{
		Flags: flags,
		Name:  name,
		Type:  typ,
		Index: index,
	}
	m.locals[m.PC] = v
	return v
}

// callee runs blk up to its final REF. That REF names the function. The
// arguments sit under it, last on top, and are returned dereferenced.
// A run error is panicked so the outer run can recover it.
func (m *Machine) callee(blk Block) (*runtime.Function, []any) {
	refPC := blk.End - 4
	if blk.Begin+4 < refPC {
		m.PC = blk.Begin + 4
		if err := m.run(refPC); err != nil {
			panic(err)
		}
	}
	_, op, arg := t3xf.Decode(m.Code[refPC:])
	if op != opcode.REF {
		panic(fmt.Errorf("expected REF, got %s", op))
	}
	addr := int(arg.(t3xf.Reference))
	fn := m.Tags[addr>>2].(*runtime.Function)
	args := make([]any, len(fn.Parameters))
	for i := len(args) - 1; i >= 0; i-- {
		args[i] = m.popValue()
	}
	return fn, args
}

var (
	errRepeat = errors.New("repeat")
	errBreak  = errors.New("break")
)

// runAlt evaluates the steps in blk until one matches, no step can match,
// or the component is stopped. A step that can never match, with nothing
// left that might, is a deadlock. op is ALT, ALTND (no defaults), or
// INTERLEAVE, which repeats until each of its steps has matched once.
func (m *Machine) runAlt(blk Block, op opcode.Opcode) error {
	savedW, savedN, savedWake, savedTaken, savedSnap := m.altWait, m.altNever, m.wake, m.taken, m.snap
	defer func() {
		m.altWait, m.altNever, m.wake, m.taken, m.snap = savedW, savedN, savedWake, savedTaken, savedSnap
	}()

	enter, leave, wait := k3log.ALEN, k3log.ALLV, k3log.ALWT
	m.taken = nil
	if op == opcode.INTERLEAVE {
		enter, leave, wait = k3log.ILEN, k3log.ILLV, k3log.WAIT
		m.taken = map[int]bool{}
	}
	frame, depth := m.Frame, len(m.Frames)
	who, at := m.origin()
	if at != k3log.NoPos {
		at = blk.Begin
	}
	if err := m.event(enter, who, at); err != nil {
		return &Error{PC: m.PC, Err: err}
	}
	for {
		// Subscribe before reading Stopped. notify closes the channel
		// wait just returned; a later channel is not the one stop closes.
		var w <-chan struct{}
		if m.tc != nil {
			w = m.tc.wait()
		}
		if m.stopped() {
			return nil
		}
		m.chosen, m.open = nil, 0
		m.altWait, m.altNever, m.wake = false, false, time.Time{}
		if m.tc != nil {
			m.tc.ports.TakeSnapshot(m.self)
		}
		m.snap = m.takeSnapshot()
		err := m.runBlock(blk)
		if err == nil && m.chosen == nil && op == opcode.ALT {
			err = m.tryDefaults()
		}
		// The body of the alternative taken runs after the snapshot.
		m.snap = nil
		if err == nil && m.chosen != nil {
			if m.taken != nil {
				m.taken[m.chosenPC] = true
			}
			err = m.runBlock(*m.chosen)
		}
		if errors.Is(err, errRepeat) || errors.Is(err, errBreak) {
			for len(m.Frames) > depth {
				leaving, args := m.popFrame()
				if leaving.leave != "" {
					m.log(leaving.leave, m.signature(leaving.Fn, args, true))
				}
			}
			m.Frame = frame
		}
		switch {
		case errors.Is(err, errRepeat):
			continue
		case errors.Is(err, errBreak):
			m.chosen = &Block{}
		case err != nil:
			return err
		}
		if m.stopped() {
			return nil
		}
		if m.chosen != nil && op == opcode.INTERLEAVE && m.open > 1 {
			continue
		}
		if m.chosen != nil || !m.altWait || op == opcode.INTERLEAVE && m.open == 0 {
			if err := m.event(leave, who, at); err != nil {
				return &Error{PC: m.PC, Err: err}
			}
			if m.chosen == nil && m.altNever {
				return m.fault(k3log.ErrDEAD)
			}
			return nil
		}
		var timeout <-chan time.Time
		wake := "maximum"
		if !m.wake.IsZero() {
			wake = m.wake.Format(stampLayout)
			timeout = time.After(time.Until(m.wake))
		}
		m.log(wait, wake)
		select {
		case <-w:
		case <-timeout:
		}
		if m.stopped() {
			return nil
		}
	}
}

// stampLayout is the k3log timestamp format, also used for timer deadlines.
const stampLayout = "20060102T150405.000000"

// tryDefaults evaluates the active defaults, newest first, until one
// matches. A matching default has already run its step body.
func (m *Machine) tryDefaults() error {
	for i := len(m.self.Defaults) - 1; i >= 0 && m.chosen == nil; i-- {
		d := m.self.Defaults[i]
		m.asDefault = true
		v, err := m.call(d.Fn, d.Args...)
		if err != nil {
			return err
		}
		if v == true {
			m.chosen = &Block{}
		}
	}
	return nil
}

func isPortValueType(typ runtime.Type) bool {
	switch typ := runtime.BaseType(typ).(type) {
	case runtime.PortType, *runtime.PortType:
		return true
	case runtime.ArrayType:
		return isPortValueType(typ.ElementType)
	case *runtime.ArrayType:
		return isPortValueType(typ.ElementType)
	}
	return false
}

func (m *Machine) newPorts(name string, typ runtime.Type) (any, bool) {
	if !isPortValueType(typ) {
		return nil, false
	}
	switch base := runtime.BaseType(typ).(type) {
	case runtime.PortType, *runtime.PortType:
		return runtime.NewPort(m.self, name, typ), true
	case runtime.ArrayType:
		_, n := m.arrayBounds(typ)
		values := make([]any, n)
		for i := range values {
			values[i], _ = m.newPorts(fmt.Sprintf("%s[%d]", name, i), base.ElementType)
		}
		return values, true
	case *runtime.ArrayType:
		_, n := m.arrayBounds(typ)
		values := make([]any, n)
		for i := range values {
			values[i], _ = m.newPorts(fmt.Sprintf("%s[%d]", name, i), base.ElementType)
		}
		return values, true
	}
	return nil, false
}

func (m *Machine) port(opnd any) (*runtime.Port, error) {
	port, ok := m.deref(opnd).(*runtime.Port)
	if !ok {
		return nil, m.fault(k3log.ErrINOP, fmt.Sprintf("port operation on %T", m.deref(opnd)))
	}
	return port, nil
}

func (m *Machine) ports() ([]*runtime.Port, error) {
	if m.tc == nil {
		return nil, m.fault(k3log.ErrINOP, "port operation outside a testcase")
	}
	return m.tc.ports.Ports(m.self), nil
}

// timer returns the timer opnd refers to. A declared timer holds its
// default duration until it is first used.
func (m *Machine) timer(opnd any) *runtime.Timer {
	if t, ok := m.deref(opnd).(*runtime.Timer); ok {
		return t
	}
	t := &runtime.Timer{Name: m.operandName(opnd), Default: m.deref(opnd)}
	m.store(opnd, t)
	m.self.Timers = append(m.self.Timers, t)
	return t
}

func (m *Machine) replacingTimer(old, next any) {
	timer, ok := old.(*runtime.Timer)
	if !ok {
		return
	}
	if replacement, ok := next.(*runtime.Timer); ok && replacement == timer {
		return
	}
	m.removeTimer(timer)
}

// removeTimer stops t and removes it from the component's active timers.
func (m *Machine) removeTimer(t *runtime.Timer) {
	t.Stop()
	m.self.Timers = slices.DeleteFunc(m.self.Timers, func(other *runtime.Timer) bool {
		return other == t
	})
}

// timeout logs the outcome of a timeout step on the timers called name.
// ready is the outcome when one has expired, "" otherwise.
func (m *Machine) timeout(name, ready string, running bool, deadline time.Time) bool {
	switch {
	case ready != "":
		m.log(k3log.TMTO, name, ready)
	case running:
		m.altWait = true
		if m.wake.IsZero() || deadline.Before(m.wake) {
			m.wake = deadline
		}
		m.log(k3log.TMTO, name, "wait("+deadline.Format(stampLayout)+")")
	default:
		m.altNever = true
		outcome := "never(inactive)"
		if name == "(any)" {
			outcome = "never(all inactive)"
		}
		m.log(k3log.TMTO, name, outcome)
	}
	return ready != ""
}

func (m *Machine) portComm(op opcode.Opcode) error {
	anyPort := op == opcode.RECEIVE1 || op == opcode.RECEIVEC1 || op == opcode.TRIGGER1 || op == opcode.CHECK1
	var selected *runtime.Port
	var arrayName string
	if !anyPort {
		var err error
		selected, err = m.port(m.Pop())
		if err != nil {
			return err
		}
	} else if m.SP > 0 {
		if marker, ok := m.Stack[m.SP-1].(string); ok && strings.HasPrefix(marker, "__port_from:") {
			m.SP--
			arrayName = strings.TrimPrefix(marker, "__port_from:")
		}
	}
	ablk := m.Pop().(Block)
	mblk := m.Pop().(Block)

	var ports []*runtime.Port
	if selected != nil {
		ports = []*runtime.Port{selected}
	} else {
		var err error
		ports, err = m.ports()
		if err != nil {
			return err
		}
		if arrayName != "" {
			ports = slices.DeleteFunc(ports, func(port *runtime.Port) bool {
				return port.Name != arrayName && !strings.HasPrefix(port.Name, arrayName+"[")
			})
		}
	}

	type candidate struct {
		port    *runtime.Port
		message runtime.Message
	}
	var queued []candidate
	waiting := false
	for _, port := range ports {
		if message, ok := m.tc.ports.Snapshot(port); ok {
			queued = append(queued, candidate{port, message})
		} else if m.tc.ports.Status(port) == runtime.PortStarted {
			waiting = true
		}
	}

	for _, candidate := range queued {
		context := &commContext{port: candidate.port, message: candidate.message}
		matched, err := m.runCommBlock(context, mblk, false)
		if err != nil {
			return err
		}
		if !matched {
			if op == opcode.TRIGGER || op == opcode.TRIGGER1 {
				m.tc.ports.Consume(candidate.port)
				m.log(k3log.PTDS, candidate.port.String(), "message "+formatValue(candidate.message.Value, candidate.message.Type), "trigger mismatch")
				m.logPortComm(op, candidate.port.String(), "repeat", false)
				m.log(k3log.ALRP)
				return errRepeat
			}
			continue
		}
		if _, err := m.runCommBlock(context, ablk, true); err != nil {
			return err
		}
		if op == opcode.RECEIVE || op == opcode.RECEIVE1 || op == opcode.TRIGGER || op == opcode.TRIGGER1 {
			m.tc.ports.Consume(candidate.port)
		}
		m.logPortComm(op, candidate.port.String(), "ready", false)
		m.Push(true)
		return nil
	}

	name := "(any)"
	if selected != nil {
		name = selected.String()
	}
	switch {
	case waiting:
		m.altWait = true
		m.logPortComm(op, name, "wait", len(queued) == 0)
	default:
		m.altNever = true
		m.logPortComm(op, name, "never", len(queued) == 0)
	}
	m.Push(false)
	return nil
}

func (m *Machine) runCommBlock(context *commContext, block Block, copyValue bool) (bool, error) {
	if block.Begin >= block.End {
		return true, nil
	}
	context.copyValue = copyValue
	saved := m.comm
	m.comm = context
	err := m.runBlock(block)
	m.comm = saved
	if err != nil {
		return false, err
	}
	if copyValue {
		return true, nil
	}
	value, ok := m.popValue().(bool)
	if !ok {
		return false, fmt.Errorf("communication match block did not return boolean")
	}
	return value, nil
}

func (m *Machine) logPortComm(op opcode.Opcode, name, outcome string, empty bool) {
	switch op {
	case opcode.RECEIVE, opcode.RECEIVE1:
		if empty {
			m.log(k3log.PTRE, name, "-")
		} else {
			m.log(k3log.PTRX, name, "-", outcome)
		}
	case opcode.TRIGGER, opcode.TRIGGER1:
		m.log(k3log.PTTR, name, "-", outcome)
	default:
		m.log(k3log.PTCK, name, "-", outcome)
	}
}

// fault reports a TTCN-3 runtime error at m.PC and aborts this behaviour.
func (m *Machine) fault(cat k3log.Category, fields ...string) error {
	if m.tc != nil {
		who, pc := m.origin()
		if err := m.event(cat, who, pc, fields...); err != nil {
			return &Error{PC: m.PC, Err: err}
		}
		if m.self != nil {
			m.self.SetVerdict(runtime.VerdictError)
		}
		if frames := m.backtrace(); len(frames) > 0 {
			if err := m.event(k3log.BCTR, who, k3log.NoPos, frames...); err != nil {
				return &Error{PC: m.PC, Err: err}
			}
		}
	}
	return &Error{PC: m.PC, Err: fmt.Errorf("%w (%s)", errFault, cat.ID())}
}

// backtrace lists invocation sites, innermost first. Each frame is
// "file:line:name()". The outermost frame is the execute or startc.
func (m *Machine) backtrace() []string {
	var frames []string
	if len(m.Frames) > 0 && m.Fn != nil {
		frames = append(frames, m.traceFrame(m.Frames[len(m.Frames)-1].PC, m.Fn.Name))
		for i := len(m.Frames) - 1; i > 0; i-- {
			frames = append(frames, m.traceFrame(m.Frames[i-1].PC, m.Frames[i].Fn.Name))
		}
	}
	root := m.Fn
	if len(m.Frames) > 0 {
		root = m.Frames[0].Fn
	}
	if root != nil && m.callPC >= 0 {
		frames = append(frames, m.traceFrame(m.callPC, root.Name))
	}
	return frames
}

func (m *Machine) traceFrame(pc int, name string) string {
	name = name[strings.LastIndex(name, ".")+1:]
	return m.Pos(pc) + ":" + name + "()"
}

// execute runs testcase tc on a new MTC and returns its final verdict.
// callPC is the execute instruction in the control part.
func (m *Machine) execute(tc *runtime.Function, args []any, guard string, callPC int) (runtime.Verdict, error) {
	args = cloneSlice(args)
	var err error
	emit := func(cat k3log.Category, comp string, pc int, fields ...string) {
		if err == nil {
			err = m.event(cat, comp, pc, fields...)
		}
	}
	emit(k3log.TCST, "k3r", k3log.NoPos, tc.Name, guard)
	mtc := m.spawn(runtime.NewComponent("MTC"))
	mtc.tc = &testcase{
		mtc:   mtc.self,
		ptcs:  map[*runtime.Component]*Machine{},
		ports: runtime.NewPortSet(m.PortAdapter),
		ch:    make(chan struct{}),
	}
	if err == nil {
		err = mtc.initComponent(m, tc.RunsOn)
	}
	emit(k3log.COCR, "k3r", k3log.NoPos, "MTC", "once")
	if tc.System != nil {
		sys := m.spawn(runtime.NewComponent("SYSTEM"))
		sys.tc = mtc.tc
		if err == nil {
			err = sys.initComponent(m, tc.System)
		}
		emit(k3log.COCR, "k3r", k3log.NoPos, "SYSTEM", "once")
		mtc.tc.system = sys.self
	}
	mtc.self.Start()
	mtc.callPC = callPC
	emit(k3log.COST, "MTC", tc.Body.(Block).Begin, tc.Name)
	emit(k3log.TCEN, "MTC", k3log.NoPos, mtc.signature(tc, args, false))
	if err != nil {
		return "", err
	}
	runErr := mtc.Execute(tc, args...)
	mtc.tc.stopAll()
	mtc.tc.notify()
	mtc.tc.wg.Wait()
	if errors.Is(runErr, errFault) {
		emit(k3log.TCFI, "k3r", k3log.NoPos, tc.Name, string(runtime.VerdictError))
		return runtime.VerdictError, err
	}
	if runErr != nil {
		return "", runErr
	}
	v := mtc.self.Verdict()
	if !mtc.self.Stopped() {
		emit(k3log.TCLV, "MTC", k3log.NoPos, mtc.signature(tc, args, true))
	}
	emit(k3log.COFI, "MTC", k3log.NoPos, string(v))
	emit(k3log.TCFI, "k3r", k3log.NoPos, tc.Name, string(v))
	return v, err
}

func (m *Machine) logOut() io.Writer {
	if m.Out != nil {
		return m.Out
	}
	return os.Stdout
}

// event raises a k3log event from component at code offset pc.
func (m *Machine) event(cat k3log.Category, component string, pc int, fields ...string) error {
	eventMu.Lock()
	defer eventMu.Unlock()
	return k3log.WriteEvent(m.logOut(), m.Image, cat, component, pc, fields...)
}

// origin returns the component raising events and the code offset reported
// with them. A machine inside a testcase reports its component and m.PC.
func (m *Machine) origin() (component string, pc int) {
	if m.creator != nil {
		return m.creator.origin()
	}
	if m.tc == nil {
		return "k3r", k3log.NoPos
	}
	return m.self.Name, m.PC
}

// logText formats the operand of log, action, or a reason. The items of a
// collected list are concatenated.
func (m *Machine) logText(obj any) string {
	coll, ok := obj.(logCollection)
	if !ok {
		coll = logCollection{obj}
	}
	var b strings.Builder
	for _, item := range coll {
		b.WriteString(formatValue(m.deref(item), m.exprType(item)))
	}
	return b.String()
}

// verdictArg reads a setverdict operand. A collected list is the verdict
// followed by the reason text.
func (m *Machine) verdictArg(obj any) (runtime.Verdict, string) {
	if x, ok := obj.(logCollection); ok {
		return m.deref(x[0]).(runtime.Verdict), m.logText(x[1:])
	}
	return m.deref(obj).(runtime.Verdict), ""
}

// binaryLiteral decodes the payload of a bits, nibbles, or octets
// instruction. The first element is in the high bits.
func binaryLiteral(unit byte, s *t3xf.String) runtime.Binary {
	n := s.Len()
	if unit == 'B' {
		var bits strings.Builder
		for _, c := range s.Bytes() {
			fmt.Fprintf(&bits, "%08b", c)
		}
		return runtime.Binary{Unit: unit, Digits: bits.String()[:n]}
	}
	if unit == 'O' {
		n *= 2
	}
	return runtime.Binary{Unit: unit, Digits: strings.ToUpper(hex.EncodeToString(s.Bytes()))[:n]}
}

// typeName names the type of an assignment target as vach reports it: an
// element of a named type is "T.field" or "T[-]", an element of a string or
// an anonymous type is named by its own type.
func (m *Machine) typeName(opnd any) string {
	named := func(t runtime.Type) string {
		if tt, ok := t.(runtime.TemplateType); ok {
			t = tt.BaseType
		}
		var name string
		for {
			n, ok := t.(*runtime.NamedType)
			if !ok {
				return name
			}
			name, t = n.Name, n.Type
		}
	}
	if v, ok := opnd.(*runtime.Var); ok && v.Flags&runtime.Inout != 0 {
		return m.typeName(m.Stack[m.BP+v.Index])
	}
	ref, ok := opnd.(*ElementRef)
	if !ok {
		return named(m.exprType(opnd))
	}
	if mapTyp := m.exprType(ref.Container); mapTyp != nil {
		if _, ok := runtime.MapValueType(mapTyp); ok {
			if name := named(mapTyp); name != "" {
				return name + ".to[-]"
			}
		}
	}
	base := m.exprType(ref.Container)
	fields := runtime.RecordFields(base)
	if fields == nil {
		fields = runtime.UnionFields(base)
	}
	if f := fieldByIndex(fields, m.deref(ref.Index)); f != nil {
		if name := named(base); name != "" {
			return name + "." + f.Name
		}
		parent := m.typeName(ref.Container)
		if parent != "" {
			return parent + "." + f.Name
		}
		return named(m.exprType(ref))
	}
	if name := named(base); name != "" {
		if _, ok := runtime.ElemType(base); ok {
			return name + "[-]"
		}
		return cmp.Or(named(m.exprType(ref)), name)
	}
	parent := m.typeName(ref.Container)
	if parent == "" {
		return named(m.exprType(ref))
	}
	return parent + "[-]"
}

// checkElement faults when ref reads into an unbound or omitted value, or
// past the end of a list.
func (m *Machine) checkElement(ref *ElementRef) error {
	name := m.operandName(ref.Container)
	switch c := m.deref(ref.Container).(type) {
	case nil, runtime.Undefined:
		return m.fault(k3log.ErrUNDF, name)
	case runtime.TemplateLiteral:
		if c == runtime.OmitVal {
			return m.fault(k3log.ErrOMIT, name)
		}
	case []any:
		typ := m.exprType(ref.Container)
		if _, ok := runtime.ElemType(typ); !ok {
			break
		}
		if i, ok := ref.Index.(int); ok && (i < 0 || i >= len(c)) {
			lo, _ := m.arrayBounds(typ)
			return m.fault(k3log.ErrARRV, name, strconv.Itoa(lo+i))
		}
	case map[any]any:
		return nil
	}
	return nil
}

func (m *Machine) operandName(opnd any) string {
	switch x := opnd.(type) {
	case *runtime.Var:
		if x.Name == "(anon)" {
			return "(anon)"
		}
		if x.Flags&runtime.Member != 0 {
			return m.self.Name + "." + x.Name
		}
		if x.Flags&(runtime.Local|runtime.Parameter) != 0 && m.Fn != nil && m.Fn.Name != "" {
			base := x.Name[strings.LastIndex(x.Name, ".")+1:]
			return m.Fn.Name + "." + base
		}
		return x.Name
	case *ElementRef:
		if component, ok := m.deref(x.Container).(*runtime.Component); ok {
			return component.Name + "." + fmt.Sprint(m.deref(x.Index))
		}
		parent := m.operandName(x.Container)
		base := m.exprType(x.Container)
		fields := runtime.RecordFields(base)
		if fields == nil {
			fields = runtime.UnionFields(base)
		}
		if f := fieldByIndex(fields, m.deref(x.Index)); f != nil {
			return parent + "." + f.Name
		}
		if _, ok := runtime.MapValueType(base); ok {
			idx := m.deref(x.Index)
			if s, ok := idx.(string); ok {
				return parent + "[" + runtime.Quote(s) + "]"
			}
		}
		return fmt.Sprintf("%s[%v]", parent, m.deref(x.Index))
	}
	return "(anon)"
}

func (m *Machine) matchPath(opnd any) string {
	name := m.operandName(opnd)
	if name == "(anon)" || strings.HasPrefix(name, "(anon).") || strings.HasPrefix(name, "(anon)[") {
		return "value"
	}
	return name
}

func (m *Machine) checkDefined(val any, typ runtime.Type, name string, allowTemplate, allowOmit bool) (k3log.Category, string) {
	if val == runtime.OmitVal {
		if !allowOmit && !allowTemplate {
			return k3log.ErrOMIT, name
		}
		return "", ""
	}
	switch v := val.(type) {
	case nil, runtime.Undefined:
		return k3log.ErrUNDF, name
	case runtime.ModifiedTemplate:
		if !allowTemplate {
			return k3log.ErrMANY, name
		}
		return m.checkDefined(v.Val, typ, name, true, true)
	case runtime.ValueList:
		if !allowTemplate {
			return k3log.ErrMANY, name
		}
		for _, item := range v {
			if cat, bad := m.checkDefined(item, typ, name, true, true); cat != "" {
				return cat, bad
			}
		}
	case runtime.TemplateList:
		if !allowTemplate {
			return k3log.ErrMANY, name
		}
		itemType := typ
		if v.Kind != "complement" {
			if et, ok := runtime.ElemType(typ); ok {
				itemType = et
			}
		}
		for _, item := range v.Items {
			if cat, bad := m.checkDefined(item, itemType, name, true, true); cat != "" {
				return cat, bad
			}
		}
	case runtime.TemplateLiteral, runtime.Infinity, runtime.Range, runtime.Pattern:
		if !allowTemplate {
			return k3log.ErrMANY, name
		}
	case []any:
		if uFields := runtime.UnionFields(typ); uFields != nil {
			f := activeUnionField(uFields, v)
			if f == nil {
				return k3log.ErrUNDF, name
			}
			return m.checkDefined(v[f.Index], f.Type, name+"."+f.Name, allowTemplate, true)
		}
		if fields := runtime.RecordFields(typ); fields != nil {
			for i, f := range fields {
				if i >= len(v) {
					return k3log.ErrUNDF, name + "." + f.Name
				}
				if cat, bad := m.checkDefined(v[i], f.Type, name+"."+f.Name, allowTemplate, true); cat != "" {
					return cat, bad
				}
			}
			return "", ""
		}
		elemType, _ := runtime.ElemType(typ)
		lo, _ := m.arrayBounds(typ)
		for i, elem := range v {
			if cat, bad := m.checkDefined(elem, elemType, fmt.Sprintf("%s[%d]", name, lo+i), allowTemplate, true); cat != "" {
				return cat, bad
			}
		}
	}
	return "", ""
}

func (m *Machine) execMatch(logEvent bool) (bool, error) {
	tmplOp := m.Pop()
	exprOp := m.Pop()
	exprType := m.exprType(exprOp)
	tmplType := m.exprType(tmplOp)
	typ := tmplType
	if typ == nil {
		typ = exprType
	}
	evalOp := func(opnd any) any {
		if blk, ok := opnd.(Block); ok {
			v := &runtime.Var{Name: "(anon)", Type: runtime.TemplateType{BaseType: runtime.BaseType(typ)}, Index: -1}
			m.store(v, blk)
			return v.Value
		}
		return m.deref(opnd)
	}
	exprVal := evalOp(exprOp)
	tmplVal := evalOp(tmplOp)

	lhsType, rhsType := exprType, tmplType
	if lhsType == nil {
		lhsType = typ
	}
	if rhsType == nil {
		rhsType = typ
	}
	if cat, bad := m.checkDefined(exprVal, lhsType, m.operandName(exprOp), true, true); cat != "" {
		return false, m.fault(cat, bad)
	}
	if cat, bad := m.checkDefined(tmplVal, rhsType, m.operandName(tmplOp), true, true); cat != "" {
		return false, m.fault(cat, bad)
	}

	var matched bool
	var mismatches []string
	if typ != nil && runtime.IsCompatible(exprVal, runtime.BaseType(typ)) != nil {
		mismatches = []string{fmt.Sprintf("%s:%s~%s", m.matchPath(exprOp), formatValue(exprVal, exprType), formatValue(tmplVal, tmplType))}
	} else {
		matched, mismatches = m.matchValue(exprVal, tmplVal, typ, m.matchPath(exprOp))
	}
	if logEvent {
		who, pc := m.origin()
		lhsStr := formatValue(exprVal, lhsType)
		rhsStr := formatValue(tmplVal, rhsType)
		if err := m.event(k3log.MATC, who, pc, lhsStr, rhsStr, strings.Join(mismatches, ",")); err != nil {
			return false, &Error{PC: m.PC, Err: err}
		}
	}
	return matched, nil
}

func (m *Machine) matchValue(exprVal, tmpl any, typ runtime.Type, path string) (bool, []string) {
	mismatch := func() (bool, []string) {
		return false, []string{fmt.Sprintf("%s:%s~%s", path, formatValue(exprVal, typ), formatValue(tmpl, typ))}
	}
	if mt, ok := tmpl.(runtime.ModifiedTemplate); ok {
		if mt.IfPresent && exprVal == runtime.OmitVal {
			return true, nil
		}
		if mt.Length != nil {
			if exprVal == runtime.OmitVal {
				return mismatch()
			}
			if actualLen, ok := valueLength(exprVal); ok && !matchLength(actualLen, mt.Length) {
				return false, []string{fmt.Sprintf("%s:length(%d)~%s", path, actualLen, formatLengthConstraint(mt.Length))}
			}
		}
		ok, mis := m.matchValue(exprVal, mt.Val, typ, path)
		if ok {
			return true, nil
		}
		switch mt.Val.(type) {
		case runtime.Pattern, runtime.ValueList, runtime.TemplateList, []any:
			return false, mis
		default:
			return mismatch()
		}
	}

	if tmpl == runtime.AnyOrNone {
		return true, nil
	}
	if tmpl == runtime.Any {
		if exprVal == runtime.OmitVal {
			return mismatch()
		}
		return true, nil
	}
	if tmpl == runtime.OmitVal || exprVal == runtime.OmitVal {
		if exprVal == tmpl {
			return true, nil
		}
		return mismatch()
	}

	switch t := tmpl.(type) {
	case runtime.ValueList:
		for _, item := range t {
			if ok, _ := m.matchValue(exprVal, item, typ, path); ok {
				return true, nil
			}
		}
		return mismatch()

	case runtime.TemplateList:
		switch t.Kind {
		case "complement":
			for _, item := range t.Items {
				if ok, _ := m.matchValue(exprVal, item, typ, path); ok {
					return mismatch()
				}
			}
			return true, nil
		case "subset", "superset":
			exprSlice, ok := exprVal.([]any)
			if !ok {
				return mismatch()
			}
			elemType, _ := runtime.ElemType(typ)
			left, right := exprSlice, t.Items
			if t.Kind == "superset" {
				left, right = t.Items, exprSlice
			}
			if bipartiteMatch(left, right, func(li, ri int) bool {
				ev, tv := left[li], right[ri]
				if t.Kind == "superset" {
					ev, tv = right[ri], left[li]
				}
				ok, _ := m.matchValue(ev, tv, elemType, path)
				return ok
			}) {
				return true, nil
			}
			return mismatch()
		}

	case runtime.Range:
		if inRange(exprVal, t.Lo, t.Hi) {
			return true, nil
		}
		return mismatch()

	case runtime.Pattern:
		s, ok := exprVal.(string)
		if b, isBin := exprVal.(runtime.Binary); isBin {
			s, ok, t = b.Digits, true, runtime.Pattern(strings.ToUpper(string(t)))
		}
		if ok {
			matched, err := t.Match(s, m.patternRef)
			if err != nil {
				panic(err)
			}
			if matched {
				return true, nil
			}
		}
		return mismatch()

	case []any:
		exprSlice, ok := exprVal.([]any)
		if !ok {
			return mismatch()
		}
		base := runtime.BaseType(typ)
		if uFields := runtime.UnionFields(base); uFields != nil {
			lf := activeUnionField(uFields, exprSlice)
			rf := activeUnionField(uFields, t)
			if lf == nil || rf == nil {
				return mismatch()
			}
			if lf.Name != rf.Name {
				return false, []string{fmt.Sprintf("%s:%s~%s", path, lf.Name, rf.Name)}
			}
			return m.matchValue(exprSlice[lf.Index], t[rf.Index], lf.Type, path+"."+lf.Name)
		}
		if fields := runtime.RecordFields(base); fields != nil {
			var mis []string
			for i, f := range fields {
				var lv, rv any = runtime.Undefined{}, runtime.Undefined{}
				if i < len(exprSlice) {
					lv = exprSlice[i]
				}
				if i < len(t) {
					rv = t[i]
				}
				if ok, subMis := m.matchValue(lv, rv, f.Type, path+"."+f.Name); !ok {
					mis = append(mis, subMis...)
				}
			}
			return len(mis) == 0, mis
		}
		switch base.(type) {
		case runtime.SetOfType, *runtime.SetOfType:
			return m.matchSetOf(exprSlice, t, typ, path)
		default:
			return m.matchRecordOf(exprSlice, t, typ, path)
		}
	}

	if equal(exprVal, tmpl) {
		return true, nil
	}
	return mismatch()
}

func (m *Machine) matchRecordOf(exprSlice, tmplSlice []any, typ runtime.Type, path string) (bool, []string) {
	base := runtime.BaseType(typ)
	elemType, _ := runtime.ElemType(base)
	lo, _ := m.arrayBounds(base)

	hasWild := false
	for _, item := range tmplSlice {
		if item == runtime.AnyOrNone {
			hasWild = true
			break
		}
		if tl, ok := item.(runtime.TemplateList); ok && tl.Kind == "permutation" {
			hasWild = true
			break
		}
	}
	if !hasWild {
		if len(exprSlice) != len(tmplSlice) {
			return false, []string{fmt.Sprintf("%s:length(%d)~length(%d)", path, len(exprSlice), len(tmplSlice))}
		}
		var mis []string
		for i := range exprSlice {
			if ok, subMis := m.matchValue(exprSlice[i], tmplSlice[i], elemType, fmt.Sprintf("%s[%d]", path, lo+i)); !ok {
				mis = append(mis, subMis...)
			}
		}
		return len(mis) == 0, mis
	}

	var probed []string
	var backtrack func(ei, ti int) bool
	backtrack = func(ei, ti int) bool {
		if ti == len(tmplSlice) {
			return ei == len(exprSlice)
		}
		if tmplSlice[ti] == runtime.AnyOrNone {
			for nextEi := ei; nextEi <= len(exprSlice); nextEi++ {
				if backtrack(nextEi, ti+1) {
					return true
				}
			}
			return false
		}
		if tl, ok := tmplSlice[ti].(runtime.TemplateList); ok && tl.Kind == "permutation" {
			k := len(tl.Items)
			if ei+k > len(exprSlice) {
				return false
			}
			window := exprSlice[ei : ei+k]
			if !bipartiteMatch(tl.Items, window, func(pi, wi int) bool {
				ok, _ := m.matchValue(window[wi], tl.Items[pi], elemType, path)
				return ok
			}) {
				return false
			}
			return backtrack(ei+k, ti+1)
		}
		if ei >= len(exprSlice) {
			return false
		}
		ok, subMis := m.matchValue(exprSlice[ei], tmplSlice[ti], elemType, fmt.Sprintf("%s[%d]", path, lo+ei))
		if !ok {
			probed = append(probed, subMis...)
			return false
		}
		return backtrack(ei+1, ti+1)
	}

	if backtrack(0, 0) {
		return true, nil
	}
	probed = append(probed, fmt.Sprintf("%s:%s~%s", path, formatValue(exprSlice, typ), formatValue(tmplSlice, typ)))
	return false, probed
}

func (m *Machine) matchSetOf(exprSlice, tmplSlice []any, typ runtime.Type, path string) (bool, []string) {
	elemType, _ := runtime.ElemType(typ)
	hasStar := false
	var nonStar []any
	for _, item := range tmplSlice {
		if item == runtime.AnyOrNone {
			hasStar = true
		} else {
			nonStar = append(nonStar, item)
		}
	}
	if (hasStar && len(exprSlice) >= len(nonStar)) || (!hasStar && len(exprSlice) == len(nonStar)) {
		if bipartiteMatch(nonStar, exprSlice, func(ti, ei int) bool {
			ok, _ := m.matchValue(exprSlice[ei], nonStar[ti], elemType, path)
			return ok
		}) {
			return true, nil
		}
	}

	usedExpr := make([]bool, len(exprSlice))
	var unmatchedTmpl []any
	for _, t := range nonStar {
		paired := false
		for ei := range exprSlice {
			if !usedExpr[ei] {
				if ok, _ := m.matchValue(exprSlice[ei], t, elemType, path); ok {
					usedExpr[ei] = true
					paired = true
					break
				}
			}
		}
		if !paired {
			unmatchedTmpl = append(unmatchedTmpl, t)
		}
	}

	var unmatchedExprIdx []int
	for ei, used := range usedExpr {
		if !used {
			unmatchedExprIdx = append(unmatchedExprIdx, ei)
		}
	}

	var mis []string
	for _, t := range unmatchedTmpl {
		if len(unmatchedExprIdx) == 0 {
			mis = append(mis, fmt.Sprintf("%s:{}~%s", path, formatValue(t, elemType)))
		} else {
			for _, ei := range unmatchedExprIdx {
				_, subMis := m.matchValue(exprSlice[ei], t, elemType, fmt.Sprintf("%s[%d]", path, ei))
				mis = append(mis, subMis...)
			}
		}
	}
	if !hasStar && len(unmatchedExprIdx) > 0 {
		leftover := make([]any, len(unmatchedExprIdx))
		for i, ei := range unmatchedExprIdx {
			leftover[i] = exprSlice[ei]
		}
		mis = append(mis, fmt.Sprintf("%s:%s~{}", path, formatValue(leftover, typ)))
	}
	if len(mis) == 0 {
		mis = append(mis, fmt.Sprintf("%s:%s~%s", path, formatValue(exprSlice, typ), formatValue(tmplSlice, typ)))
	}
	return false, mis
}

func bipartiteMatch(left, right []any, canMatch func(li, ri int) bool) bool {
	if len(left) > len(right) {
		return false
	}
	matchR := make([]int, len(right))
	for i := range matchR {
		matchR[i] = -1
	}
	var dfs func(li int, seen []bool) bool
	dfs = func(li int, seen []bool) bool {
		for ri := range right {
			if seen[ri] || !canMatch(li, ri) {
				continue
			}
			seen[ri] = true
			if matchR[ri] < 0 || dfs(matchR[ri], seen) {
				matchR[ri] = li
				return true
			}
		}
		return false
	}
	for li := range left {
		if !dfs(li, make([]bool, len(right))) {
			return false
		}
	}
	return true
}

func valueLength(v any) (int, bool) {
	switch v.(type) {
	case string, runtime.Binary, []any:
		n, _ := runtime.LengthOf(v)
		return n, true
	}
	return 0, false
}

func matchLength(actual int, spec any) bool {
	switch s := spec.(type) {
	case int:
		return actual == s
	case runtime.Range:
		return inRange(actual, s.Lo, s.Hi)
	}
	return false
}

func inRange(val, lo, hi any) bool {
	vf, ok := toFloat(val)
	if !ok {
		return false
	}
	if lo != runtime.NegInfinity {
		lf, ok := toFloat(lo)
		if !ok || vf < lf {
			return false
		}
	}
	if hi != runtime.PosInfinity {
		hf, ok := toFloat(hi)
		if !ok || vf > hf {
			return false
		}
	}
	return true
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case float64:
		return x, true
	case runtime.EnumValue:
		return float64(x.Val), true
	}
	return 0, false
}

// patternRef resolves {name} in a pattern to the value of the module
// constant, module parameter, or parameterless template called name.
func (m *Machine) patternRef(name string) any {
	switch obj := m.Names[m.Module(m.PC)+"."+name].(type) {
	case *runtime.Var:
		return m.deref(obj)
	case *runtime.Function:
		v, err := m.call(obj)
		if err != nil {
			panic(err)
		}
		return v
	}
	return nil
}

// call runs fn with args from Go code and returns its result, or nil when
// fn returns nothing.
func (m *Machine) call(fn *runtime.Function, args ...any) (any, error) {
	saved, sp, depth := m.PC, m.SP, len(m.Frames)
	for _, a := range args {
		m.Push(a)
	}
	m.fnen(fn)
	if len(m.Frames) > depth {
		if err := m.run(-1); err != nil {
			return nil, err
		}
	}
	m.PC = saved
	if m.SP > sp {
		return m.popValue(), nil
	}
	return nil, nil
}

func activeUnionField(fields []*runtime.Var, slice []any) *runtime.Var {
	for _, f := range fields {
		if f.Index >= 0 && f.Index < len(slice) && slice[f.Index] != nil {
			if _, undef := slice[f.Index].(runtime.Undefined); !undef {
				return f
			}
		}
	}
	return nil
}

func formatItems(items []any, typ runtime.Type) string {
	var b strings.Builder
	for i, item := range items {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(formatValue(item, typ))
	}
	return b.String()
}

func formatLengthConstraint(l any) string {
	if r, ok := l.(runtime.Range); ok {
		return fmt.Sprintf("length(%s..%s)", formatValue(r.Lo, nil), formatValue(r.Hi, nil))
	}
	return fmt.Sprintf("length(%s)", formatValue(l, nil))
}

func valueTypeName(value any) string {
	switch value.(type) {
	case int:
		return "integer"
	case float64:
		return "float"
	case bool:
		return "boolean"
	case string:
		return "charstring"
	case runtime.Binary:
		return "bitstring"
	case runtime.Verdict:
		return "verdicttype"
	default:
		return "(anon)"
	}
}

func portIndex(name string) any {
	var indices []any
	for {
		open := strings.IndexByte(name, '[')
		if open < 0 {
			break
		}
		close := strings.IndexByte(name[open:], ']')
		if close < 0 {
			break
		}
		index, err := strconv.Atoi(name[open+1 : open+close])
		if err == nil {
			indices = append(indices, index)
		}
		name = name[open+close+1:]
	}
	if len(indices) == 1 {
		return indices[0]
	}
	return indices
}

func formatValue(v any, typ runtime.Type) string {
	base := runtime.BaseType(typ)
	switch x := v.(type) {
	case int:
		if et, ok := base.(runtime.EnumeratedType); ok {
			if terms, ok := et.Body.([]*runtime.Var); ok {
				for _, t := range terms {
					if ev, ok := t.Value.(runtime.EnumValue); ok && ev.Val == x {
						return ev.Name
					}
				}
			}
		}
		return strconv.Itoa(x)
	case string:
		return runtime.Quote(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		if base == runtime.TimerType {
			return (&runtime.Timer{Default: x}).String()
		}
		return runtime.FormatFloat(x)
	case nil, runtime.Undefined:
		return "-"
	case runtime.Range:
		return fmt.Sprintf("(%s..%s)", formatValue(x.Lo, nil), formatValue(x.Hi, nil))
	case runtime.ValueList:
		return "(" + formatItems(x, typ) + ")"
	case runtime.TemplateList:
		itemType := typ
		if x.Kind != "complement" {
			if et, ok := runtime.ElemType(base); ok {
				itemType = et
			}
		}
		return x.Kind + "(" + formatItems(x.Items, itemType) + ")"
	case runtime.ModifiedTemplate:
		s := formatValue(x.Val, typ)
		if x.Length != nil {
			s += " " + formatLengthConstraint(x.Length)
		}
		if x.IfPresent {
			s += " ifpresent"
		}
		return s
	case []any:
		if uFields := runtime.UnionFields(base); uFields != nil {
			if f := activeUnionField(uFields, x); f != nil {
				return "{" + f.Name + ":=" + formatValue(x[f.Index], f.Type) + "}"
			}
			return "{}"
		}
		if fields := runtime.RecordFields(base); fields != nil {
			_, isSet := base.(runtime.SetType)
			var b strings.Builder
			b.WriteByte('{')
			n := len(x)
			if isSet {
				n = len(fields)
			}
			for i := 0; i < n; i++ {
				if i > 0 {
					b.WriteByte(',')
				}
				var elem any = runtime.Undefined{}
				if i < len(x) {
					elem = x[i]
				}
				var ft runtime.Type
				if i < len(fields) {
					if isSet {
						b.WriteString(fields[i].Name)
						b.WriteString(":=")
					}
					ft = fields[i].Type
				}
				b.WriteString(formatValue(elem, ft))
			}
			b.WriteByte('}')
			return b.String()
		}
		elemType, _ := runtime.ElemType(base)
		return "{" + formatItems(x, elemType) + "}"
	default:
		return fmt.Sprint(x)
	}
}

func (m *Machine) stopped() bool {
	return m.self != nil && m.self.Stopped()
}

// runBlock executes blk and restores PC. Begin >= End is an empty block.
// PC is restored when blk finishes normally, but not on error or when a
// RETURN in blk has left the current frame.
func (m *Machine) runBlock(blk Block) error {
	if blk.Begin >= blk.End {
		return nil
	}
	saved, depth := m.PC, len(m.Frames)
	m.PC = blk.Begin + 4
	err := m.run(blk.End)
	if err == nil && len(m.Frames) >= depth {
		m.PC = saved
	}
	return err
}

// startBehaviour runs fn on comp in a new goroutine.
// callPC is the startc instruction.
func (m *Machine) startBehaviour(comp *runtime.Component, fn *runtime.Function, args []any, callPC int) {
	ptc := m.tc.machine(comp)
	ptc.callPC = callPC
	args = cloneSlice(args)
	comp.Start()
	m.tc.wg.Add(1)
	go func() {
		dead := false
		defer func() {
			if recover() != nil {
				comp.SetVerdict(runtime.VerdictError)
			}
			v := comp.Verdict()
			if !dead {
				_ = ptc.event(k3log.COFI, comp.Name, k3log.NoPos, string(v))
			}
			m.tc.mtc.SetVerdict(v)
			comp.Finish()
			m.tc.notify()
			m.tc.wg.Done()
		}()
		body := fn.Body.(Block)
		_ = ptc.event(k3log.COST, comp.Name, body.Begin, fn.Name)
		_ = ptc.event(k3log.FNEN, comp.Name, callPC, ptc.signature(fn, args, false))
		err := ptc.Execute(fn, args...)
		switch {
		case errors.Is(err, errFault):
			dead = true
		case err != nil:
			comp.SetVerdict(runtime.VerdictError)
		case !ptc.stopped():
			_ = ptc.event(k3log.FNLV, comp.Name, callPC, ptc.signature(fn, args, true))
		}
	}()
}

// spawn returns a machine for component c with m's image, output, and a
// copy of its globals.
func (m *Machine) spawn(c *runtime.Component) *Machine {
	s := New(m.Image)
	s.Out = m.Out
	s.Globals = cloneSlice(m.Globals)
	s.self = c
	return s
}

// initComponent declares the variables, constants, and timers of component
// type typ and of the types it extends. creator reports the events.
func (m *Machine) initComponent(creator *Machine, typ runtime.Type) error {
	ct, ok := runtime.BaseType(typ).(runtime.ComponentType)
	if !ok {
		return nil
	}
	for _, ext := range ct.Extends {
		if err := m.initComponent(creator, ext); err != nil {
			return err
		}
	}
	saved := m.Frame
	m.creator = creator
	m.Frame = Frame{SP: m.SP, Fn: &runtime.Function{Flags: runtime.Member}}
	err := m.runBlock(ct.Body.(Block))
	m.Frame, m.creator = saved, nil
	return err
}

func cloneSlice(s []any) []any {
	cp := make([]any, len(s))
	for i, v := range s {
		if sub, ok := v.([]any); ok {
			v = cloneSlice(sub)
		}
		cp[i] = v
	}
	return cp
}

func equal(a, b any) bool {
	switch x := a.(type) {
	case []any:
		y, ok := b.([]any)
		return ok && slices.EqualFunc(x, y, equal)
	case runtime.ValueList:
		y, ok := b.(runtime.ValueList)
		return ok && slices.EqualFunc(x, y, equal)
	case runtime.TemplateList:
		y, ok := b.(runtime.TemplateList)
		return ok && x.Kind == y.Kind && slices.EqualFunc(x.Items, y.Items, equal)
	case runtime.ModifiedTemplate:
		y, ok := b.(runtime.ModifiedTemplate)
		return ok && x.IfPresent == y.IfPresent && equal(x.Val, y.Val) && equal(x.Length, y.Length)
	case runtime.EnumValue:
		switch y := b.(type) {
		case runtime.EnumValue:
			return x.Name == y.Name
		case int:
			return x.Val == y
		}
		return false
	case int:
		if y, ok := b.(runtime.EnumValue); ok {
			return x == y.Val
		}
		return a == b
	default:
		return a == b
	}
}

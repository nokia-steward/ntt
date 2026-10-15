package interpreter

import (
	"context"
	"fmt"
	"math/big"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/runtime/tl"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// executeHandlerKey binds the control part's execute() implementation
// into the scope the control body runs in. Only a control part sets it,
// so an `execute(...)` reached from anywhere else stays inert instead of
// recursively launching testcases.
const executeHandlerKey = "\x00ttcn3:execute"

// executeHandler runs one `execute(TC(args) [, timeout] [, host])` and
// reports the resulting testcase verdict. It is carried in the control
// scope, so it satisfies runtime.Object; it is never a TTCN-3 value and
// compares equal only to itself.
type executeHandler struct {
	run func(tcName string, args []runtime.Object, timeout float64, hasTimeout bool, host string, hasHost bool) runtime.Verdict
}

func (executeHandler) Type() runtime.ObjectType { return runtime.ObjectType("EXECUTE_HANDLER") }
func (executeHandler) Inspect() string          { return "execute-handler" }
func (h executeHandler) Equal(o runtime.Object) bool {
	other, ok := o.(executeHandler)
	return ok && &other == &h
}

// controlBudget bounds what a control part does itself — the time it
// spends outside the testcases it executes, which are bounded each
// (TestcaseOptions.TestcaseTimeout) — and ends with the run's context.
// Its loops stop once it is spent, and it executes no further testcase.
type controlBudget struct {
	mu      sync.Mutex
	left    time.Duration
	since   time.Time
	timer   *time.Timer
	limit   time.Duration // 0: no time of its own, only the context
	ctx     context.Context
	expired chan struct{}
	once    sync.Once
	// steps counts the iterations of the control part's loops, which
	// maxSteps, when positive, bounds: time of a control part run for
	// a testcase's actuals is told by them, not the clock.
	steps, maxSteps int64
}

const controlBudgetKey = "\x00ttcn3:control-budget"

func newControlBudget(ctx context.Context, d time.Duration) *controlBudget {
	if ctx == nil {
		ctx = context.Background()
	}
	b := &controlBudget{left: d, limit: d, ctx: ctx, expired: make(chan struct{})}
	b.resume()
	return b
}

func (b *controlBudget) resume() {
	if b.limit <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.since = time.Now()
	b.timer = time.AfterFunc(b.left, func() { b.once.Do(func() { close(b.expired) }) })
}

func (b *controlBudget) pause() {
	if b.limit <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.timer.Stop() {
		if b.left -= time.Since(b.since); b.left < 0 {
			b.left = 0
		}
	}
}

func (b *controlBudget) spent() bool {
	if b.maxSteps > 0 && b.steps >= b.maxSteps {
		return true
	}
	select {
	case <-b.expired:
		return true
	default:
		return b.ctx.Err() != nil
	}
}

func (*controlBudget) Type() runtime.ObjectType { return runtime.ObjectType("CONTROL_BUDGET") }
func (*controlBudget) Inspect() string          { return "control budget" }
func (b *controlBudget) Equal(o runtime.Object) bool {
	other, ok := o.(*controlBudget)
	return ok && other == b
}

// controlClock is a control part's virtual clock: its own timers run on
// it, and each testcase it executes moves it on by the virtual time that
// testcase took. Bound only on the virtual clock.
type controlClock struct{ now float64 }

const controlClockKey = "\x00ttcn3:control-clock"

func (*controlClock) Type() runtime.ObjectType { return runtime.ObjectType("CONTROL_CLOCK") }
func (*controlClock) Inspect() string          { return "control clock" }
func (c *controlClock) Equal(o runtime.Object) bool {
	other, ok := o.(*controlClock)
	return ok && other == c
}

// controlClockOf is the control part's clock env runs on, or nil — in a
// testcase, or on the real clock.
func controlClockOf(env runtime.Scope) *controlClock {
	if env == nil {
		return nil
	}
	v, ok := env.Get(controlClockKey)
	if !ok {
		return nil
	}
	c, _ := v.(*controlClock)
	return c
}

// controlWait waits d on the real clock, for a timer of the control part
// env is the scope of, and reports whether it waited it all: the control
// part's time running out, or the run's, cuts it short.
func controlWait(env runtime.Scope, d time.Duration) bool {
	var expired, done <-chan struct{}
	if v, ok := env.Get(controlBudgetKey); ok {
		if b, ok := v.(*controlBudget); ok {
			expired, done = b.expired, b.ctx.Done()
		}
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-expired:
	case <-done:
	}
	return false
}

// controlBudgetSpent reports whether env is a control part's whose budget
// is spent, at a loop's iteration, which it counts.
func controlBudgetSpent(env runtime.Scope) bool {
	v, ok := env.Get(controlBudgetKey)
	if !ok {
		return false
	}
	b, ok := v.(*controlBudget)
	if !ok {
		return false
	}
	b.steps++
	return b.spent()
}

// ControlPartIsLoadBearing reports whether a module's `control` part
// decides something that running its first testcase on its own cannot
// reproduce, so a caller should drive the module through RunControlWith.
//
// It is true when the control part executes more than one testcase — the
// selection, ordering and argument threading between them are the
// control part's job — or when a single execute() carries a timeout or
// host operand, both of which can make the testcase yield `error`
// without its body ever running to a verdict (ETSI 26.1).
//
// It is deliberately false for the overwhelmingly common
// `control { execute(TheOnlyTestcase()); }`, where running the testcase
// directly is equivalent and far better exercised.
func ControlPartIsLoadBearing(trees []*ttcn3.Tree, module string) bool {
	control := findControlPart(findModule(trees, module))
	if control == nil {
		return false
	}
	executes := 0
	loadBearing := false
	control.Body.Inspect(func(n syntax.Node) bool {
		call, ok := n.(*syntax.CallExpr)
		if !ok {
			return true
		}
		id, ok := call.Fun.(*syntax.Ident)
		if !ok || id.String() != "execute" || call.Args == nil {
			return true
		}
		executes++
		if len(call.Args.List) > 1 {
			loadBearing = true
		}
		return true
	})
	return loadBearing || executes > 1
}

// RunControlWith executes a module's `control` part as a program (ETSI
// ES 201 873-1 clause 26): statements run in order, and each
// `execute(...)` runs a testcase to completion on its own executor and
// yields its verdict. The returned verdict is the aggregate over every
// testcase the control part actually ran, merged by TTCN-3 verdict
// precedence (none < pass < inconc < fail < error), which is what the
// suite-level verdict of a module amounts to.
//
// A control part that runs no testcase at all yields `none`.
func RunControlWith(trees []*ttcn3.Tree, module string, opts TestcaseOptions) (verdict runtime.Verdict, reason string, err error) {
	// Registered first so it runs last, after a panic has been recovered.
	var control *syntax.ControlPart
	// One clock for the control part and the testcases it runs, so the
	// log's timestamps do not go backwards (see tl.Monotonic).
	if opts.TestLogger != nil {
		if _, ok := opts.TestLogger.(*tl.Monotonic); !ok {
			opts.TestLogger = &tl.Monotonic{Next: opts.TestLogger}
		}
	}
	defer func() {
		if control != nil {
			tlControl(opts.TestLogger, control, "tliCtrlTerminated")
		}
	}()
	defer func() {
		if r := recover(); r != nil {
			verdict = runtime.ErrorVerdict
			reason = fmt.Sprintf("interpreter panic: %v", r)
			if interpreterPanicDebug() {
				reason += "\n" + string(debug.Stack())
			}
			err = nil
		}
	}()
	evalDepth.reset()

	modNode := findModule(trees, module)
	if modNode == nil {
		return runtime.ErrorVerdict, "", fmt.Errorf("module %q not found", module)
	}
	cp := findControlPart(modNode)
	if cp == nil {
		return runtime.ErrorVerdict, "", fmt.Errorf("module %q has no control part", module)
	}

	env, initErr := newModuleEnv(trees, modNode, module, opts)
	if initErr != "" {
		return runtime.ErrorVerdict, initErr, nil
	}
	control = cp
	tlControl(opts.TestLogger, control, "tliCtrlStart")

	agg := runtime.NoneVerdict
	aggReason := ""
	ran := false

	ctrlEnv := runtime.NewEnv(moduleScopeOf(env, module))
	ctrlEnv.Set(runtime.ScopeNameKey, runtime.NewCharstring("control"))
	// Bind the handler on the MODULE scope, not the control scope:
	// fixtures routinely wrap execute() in a helper function, and a
	// function body evaluates in its own closure (the module scope), so a
	// control-scope binding would be invisible there. Each execute()
	// builds a fresh module scope of its own, which does not carry the
	// handler, so a testcase body cannot re-enter this.
	var budget *controlBudget
	if opts.TestcaseTimeout > 0 || opts.Context != nil {
		budget = newControlBudget(opts.Context, opts.TestcaseTimeout)
		defer budget.pause()
		env.Set(controlBudgetKey, budget)
	}
	var clock *controlClock
	if opts.DeterministicClock {
		clock = &controlClock{}
		env.Set(controlClockKey, clock)
	}
	env.Set(executeHandlerKey, executeHandler{
		run: func(tcName string, args []runtime.Object, timeout float64, hasTimeout bool, host string, hasHost bool) runtime.Verdict {
			if budget != nil {
				if budget.spent() {
					return runtime.ErrorVerdict
				}
				budget.pause()
				defer budget.resume()
			}
			tlExecute(opts.TestLogger, control, modNode, module, tcName, args, timeout, hasTimeout)
			v, r, took := runOneExecute(trees, modNode, module, tcName, args, timeout, hasTimeout, host, hasHost, opts)
			if clock != nil {
				clock.now += took
			}
			ran = true
			if verdictRank(v) > verdictRank(agg) {
				agg, aggReason = v, r
			}
			return v
		}})

	tlControl(opts.TestLogger, control, "tliSEnter", tlArg("name", tl.QualifiedName(module, "control")), tlArg("kind", tl.String("control")))
	defer tlControl(opts.TestLogger, control, "tliSLeave", tlArg("name", tl.QualifiedName(module, "control")), tlArg("kind", tl.String("control")))
	if res := eval(control.Body, ctrlEnv); runtime.IsError(res) {
		e, _ := res.(*runtime.Error)
		return runtime.ErrorVerdict, "control: " + e.Inspect(), nil
	}
	if budget != nil && budget.spent() {
		if budget.limit <= 0 || budget.ctx.Err() != nil {
			return runtime.ErrorVerdict, "control part stopped: " + context.Cause(budget.ctx).Error(), nil
		}
		return runtime.ErrorVerdict, fmt.Sprintf("control part did not terminate within %v, besides its testcases", budget.limit), nil
	}
	if !ran {
		return runtime.NoneVerdict, "control part executed no testcase", nil
	}
	return agg, aggReason, nil
}

// controlRunTo returns what module's control part gives testcase tcName
// at its first execute() of it — the actual parameters and the time
// limit — for a testcase run on its own; found is false when the control
// part does not reach one. The control part is run once for the module's
// tree, the run's other trees and module parameters (controlActuals).
func controlRunTo(trees []*ttcn3.Tree, modNode *syntax.Module, module, tcName string, opts TestcaseOptions) (args []runtime.Object, timeout float64, hasTimeout, found bool) {
	var own *ttcn3.Tree
	for _, t := range trees {
		if t == nil || t.Root == nil {
			continue
		}
		for _, m := range t.Modules() {
			if m.Node == syntax.Node(modNode) {
				own = t
			}
		}
	}
	if own == nil {
		return nil, 0, false, false
	}
	var key strings.Builder
	key.WriteString("interpreter.control:" + module)
	for _, t := range trees {
		fmt.Fprintf(&key, " %p", t)
	}
	params := make([]string, 0, len(opts.ModuleParameters))
	for k, v := range opts.ModuleParameters {
		params = append(params, k+"="+v)
	}
	sort.Strings(params)
	key.WriteString(" " + strings.Join(params, ";"))
	calls := own.Memo(key.String(), func() interface{} { return controlActuals(trees, modNode, module, opts) }).(map[string]executeCall)
	c, ok := calls[tcName]
	if !ok {
		return nil, 0, false, false
	}
	for _, a := range c.args {
		if a != nil && !plainValue(a) {
			return nil, 0, false, false
		}
	}
	args = make([]runtime.Object, len(c.args))
	for i, a := range c.args {
		if a != nil {
			args[i] = runtime.Share(a)
		}
	}
	return args, c.timeout, c.hasTimeout, true
}

// executeCall is what an execute() gives its testcase.
type executeCall struct {
	args       []runtime.Object
	timeout    float64
	hasTimeout bool
}

// controlActuals runs module's control part and returns, for each testcase
// it executes, what its first execute() of it gives. The control part runs
// on a virtual clock of its own, whatever the run's, so its timers wait
// for nothing; the testcases it executes are not run, each yielding none,
// nor is anything it logs written; and it stops once it has executed every
// testcase of the module, or after controlRunSteps loop iterations. An
// actual that is no plain value (an object, say) is not taken.
func controlActuals(trees []*ttcn3.Tree, modNode *syntax.Module, module string, opts TestcaseOptions) (calls map[string]executeCall) {
	calls = map[string]executeCall{}
	cp := findControlPart(modNode)
	if cp == nil {
		return calls
	}
	defer func() {
		if r := recover(); r != nil {
			calls = map[string]executeCall{}
		}
	}()
	own := map[string]bool{}
	for _, d := range modNode.Defs {
		if fn, ok := d.Def.(*syntax.FuncDecl); ok && fn.IsTest() {
			own[syntax.Name(fn.Name)] = true
		}
	}
	captured := 0
	inner := opts
	inner.TestLogger, inner.ModuleParamWarning, inner.actualArgs = nil, nil, nil
	env, initErr := newModuleEnv(trees, modNode, module, inner)
	if initErr != "" {
		return calls
	}
	// Bounded by the iterations of its loops, so a run is like the
	// last whatever the machine's load; and, should one iteration take
	// long, by the clock.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	budget := newControlBudget(ctx, 10*time.Second)
	budget.maxSteps = controlRunSteps
	defer budget.pause()
	env.Set(controlBudgetKey, budget)
	env.Set(controlClockKey, &controlClock{})
	env.Set(controlQuietKey, runtime.NewBool(true))
	env.Set(executeHandlerKey, executeHandler{
		run: func(name string, a []runtime.Object, d float64, hasD bool, _ string, _ bool) runtime.Verdict {
			if _, seen := calls[name]; !seen {
				calls[name] = executeCall{args: a, timeout: d, hasTimeout: hasD}
				if own[name] {
					captured++
				}
				if captured >= len(own) {
					// The control part's loops end, as when its time is up.
					budget.once.Do(func() { close(budget.expired) })
				}
			}
			return runtime.NoneVerdict
		}})
	ctrlEnv := runtime.NewEnv(moduleScopeOf(env, module))
	ctrlEnv.Set(runtime.ScopeNameKey, runtime.NewCharstring("control"))
	eval(cp.Body, ctrlEnv)
	return calls
}

// controlRunSteps bounds the loop iterations of a control part run for
// a testcase's actuals.
const controlRunSteps = 100000

// plainValue reports whether v is a value of its own, which a testcase
// can take from a control part run for its actuals: not an object, a
// function, a component, port or timer reference, nor a value
// holding one — those belong to the run.
func plainValue(v runtime.Object) bool {
	switch x := v.(type) {
	case *runtime.ClassInstance, *runtime.ClassDesc, *runtime.Function, *runtime.ComponentRef,
		*runtime.PortRef, *runtime.TimerHandle:
		return false
	case *runtime.Record:
		for _, f := range x.Fields {
			if !plainValue(f) {
				return false
			}
		}
	case *runtime.List:
		for _, e := range x.Elements {
			if !plainValue(e) {
				return false
			}
		}
	}
	return true
}

// controlQuietKey marks a control part that runs to compute a testcase's
// actuals (controlActuals): what it logs is not written.
const controlQuietKey = "\x00ttcn3:control-quiet"

// runOneExecute runs a single testcase named by an execute() statement.
//
// The `timeout` and `host` operands are the two ways execute() can fail
// without the body ever producing a verdict (ETSI 26.1): a testcase that
// does not terminate within the timeout is stopped with `error`, and a
// host the runtime cannot resolve is `error` too. Neither is a testcase
// failure, so both bypass the body's own verdict.
//
// It also returns the virtual time the testcase took.
func runOneExecute(trees []*ttcn3.Tree, modNode *syntax.Module, module, tcName string, args []runtime.Object, timeout float64, hasTimeout bool, host string, hasHost bool, opts TestcaseOptions) (runtime.Verdict, string, float64) {
	if hasHost && !hostIsLocal(host) {
		return runtime.ErrorVerdict, fmt.Sprintf("execute: unknown host %q", host), 0
	}
	tcNode := findTestcase(modNode, tcName)
	if tcNode == nil {
		return runtime.ErrorVerdict, fmt.Sprintf("execute: testcase %q not found in %q", tcName, module), 0
	}

	inner := opts
	// A non-nil (possibly empty) slice means "these are the actuals";
	// nil would send runTestcaseIn back to mining the source statically.
	inner.actualArgs = args
	if inner.actualArgs == nil {
		inner.actualArgs = []runtime.Object{}
	}

	// An execute() timeout bounds the testcase in the test system's time
	// (runTestcaseIn): virtual time on the virtual clock, real time on the
	// real one.
	if hasTimeout {
		if timeout <= 0 {
			return runtime.ErrorVerdict, "execute: non-positive timeout", 0
		}
		inner.executeTimeout, inner.hasExecuteTimeout = timeout, true
	}

	if opts.TestcaseTimeout > 0 {
		parent := opts.Context
		if parent == nil {
			parent = context.Background()
		}
		ctx, cancel := context.WithTimeout(parent, opts.TestcaseTimeout)
		defer cancel()
		inner.Context = ctx
	}

	// Each execute() gets a module scope of its own, so module-level
	// state one testcase mutated cannot leak into the next.
	env, initErr := newModuleEnv(trees, modNode, module, inner)
	if initErr != "" {
		return runtime.ErrorVerdict, initErr, 0
	}
	exec := runtime.NewTestcaseExec(module + "." + tcName)
	v, r, err := runTestcaseIn(env, exec, trees, modNode, module, tcName, tcNode, inner)
	if err != nil {
		v, r = runtime.ErrorVerdict, err.Error()
	}
	tlTestcaseTerminated(exec, v, r)
	return v, r, exec.VirtualClock()
}

// hostIsLocal reports whether an execute()'s host operand names a host
// this runtime can run on. Only the local host is supported, so any
// other name is an unresolvable host and execute() yields `error`.
func hostIsLocal(host string) bool {
	switch host {
	case "", "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// verdictRank orders verdicts by TTCN-3 precedence so an aggregate keeps
// the worst outcome seen (ETSI 26.2: none < pass < inconc < fail < error).
func verdictRank(v runtime.Verdict) int {
	switch v {
	case runtime.NoneVerdict:
		return 0
	case runtime.PassVerdict:
		return 1
	case runtime.InconcVerdict:
		return 2
	case runtime.FailVerdict:
		return 3
	case runtime.ErrorVerdict:
		return 4
	}
	return 0
}

// evalExecute evaluates `execute(TC(args) [, timeout] [, host])` inside a
// control part and yields the testcase's verdict, so
// `v := execute(TC())` and `if (execute(TC()) == pass)` both work.
//
// A testcase can only be executed from the control part, or from a
// function or altstep called directly or indirectly from it (ETSI ES 201
// 873-1 clauses 16.3 and 26.2). Only the control part binds a handler,
// so its absence means the call sits inside test behaviour and is a
// dynamic error rather than a silent no-op — which is exactly what
// NegSem_1603_testcases_001, NegSyn_1603_testcases_004 and
// NegSem_2602_TheControlPart_031 check.
func evalExecute(n *syntax.CallExpr, env runtime.Scope) runtime.Object {
	h, ok := env.Get(executeHandlerKey)
	if !ok {
		return runtime.Errorf("execute: a testcase can only be executed from the control part")
	}
	handler, ok := h.(executeHandler)
	if !ok || handler.run == nil {
		return runtime.Errorf("execute: a testcase can only be executed from the control part")
	}
	if n.Args == nil || len(n.Args.List) == 0 {
		return runtime.Errorf("execute: missing testcase")
	}

	call, ok := n.Args.List[0].(*syntax.CallExpr)
	if !ok {
		return runtime.Undefined
	}
	id, ok := call.Fun.(*syntax.Ident)
	if !ok {
		return runtime.Undefined
	}

	var args []runtime.Object
	if call.Args != nil {
		for _, a := range call.Args.List {
			// `-` selects the formal's default; carry it as a nil slot
			// so bindTestcaseParamsWithValues falls back rather than
			// binding the literal dash.
			if isDashExpr(a) {
				args = append(args, nil)
				continue
			}
			v := eval(a, env)
			if runtime.IsError(v) {
				v = runtime.Undefined
			}
			args = append(args, v)
		}
	}

	timeout, hasTimeout := 0.0, false
	host, hasHost := "", false
	for i, extra := range n.Args.List[1:] {
		if isDashExpr(extra) {
			continue
		}
		v := eval(extra, env)
		switch i {
		case 0:
			if f, ok := numericSeconds(v); ok {
				timeout, hasTimeout = f, true
			}
		case 1:
			if s, ok := v.(*runtime.String); ok && s != nil {
				host, hasHost = s.String(), true
			}
		}
	}
	return handler.run(id.String(), args, timeout, hasTimeout, host, hasHost)
}

// numericSeconds coerces an execute() timeout operand to seconds.
func numericSeconds(v runtime.Object) (float64, bool) {
	switch t := v.(type) {
	case runtime.Float:
		return float64(t), true
	case runtime.Int:
		if t.Int == nil {
			return 0, false
		}
		f, _ := new(big.Float).SetInt(t.Int).Float64()
		return f, true
	}
	return 0, false
}

// findModule returns the named module across the parsed trees.
func findModule(trees []*ttcn3.Tree, module string) *syntax.Module {
	for _, t := range trees {
		if t == nil || t.Root == nil {
			continue
		}
		for _, m := range t.Modules() {
			if mod, ok := m.Node.(*syntax.Module); ok && syntax.Name(mod.Name) == module {
				return mod
			}
		}
	}
	return nil
}

// findControlPart returns the module's control part, or nil.
func findControlPart(mod *syntax.Module) *syntax.ControlPart {
	if mod == nil {
		return nil
	}
	for _, d := range mod.Defs {
		if cp, ok := d.Def.(*syntax.ControlPart); ok && cp.Body != nil {
			return cp
		}
	}
	return nil
}

// findTestcase returns the named testcase declared in mod, or nil.
func findTestcase(mod *syntax.Module, name string) *syntax.FuncDecl {
	if mod == nil {
		return nil
	}
	for _, d := range mod.Defs {
		fn, ok := d.Def.(*syntax.FuncDecl)
		if ok && fn.IsTest() && syntax.Name(fn.Name) == name {
			return fn
		}
	}
	return nil
}

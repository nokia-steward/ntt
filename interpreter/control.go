package interpreter

import (
	"context"
	"fmt"
	"math/big"
	"runtime/debug"
	"time"

	"github.com/nokia/ntt/runtime"
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
	control := findControlPart(modNode)
	if control == nil {
		return runtime.ErrorVerdict, "", fmt.Errorf("module %q has no control part", module)
	}

	env, initErr := newModuleEnv(trees, modNode, module, opts)
	if initErr != "" {
		return runtime.ErrorVerdict, initErr, nil
	}

	agg := runtime.NoneVerdict
	aggReason := ""
	ran := false

	ctrlEnv := runtime.NewEnv(env)
	ctrlEnv.Set(runtime.ScopeNameKey, runtime.NewCharstring("control"))
	// Bind the handler on the MODULE scope, not the control scope:
	// fixtures routinely wrap execute() in a helper function, and a
	// function body evaluates in its own closure (the module scope), so a
	// control-scope binding would be invisible there. Each execute()
	// builds a fresh module scope of its own, which does not carry the
	// handler, so a testcase body cannot re-enter this.
	env.Set(executeHandlerKey, executeHandler{
		run: func(tcName string, args []runtime.Object, timeout float64, hasTimeout bool, host string, hasHost bool) runtime.Verdict {
			v, r := runOneExecute(trees, modNode, module, tcName, args, timeout, hasTimeout, host, hasHost, opts)
			ran = true
			if verdictRank(v) > verdictRank(agg) {
				agg, aggReason = v, r
			}
			return v
		}})

	if res := eval(control.Body, ctrlEnv); runtime.IsError(res) {
		e, _ := res.(*runtime.Error)
		return runtime.ErrorVerdict, "control: " + e.Inspect(), nil
	}
	if !ran {
		return runtime.NoneVerdict, "control part executed no testcase", nil
	}
	return agg, aggReason, nil
}

// runOneExecute runs a single testcase named by an execute() statement.
//
// The `timeout` and `host` operands are the two ways execute() can fail
// without the body ever producing a verdict (ETSI 26.1): a testcase that
// does not terminate within the timeout is stopped with `error`, and a
// host the runtime cannot resolve is `error` too. Neither is a testcase
// failure, so both bypass the body's own verdict.
func runOneExecute(trees []*ttcn3.Tree, modNode *syntax.Module, module, tcName string, args []runtime.Object, timeout float64, hasTimeout bool, host string, hasHost bool, opts TestcaseOptions) (runtime.Verdict, string) {
	if hasHost && !hostIsLocal(host) {
		return runtime.ErrorVerdict, fmt.Sprintf("execute: unknown host %q", host)
	}
	tcNode := findTestcase(modNode, tcName)
	if tcNode == nil {
		return runtime.ErrorVerdict, fmt.Sprintf("execute: testcase %q not found in %q", tcName, module)
	}

	inner := opts
	// A non-nil (possibly empty) slice means "these are the actuals";
	// nil would send runTestcaseIn back to mining the source statically.
	inner.actualArgs = args
	if inner.actualArgs == nil {
		inner.actualArgs = []runtime.Object{}
	}

	// An execute() timeout bounds the testcase the same way the
	// harness's per-case budget does, so reuse the context mechanism
	// rather than inventing a second stop path. The virtual clock does
	// not apply: `execute(TC(), 2.0)` bounds the whole testcase in real
	// time, it is not a TTCN-3 timer the scheduler can fast-forward.
	var deadline context.Context
	if hasTimeout {
		d := time.Duration(timeout * float64(time.Second))
		if d <= 0 {
			return runtime.ErrorVerdict, "execute: non-positive timeout"
		}
		parent := inner.Context
		if parent == nil {
			parent = context.Background()
		}
		ctx, cancel := context.WithTimeout(parent, d)
		defer cancel()
		inner.Context = ctx
		deadline = ctx
	}

	// Each execute() gets a module scope of its own, so module-level
	// state one testcase mutated cannot leak into the next.
	env, initErr := newModuleEnv(trees, modNode, module, inner)
	if initErr != "" {
		return runtime.ErrorVerdict, initErr
	}
	v, r, err := runTestcaseIn(env, runtime.NewTestcaseExec(module+"."+tcName),
		trees, modNode, module, tcName, tcNode, inner)
	if err != nil {
		return runtime.ErrorVerdict, err.Error()
	}
	if deadline != nil && deadline.Err() == context.DeadlineExceeded {
		return runtime.ErrorVerdict, fmt.Sprintf("execute: testcase did not terminate within %gs", timeout)
	}
	return v, r
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
			if dash, ok := a.(*syntax.Ident); ok && dash.String() == "-" {
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
		if dash, ok := extra.(*syntax.Ident); ok && dash.String() == "-" {
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

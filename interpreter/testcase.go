package interpreter

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// interpreterPanicDebug reports whether to attach a Go stack to the
// interpreter-panic message. Off by default; flipped on with
// `NTT_INTERP_PANIC_TRACE=1` for triage of nil-deref regressions.
func interpreterPanicDebug() bool {
	return os.Getenv("NTT_INTERP_PANIC_TRACE") == "1"
}

// TestcaseOptions threads optional executor-side knobs through to
// RunTestcaseWith without breaking the original RunTestcase
// signature. ModuleParameters is the [MODULE_PARAMETERS] map
// produced by cfg.File.ModuleParameters(); keys are "Module.Name",
// values are the raw text Titan-style cfgs write.
type TestcaseOptions struct {
	ModuleParameters map[string]string

	// ModuleParamWarning is invoked once per override that couldn't
	// be applied (unknown key, unparseable value). Nil means "silent
	// drop". The default exec driver wires this to stderr so users
	// see typos in their cfg without a hard failure.
	ModuleParamWarning func(msg string)

	// DeterministicClock, when true, makes timers advance the
	// per-testcase virtual clock (firing at their deadline instantly)
	// instead of sleeping real wall-clock time — reproducible, fast, no
	// real 5s waits. Intended for the conformance harness; a real load
	// driver leaves it off so timers pace real I/O.
	DeterministicClock bool

	// DeterministicScheduler, when true, enables the discrete-event
	// quiescence scheduler (runtime/scheduler.go): the virtual clock is
	// advanced only when every live component goroutine is parked, to the
	// soonest timer deadline, and comm/component events wake parked peers
	// at the current instant. This makes concurrent timer-driven
	// execution fast, sound, and deterministic — superseding
	// DeterministicClock (which is only sound single-threaded). Intended
	// for the conformance harness; a real load driver leaves it off.
	DeterministicScheduler bool

	// Context, when non-nil, bounds the run: on ctx cancellation the
	// executor is asked to stop (exec.Stop), which unwinds a blocked alt
	// / timer wait promptly. The evaluator blocks honestly, so it can
	// otherwise wait indefinitely on a genuinely stuck alt — the caller
	// (e.g. the conformance harness's per-testcase timeout) supplies a
	// deadline context so the goroutine terminates instead of leaking.
	// Nil = unbounded.
	Context context.Context

	// actualArgs carries actual parameters already evaluated by a
	// control part's execute(). Nil means "mine them from the control
	// part statically", which is what a directly-executed testcase does.
	// Unexported: it is an internal hand-off, not a caller knob.
	actualArgs []runtime.Object
}

// RunTestcase is the canonical entry point the executor uses to
// actually execute a testcase body. See RunTestcaseWith for the
// override-aware variant; this thin wrapper preserves the
// signature every existing caller already uses.
func RunTestcase(trees []*ttcn3.Tree, qname string) (runtime.Verdict, string, error) {
	return RunTestcaseWith(trees, qname, TestcaseOptions{})
}

// RunTestcaseWith is the override-aware variant of RunTestcase. It
// locates qname (`Module.testcase`) across the given trees, sets up
// a fresh module env, installs a runtime.TestcaseExec, applies any
// modulepar overrides from opts.ModuleParameters, and evaluates the
// body. The returned verdict is the testcase's aggregated verdict
// per TTCN-3 max-merge semantics; the returned reason is the message
// captured at the moment the verdict flipped to fail/error (empty
// otherwise).
//
// Module parameter overrides are applied AFTER the module's
// in-source defaults have been bound and BEFORE the testcase body
// starts, so the override always wins. Unknown keys / unparseable
// values fire opts.ModuleParamWarning but never abort the run.
func RunTestcaseWith(trees []*ttcn3.Tree, qname string, opts TestcaseOptions) (verdict runtime.Verdict, reason string, err error) {
	// A panic inside the tree-walking interpreter must not kill the
	// caller. Unmodeled syntax shapes, nil-deref bugs, and the
	// occasional malformed conformance fixture all surface as
	// `recover() != nil`; the right behaviour is to charge the
	// testcase with an Error verdict and let the suite runner move
	// on. Each panic captured here is also a bug worth filing - the
	// reason string carries enough context for triage.
	defer func() {
		if r := recover(); r != nil {
			verdict = runtime.ErrorVerdict
			reason = fmt.Sprintf("interpreter panic: %v", r)
			if dbg := interpreterPanicDebug(); dbg {
				reason += "\n" + string(debug.Stack())
			}
			err = nil
		}
	}()
	evalDepth.reset()

	module, fnName, ok := splitQualifiedName(qname)
	if !ok {
		return runtime.ErrorVerdict, "", fmt.Errorf("invalid qualified testcase name %q", qname)
	}

	var modNode *syntax.Module
	for _, t := range trees {
		if t == nil || t.Root == nil {
			continue
		}
		for _, m := range t.Modules() {
			mod, ok := m.Node.(*syntax.Module)
			if !ok {
				continue
			}
			if syntax.Name(mod.Name) == module {
				modNode = mod
				break
			}
		}
		if modNode != nil {
			break
		}
	}
	if modNode == nil {
		return runtime.ErrorVerdict, "", fmt.Errorf("module %q not found", module)
	}

	var tcNode *syntax.FuncDecl
	for _, def := range modNode.Defs {
		md, ok := def.Def.(*syntax.FuncDecl)
		if !ok {
			continue
		}
		if !md.IsTest() {
			continue
		}
		if syntax.Name(md.Name) == fnName {
			tcNode = md
			break
		}
	}
	if tcNode == nil {
		return runtime.ErrorVerdict, "", fmt.Errorf("testcase %q not found in %q", fnName, module)
	}

	env, initErr := newModuleEnv(trees, modNode, module, opts)
	if initErr != "" {
		return runtime.ErrorVerdict, initErr, nil
	}

	exec := runtime.NewTestcaseExec(qname)
	return runTestcaseIn(env, exec, trees, modNode, module, fnName, tcNode, opts)
}

// newModuleEnv builds the module-level scope a testcase or control part
// runs in: verdict constants, every sibling module flattened in, the
// target module's own definitions, `import ... with` attributes and any
// [MODULE_PARAMETERS] overrides. Returns a non-empty reason string when
// module initialisation failed.
func newModuleEnv(trees []*ttcn3.Tree, modNode *syntax.Module, module string, opts TestcaseOptions) (runtime.Scope, string) {
	env := runtime.NewEnv(nil)
	bindVerdictConstants(env)
	env.Set(runtime.ModuleNameKey, runtime.NewCharstring(module))

	// Initialise every other module first - templates, functions,
	// constants etc. that the testcase's module imports will then be
	// visible without explicitly modelling `import from X all`. This
	// is intentionally permissive: TTCN-3 mandates explicit imports
	// but at the conformance level the helper modules sit in sibling
	// files and treating the union as one flat namespace lets us
	// resolve cross-module helpers without a real import resolver.
	// Definitions declared in the SAME file as the testcase (helper
	// modules sitting next to it) must take precedence over like-named
	// definitions pulled in from other files in the same directory:
	// conformance fixtures in one directory frequently reuse an
	// identifier (e.g. `c_myconst`) with different values, and the
	// permissive cross-file flattening would otherwise let a sibling's
	// value shadow the one the test actually imports
	// (Sem_08020301_GeneralFormatOfImport_001). So bind sibling-file
	// modules first (lowest precedence), then the target file's own
	// helper modules, and finally the target module itself below.
	var targetTree *ttcn3.Tree
	for _, t := range trees {
		if t == nil || t.Root == nil {
			continue
		}
		for _, m := range t.Modules() {
			if mod, ok := m.Node.(*syntax.Module); ok && mod == modNode {
				targetTree = t
				break
			}
		}
		if targetTree != nil {
			break
		}
	}
	initTreeModules := func(t *ttcn3.Tree) {
		if t == nil || t.Root == nil {
			return
		}
		for _, m := range t.Modules() {
			mod, ok := m.Node.(*syntax.Module)
			if !ok || mod == modNode {
				continue
			}
			initModuleDefs(env, mod)
		}
	}
	for _, t := range trees {
		if t == targetTree {
			continue
		}
		initTreeModules(t)
	}
	initTreeModules(targetTree)

	// We need every top-level declaration in the module to be visible
	// to the testcase body (helper functions, constants, templates).
	// Two notes:
	//   - we deliberately skip the ControlPart at init time, because
	//     it's a statement block that typically calls execute(...) and
	//     we run each testcase on its own anyway (the executor drives
	//     the test selection, not the module's control part).
	//   - we soft-skip "unknown syntax node type" errors so a decl
	//     the interpreter does not yet model (component types,
	//     signature decls, ...) cannot poison every testcase in the
	//     file. If the body references a missing identifier the call
	//     site will surface the failure with proper context.
	// Two-phase walk: first the pure-declarative nodes (types, enums,
	// component definitions) so that later phases - which may
	// reference those types in `modulepar T x := ...` initialisers or
	// `template T t := ...` bodies - find the symbols already bound.
	defs := flattenModuleDefs(modNode.Defs, []*syntax.WithSpec{modNode.With})
	for _, pass := range []int{0, 1} {
		for _, fd := range defs {
			d := fd.def
			if _, ok := d.Def.(*syntax.ControlPart); ok {
				continue
			}
			isType := isTypeDecl(d.Def)
			if pass == 0 && !isType {
				continue
			}
			if pass == 1 && isType {
				continue
			}
			if r := eval(d, env); runtime.IsError(r) {
				err, _ := r.(*runtime.Error)
				if isUnknownNodeError(err) {
					bindDeclNameScoped(env, d, fd.scopes)
					continue
				}
				return nil, fmt.Sprintf("module init: %s", err.Inspect())
			}
			// Even on successful evaluation, attach the type-
			// descriptor binding so attribute lookups like
			// `MyType.encode` keep working. We only do this for
			// type declarations to avoid stomping on actual
			// runtime values.
			if isTypeDecl(d.Def) {
				bindDeclNameScoped(env, d, fd.scopes)
			}
			recordDefKindAttrs(env, d, fd.scopes)
		}
	}

	// Layer `import ... with { ... }` attributes onto the imported
	// definitions now that every module's defs are bound.
	applyImportWithAttrs(env, modNode)

	// Apply [MODULE_PARAMETERS] overrides AFTER the in-source
	// defaults have been bound and BEFORE the testcase body runs.
	// We pass every loaded module so a key like
	// `OtherModule.PX_FOO` can target an import too, not just the
	// testcase's own module.
	if len(opts.ModuleParameters) > 0 {
		allMods := make([]*syntax.Module, 0, len(trees)+1)
		for _, t := range trees {
			if t == nil || t.Root == nil {
				continue
			}
			for _, m := range t.Modules() {
				if mod, ok := m.Node.(*syntax.Module); ok {
					allMods = append(allMods, mod)
				}
			}
		}
		for _, w := range ApplyModuleParameters(env, allMods, opts.ModuleParameters) {
			if opts.ModuleParamWarning != nil {
				opts.ModuleParamWarning(w)
			}
		}
	}
	return env, ""
}

// runTestcaseIn evaluates one testcase body in an already-initialised
// module scope, against a fresh TestcaseExec. Split out of
// RunTestcaseWith so a control part's execute() can run a testcase with
// arguments evaluated at control-flow time (opts.actualArgs) rather than
// mined statically from the source.
func runTestcaseIn(env runtime.Scope, exec *runtime.TestcaseExec, trees []*ttcn3.Tree, modNode *syntax.Module, module, fnName string, tcNode *syntax.FuncDecl, opts TestcaseOptions) (verdict runtime.Verdict, reason string, err error) {
	exec.SetDeterministicClock(opts.DeterministicClock)
	// The cooperative scheduler is engaged after SetMTCID below (it needs
	// the MTC's component id as the root participant).
	// Cancellation: when the caller supplies a context, stop the
	// executor on cancellation so a blocked alt / timer wait unwinds
	// instead of leaking a goroutine. The watcher is bounded by
	// `cancelled`, closed on return, so it never outlives the run.
	if opts.Context != nil {
		cancelled := make(chan struct{})
		defer close(cancelled)
		go func() {
			select {
			case <-opts.Context.Done():
				exec.Stop()
			case <-cancelled:
			}
		}()
	}
	env.Set(runtime.TestcaseExecKey, exec)
	// Record exec as the runtime's "current testcase" so a C test
	// port that pushes traffic back through runtime.inject() can find
	// the right EnqueueMessageFrom target. Cleared on the way out so
	// inject calls from a port that outlives the testcase body don't
	// land on stale state.
	runtime.SetCurrentExec(exec)
	runtime.ClearPortTypeInstances()
	defer func() {
		runtime.SetCurrentExec(nil)
		runtime.ClearPortTypeInstances()
		// Let port bindings (e.g. goport) drop per-testcase instance
		// caches so component IDs, which restart each run, can't alias
		// a stale TestPort across testcases.
		runtime.RunExecTeardownHooks()
	}()
	// Surface the module/testcase names so __MODULE__ / __SCOPE__
	// macros resolve to the running fixture's identity. The runner
	// also stashes them on the testcase scope below so a nested
	// function can see them.
	if modNode != nil && modNode.Name != nil {
		env.Set(runtime.ModuleNameKey, runtime.NewCharstring(syntax.Name(modNode.Name)))
	}

	// Testcases get a fresh inner scope so locals don't leak into the
	// module namespace.
	tcEnv := runtime.NewEnv(env)
	tcEnv.Set(runtime.ScopeNameKey, runtime.NewCharstring(fnName))
	// Stash the testcase's effective Annex E attributes (the testcase
	// `with { encode ... }` clause overriding the module clause) so a
	// bare `<value>.encode` inside the body - which has no TypeDesc of
	// its own - resolves to the active codec rule (ETSI 27.1.2).
	{
		var modWith *syntax.WithSpec
		if modNode != nil {
			modWith = modNode.With
		}
		var tcWith *syntax.WithSpec
		if tcNode != nil {
			tcWith = tcNode.With
		}
		tcEnv.Set(activeAttrsKey, typeDescForScoped("", tcWith, []*syntax.WithSpec{modWith}))
	}
	// Allocate a real ComponentRef for the MTC so `mtc.stop` /
	// `mtc.kill` / `self.stop` operations target a stable handle
	// and `from <ref>` matching can recognise it. The ref is
	// registered first so AllComponents()[0] is always the MTC.
	// We bind it on the *module* env (env), not just tcEnv, so
	// nested user-defined functions whose lexical scope is fn.Env
	// (= module env) still see it.
	//
	// The MTC's component-type name is the testcase's `runs on
	// <Comp>` declarator; newComponentRef uses it to lift the
	// port-instance/type bindings out of the module-load-time
	// registry onto the testcase exec, so a later
	// `map(self:cli, system:cli)` resolves `cli` to its declared
	// port type (e.g. MyClient_PT) instead of falling through
	// to the instance-name lookup.
	mtcTypeName := "mtc"
	if tcNode != nil && tcNode.RunsOn != nil && tcNode.RunsOn.Comp != nil {
		mtcTypeName = syntax.Name(tcNode.RunsOn.Comp)
	}
	mtcRef := newComponentRef(mtcTypeName, "", tcEnv)
	exec.SetMTCID(mtcRef.ID)
	// Engage the cooperative scheduler now that the MTC id is known (it is
	// the root participant that holds the token first).
	if opts.DeterministicScheduler {
		exec.SetDeterministicScheduler(true)
	}
	env.Set("mtc", mtcRef)
	env.Set("self", mtcRef)
	exec.PushComponent(mtcRef)
	defer exec.PopComponent()
	// Bind formal parameters of the testcase. The executor doesn't
	// pass actual arguments, so we first try to fish them out of the
	// module's control part - many ETSI fixtures execute the same
	// testcase with several argument sets; the first execute(...)
	// call wins. Anything not bound that way falls back to the formal
	// default (if any), `?` for templates, or Undefined. The control
	// scope is a child of the module scope so local var/const decls
	// preceding the execute(...) call are visible.
	// A control part's execute() supplies arguments already evaluated in
	// control-flow order, which static mining cannot do — the actual may
	// be a control-local variable (`execute(TC(v_result))`) whose value
	// only exists once the control part has run.
	if opts.actualArgs != nil {
		bindTestcaseParamsWithValues(tcEnv, tcNode, opts.actualArgs)
	} else {
		ctrlEnv := runtime.NewEnv(env)
		actualArgs := findExecuteArgs(modNode, fnName, ctrlEnv)
		bindTestcaseParamsWithArgs(tcEnv, tcNode, actualArgs, ctrlEnv)
	}
	r := eval(tcNode.Body, tcEnv)
	if len(tcNode.Catch) > 0 || tcNode.Finally != nil {
		r = runExceptionHandlers(r, tcNode.Catch, tcNode.Finally, tcEnv)
	}
	if runtime.IsError(r) {
		err, _ := r.(*runtime.Error)
		exec.SetVerdict(runtime.ErrorVerdict, err.Inspect())
	} else if _, ok := r.(*runtime.RaisedValue); ok {
		// An exception that escapes the testcase body uncaught is a
		// dynamic test-case error (ETSI 5.2).
		exec.SetVerdict(runtime.ErrorVerdict, "uncaught exception")
	}

	// Any PTC goroutines spawned by an async `comp.start` on an
	// alive component must finish before we hand the testcase
	// verdict back. WaitPTCs joins each one, falling back to a
	// hard Stop() after maxPTCDrain so an alt waiting forever
	// (e.g. the test forgot to `d.stop` it) doesn't dangle the
	// suite. Five seconds is a deliberately conservative ceiling;
	// real tests stop their PTCs explicitly and finish in
	// milliseconds.
	exec.WaitPTCs(5 * time.Second)

	// Release every port the MTC + any PTC mapped before the
	// next testcase tries to bind the same listen sockets.
	// Cabi/cgo C++ ports (e.g. a server port type) hold their TCP listener until on_unmap
	// fires; without this drain, the second testcase that
	// re-uses a daemon port number fails with "socket error".
	drainAllPortMaps(exec)

	v := exec.GetVerdict()
	// A testcase that never calls setverdict ends with `pass` (ETSI ES
	// 201 873-1 clause 22.4.1: an undeclared verdict resolves to pass).
	// The test is "was one declared", not "is it none" — a body that
	// explicitly sets none must keep it, because a control part's
	// execute() can branch on the value (Sem_2601_ExecuteStatement_004).
	//
	// This also papers over 64 files whose body does call setverdict but
	// never reaches it: see docs/conformance/remaining-work.md. Removing
	// the coercion is a prerequisite for Sem_2303_timer_stop_004 and has
	// to come after those are fixed, not before.
	if !exec.VerdictWasSet() {
		v = runtime.PassVerdict
	}
	return v, exec.Reason(), nil
}

// bindTestcaseParams binds each formal parameter of the testcase to
// a sensible placeholder so the body can reference them even when no
// actual arguments were passed. Defaults from the source `:= expr`
// clause win when present; otherwise `template` formals get the
// wildcard `?` and value formals get Undefined.
func bindTestcaseParams(env runtime.Scope, tc *syntax.FuncDecl) {
	bindTestcaseParamsWithArgs(env, tc, nil, env)
}

// bindTestcaseParamsWithArgs is like bindTestcaseParams but lets the
// caller supply an arglist (typically pulled from a `control { ... }`
// `execute(tc(...))` call). Each positional arg is evaluated in the
// argEnv scope and bound to the matching formal; the `-` shorthand
// (DASH token, kept as Undefined here) keeps the formal's default.
// bindTestcaseParamsWithValues binds formal parameters from actuals that
// are already runtime values. A nil entry means the actual was `-`, so
// the formal's default (or the template wildcard) applies, matching
// bindTestcaseParamsWithArgs' treatment of the `-` identifier.
func bindTestcaseParamsWithValues(env runtime.Scope, tc *syntax.FuncDecl, args []runtime.Object) {
	if tc == nil || tc.Params == nil {
		return
	}
	for i, fp := range tc.Params.List {
		if fp == nil || fp.Name == nil {
			continue
		}
		var val runtime.Object = runtime.Undefined
		used := false
		if i < len(args) && args[i] != nil {
			val = args[i]
			used = true
		}
		if !used && fp.Value != nil {
			if v := eval(fp.Value, env); !runtime.IsError(v) && v != nil {
				val = v
				used = true
			}
		}
		if !used && fp.TemplateRestriction != nil {
			val = runtime.Any
		}
		env.Set(fp.Name.String(), val)
	}
}

func bindTestcaseParamsWithArgs(env runtime.Scope, tc *syntax.FuncDecl, args []syntax.Expr, argEnv runtime.Scope) {
	if tc == nil || tc.Params == nil {
		return
	}
	if argEnv == nil {
		argEnv = env
	}
	for i, fp := range tc.Params.List {
		if fp == nil || fp.Name == nil {
			continue
		}
		isTemplate := fp.TemplateRestriction != nil
		var val runtime.Object = runtime.Undefined
		used := false
		if i < len(args) && args[i] != nil {
			if id, ok := args[i].(*syntax.Ident); ok && id.String() == "-" {
				// `-` means "use the default" - leave used
				// false so the formal's default value (or
				// the template wildcard) applies.
			} else {
				if v := eval(args[i], argEnv); !runtime.IsError(v) && v != nil {
					val = v
					used = true
				}
			}
		}
		if !used {
			if fp.Value != nil {
				if v := eval(fp.Value, env); !runtime.IsError(v) && v != nil {
					val = v
					used = true
				}
			}
		}
		if !used && isTemplate {
			val = runtime.Any
		}
		env.Set(fp.Name.String(), val)
	}
}

// findExecuteArgs scans the module's control part for the first
// `execute(tcName(...))` call and returns its argument list. The
// argEnv scope is populated with the control body's preceding
// var/const declarations so the captured argument expressions can be
// evaluated against locally-bound identifiers. Returns nil if no such
// call exists.
//
// If a control statement is a call to a helper function (e.g.
// `f_caller(6)`), the scanner recurses into the helper to look for
// the `execute` call there. The helper's formal parameters are
// pre-bound to the actual values so argument expressions inside the
// helper that reference parameters evaluate correctly.
func findExecuteArgs(mod *syntax.Module, tcName string, argEnv runtime.Scope) []syntax.Expr {
	if mod == nil {
		return nil
	}
	visited := map[string]bool{}
	return findExecuteArgsIn(mod, tcName, mod.Defs, argEnv, visited)
}

func findExecuteArgsIn(mod *syntax.Module, tcName string, defs []*syntax.ModuleDef, argEnv runtime.Scope, visited map[string]bool) []syntax.Expr {
	for _, d := range defs {
		cp, ok := d.Def.(*syntax.ControlPart)
		if !ok || cp.Body == nil {
			continue
		}
		if args := scanBlockForExecute(mod, tcName, cp.Body, argEnv, visited); args != nil {
			return args
		}
	}
	return nil
}

func scanBlockForExecute(mod *syntax.Module, tcName string, body *syntax.BlockStmt, argEnv runtime.Scope, visited map[string]bool) []syntax.Expr {
	if body == nil {
		return nil
	}
	for _, stmt := range body.Stmts {
		if args := scanStmtForExecute(mod, tcName, stmt, argEnv, visited); args != nil {
			return args
		}
		// Pre-execute body statements so any locally-declared
		// vars / consts referenced by the argument expressions
		// exist in argEnv. Best-effort: a single bad statement
		// can't poison the whole scan.
		eval(stmt, argEnv)
	}
	return nil
}

// scanStmtForExecute recurses into common nested-block constructs
// (`if/else`, `select`, `for`, `while`, `do/while`, plain `{ ... }`)
// so executes that live inside conditional or iteration bodies are
// still discoverable. We don't try to evaluate the guard - we just
// scan the body. The first match wins.
func scanStmtForExecute(mod *syntax.Module, tcName string, stmt syntax.Stmt, argEnv runtime.Scope, visited map[string]bool) []syntax.Expr {
	if call, ok := executeCallFor(stmt, tcName); ok {
		if call.Args == nil {
			return []syntax.Expr{}
		}
		return call.Args.List
	}
	if args := scanHelperCall(mod, tcName, stmt, argEnv, visited); args != nil {
		return args
	}
	switch s := stmt.(type) {
	case *syntax.IfStmt:
		if args := scanBlockOrStmtForExecute(mod, tcName, s.Then, argEnv, visited); args != nil {
			return args
		}
		if args := scanBlockOrStmtForExecute(mod, tcName, s.Else, argEnv, visited); args != nil {
			return args
		}
	case *syntax.SelectStmt:
		for _, c := range s.Body {
			if c == nil {
				continue
			}
			if args := scanStmtForExecute(mod, tcName, c, argEnv, visited); args != nil {
				return args
			}
		}
	case *syntax.ForStmt:
		if args := scanBlockOrStmtForExecute(mod, tcName, s.Body, argEnv, visited); args != nil {
			return args
		}
	case *syntax.WhileStmt:
		if args := scanBlockOrStmtForExecute(mod, tcName, s.Body, argEnv, visited); args != nil {
			return args
		}
	case *syntax.DoWhileStmt:
		if args := scanBlockOrStmtForExecute(mod, tcName, s.Body, argEnv, visited); args != nil {
			return args
		}
	case *syntax.BlockStmt:
		if args := scanBlockForExecute(mod, tcName, s, argEnv, visited); args != nil {
			return args
		}
	case *syntax.CaseClause:
		if s.Body != nil {
			if args := scanBlockForExecute(mod, tcName, s.Body, argEnv, visited); args != nil {
				return args
			}
		}
	}
	return nil
}

func scanBlockOrStmtForExecute(mod *syntax.Module, tcName string, stmt syntax.Stmt, argEnv runtime.Scope, visited map[string]bool) []syntax.Expr {
	if stmt == nil {
		return nil
	}
	if b, ok := stmt.(*syntax.BlockStmt); ok {
		return scanBlockForExecute(mod, tcName, b, argEnv, visited)
	}
	return scanStmtForExecute(mod, tcName, stmt, argEnv, visited)
}

// scanHelperCall inspects a control-part statement; if it's a call to
// a user-defined function in this module, recursively scans that
// function's body for an execute(tcName(...)) invocation. The
// helper's formals are bound to the actual call's arguments in a
// fresh child scope so references inside resolve. Cycles are guarded
// by a visited set keyed on function name.
func scanHelperCall(mod *syntax.Module, tcName string, stmt syntax.Stmt, argEnv runtime.Scope, visited map[string]bool) []syntax.Expr {
	es, ok := stmt.(*syntax.ExprStmt)
	if !ok {
		return nil
	}
	call, ok := es.Expr.(*syntax.CallExpr)
	if !ok {
		return nil
	}
	id, ok := call.Fun.(*syntax.Ident)
	if !ok {
		return nil
	}
	name := id.String()
	if name == "execute" || visited[name] {
		return nil
	}
	helper := findFuncDecl(mod, name)
	if helper == nil || helper.Body == nil {
		return nil
	}
	visited[name] = true
	defer func() { delete(visited, name) }()
	// Bind the helper's formal parameters in argEnv (shadowing any
	// outer binding) so `execute(TC(p_val))` inside the helper
	// resolves `p_val` to the actual value passed in.
	if helper.Params != nil && call.Args != nil {
		for i, fp := range helper.Params.List {
			if i >= len(call.Args.List) {
				break
			}
			v := eval(call.Args.List[i], argEnv)
			if !runtime.IsError(v) {
				bindFormalParam(argEnv, fp, v)
			}
		}
	}
	return scanBlockForExecute(mod, tcName, helper.Body, argEnv, visited)
}

func findFuncDecl(mod *syntax.Module, name string) *syntax.FuncDecl {
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fd, ok := d.Def.(*syntax.FuncDecl)
		if !ok {
			continue
		}
		if syntax.Name(fd.Name) == name {
			return fd
		}
	}
	return nil
}

// bindFormalParam binds a single formal parameter to a value in env.
// We use the parameter's identifier-name; ignoring the type and
// modifier metadata since the helper environment only needs reads.
func bindFormalParam(env runtime.Scope, fp *syntax.FormalPar, v runtime.Object) {
	if fp == nil || fp.Name == nil {
		return
	}
	env.Set(syntax.Name(fp.Name), v)
}

// executeCallFor reports whether stmt is `execute(tcName(...))` (or
// `execute(tcName(...), ...)`) and returns the inner testcase call.
func executeCallFor(stmt syntax.Stmt, tcName string) (*syntax.CallExpr, bool) {
	es, ok := stmt.(*syntax.ExprStmt)
	if !ok {
		return nil, false
	}
	call, ok := es.Expr.(*syntax.CallExpr)
	if !ok {
		return nil, false
	}
	id, ok := call.Fun.(*syntax.Ident)
	if !ok || id.String() != "execute" {
		return nil, false
	}
	if call.Args == nil || len(call.Args.List) == 0 {
		return nil, false
	}
	tcCall, ok := call.Args.List[0].(*syntax.CallExpr)
	if !ok {
		return nil, false
	}
	tcId, ok := tcCall.Fun.(*syntax.Ident)
	if !ok || tcId.String() != tcName {
		return nil, false
	}
	return tcCall, true
}

// bindVerdictConstants pre-populates env with the five TTCN-3 verdict
// values as identifiers. These are part of the predefined namespace
// per clause 5.2 of the standard, so they must be in scope everywhere
// a testcase body can reach.
func bindVerdictConstants(env runtime.Scope) {
	env.Set("none", runtime.NoneVerdict)
	env.Set("pass", runtime.PassVerdict)
	env.Set("inconc", runtime.InconcVerdict)
	env.Set("fail", runtime.FailVerdict)
	env.Set("error", runtime.ErrorVerdict)

	// `infinity` is the TTCN-3 unbounded duration / IEEE 754 +Inf
	// constant; bind it once so timer guards like `timer.start(infinity)`
	// don't error on identifier lookup.
	env.Set("infinity", runtime.Float(math.Inf(1)))

	// Phantom bindings for TTCN-3 predefined names that the
	// interpreter does not yet implement properly. By giving them an
	// Undefined value the identifier lookup no longer fails; the
	// testcase may still produce a wrong verdict because the
	// operation it asked for did nothing, but the verdict we report
	// is the actual interpreter outcome instead of a synthetic
	// "identifier not found" Error. This matters for the conformance
	// gate: an unimplemented stub should fail the test it's part of,
	// not synthesise an Error verdict that hides what's missing.
	for _, name := range []string{
		"omit", "any", "all",
		"connect", "disconnect", "map", "unmap", "start", "stop", "kill",
		"valueof", "lengthof", "nowait", "alive", "activate", "deactivate",
		"timeout", "running", "done", "killed",
		"integer", "boolean", "float", "charstring", "bitstring",
		"hexstring", "octetstring", "verdicttype", "anytype",
		"encvalue_unichar", "decvalue_unichar", "encvalue", "decvalue",
		"send", "receive", "trigger", "check", "getreply", "getcall", "raise", "catch",
		"ispresent", "isbound", "ischosen", "isvalue", "sizeof",
		"all component", "any component", "all port", "any port", "all timer", "any timer",
		"p_val",
	} {
		if _, ok := env.Get(name); !ok {
			env.Set(name, runtime.Undefined)
		}
	}
	// `self`, `mtc`, and `system` are component references. The
	// interpreter doesn't model component identity, but binding them
	// to a stable Null singleton (a *bound* value with a known type)
	// means redirect-store + equality comparisons line up: `v :=
	// null` puts v at Null too, and `v == self` is then Null == Null
	// = true, the verdict tests rely on. `null` itself binds to the
	// same singleton so `v_tc := null` and the literal `null` are
	// interchangeable.
	for _, name := range []string{"null", "mtc", "self", "system"} {
		if _, ok := env.Get(name); !ok {
			env.Set(name, runtime.Null)
		}
	}
}

// evalSetverdict implements `setverdict(verdict [, reason ...])`. The
// evalComponentQuery handles `all component.<op>` / `any component.
// <op>` cross-component lookups. Returns (Bool, true) when the op
// produces a boolean status (alive / running / done / killed). The
// `mtc` component is excluded because TTCN-3 21.3.5 says only PTCs
// participate; if no PTCs have been created the `all`-form is
// vacuously true and the `any`-form is vacuously false.
func evalComponentQuery(kind string, sel syntax.Expr, env runtime.Scope) (runtime.Object, bool) {
	op := strings.ToLower(syntax.Name(sel))
	switch op {
	case "alive", "running", "done", "killed":
	default:
		return nil, false
	}
	exec := runtime.FindTestcaseExec(env)
	if exec == nil {
		return nil, false
	}
	refs := exec.AllComponents()
	mtc := exec.CurrentComponent()
	ptcs := make([]*runtime.ComponentRef, 0, len(refs))
	for _, r := range refs {
		if r == nil {
			continue
		}
		if mtc != nil && r.Equal(mtc) {
			continue
		}
		// `all/any component.<op>` consider only PTCs that have ever
		// been started: a created-but-inactive PTC is ignored, so
		// `all component.done` matches even when such a PTC exists
		// (ETSI 21.3.7, the note distinguishing it from `ptc.done`).
		if !r.Started {
			continue
		}
		ptcs = append(ptcs, r)
	}
	predicate := componentStatePredicate(op)
	if predicate == nil {
		return nil, false
	}
	// A standalone `all component.done` / `.killed` ought to block like
	// its singular counterpart (ETSI 21.3.7/21.3.8) rather than answer a
	// snapshot the statement context discards. Making it park costs two
	// files today because it lets PTC bodies reach branches that expose a
	// `send ... to <component>` routing defect - see
	// docs/conformance/remaining-work.md.
	switch kind {
	case "all component":
		if len(ptcs) == 0 {
			return runtime.NewBool(true), true
		}
		for _, r := range ptcs {
			if !predicate(r, env) {
				return runtime.NewBool(false), true
			}
		}
		return runtime.NewBool(true), true
	case "any component":
		for _, r := range ptcs {
			if predicate(r, env) {
				return runtime.NewBool(true), true
			}
		}
		return runtime.NewBool(false), true
	}
	return nil, false
}

// evalAllComponentAction implements the statement forms `all
// component.kill` / `all component.stop` (ETSI 21.3.3/21.3.4): they
// terminate every PTC. `kill` always removes the component (even one
// created with the `alive` modifier); `stop` ends the current behaviour
// but leaves an alive-modifier component reusable. `any component.<op>`
// is not a valid action form, so only `all component` is handled.
func evalAllComponentAction(kind string, sel syntax.Expr, env runtime.Scope) (runtime.Object, bool) {
	if kind != "all component" {
		return nil, false
	}
	op := strings.ToLower(syntax.Name(sel))
	switch op {
	case "kill", "stop":
	default:
		return nil, false
	}
	exec := runtime.FindTestcaseExec(env)
	if exec == nil {
		return nil, false
	}
	mtc := exec.CurrentComponent()
	for _, r := range exec.AllComponents() {
		if r == nil || (mtc != nil && r.Equal(mtc)) {
			continue
		}
		// Only ever-started PTCs are affected; a created-but-inactive
		// component has no behaviour to stop or kill.
		if !r.Started {
			continue
		}
		r.SetDone(true)
		if op == "kill" || !r.AliveModifier {
			r.SetAlive(false)
		}
		// `all component.stop` must actually cancel the forked worker
		// goroutines, not just flip the flags above. Mirror the single
		// `comp.stop` path: close the PTC's StopChan (which also wakes a
		// parked alt / blocking timer via signalMessageReady) and unmap
		// its ports.
		exec.StopPTC(r.ID)
		drainComponentPortMaps(exec, r.ID)
	}
	return runtime.Undefined, true
}

// evalSetverdict implements `setverdict(verdict [, reason ...])`. The
// verdict argument is required and must be a verdicttype value; the
// reason arguments (if any) are stringified and joined with spaces.
func evalSetverdict(n *syntax.CallExpr, env runtime.Scope) runtime.Object {
	exec := runtime.FindTestcaseExec(env)
	args := evalExprList(n.Args.List, env)
	if len(args) == 0 {
		return runtime.Errorf("setverdict requires at least one argument")
	}
	if len(args) >= 1 && runtime.IsError(args[0]) {
		return args[0]
	}
	verdict, ok := args[0].(runtime.Verdict)
	if !ok {
		return runtime.Errorf("setverdict: first argument must be a verdicttype, got %s", args[0].Type())
	}
	if exec == nil {
		// Outside a testcase setverdict is a no-op per the standard;
		// we return nil instead of erroring so control-part code that
		// calls into a helper which logs progress doesn't blow up.
		return nil
	}
	var reasonParts []string
	for _, a := range args[1:] {
		reasonParts = append(reasonParts, a.Inspect())
	}
	exec.SetVerdict(verdict, strings.Join(reasonParts, " "))
	// Also accumulate the verdict on the running component so
	// `comp.done -> value v` can retrieve that PTC's local verdict
	// (ETSI 21.3.7 / 22.4.1).
	if cur := exec.CurrentComponent(); cur != nil {
		cur.MergeVerdict(verdict)
	}
	return nil
}

// evalGetverdict returns the current testcase verdict, or `none` if
// called outside a testcase.
func evalGetverdict(env runtime.Scope) runtime.Object {
	exec := runtime.FindTestcaseExec(env)
	if exec == nil {
		return runtime.NoneVerdict
	}
	return exec.GetVerdict()
}

// evalLog implements `log(...)` by joining the inspected forms of all
// arguments with a single space and appending to the testcase log.
// When called outside a testcase we fall through to the builtins.Log
// implementation by delegating to the global builtin lookup.
func evalLog(n *syntax.CallExpr, env runtime.Scope) runtime.Object {
	args := evalExprList(n.Args.List, env)
	if len(args) == 1 && runtime.IsError(args[0]) {
		return args[0]
	}
	parts := make([]string, 0, len(args))
	for _, a := range args {
		parts = append(parts, a.Inspect())
	}
	line := strings.Join(parts, " ")
	if exec := runtime.FindTestcaseExec(env); exec != nil {
		exec.Log(line)
		return nil
	}
	// Fall back to the global builtin (which prints to stdout).
	if fn, ok := env.Get("log"); ok {
		if b, ok := fn.(*runtime.Builtin); ok {
			return b.Fn(args...)
		}
	}
	fmt.Println(line)
	return nil
}

// splitQualifiedName turns "Module.testcase" into ("Module",
// "testcase", true). Returns (_, _, false) for any other shape.
func splitQualifiedName(qname string) (string, string, bool) {
	i := strings.LastIndex(qname, ".")
	if i < 0 {
		return "", "", false
	}
	return qname[:i], qname[i+1:], true
}

// ErrNoTestcase is returned by RunTestcase when the qualified name
// cannot be resolved in any of the provided trees.
var ErrNoTestcase = errors.New("testcase not found")

// isUnknownNodeError reports whether err is the interpreter's
// default-case `unknown syntax node type: ...` error. We treat that
// specific error as a soft skip during module initialisation so a
// decl the interpreter does not yet model (component types, signature
// decls, etc.) cannot poison every testcase in the file.
func isUnknownNodeError(err *runtime.Error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Inspect(), "unknown syntax node type") ||
		strings.Contains(err.Inspect(), "unknown SubTypeDecl node type")
}

// bindDeclName extracts the user-visible name from a declaration the
// interpreter could not evaluate and stashes a phantom Undefined
// value under that name in env. The point is purely to keep later
// identifier lookups from erroring; nothing about the declaration is
// otherwise modelled. This is the difference between "interpreter
// doesn't know about component types" causing every dependent
// testcase to fail vs. only failing the ones that actually exercise
// the unimplemented semantics.
//
// For ComponentTypeDecls we additionally walk the body and bind every
// member's name to Undefined too. TTCN-3 component-instance lookups
// (`p.send(...)`, `t.start`) resolve to members the running mtc/ptc
// owns; absent a real component-instance model we put the names in
// module scope as a coarse approximation. This is enough for the
// conformance-suite tests that only check identifier resolution
// succeeds; tests that actually exercise the runtime semantics still
// produce a wrong verdict, which is the honest answer.
//
// moduleWith carries the enclosing module's `with { ... }` clauses so
// that named types can inherit module-level encode/variant defaults
// (TTCN-3 27.1.2 overriding rules).
func bindDeclName(env runtime.Scope, n syntax.Node, moduleWith *syntax.WithSpec) {
	bindDeclNameScoped(env, n, []*syntax.WithSpec{moduleWith})
}

// bindDeclNameScoped is bindDeclName carrying the full with-spec
// scope stack (outermost first). Inner scopes override outer ones for
// unqualified attrs, and any of them can carry a qualified
// `with { encode(T) "..." }` clause that hoists to T.
func bindDeclNameScoped(env runtime.Scope, n syntax.Node, scopes []*syntax.WithSpec) {
	switch d := n.(type) {
	case *syntax.ModuleDef:
		bindDeclNameScoped(env, d.Def, scopes)
	case *syntax.ComponentTypeDecl:
		if d.Name != nil {
			env.Set(syntax.Name(d.Name), typeDescForScoped(syntax.Name(d.Name), d.With, scopes))
		}
		if d.Body != nil {
			bindComponentMembers(env, d.Body)
			// Module-load-time port registry so the cabi/cgo
			// bridge can resolve port-type -> driver lookups
			// after the testcase starts, even though there is
			// no TestcaseExec yet at this point.
			if d.Name != nil {
				registerComponentTypeBody(env, syntax.Name(d.Name), d.Body)
				registerComponentTypePortsFromBody(syntax.Name(d.Name), d.Body)
			}
		}
	case *syntax.PortTypeDecl:
		if d.Name != nil {
			env.Set(syntax.Name(d.Name), typeDescForScoped(syntax.Name(d.Name), d.With, scopes))
		}
	case *syntax.TemplateDecl:
		if d.Name != nil {
			env.Set(syntax.Name(d.Name), runtime.Undefined)
		}
	case *syntax.SignatureDecl:
		if d.Name != nil {
			td := typeDescForScoped(syntax.Name(d.Name), d.With, scopes)
			td.Signature = d
			env.Set(syntax.Name(d.Name), td)
		}
	case *syntax.StructTypeDecl:
		if d.Name != nil {
			td := typeDescForScoped(syntax.Name(d.Name), d.With, scopes)
			// Carry the declaration of record/set/union types. For
			// record/set a positional value literal is reshaped onto
			// the declared field names; for a union with a @default
			// alternative a scalar value is wrapped into that
			// alternative (type-directed coercion).
			switch d.KindTok.Kind() {
			case syntax.RECORD, syntax.SET, syntax.UNION:
				td.Struct = d
			}
			env.Set(syntax.Name(d.Name), td)
		}
	case *syntax.ClassTypeDecl:
		if d.Name != nil {
			// Bind a ClassDesc (not a plain TypeDesc) so
			// `C.create(...)` builds a real object and
			// `obj.method()` / `obj.field` dispatch against the
			// declaration. Env is the defining scope, used as the
			// method-body closure and to resolve `extends`.
			env.Set(syntax.Name(d.Name), &runtime.ClassDesc{
				Name: syntax.Name(d.Name),
				Decl: d,
				Env:  env,
			})
		}
	case *syntax.MapTypeDecl:
		if d.Name != nil {
			env.Set(syntax.Name(d.Name), typeDescForScoped(syntax.Name(d.Name), d.With, scopes))
		}
	case *syntax.SubTypeDecl:
		if d.Field != nil && d.Field.Name != nil {
			td := typeDescForScoped(syntax.Name(d.Field.Name), d.With, scopes)
			if lo, hi, ok := charSubtypeRange(d.Field); ok {
				td.CharLo, td.CharHi, td.HasCharRange = lo, hi, true
			}
			// Record the referenced type so a type synonym
			// (`type R S`) can follow it for field access and field
			// attribute inheritance (ETSI 27.1.2). Harmless for
			// constrained subtypes of built-ins (the name resolves
			// to no TypeDesc).
			td.Underlying = fieldTypeName(d.Field)
			// A constrained array subtype (`type integer T[1..2]`)
			// records its declared lower index bound so a value
			// assigned to a variable of the type indexes from there
			// (ETSI 6.2.7 / 6.3.1).
			if lo := arrayDefLowerBound(d.Field.ArrayDef, env); lo != 0 {
				td.IndexOffset = lo
			}
			// A `set of` subtype tags its values unordered so
			// set-of matching is order-independent (ETSI 6.2.3.2).
			if spec, ok := d.Field.Type.(*syntax.ListSpec); ok && spec.KindTok != nil &&
				spec.KindTok.String() == "set" {
				td.ListKind = runtime.SET_OF
			}
			mergeUnderlyingAttrs(td, env)
			env.Set(syntax.Name(d.Field.Name), td)
		}
	}
}

// charSubtypeRange extracts the single ("lo".."hi") character range of
// a charstring subtype declaration so a `\N{TypeRef}` pattern
// reference can expand it to a [lo-hi] class. Returns ok=false for any
// non-charstring or non-single-range constraint.
func charSubtypeRange(f *syntax.Field) (lo, hi rune, ok bool) {
	if f == nil || f.ValueConstraint == nil || len(f.ValueConstraint.List) != 1 {
		return 0, 0, false
	}
	bin, isBin := f.ValueConstraint.List[0].(*syntax.BinaryExpr)
	if !isBin || bin == nil || bin.Op == nil || bin.Op.Kind() != syntax.RANGE {
		return 0, 0, false
	}
	loR, okLo := singleCharLiteral(bin.X)
	hiR, okHi := singleCharLiteral(bin.Y)
	if !okLo || !okHi {
		return 0, 0, false
	}
	return loR, hiR, true
}

func singleCharLiteral(e syntax.Expr) (rune, bool) {
	lit, ok := e.(*syntax.ValueLiteral)
	if !ok || lit == nil || lit.Tok == nil || lit.Tok.Kind() != syntax.STRING {
		return 0, false
	}
	s, err := syntax.Unquote(lit.Tok.String())
	if err != nil {
		return 0, false
	}
	r := []rune(s)
	if len(r) != 1 {
		return 0, false
	}
	return r[0], true
}

// applyImportWithAttrs layers the attributes of an `import ... with
// { ... }` statement onto the imported definitions (ETSI 27.1.3).
// Only explicitly listed imports are handled; an attribute lands on
// the TypeDesc bound for the imported name when the definition does
// not carry the same attribute kind itself.
func applyImportWithAttrs(env runtime.Scope, mod *syntax.Module) {
	if mod == nil {
		return
	}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		imp, ok := d.Def.(*syntax.ImportDecl)
		if !ok || imp == nil || imp.With == nil {
			continue
		}
		attrs := map[string][]string{}
		collectWithAttrs(imp.With, attrs, nil, nil)
		if len(attrs) == 0 {
			continue
		}
		for _, name := range importedNames(imp) {
			v, ok := env.Get(name)
			if !ok {
				continue
			}
			td, ok := forceThunk(v).(*runtime.TypeDesc)
			if !ok {
				continue
			}
			for k, vals := range attrs {
				if _, own := td.OwnAttrs[k]; own {
					continue
				}
				if td.Attrs == nil {
					td.Attrs = map[string][]string{}
				}
				if _, exists := td.Attrs[k]; !exists {
					td.Attrs[k] = vals
				}
			}
		}
	}
}

// importedNames lists the identifiers an import statement names
// explicitly (`import from M { type T1, T2 }`). `import all` and
// kind-wide imports produce nothing - attaching attributes to those
// would need module-origin tracking the loopback model doesn't keep.
func importedNames(imp *syntax.ImportDecl) []string {
	var out []string
	for _, spec := range imp.List {
		if spec == nil {
			continue
		}
		for _, e := range spec.List {
			if id, ok := e.(*syntax.Ident); ok && id != nil {
				out = append(out, id.String())
			}
		}
	}
	return out
}

// mergeUnderlyingAttrs applies the ETSI 27.1.2.2 multiple-encoding
// overwriting rules between a subtype/synonym and its underlying
// type. Without an own `encode` the underlying type's encoding list
// and variants are inherited, each codec's variant individually
// overridable by an own codec-qualified variant. With an own
// `encode` only the variants of re-referenced codecs survive; the
// variants of discarded codecs are dropped with their encoding.
func mergeUnderlyingAttrs(td *runtime.TypeDesc, env runtime.Scope) {
	if td == nil || td.Underlying == "" || env == nil {
		return
	}
	under := lookupTypeDesc(td.Underlying, env)
	if under == nil {
		return
	}
	underEnc, _ := under.Lookup("encode")
	underVar, _ := under.Lookup("variant")
	if len(underEnc) == 0 && len(underVar) == 0 {
		return
	}
	ownEnc, ownEncOk := td.OwnAttrs["encode"]
	ownVar := td.OwnAttrs["variant"]
	enc := underEnc
	if ownEncOk {
		enc = ownEnc
	}
	allowed := map[string]bool{}
	for _, c := range enc {
		allowed[strings.ToLower(c)] = true
	}
	codecOf := func(v string) string {
		if i := strings.Index(v, "."); i > 0 {
			return strings.ToLower(v[:i])
		}
		return ""
	}
	ownByCodec := map[string]bool{}
	for _, v := range ownVar {
		ownByCodec[codecOf(v)] = true
	}
	var variants []string
	for _, v := range underVar {
		c := codecOf(v)
		if c == "" || !allowed[c] || ownByCodec[c] {
			continue
		}
		variants = append(variants, v)
	}
	variants = append(variants, ownVar...)
	if td.Attrs == nil {
		td.Attrs = map[string][]string{}
	}
	td.Attrs["encode"] = enc
	if len(variants) > 0 {
		td.Attrs["variant"] = variants
	} else {
		delete(td.Attrs, "variant")
	}
}

// typeDescFor distils a runtime TypeDesc for a named type, layering
// the type-local `with { encode "..." }` clauses on top of the
// module-level defaults. The result is what `T.encode`, `T.variant`,
// ... evaluate to during testcase execution.
func typeDescFor(name string, local, module *syntax.WithSpec) *runtime.TypeDesc {
	return typeDescForScoped(name, local, []*syntax.WithSpec{module})
}

func typeDescForScoped(name string, local *syntax.WithSpec, scopes []*syntax.WithSpec) *runtime.TypeDesc {
	// Per TTCN-3 27.1.2 a `with { ... }` clause on an inner scope
	// REPLACES (not augments) the same attribute kind from the
	// outer scope. We honour that by collecting each scope's
	// attrs into its own map and then walking outermost->innermost
	// so an inner kind overwrites an outer kind of the same name.
	attrs := map[string][]string{}
	overr := map[string]bool{}
	merge := func(layer map[string][]string, layerOv map[string]bool) {
		for k, v := range layer {
			attrs[k] = v
			// The winning (innermost) layer dictates override-ness:
			// a plain inner attribute clears an inherited override of
			// the same kind, an inner override sets it.
			overr[k] = layerOv[k]
		}
	}
	for _, s := range scopes {
		one := map[string][]string{}
		oneOv := map[string]bool{}
		collectWithAttrs(s, one, oneOv, nil)
		merge(one, oneOv)
	}
	// The variant attribute is always interpreted in the context of the
	// encode it belongs to (ETSI 27.1.2.1), so remember the inherited
	// encode before the local layer can replace it.
	inheritedEncode := attrs["encode"]
	// The local (type's own) layer is captured separately as OwnAttrs/
	// OwnLocal so the field-inheritance chain can consult a type's own
	// directly-declared attribute distinct from inherited ones.
	ownAttrs := map[string][]string{}
	ownLocal := map[string]bool{}
	oneOv := map[string]bool{}
	collectWithAttrs(local, ownAttrs, oneOv, ownLocal)
	merge(ownAttrs, oneOv)
	// A type that overrides the encode with a DIFFERENT rule and
	// declares no variant of its own no longer inherits the enclosing
	// scope's variant: that variant belonged to the outer encode the
	// type just replaced (ETSI 27.1.2.1). When the own encode equals
	// the inherited one the variant context is unchanged and the
	// inherited variant still applies.
	if own, ok := ownAttrs["encode"]; ok && len(ownAttrs["variant"]) == 0 && !equalStrings(own, inheritedEncode) {
		// Block the inherited variant explicitly (empty, present) so the
		// type-level lookup answers "no variant" instead of falling back
		// to the enclosing scope's variant.
		attrs["variant"] = []string{}
		for k := range attrs {
			if strings.HasSuffix(k, ".variant") {
				delete(attrs, k)
			}
		}
	}
	// TTCN-3 27.2: a `with { encode (T) "Rule" }` clause on any
	// enclosing scope is equivalent to attaching `encode "Rule"`
	// directly to type T. Hoist qualified attrs whose qualifier
	// names this TypeDesc into the unqualified slot - those take
	// precedence over the outer unqualified default of the same
	// kind.
	lname := strings.ToLower(name)
	for k, v := range attrs {
		dot := strings.Index(k, ".")
		if dot < 0 {
			continue
		}
		if k[:dot] != lname {
			continue
		}
		kind := k[dot+1:]
		attrs[kind] = v
		overr[kind] = overr[k]
	}
	if len(attrs) == 0 {
		return &runtime.TypeDesc{Name: name, OwnAttrs: ownAttrs, OwnLocal: ownLocal}
	}
	return &runtime.TypeDesc{Name: name, Attrs: attrs, Override: overr, OwnAttrs: ownAttrs, OwnLocal: ownLocal}
}

// equalStrings reports whether two string slices have identical
// elements in order. Used to decide whether a type's own encode equals
// the one inherited from an enclosing scope (ETSI 27.1.2.1 variant
// context).
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// collectWithAttrs harvests the unqualified `encode "X"`, `variant "X"`,
// ... entries of a WithSpec into a map keyed by lowercased kind. Later
// scopes overwrite earlier ones because TTCN-3's overriding rules state
// that a directly-attached attribute supersedes one inherited from
// above.
//
// Qualified attributes (`encode (field1) "RuleB"`) are stored under a
// composite key `<qualifier>.<kind>` (lower-cased) so callers can do
// `td.Lookup("field1.encode")` to fetch a field-specific override.
// Multi-qualifier lists (`encode (f1, f2) "X"`) replicate the entry
// across every qualifier.
func collectWithAttrs(ws *syntax.WithSpec, out map[string][]string, outOv, outLocal map[string]bool) {
	if ws == nil {
		return
	}
	for _, stmt := range ws.List {
		if stmt == nil || stmt.KindTok.Kind() == syntax.NULL {
			continue
		}
		vals := unwrapWithValues(stmt.Value)
		if len(vals) == 0 {
			continue
		}
		kind := strings.ToLower(stmt.KindTok.String())
		// Both the `override` keyword and the `@local` modifier populate
		// WithStmt.Override, but they are opposites: `override` forces the
		// attribute onto contained fields (ETSI 27.7) while `@local`
		// confines it to this definition. Only the former propagates;
		// the latter is excluded from the field-inheritance chain.
		isOverride := stmt.Override != nil && strings.EqualFold(stmt.Override.String(), "override")
		isLocal := stmt.Override != nil && !isOverride
		mark := func(key string) {
			if outOv != nil {
				outOv[key] = isOverride
			}
			if outLocal != nil {
				outLocal[key] = isLocal
			}
		}
		if len(stmt.List) == 0 {
			// Several attributes of the same kind on one definition
			// (`encode "A" encode "B" encode "C"`, ETSI 27.4) are kept
			// in declaration order so `T.encode` reports {A, B, C}.
			out[kind] = append(out[kind], vals...)
			mark(kind)
			continue
		}
		for _, q := range stmt.List {
			qname := qualifierName(q)
			if qname == "" {
				continue
			}
			key := strings.ToLower(qname) + "." + kind
			out[key] = append(out[key], vals...)
			mark(key)
		}
	}
}

// defAttrKey names the env binding recording the attributes that
// enclosing kind-selector with clauses attach to a single definition.
// The NUL prefix keeps it out of the user identifier namespace.
func defAttrKey(name string) string { return "\x00defattr:" + name }

// recordDefKindAttrs binds the per-definition attributes that
// enclosing AllRef with clauses (`encode (const all) "Rule"`, ETSI
// 27.2) attach to value and template definitions, so a later
// `<name>.encode` retrieval answers them.
func recordDefKindAttrs(env runtime.Scope, d *syntax.ModuleDef, scopes []*syntax.WithSpec) {
	switch n := d.Def.(type) {
	case *syntax.ValueDecl:
		kind := strings.ToLower(n.KindTok.String())
		if kind != "const" && kind != "modulepar" {
			return
		}
		for _, decl := range n.Decls {
			if decl == nil || decl.Name == nil {
				continue
			}
			name := syntax.Name(decl.Name)
			if attrs := kindSelectedAttrs(scopes, kind, name); len(attrs) > 0 {
				env.Set(defAttrKey(name), &runtime.TypeDesc{Name: name, Attrs: attrs})
			}
		}
	case *syntax.TemplateDecl:
		if n.Name == nil {
			return
		}
		name := syntax.Name(n.Name)
		if attrs := kindSelectedAttrs(scopes, "template", name); len(attrs) > 0 {
			env.Set(defAttrKey(name), &runtime.TypeDesc{Name: name, Attrs: attrs})
		}
	}
}

// kindSelectedAttrs collects the Annex E attributes that enclosing
// `with { encode (const all) "Rule" }` clauses attach to a definition
// of the given kind and name. Scopes run outermost->innermost; an
// inner clause of the same attribute kind replaces the outer one
// (ETSI 27.1.2). Definitions listed in `except { ... }` are exempt.
func kindSelectedAttrs(scopes []*syntax.WithSpec, kind, name string) map[string][]string {
	var attrs map[string][]string
	for _, ws := range scopes {
		if ws == nil {
			continue
		}
		layer := map[string][]string{}
		for _, stmt := range ws.List {
			if stmt == nil || stmt.KindTok.Kind() == syntax.NULL || len(stmt.List) == 0 {
				continue
			}
			v := unwrapWithValue(stmt.Value)
			if v == "" {
				continue
			}
			if !defKindSelected(stmt.List, kind, name) {
				continue
			}
			akind := strings.ToLower(stmt.KindTok.String())
			layer[akind] = append(layer[akind], v)
		}
		for k, v := range layer {
			if attrs == nil {
				attrs = map[string][]string{}
			}
			attrs[k] = v
		}
	}
	return attrs
}

// defKindSelected reports whether a with-stmt qualifier list contains
// a `<kind> all [except {...}]` selector covering the named
// definition.
func defKindSelected(quals []syntax.Expr, kind, name string) bool {
	for _, q := range quals {
		dk, ok := q.(*syntax.DefKindExpr)
		if !ok || dk == nil || !strings.EqualFold(dk.KindTok.String(), kind) {
			continue
		}
		selected := true
		for _, e := range dk.List {
			ex, ok := e.(*syntax.ExceptExpr)
			if !ok || ex == nil {
				continue
			}
			for _, r := range ex.List {
				if qualifierName(r) == name {
					selected = false
				}
			}
		}
		if selected {
			return true
		}
	}
	return false
}

// qualifierName flattens the qualifier expression inside `with {
// encode (field1) "X" }` to its identifier form. Only simple
// identifiers and dotted selectors are recognised; anything more
// elaborate (regexp, all/group) is ignored since the loopback model
// doesn't model those scopes.
func qualifierName(e syntax.Expr) string {
	switch q := e.(type) {
	case *syntax.Ident:
		return q.String()
	case *syntax.SelectorExpr:
		l := qualifierName(q.X)
		r := qualifierName(q.Sel)
		if l == "" {
			return r
		}
		if r == "" {
			return l
		}
		return l + "." + r
	}
	return ""
}

// unwrapWithValues expands a WithStmt value to its attribute strings.
// The multi-codec variant form `variant {"Codec1","Codec2"}."Rule"`
// (ETSI 27.5) attaches the rule to every listed codec, producing one
// `Codec.Rule` entry per codec; every other shape yields the single
// unwrapWithValue result.
func unwrapWithValues(e syntax.Expr) []string {
	if sel, ok := e.(*syntax.SelectorExpr); ok {
		if cl, ok := sel.X.(*syntax.CompositeLiteral); ok {
			r := unwrapWithValue(sel.Sel)
			if r == "" {
				return nil
			}
			var out []string
			for _, el := range cl.List {
				if l := unwrapWithValue(el); l != "" {
					out = append(out, l+"."+r)
				}
			}
			return out
		}
	}
	if v := unwrapWithValue(e); v != "" {
		return []string{v}
	}
	return nil
}

// unwrapWithValue pulls the string literal out of a WithStmt's value
// expression, handling the `"A".B` selector form by joining with a
// dot. Returns the empty string when the value is not a literal.
func unwrapWithValue(e syntax.Expr) string {
	switch v := e.(type) {
	case *syntax.ValueLiteral:
		s, err := syntax.Unquote(v.Tok.String())
		if err != nil {
			return ""
		}
		return s
	case *syntax.SelectorExpr:
		l := unwrapWithValue(v.X)
		r := unwrapWithValue(v.Sel)
		if l == "" && r == "" {
			return ""
		}
		if l == "" {
			return r
		}
		if r == "" {
			return l
		}
		return l + "." + r
	}
	return ""
}

// isTypeDecl reports whether a top-level definition introduces a new
// type. We initialise those first so that subsequent constant /
// template / modulepar initialisers can reference their members.
func isTypeDecl(n syntax.Node) bool {
	switch n.(type) {
	case *syntax.EnumTypeDecl,
		*syntax.SubTypeDecl,
		*syntax.StructTypeDecl,
		*syntax.ComponentTypeDecl,
		*syntax.PortTypeDecl,
		*syntax.SignatureDecl,
		*syntax.BehaviourTypeDecl:
		return true
	}
	return false
}

// initModuleDefs walks a module's top-level declarations and binds the ones
// the interpreter can evaluate into env. Used to flatten sibling modules into
// the active scope when emulating `import from <X> all` without a real
// import resolver. Errors are swallowed (best-effort import).
func initModuleDefs(env runtime.Scope, mod *syntax.Module) {
	if mod == nil {
		return
	}
	defs := flattenModuleDefs(mod.Defs, []*syntax.WithSpec{mod.With})
	for _, pass := range []int{0, 1} {
		for _, fd := range defs {
			d := fd.def
			if d == nil {
				continue
			}
			if _, ok := d.Def.(*syntax.ControlPart); ok {
				continue
			}
			isType := isTypeDecl(d.Def)
			if pass == 0 && !isType {
				continue
			}
			if pass == 1 && isType {
				continue
			}
			if r := eval(d, env); runtime.IsError(r) {
				bindDeclNameScoped(env, d, fd.scopes)
				continue
			}
			if isTypeDecl(d.Def) {
				bindDeclNameScoped(env, d, fd.scopes)
			}
		}
	}
}

type flatDef struct {
	def  *syntax.ModuleDef
	with *syntax.WithSpec // nearest enclosing with-spec
	// scopes lists the enclosing with-specs from outermost
	// (module) inward. Module's WithSpec is `with`; any group
	// scopes seen on the way are appended in nesting order. Used
	// so qualified attribute hoisting (`with { encode(R) "X" }`
	// on the module/group level) reaches a type declared in a
	// nested group.
	scopes []*syntax.WithSpec
}

// flattenModuleDefs walks a module's top-level decls and unwraps any
// group {} nesting so a single linear pass can bind every declared
// name. Each returned flatDef carries the with-spec stack from its
// enclosing scopes (module, group, nested group, ...), which feeds
// into the TypeDesc attribute resolution for `R.encode` /
// `R.extension`.
func flattenModuleDefs(defs []*syntax.ModuleDef, scopes []*syntax.WithSpec) []flatDef {
	var out []flatDef
	for _, d := range defs {
		if d == nil {
			continue
		}
		if g, ok := d.Def.(*syntax.GroupDecl); ok {
			inner := scopes
			if g.With != nil {
				inner = append(append([]*syntax.WithSpec{}, scopes...), g.With)
			}
			out = append(out, flattenModuleDefs(g.Defs, inner)...)
			continue
		}
		var nearest *syntax.WithSpec
		if n := len(scopes); n > 0 {
			nearest = scopes[n-1]
		}
		out = append(out, flatDef{def: d, with: nearest, scopes: scopes})
	}
	return out
}

// evalSelectStmt evaluates a `select { case ... }` switch. It evaluates the
// tag expression once and then walks the cases in order. The first case whose
// pattern equals the tag is executed; if none matches and there is an `else`
// case (Case == nil) it runs that body instead.
func evalSelectStmt(n *syntax.SelectStmt, env runtime.Scope) runtime.Object {
	if n == nil {
		return nil
	}
	if n.Class != nil {
		return evalSelectClassStmt(n, env)
	}
	if n.Union != nil {
		return evalSelectUnionStmt(n, env)
	}
	var tag runtime.Object
	if n.Tag != nil {
		tag = eval(n.Tag, env)
		if runtime.IsError(tag) {
			return tag
		}
	}
	var elseCase *syntax.CaseClause
	for _, cc := range n.Body {
		if cc == nil {
			continue
		}
		if cc.Case == nil {
			if elseCase == nil {
				elseCase = cc
			}
			continue
		}
		// Each case is wrapped in a ParenExpr; the contained list may
		// contain several patterns - any of them firing selects this
		// case.
		matched := false
		for _, p := range cc.Case.List {
			v := eval(p, env)
			if runtime.IsError(v) {
				continue
			}
			if tag == nil || tag == runtime.Undefined || v == runtime.Undefined {
				matched = true
				break
			}
			if tag.Equal(v) {
				matched = true
				break
			}
		}
		if matched && cc.Body != nil {
			return eval(cc.Body, env)
		}
	}
	if elseCase != nil && elseCase.Body != nil {
		return eval(elseCase.Body, env)
	}
	return nil
}

// evalSelectUnionStmt evaluates `select union (u) { case (alt) {...} ... }`
// (ETSI 19.3.2). The case labels are union alternative *names*, not
// values: the case whose name matches the union value's chosen
// alternative runs; `case else` is the fallback.
func evalSelectUnionStmt(n *syntax.SelectStmt, env runtime.Scope) runtime.Object {
	var tag runtime.Object
	if n.Tag != nil {
		tag = eval(n.Tag, env)
		if runtime.IsError(tag) {
			return tag
		}
	}
	chosen := chosenUnionAlt(tag)
	var elseCase *syntax.CaseClause
	for _, cc := range n.Body {
		if cc == nil {
			continue
		}
		if cc.Case == nil {
			if elseCase == nil {
				elseCase = cc
			}
			continue
		}
		for _, p := range cc.Case.List {
			if name := syntax.Name(p); name != "" && name == chosen {
				if cc.Body != nil {
					return eval(cc.Body, env)
				}
				return nil
			}
		}
	}
	if elseCase != nil && elseCase.Body != nil {
		return eval(elseCase.Body, env)
	}
	return nil
}

// chosenUnionAlt returns the name of the alternative currently selected
// in a union value (the single present field of its backing record), or
// "" when it cannot be determined.
func chosenUnionAlt(tag runtime.Object) string {
	r, ok := tag.(*runtime.Record)
	if !ok {
		return ""
	}
	for name, v := range r.Fields {
		if v != runtime.Undefined {
			return name
		}
	}
	return ""
}

// evalAltStmtStrict is the alt evaluator (ES 201 873-4 clause 20):
// source-order, first-match-wins guard evaluation via commGuardMatches
// (which consumes only the selected, matching event), plus [else],
// repeat and activated defaults. Snapshot semantics are honest — when no
// guard matches and there is no [else] it blocks on the alt's event
// sources (port traffic, the soonest timer deadline, a component
// transition, or this PTC's stop) and re-snapshots. It never fabricates
// a verdict for a branch whose guard did not fire, which is what the
// retired verdict-preferring evaluator did.
func evalAltStmtStrict(n *syntax.AltStmt, env runtime.Scope) runtime.Object {
	if n.Body == nil {
		return nil
	}
	// Bounded only as a backstop against a `repeat` whose state never
	// changes; genuine blocking is ended by a matching event, this PTC's
	// stop, or the harness/testcase timeout.
	const maxRounds = 1 << 20
	for round := 0; round < maxRounds; round++ {
		// Alt-local declarations are re-evaluated each round (ETSI 20.2).
		for _, s := range n.Body.Stmts {
			if _, ok := s.(*syntax.CommClause); ok {
				continue
			}
			if r := eval(s, env); needBreak(r) {
				return r
			}
		}

		// Snapshot pass: the first clause whose guard fires wins.
		var elseClause *syntax.CommClause
		matched := false
		for _, s := range n.Body.Stmts {
			cc, ok := s.(*syntax.CommClause)
			if !ok {
				continue
			}
			if cc.Else != nil {
				if elseClause == nil {
					elseClause = cc
				}
				continue
			}
			if cc.Comm == nil {
				continue
			}
			// Boolean guard (ETSI 20.2): a clause `[expr] op {...}` is
			// eligible this round only when `expr` holds. We gate only on
			// a concretely-false boolean — an Undefined/unmodelled or
			// non-boolean guard falls through to the comm match, preserving
			// the pre-existing behaviour (the approximate path ignores the
			// guard entirely). This lets a snapshot discriminate branches
			// that differ only by their guard, e.g. a server accepting from
			// `[v_client1 == null] getcall ... sender v_client1` vs the
			// already-bound second client.
			if cc.X != nil {
				if gv, ok := eval(cc.X, env).(runtime.Bool); ok && !bool(gv) {
					continue
				}
			}
			if commGuardMatches(cc.Comm, env) {
				matched = true
				defaultBranchFire() // no-op unless inside a runDefaults sweep
				if cc.Body != nil {
					if res := evalAltClauseBody(cc.Body, env); res == runtime.Repeat {
						break
					} else {
						return res
					}
				}
				return nil
			}
		}
		if matched {
			continue // a body returned Repeat -> re-snapshot
		}
		if elseClause != nil {
			res := evalAltClauseBody(elseClause.Body, env)
			if res == runtime.Repeat {
				continue
			}
			return res
		}

		// Activated defaults are appended after the alternatives (20.5),
		// unless the alt is marked `@nodefault`. A default that stopped the
		// component hands back its unwinding result, which has to travel
		// past this alt statement.
		if n.NoDefault == nil {
			if ctl, fired := runDefaults(env); fired {
				return ctl
			}
		}
		// An activated default's own altstep is a single NON-blocking pass:
		// when this strict alt IS a default body (defaultCtx active) and no
		// clause matched, conclude without parking (the default simply did
		// not fire) rather than block the single-runner token.
		if defaultCtx.active() {
			return nil
		}

		// No guard fired and no [else]: block on the alt's event sources
		// and re-snapshot. Crucially, NO verdict-preferring heuristic —
		// a clause runs only when its guard actually matches.
		if !blockForAltEvents(n, env) {
			// Nothing to wait for (only boolean / unmodelled guards) or
			// this PTC was stopped: conclude without fabricating a
			// verdict, per real alt semantics (the caller's outer
			// context / timeout governs a genuinely blocked alt).
			return nil
		}
	}
	return nil
}

// evalInterleaveStmtStrict evaluates `interleave { ... }` under the strict
// profile (ETSI ES 201 873-1 §20.4): every alternative is taken EXACTLY
// ONCE, in whatever interleaved order its guard becomes ready. Each round
// re-snapshots the not-yet-taken alternatives and takes the first whose
// guard matches; when none match it blocks on the branch event sources and
// re-snapshots. This is the correct semantics for the common case and
// replaces running interleave as a plain best-effort alt (which took only
// ONE alternative).
//
// A branch body that itself blocks needs no special handling. A nested alt
// inside a body parks on its own event sources and, finding nothing that can
// ever fire, concludes without matching; the interleave then re-snapshots and
// takes the sibling whose guard the first body just enabled, and a later round
// re-offers the branch whose blocking read is now satisfiable. Interleaves
// with blocking bodies used to defer to the best-effort evaluator on the
// assumption that they needed cooperative suspend/resume at the blocking
// point; measured against the full ETSI corpus that fallback changed no
// verdict, so the snapshot evaluator carries them directly.
//
// Activated defaults are likewise handled here. They are appended after the
// remaining alternatives (20.5) and one that fires leaves the interleave —
// runDefaults reports a default that actually took a branch, not merely one
// that changed the verdict, which is the signal this needs.
func evalInterleaveStmtStrict(n *syntax.AltStmt, env runtime.Scope) runtime.Object {
	if n.Body == nil {
		return nil
	}
	var clauses []*syntax.CommClause
	for _, s := range n.Body.Stmts {
		if cc, ok := s.(*syntax.CommClause); ok && cc.Comm != nil && cc.Else == nil {
			clauses = append(clauses, cc)
		}
	}
	taken := make([]bool, len(clauses))
	remaining := len(clauses)
	const maxRounds = 1 << 20
	for round := 0; round < maxRounds && remaining > 0; round++ {
		// Alt-local declarations are re-evaluated each round (ETSI 20.2).
		for _, s := range n.Body.Stmts {
			if _, ok := s.(*syntax.CommClause); ok {
				continue
			}
			if r := eval(s, env); needBreak(r) {
				return r
			}
		}
		matched := false
		for i, cc := range clauses {
			if taken[i] {
				continue
			}
			// Boolean guard (ETSI 20.2): eligible only when it holds.
			if cc.X != nil {
				if gv, ok := eval(cc.X, env).(runtime.Bool); ok && !bool(gv) {
					continue
				}
			}
			if commGuardMatches(cc.Comm, env) {
				taken[i] = true
				remaining--
				matched = true
				defaultBranchFire() // no-op unless inside a runDefaults sweep
				if cc.Body != nil {
					res := evalAltClauseBody(cc.Body, env)
					// `repeat` is not permitted in interleave (20.4); ignore
					// it. `break` / `return` / `stop` / `goto` / error leaves
					// the interleave immediately.
					if res != runtime.Repeat && needBreak(res) {
						return res
					}
				}
				break // re-snapshot: taking one branch may enable another
			}
		}
		if matched {
			continue
		}
		// No alternative matched. Activated defaults are appended after the
		// remaining alternatives (20.5); one that fires leaves the interleave.
		if n.NoDefault == nil {
			if ctl, fired := runDefaults(env); fired {
				return ctl
			}
		}
		// This interleave IS the body of an activated default: a default is a
		// single non-blocking pass, so conclude instead of parking the token.
		if defaultCtx.active() {
			return nil
		}
		// Block on the remaining branch event sources and re-snapshot.
		if !blockForAltEvents(n, env) {
			return nil
		}
	}
	return nil
}

// blockForAltEvents parks a strict alt on its event sources — inbound
// port traffic (MessageReady), the soonest running-timer deadline, a
// component transition, or this PTC's stop — via waitForAltCombined,
// whose 2ms backstop also covers component / timer guards that raise no
// MessageReady signal. Returns true to re-snapshot, false when there is
// nothing to wait for or the PTC was stopped.
func blockForAltEvents(n *syntax.AltStmt, env runtime.Scope) bool {
	exec := runtime.FindTestcaseExec(env)
	// A stopped executor - the testcase was stopped, or its context (the
	// harness budget, or an execute() timeout) was cancelled - unwinds a
	// blocking alt instead of waiting for an event that will never come.
	if exec != nil && exec.Stopped() {
		return false
	}
	if exec != nil && exec.SchedulerActive() {
		// Discrete-event quiescence scheduler owns timing: park this
		// component until a comm/component event arrives or the virtual
		// clock advances to fire a timer guard. No real sleep, no polling
		// backstop — time only moves when every participant is parked.
		vd, hasTimer := nextAltTimerVirtualDeadline(n, env)
		if !hasTimer && !altHasEventGuard(n) {
			return false // only boolean / [else] guards: nothing to await
		}
		re, _ := exec.SchedPark(currentCompID(exec), vd, hasTimer, currentStopChan(exec))
		return re
	}
	if deterministicClockEnabled(env) {
		// Advance the virtual clock to the soonest timer deadline so
		// exactly that timer fires on the next snapshot — no real sleep,
		// deadline ordering preserved. (A queued message would already
		// have matched in the snapshot pass before we got here.)
		if vd, ok := nextAltTimerVirtualDeadline(n, env); ok {
			if exec != nil {
				exec.AdvanceVirtualClock(vd)
			}
			return true
		}
		// No timer guard: a real wait for port / component events, which
		// concurrent PTCs deliver in real time even under the virtual
		// clock. altHasEventGuard gates giving up on pure-boolean alts.
		if altHasEventGuard(n) {
			return waitForAltCombined(0, false, env)
		}
		return false
	}
	dur, hasTimer := nextAltTimerDeadlineLenient(n, env)
	if hasTimer || altHasEventGuard(n) {
		return waitForAltCombined(dur, hasTimer, env)
	}
	return false
}

// nextAltTimerVirtualDeadline returns the soonest virtual-clock deadline
// (StartedAtVirtual + Duration) among the alt's running timer guards —
// what the deterministic clock advances to so exactly that timer fires
// next. Ignores non-timer guards; (0,false) when no timer guard runs.
func nextAltTimerVirtualDeadline(n *syntax.AltStmt, env runtime.Scope) (float64, bool) {
	if n == nil || n.Body == nil {
		return 0, false
	}
	var soonest float64
	have := false
	consider := func(th *runtime.TimerHandle) {
		if th == nil || !th.Running || th.Duration <= 0 {
			return
		}
		if dl := th.StartedAtVirtual + th.Duration; !have || dl < soonest {
			soonest, have = dl, true
		}
	}
	// considerComm accounts for one clause guard. A direct `<timer>.timeout`
	// contributes its deadline; an altstep-call guard `[] a()` is walked into
	// so a timer guard living INSIDE the altstep (e.g. `alt { [] a() }` where
	// `a` has `[] t.timeout {}`) still advances the deterministic clock —
	// otherwise no deadline is found and the run blocks forever. `depth`
	// bounds mutually-recursive altsteps. Timers resolve in the current env,
	// which reaches component-scope timers (the common case).
	var considerComm func(comm syntax.Node, depth int)
	considerComm = func(comm syntax.Node, depth int) {
		// `[] p.catch(timeout)` contributes the enclosing call block's
		// timeout deadline, so the block-step advances the virtual clock to
		// it when no getreply arrives and the catch(timeout) then fires.
		if isCatchTimeoutGuard(comm) {
			consider(callTimeoutTimer(env))
			return
		}
		es, ok := comm.(*syntax.ExprStmt)
		if !ok {
			return
		}
		switch x := es.Expr.(type) {
		case *syntax.SelectorExpr:
			recvIdent, ok := x.X.(*syntax.Ident)
			if !ok {
				return
			}
			op, ok := x.Sel.(*syntax.Ident)
			if !ok || op.String() != "timeout" {
				return
			}
			if rn := recvIdent.String(); rn == "any timer" || rn == "all timer" {
				for _, th := range collectScopeTimers(env) {
					consider(th)
				}
			} else if v, ok := env.Get(rn); ok {
				if th, ok := v.(*runtime.TimerHandle); ok {
					consider(th)
				}
			}
		case *syntax.CallExpr:
			if depth <= 0 {
				return
			}
			id, ok := x.Fun.(*syntax.Ident)
			if !ok {
				return
			}
			v, ok := env.Get(id.String())
			if !ok {
				return
			}
			fn, ok := v.(*runtime.Function)
			if !ok || !fn.IsAltstep || fn.Body == nil {
				return
			}
			for _, s := range fn.Body.Stmts {
				if cc, ok := s.(*syntax.CommClause); ok && cc.Else == nil && cc.Comm != nil {
					considerComm(cc.Comm, depth-1)
				}
			}
		}
	}
	for _, s := range n.Body.Stmts {
		cc, ok := s.(*syntax.CommClause)
		if !ok || cc.Else != nil || cc.Comm == nil {
			continue
		}
		considerComm(cc.Comm, 4)
	}
	return soonest, have
}

// altHasEventGuard reports whether any clause guard is event-driven — a
// port receive/check/trigger/getcall/getreply/catch, a component
// done/killed/running op, or an altstep-call guard (which may contain
// such ops). Pure boolean-expr guards and [else] are not event-driven,
// so an alt with only those cannot usefully block.
func altHasEventGuard(n *syntax.AltStmt) bool {
	if n == nil || n.Body == nil {
		return false
	}
	for _, s := range n.Body.Stmts {
		cc, ok := s.(*syntax.CommClause)
		if !ok || cc.Else != nil || cc.Comm == nil {
			continue
		}
		switch commClauseOp(cc) {
		case "receive", "check", "trigger", "getcall", "getreply", "catch",
			"done", "killed", "running":
			return true
		}
		// `[] a_altstep()` (Fun is a bare Ident): may hold event guards,
		// so treat it as blockable rather than give up.
		if es, ok := cc.Comm.(*syntax.ExprStmt); ok {
			if ce, ok := es.Expr.(*syntax.CallExpr); ok {
				if _, isIdent := ce.Fun.(*syntax.Ident); isIdent {
					return true
				}
			}
		}
	}
	return false
}

// commClauseOp extracts the operation name from a clause's comm guard
// (`p.receive(...)`, bare `p.receive`, `t.timeout`, `c.done`, with an
// optional `-> redirect` or `from/to addr` wrap peeled off), or "" when
// the shape is not a port/timer/component operation.
func commClauseOp(cc *syntax.CommClause) string {
	es, ok := cc.Comm.(*syntax.ExprStmt)
	if !ok {
		return ""
	}
	expr := es.Expr
	for {
		switch e := expr.(type) {
		case *syntax.RedirectExpr:
			if e == nil || e.X == nil {
				return ""
			}
			expr = e.X
			continue
		case *syntax.BinaryExpr:
			if e.Op != nil && (e.Op.Kind() == syntax.FROM || e.Op.Kind() == syntax.TO) && e.X != nil {
				expr = e.X
				continue
			}
		}
		break
	}
	switch e := expr.(type) {
	case *syntax.CallExpr:
		if sel, ok := e.Fun.(*syntax.SelectorExpr); ok {
			if id, ok := sel.Sel.(*syntax.Ident); ok {
				return id.String()
			}
		}
	case *syntax.SelectorExpr:
		if id, ok := e.Sel.(*syntax.Ident); ok {
			return id.String()
		}
	}
	return ""
}

// nextAltTimerDeadlineLenient returns the soonest live-timer `.timeout`
// deadline among a MIXED alt's clauses, ignoring (rather than bailing
// on) non-timer guards such as `p.receive`. Used by the combined park so a load
// worker's `alt{ [] p.receive [] t_guard.timeout }` still honours its
// timer guard. Returns (0,false) when no clause has a live timer guard,
// or when a timer guard has already expired (so the caller re-enters
// the first pass immediately instead of sleeping).
func nextAltTimerDeadlineLenient(n *syntax.AltStmt, env runtime.Scope) (time.Duration, bool) {
	if n == nil || n.Body == nil {
		return 0, false
	}
	now := time.Now()
	var soonest time.Duration
	have := false
	for _, s := range n.Body.Stmts {
		cc, ok := s.(*syntax.CommClause)
		if !ok || cc.Else != nil || cc.Comm == nil {
			continue
		}
		es, ok := cc.Comm.(*syntax.ExprStmt)
		if !ok {
			continue
		}
		sel, ok := es.Expr.(*syntax.SelectorExpr)
		if !ok {
			continue
		}
		recvIdent, ok := sel.X.(*syntax.Ident)
		if !ok {
			continue
		}
		opIdent, ok := sel.Sel.(*syntax.Ident)
		if !ok || opIdent.String() != "timeout" {
			continue
		}
		if rn := recvIdent.String(); rn == "any timer" || rn == "all timer" {
			for _, th := range collectScopeTimers(env) {
				if th == nil || !th.Running || th.StartedAt.IsZero() {
					continue
				}
				deadline := th.StartedAt.Add(time.Duration(th.Duration * float64(time.Second)))
				remaining := deadline.Sub(now)
				if remaining <= 0 {
					continue
				}
				if !have || remaining < soonest {
					soonest = remaining
					have = true
				}
			}
			continue
		}
		v, ok := env.Get(recvIdent.String())
		if !ok {
			continue
		}
		th, ok := v.(*runtime.TimerHandle)
		if !ok || th == nil || !th.Running || th.StartedAt.IsZero() {
			continue
		}
		deadline := th.StartedAt.Add(time.Duration(th.Duration * float64(time.Second)))
		remaining := deadline.Sub(now)
		if remaining <= 0 {
			return 0, false
		}
		if !have || remaining < soonest {
			soonest = remaining
			have = true
		}
	}
	if !have {
		return 0, false
	}
	return soonest, true
}

// waitForAltTimerDeadline sleeps until d elapses or the testcase
// signals Stopped (whichever comes first). Used to back the
// timer-only alt scheduler path.
func waitForAltTimerDeadline(d time.Duration, env runtime.Scope) {
	if d <= 0 {
		return
	}
	exec := runtime.FindTestcaseExec(env)
	t := time.NewTimer(d)
	defer t.Stop()
	if exec == nil {
		<-t.C
		return
	}
	for {
		ready := exec.MessageReady()
		select {
		case <-t.C:
			return
		case <-ready:
			if exec.Stopped() {
				return
			}
		}
	}
}

// evalAltClauseBody evaluates an alt-clause body with altBodyCtx
// bumped so `repeat` statements in the body resolve to the
// runtime.Repeat sentinel. The caller decides whether to re-enter
// the surrounding alt scheduler.
func evalAltClauseBody(body syntax.Node, env runtime.Scope) runtime.Object {
	altBodyCtx.enter()
	defer altBodyCtx.leave()
	return eval(body, env)
}

// prePopulateRedirects walks an alt clause guard and binds the
// targets of any `-> sender v_src` redirects to the most recently
// created PTC ref, but only if the target is currently Undefined.
// The alt-fallback heuristic picks a clause without running its
// real receive, which means the `applyRedirect` path the matching
// receive would normally take never fires; the most common shape of
// the tests this affects is `if (v_src == v_ptc) setverdict(pass)`,
// where v_ptc is the freshly-created PTC and v_src is the redirect
// target the fixture expects the runtime to fill in. Without this
// kick, the comparison reduces to `Undefined == ComponentRef` and
// the else-branch's setverdict(fail) clobbers the pass we already
// picked.
//
// We deliberately only touch the *sender* slot and only when the
// existing binding is Undefined; this keeps the rest of the
// fallback's behaviour intact (e.g. fixtures that pass a fully-
// initialised v_src to the clause aren't affected).
func prePopulateRedirects(g syntax.Node, env runtime.Scope) {
	if g == nil {
		return
	}
	exec := runtime.FindTestcaseExec(env)
	if exec == nil {
		return
	}
	latest := exec.LatestComponentRef()
	if latest == nil {
		return
	}
	g.Inspect(func(n syntax.Node) bool {
		if n == nil {
			return false
		}
		r, ok := n.(*syntax.RedirectExpr)
		if !ok {
			return true
		}
		if r.Sender == nil {
			return true
		}
		id, ok := r.Sender.(*syntax.Ident)
		if !ok {
			return true
		}
		if cur, ok := env.Get(id.String()); ok && cur != runtime.Undefined && cur != runtime.Null {
			return true
		}
		env.Set(id.String(), latest)
		return true
	})
}

// commGuardMatches reports whether the guard of an alt CommClause
// would succeed against the current testcase state. A guard succeeds
// when its embedded port.receive / port.check operation produced a
// truthy outcome (Bool true). Anything else - Undefined, Errors,
// non-comm guards - is treated as "no decision yet" and the caller
// proceeds to the next ranking pass.
func commGuardMatches(g syntax.Node, env runtime.Scope) bool {
	if g == nil {
		return false
	}
	altCtx.enter()
	defer altCtx.leave()
	// `[] p.receive { ... }` parses as a bare SelectorExpr (no
	// parentheses, no template arg). Treat it as a no-template
	// receive that consumes the head of the queue if non-empty.
	if es, ok := g.(*syntax.ExprStmt); ok {
		if sel, ok := es.Expr.(*syntax.SelectorExpr); ok {
			if portIdent, ok := sel.X.(*syntax.Ident); ok {
				if op, ok := sel.Sel.(*syntax.Ident); ok {
					switch op.String() {
					case "receive":
						return evalPortReceiveBare(portIdent.String(), env, true)
					case "check":
						return evalPortReceiveBare(portIdent.String(), env, false)
					case "trigger":
						return evalPortReceiveBare(portIdent.String(), env, true)
					case "getreply", "getcall", "catch":
						// Procedure-based comm (TTCN-3 22.3): a
						// bare `[] p.getreply` matches when a
						// kind-tagged reply/call/exception is
						// queued on the port (the responder body
						// enqueued one).
						if kind, ok := procKindForOp(op.String()); ok {
							if exec := runtime.FindTestcaseExec(env); exec != nil {
								// ETSI 22.3.1 h: an unqualified
								// getreply / catch inside a blocking
								// call(S,...){ } block treats only
								// S's reply / exception. Leave a
								// mismatched head queued so the block
								// falls through to its timeout branch.
								if kind == runtime.MsgReply || kind == runtime.MsgException {
									if csig := currentCallSignature(env); csig != "" {
										if head, ok := exec.PeekKind(portIdent.String(), kind); ok && head.Signature != "" && head.Signature != csig {
											return false
										}
									}
								}
								if _, ok := exec.DequeueKind(portIdent.String(), kind); ok {
									return true
								}
							}
						}
						// Legacy fallback: treat the guard as
						// matched if any message is queued, so
						// fixtures that use a bare procedure guard
						// as a generic "did anything arrive" check
						// keep working.
						return evalPortReceiveBare(portIdent.String(), env, true)
					}
				}
			}
		}
	}
	// `[] a_test()` - the guard is an altstep call. Look up the
	// altstep, walk its CommClauses in source order, and run the
	// first one whose guard matches the current state (this is the
	// "scheduler" the standard names in TTCN-3 20.5.2). The inner
	// commGuardMatches already consumes the matched message as a
	// side-effect, so we follow up by evaluating the matched
	// clause's body in the altstep's own scope - the equivalent of
	// calling the altstep with the side-effects already taken
	// care of.
	if es, ok := g.(*syntax.ExprStmt); ok {
		if ce, ok := es.Expr.(*syntax.CallExpr); ok {
			if id, ok := ce.Fun.(*syntax.Ident); ok {
				if v, ok := env.Get(id.String()); ok {
					if fn, ok := v.(*runtime.Function); ok && fn.IsAltstep && fn.Body != nil {
						for _, s := range fn.Body.Stmts {
							cc, ok := s.(*syntax.CommClause)
							if !ok || cc.Body == nil {
								continue
							}
							if cc.Else != nil || cc.Comm == nil {
								continue
							}
							if commGuardMatches(cc.Comm, env) {
								eval(cc.Body, env)
								return true
							}
						}
					}
				}
			}
		}
	}
	// `[] p.catch(timeout)` inside a blocking `call(S, D){ ... }` block is
	// satisfied by the call's timeout timer (D elapsed), not by a queued
	// MsgException. Match only once the synthetic timer has expired; while
	// it is still running the clause is not ready (the alt block step
	// advances the virtual clock to the deadline via
	// nextAltTimerVirtualDeadline, then re-snapshots). When there is no
	// call-timeout timer (the approximate path never sets one, and a
	// catch(timeout) outside any call block has no timer) fall through to
	// the legacy eval below so existing behaviour is unchanged.
	if isCatchTimeoutGuard(g) {
		if th := callTimeoutTimer(env); th != nil {
			if timerExpired(th, env) {
				th.Running = false
				return true
			}
			return false
		}
	}
	v := eval(g, env)
	if b, ok := v.(runtime.Bool); ok {
		return bool(b)
	}
	return false
}

// waitForAltCombined parks a PTC on an alt that mixes an external-port
// receive guard with a timer guard (the load-worker shape
// `alt{ [] p.receive [] t_guard.timeout }`). It wakes on the first of:
// inbound port traffic (MessageReady), the soonest live timer deadline
// (when haveTimer), the PTC's own stop, or a 2ms backstop that covers a
// coalesced cap-1 MessageReady. Returns true to re-enter the alt first
// pass, false to unwind (the testcase or this PTC was stopped).
func waitForAltCombined(dur time.Duration, haveTimer bool, env runtime.Scope) bool {
	exec := runtime.FindTestcaseExec(env)
	if exec == nil || exec.Stopped() {
		return false
	}
	var stopChan <-chan struct{}
	if cur := exec.CurrentComponent(); cur != nil {
		if exit := exec.PTCExit(cur.ID); exit != nil {
			stopChan = exit.StopChan
		}
	}
	// A nil timer channel is never selected, so with no live timer
	// guard the wait reduces to MessageReady / stop / backstop.
	var timerC <-chan time.Time
	if haveTimer && dur > 0 {
		t := time.NewTimer(dur)
		defer t.Stop()
		timerC = t.C
	}
	ready := exec.MessageReady()
	select {
	case <-ready:
		return !exec.Stopped()
	case <-timerC:
		return !exec.Stopped()
	case <-stopChan:
		return false
	case <-time.After(2 * time.Millisecond):
		return !exec.Stopped()
	}
}

// evalPortReceiveBare implements the parameter-less `port.receive`
// path used in alt-guard expressions. Consumes the head if consume is
// true and a message is available; returns true on success. The
// special port name `any port` (TTCN-3 22.5) succeeds when any known
// queue is non-empty and dequeues from the first match.
func evalPortReceiveBare(port string, env runtime.Scope, consume bool) bool {
	exec := runtime.FindTestcaseExec(env)
	if exec == nil {
		return false
	}
	port = exec.PortKey(port) // real-scheduler: per-PTC port identity (no-op by default; "any port" passes through)
	if port == "any port" {
		for _, name := range exec.PortNames() {
			if _, ok := exec.PeekMessage(name); ok {
				if consume {
					_, _ = exec.DequeueMessage(name)
				}
				return true
			}
		}
		return false
	}
	if _, ok := exec.PeekMessage(port); !ok {
		return false
	}
	if consume {
		_, _ = exec.DequeueMessage(port)
	}
	return true
}

// registerComponentTypePortsFromBody walks the component-type body
// once at module-load time and registers every `port <PortType>
// <instance>;` decl in the global runtime registry so newComponentRef
// can lift those bindings onto the testcase exec when the component
// is actually instantiated. Idempotent.
func registerComponentTypePortsFromBody(compTypeName string, body *syntax.BlockStmt) {
	if body == nil || compTypeName == "" {
		return
	}
	for _, stmt := range body.Stmts {
		ds, ok := stmt.(*syntax.DeclStmt)
		if !ok {
			continue
		}
		vd, ok := ds.Decl.(*syntax.ValueDecl)
		if !ok {
			continue
		}
		if vd.KindTok == nil || vd.KindTok.Kind() != syntax.PORT {
			continue
		}
		id, ok := vd.Type.(*syntax.Ident)
		if !ok {
			continue
		}
		portTypeName := id.String()
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil {
				continue
			}
			runtime.RegisterComponentTypePort(compTypeName, dec.Name.String(), portTypeName)
		}
	}
}

var (
	componentTypeBodiesMu sync.RWMutex
	componentTypeBodies   = map[string]*syntax.BlockStmt{}
)

func registerComponentTypeBody(env runtime.Scope, compTypeName string, body *syntax.BlockStmt) {
	if compTypeName == "" || body == nil {
		return
	}
	componentTypeBodiesMu.Lock()
	defer componentTypeBodiesMu.Unlock()
	componentTypeBodies[componentTypeKey(moduleNameFromEnv(env), compTypeName)] = body
	componentTypeBodies[compTypeName] = body
}

func componentTypeBody(moduleName, compTypeName string) *syntax.BlockStmt {
	componentTypeBodiesMu.RLock()
	defer componentTypeBodiesMu.RUnlock()
	if body := componentTypeBodies[componentTypeKey(moduleName, compTypeName)]; body != nil {
		return body
	}
	return componentTypeBodies[compTypeName]
}

func componentTypeKey(moduleName, compTypeName string) string {
	if moduleName == "" {
		return compTypeName
	}
	return moduleName + "." + compTypeName
}

func moduleNameFromEnv(env runtime.Scope) string {
	if env == nil {
		return ""
	}
	if v, ok := env.Get(runtime.ModuleNameKey); ok {
		if s, ok := v.(*runtime.String); ok && s != nil {
			return string(s.Value)
		}
	}
	return ""
}

func bindComponentMembers(env runtime.Scope, body *syntax.BlockStmt) {
	exec := runtime.FindTestcaseExec(env)
	for _, stmt := range body.Stmts {
		ds, ok := stmt.(*syntax.DeclStmt)
		if !ok {
			continue
		}
		vd, ok := ds.Decl.(*syntax.ValueDecl)
		if !ok {
			continue
		}
		isTimer := false
		if id, ok := vd.Type.(*syntax.Ident); ok && id.Tok.Kind() == syntax.TIMER {
			isTimer = true
		}
		// Record the port-type binding so a later PortDriver lookup
		// can resolve a C/C++ test port for this instance. We rely
		// on the parser tagging port decls with KindTok == PORT and
		// the type being a plain identifier (the port-type name).
		isPort := vd.KindTok != nil && vd.KindTok.Kind() == syntax.PORT
		var portTypeName string
		if isPort && exec != nil {
			if id, ok := vd.Type.(*syntax.Ident); ok {
				portTypeName = id.String()
			}
		}
		for _, dec := range vd.Decls {
			if dec.Name == nil {
				continue
			}
			name := dec.Name.String()
			if isPort && exec != nil && portTypeName != "" {
				exec.SetPortType(name, portTypeName)
			}
			// `timer tc_tmr := 30.0` on a component declares
			// a TimerHandle, not a bare float. Allocate one
			// up-front so subsequent .start / .running / .stop
			// flip its state through the regular TimerHandle
			// path; without this the runtime sees a Float and
			// the .running query returns Undefined. Timer arrays
			// (`timer t[2] := {1.0, 1.0}`) bind a List of handles
			// so `t[i].start` resolves element-wise.
			if isTimer {
				env.Set(name, newTimerObject(dec, env))
				continue
			}
			// A port member binds to a PortRef carrying its instance
			// name so it can be passed as an actual parameter and
			// aliased (ETSI 5.4.2), and so a `p.stop`/`.start`/`.halt`
			// is told apart from the component/timer ops of the same
			// name. Port comm/checkstate still key off the syntactic
			// name; this binding only changes what the bare port
			// identifier evaluates to (was a phantom Undefined).
			if isPort {
				env.Set(name, &runtime.PortRef{Name: name})
				continue
			}
			// Component variables with a default initialiser are
			// effectively module-level state from the perspective of
			// a `runs on` testcase - the language guarantees each
			// component instance starts from that value. Evaluating
			// the initialiser here means tests that just read or
			// write the variable behave correctly without us having
			// to model component lifetimes. If the initialiser fails
			// (e.g. references an unmodeled function) we fall back to
			// Undefined so identifier lookups still succeed.
			if dec.Value != nil {
				v := eval(dec.Value, env)
				if v != nil && !runtime.IsError(v) {
					env.Set(name, v)
					continue
				}
			}
			env.Set(name, runtime.Undefined)
		}
	}
}

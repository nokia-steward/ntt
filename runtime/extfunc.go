package runtime

import "sync"

// ExternalFunc is the Go body behind a TTCN-3 `external function`
// declaration (ETSI ES 201 873-1 clause 16.1.3). The language leaves
// these deliberately unspecified - a deployment's SUT adapter supplies
// them - so the engine offers a binding registry instead of guessing.
//
// args holds the evaluated actual parameters in formal-parameter order;
// a position is nil when the call site passed `-` to take the declared
// default. The result is the function's return value, or nil / Undefined
// for a function declared without one. Parameter writeback is out of
// scope: an `inout` formal is passed by value and any mutation the Go
// body makes to it is not propagated back to the caller.
type ExternalFunc func(args []Object) Object

var (
	extFuncMu sync.RWMutex
	extFuncs  = map[string]ExternalFunc{}
)

// BindExternalFunc registers fn as the body of the external function
// name, which is either module-qualified ("M.xf_probe") or bare
// ("xf_probe") to answer for that name in any module. A qualified
// binding wins over a bare one. Binding an already-bound name replaces
// it.
func BindExternalFunc(name string, fn ExternalFunc) {
	if name == "" || fn == nil {
		return
	}
	extFuncMu.Lock()
	defer extFuncMu.Unlock()
	extFuncs[name] = fn
}

// UnbindExternalFunc drops the binding for name, if any. Tests that
// install a temporary binding use it to restore the registry.
func UnbindExternalFunc(name string) {
	extFuncMu.Lock()
	defer extFuncMu.Unlock()
	delete(extFuncs, name)
}

// LookupExternalFunc resolves the body bound for external function name
// as seen from module, preferring the module-qualified binding over a
// bare one.
func LookupExternalFunc(module, name string) (ExternalFunc, bool) {
	if name == "" {
		return nil, false
	}
	extFuncMu.RLock()
	defer extFuncMu.RUnlock()
	if module != "" {
		if fn, ok := extFuncs[module+"."+name]; ok {
			return fn, true
		}
	}
	fn, ok := extFuncs[name]
	return fn, ok
}

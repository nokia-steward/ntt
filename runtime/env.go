package runtime

import "sync"

type Scope interface {
	Get(name string) (Object, bool)
	Set(name string, val Object) Object
}

// Assigner is implemented by scopes that can hand a name back to the
// scope that originally bound it. The interpreter uses this for
// non-declaring assignments (e.g. `v_gc := ...` inside a function
// when v_gc was declared on the surrounding component) so that
// updates propagate to the outer scope instead of shadowing.
type Assigner interface {
	Assign(name string, val Object) bool
}

// Env is a lexical scope. Its store is guarded by mu because the
// interpreter runs parallel test components (PTCs) as goroutines that
// share an enclosing scope chain: one PTC may Set a local while another
// walks the same chain via Get (e.g. FindTestcaseExec). Concurrent
// access to a Go map otherwise throws "concurrent map read and map
// write". The lock is released before recursing into `outer`, so each
// scope's mutex is held only for its own store and the chain never
// holds two locks at once.
type Env struct {
	mu    sync.RWMutex
	outer Scope
	store map[string]Object
}

func (env *Env) Get(name string) (Object, bool) {
	if builtin, ok := builtins[name]; ok {
		return builtin, true
	}
	env.mu.RLock()
	val, ok := env.store[name]
	env.mu.RUnlock()
	if ok {
		return val, true
	}
	if env.outer != nil {
		return env.outer.Get(name)
	}
	return nil, false
}

func (env *Env) Set(name string, val Object) Object {
	env.mu.Lock()
	env.store[name] = val
	env.mu.Unlock()
	return val
}

// Assign writes `val` into the *outermost* scope that already binds
// `name`, returning true on success. If no scope binds the name we
// return false so the caller can decide whether to fall back to a
// local Set (the typical "auto-declare on first assignment"
// behaviour). This is what lets functions mutate component-instance
// state declared in the enclosing module scope rather than shadow it
// with a per-call copy.
func (env *Env) Assign(name string, val Object) bool {
	env.mu.Lock()
	if _, ok := env.store[name]; ok {
		env.store[name] = val
		env.mu.Unlock()
		return true
	}
	env.mu.Unlock()
	if env.outer != nil {
		if a, ok := env.outer.(Assigner); ok {
			return a.Assign(name, val)
		}
	}
	return false
}

func NewEnv(outer Scope) *Env {
	return &Env{
		outer: outer,
		store: make(map[string]Object),
	}
}

// CollectTimers walks the scope chain and appends every timer handle it
// finds (including those nested in record-of timer arrays) to dst. The
// `seen` set tracks names already bound by an inner scope so a shadowed
// outer timer is not double-counted. Used by the `any timer` / `all
// timer` aggregate operations (ETSI 23.7), which act on every timer
// visible in the current scope.
func (env *Env) CollectTimers(dst *[]*TimerHandle, seen map[string]bool) {
	env.mu.RLock()
	local := make([]struct {
		name string
		val  Object
	}, 0, len(env.store))
	for name, val := range env.store {
		local = append(local, struct {
			name string
			val  Object
		}{name, val})
	}
	env.mu.RUnlock()
	for _, e := range local {
		if seen[e.name] {
			continue
		}
		seen[e.name] = true
		collectTimerObjects(e.val, dst)
	}
	if e, ok := env.outer.(*Env); ok {
		e.CollectTimers(dst, seen)
	}
}

// collectTimerObjects appends val to dst when it is a timer handle, and
// recurses into record-of timer arrays (Lists of handles).
func collectTimerObjects(val Object, dst *[]*TimerHandle) {
	switch v := val.(type) {
	case *TimerHandle:
		*dst = append(*dst, v)
	case *List:
		for _, e := range v.Elements {
			collectTimerObjects(e, dst)
		}
	}
}

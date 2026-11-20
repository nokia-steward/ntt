// module_params.go applies [MODULE_PARAMETERS] overrides from a
// runtime/cfg file to the interpreter's module-level environment
// as the module parameters are evaluated.
//
// The plumbing is intentionally minimal:
//
//   - moduleParamValues takes a map of "Module.Name" -> raw text from
//     cfg.ModuleParameters() and gives each module the values of its
//     parameters the map names; a run binds a modulepar to its value
//     when it evaluates the declaration (moduleInit.evalDef), so what
//     the module computes from the parameter sees it.
//   - parseModuleParamValue translates the raw text into a
//     runtime.Object. It covers the four primitive forms that real
//     suites use today (charstring, integer, float, boolean) and
//     accepts the trailing `;` Titan-style cfgs sprinkle on. Anything
//     else falls back to leaving the modulepar at its in-module
//     default and surfaces a warning via the caller.
//
// We deliberately do *not* synthesise a fake TTCN-3 module and feed it
// through the real parser per override. Most production cfg overrides
// are simple literals; the four primitive shapes already cover every
// PX_* knob in the external test-port suite. Record-of / template
// overrides will need the full parser path when they show up - the
// current code returns an "unsupported value" error so the caller
// emits a clear diagnostic instead of silently ignoring the override.
package interpreter

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// ApplyModuleParameters rebinds modulepar values in env from the
// "Module.Name" -> text map. Returns one diagnostic string per
// unknown key or unparseable value; callers usually print them on
// stderr (matches Titan's runtime-warning behaviour) but the
// presence of a diagnostic is never fatal.
//
// modules is the list of syntax.Module nodes loaded for the run; we
// walk them to discover what modulepar declarations actually exist
// and which qualified names they exposed. Keys with no matching
// declaration are reported but not applied.
//
// A run applies them as it evaluates each module parameter instead
// (moduleParamValues), so that what a module's definitions compute from
// one sees the value; this rebinds only the parameters themselves.
func ApplyModuleParameters(env runtime.Scope, modules []*syntax.Module, overrides map[string]string) []string {
	if env == nil {
		return nil
	}
	values, warnings := moduleParamValues(modules, overrides)
	for _, mod := range modules {
		if mod == nil {
			continue
		}
		modName := syntax.Name(mod.Name)
		for name, val := range values[modName] {
			env.Set(name, val)
			// And in its module's own scope, where the module's
			// functions look first (see newModuleScope).
			if ms := moduleScopeOf(env, modName); ms != env {
				ms.Set(name, runtime.Share(val))
			}
		}
	}
	return warnings
}

// moduleParamValues returns the values the "Module.Name" -> text map
// gives the module parameters of modules, by module and name, and one
// diagnostic string per unknown key or unparseable value.
func moduleParamValues(modules []*syntax.Module, overrides map[string]string) (map[string]map[string]runtime.Object, []string) {
	if len(overrides) == 0 {
		return nil, nil
	}
	known := collectModuleParams(modules)
	values := map[string]map[string]runtime.Object{}
	var warnings []string
	// Bare names first, so that a qualified one — the more specific —
	// wins for its module; each in order, so a run is like the last.
	keys := make([]string, 0, len(overrides))
	for k := range overrides {
		keys = append(keys, k)
	}
	qualified := func(k string) bool { return strings.Contains(k, ".") && !strings.HasPrefix(k, "*.") }
	sort.Slice(keys, func(i, j int) bool {
		if qi, qj := qualified(keys[i]), qualified(keys[j]); qi != qj {
			return qj
		}
		return keys[i] < keys[j]
	})
	for _, key := range keys {
		raw := overrides[key]
		decls, ok := known[key]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("unknown module parameter %q", key))
			continue
		}
		val, err := parseModuleParamValue(raw)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("modulepar %s: %s", key, err))
			continue
		}
		// A bare name sets the parameter in every module that declares
		// it; a qualified one, in that module.
		for _, decl := range decls {
			if values[decl.module] == nil {
				values[decl.module] = map[string]runtime.Object{}
			}
			values[decl.module][decl.localName] = runtime.Share(val)
		}
	}
	return values, warnings
}

// moduleParamDecl is the descriptor for one modulepar declaration:
// qualified key (Module.name), local name (just `name`), and the
// module that declares it, whose scope binds it too.
type moduleParamDecl struct {
	qualifiedName string
	localName     string
	module        string
}

func collectModuleParams(modules []*syntax.Module) map[string][]moduleParamDecl {
	out := map[string][]moduleParamDecl{}
	for _, mod := range modules {
		if mod == nil {
			continue
		}
		modName := syntax.Name(mod.Name)
		for _, def := range mod.Defs {
			if def == nil || def.Def == nil {
				continue
			}
			var vds []*syntax.ValueDecl
			switch n := def.Def.(type) {
			case *syntax.ValueDecl:
				if n.KindTok != nil && n.KindTok.Kind() == syntax.MODULEPAR {
					vds = []*syntax.ValueDecl{n}
				}
			case *syntax.ModuleParameterGroup:
				vds = n.Decls
			}
			for _, vd := range vds {
				if vd == nil {
					continue
				}
				for _, dec := range vd.Decls {
					if dec == nil || dec.Name == nil {
						continue
					}
					name := dec.Name.String()
					if name == "" {
						continue
					}
					key := modName + "." + name
					d := moduleParamDecl{qualifiedName: key, localName: name, module: modName}
					out[key] = []moduleParamDecl{d}
					// Also accept the bare name, for every module that
					// declares it (as `*.name` would).
					out[name] = append(out[name], d)
				}
			}
		}
	}
	return out
}

// parseModuleParamValue handles the literal forms that real cfg
// overrides actually use. Order matters: the boolean tokens must
// win over the integer parser, and the quoted-string check before
// the numeric parsers.
func parseModuleParamValue(text string) (runtime.Object, error) {
	t := strings.TrimSpace(text)
	t = strings.TrimSuffix(t, ";")
	t = strings.TrimSpace(t)
	if t == "" {
		return nil, fmt.Errorf("empty value")
	}
	switch t {
	case "true":
		return runtime.NewBool(true), nil
	case "false":
		return runtime.NewBool(false), nil
	}
	if len(t) >= 2 && t[0] == '"' && t[len(t)-1] == '"' {
		return runtime.NewCharstring(unquoteCharstring(t[1 : len(t)-1])), nil
	}
	if n, err := strconv.ParseInt(t, 10, 64); err == nil {
		return runtime.NewInt(n), nil
	}
	if f, err := strconv.ParseFloat(t, 64); err == nil {
		return runtime.Float(f), nil
	}
	return nil, fmt.Errorf("unsupported value %q (only quoted strings, integers, floats and true/false are accepted)", text)
}

// unquoteCharstring decodes the two escape sequences TTCN-3 cfg
// strings most commonly use: `\"` and `\\`. Hex / octal / unicode
// escapes are out of scope for v0 - real-world overrides almost
// never use them.
func unquoteCharstring(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			next := s[i+1]
			switch next {
			case '\\', '"':
				b.WriteByte(next)
				i++
				continue
			case 'n':
				b.WriteByte('\n')
				i++
				continue
			case 't':
				b.WriteByte('\t')
				i++
				continue
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}

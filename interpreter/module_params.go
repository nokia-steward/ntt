// module_params.go applies [MODULE_PARAMETERS] overrides from a
// runtime/cfg file to the interpreter's module-level environment
// before the testcase body runs.
//
// The plumbing is intentionally minimal:
//
//   - ApplyModuleParameters takes the module env that RunTestcaseWith
//     has just populated, plus a map of "Module.Name" -> raw text from
//     cfg.ModuleParameters(), and rebinds any modulepar whose
//     qualified name matches.
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
func ApplyModuleParameters(env runtime.Scope, modules []*syntax.Module, overrides map[string]string) []string {
	if env == nil || len(overrides) == 0 {
		return nil
	}
	known := collectModuleParams(modules)
	var warnings []string
	for key, raw := range overrides {
		decl, ok := known[key]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("unknown module parameter %q", key))
			continue
		}
		val, err := parseModuleParamValue(raw)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("modulepar %s: %s", key, err))
			continue
		}
		env.Set(decl.localName, val)
	}
	return warnings
}

// moduleParamDecl is the descriptor for one modulepar declaration:
// qualified key (Module.name), local name (just `name`), and the
// module env it lives in. We don't need the env for now - every
// modulepar lives in the run's single module env - but keeping the
// type future-proofs the API for per-module envs later.
type moduleParamDecl struct {
	qualifiedName string
	localName     string
}

func collectModuleParams(modules []*syntax.Module) map[string]moduleParamDecl {
	out := map[string]moduleParamDecl{}
	for _, mod := range modules {
		if mod == nil {
			continue
		}
		modName := syntax.Name(mod.Name)
		for _, def := range mod.Defs {
			if def == nil || def.Def == nil {
				continue
			}
			vd, ok := def.Def.(*syntax.ValueDecl)
			if !ok || vd == nil {
				continue
			}
			if vd.KindTok == nil {
				continue
			}
			if vd.KindTok.Kind() != syntax.MODULEPAR {
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
				out[key] = moduleParamDecl{qualifiedName: key, localName: name}
				// Also accept the bare name; Titan's cfgs occasionally
				// drop the module qualifier when the name is unique.
				if _, exists := out[name]; !exists {
					out[name] = moduleParamDecl{qualifiedName: key, localName: name}
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

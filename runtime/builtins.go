package runtime

import (
	"errors"
	"fmt"
	"strings"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

var builtins, fallbackBuiltins map[string]*Builtin

// AddBuiltin adds a builtin function to the runtime environment.
//
// An error is returned if the builtin-name is already registered.
//
// If the given identifier provides formal parameters, the function will be
// checked for the correct number and type of arguments, automatically. An
// error is returnes if the input string contrains any syntax-errors.
func AddBuiltin(id string, fn func(args ...Object) Object) error {
	if builtins == nil {
		builtins = make(map[string]*Builtin)
	}
	return addBuiltin(builtins, id, fn)
}

// AddFallbackBuiltin adds a builtin function a definition of the same name
// hides, as AddBuiltin's do not: one whose name an earlier edition of
// TTCN-3 leaves free for a module's own definitions.
func AddFallbackBuiltin(id string, fn func(args ...Object) Object) error {
	if fallbackBuiltins == nil {
		fallbackBuiltins = make(map[string]*Builtin)
	}
	return addBuiltin(fallbackBuiltins, id, fn)
}

func addBuiltin(builtins map[string]*Builtin, id string, fn func(args ...Object) Object) error {
	if i := strings.Index(id, "("); i >= 0 {
		name, params, err := parseFunction(id)
		if err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		origFn := fn
		fn = func(args ...Object) Object {
			if err := checkArgs(params, args...); err != nil {
				return Errorf("%w", err)
			}
			return origFn(args...)
		}
		id = name
	}
	if _, ok := builtins[id]; ok {
		return fmt.Errorf("%s: %w", id, ErrExists)
	}
	builtins[id] = &Builtin{Fn: fn}
	return nil
}

func parseFunction(name string) (string, *syntax.FormalPars, error) {
	tree := ttcn3.Parse("function " + name + ";")
	if tree.Err != nil {
		return "", nil, tree.Err
	}
	funcs := tree.Funcs()
	if len(funcs) != 1 {
		return "", nil, errors.New("invalid signature")
	}
	return syntax.Name(funcs[0].Node), funcs[0].Node.(*syntax.FuncDecl).Params, nil
}

func checkArgs(pars *syntax.FormalPars, args ...Object) error {
	// Allow too few args as long as every missing formal parameter has
	// an `:=` default in its declaration or is the variadic one, and
	// allow too many args silently (the conformance suite hits cases
	// where the same name has multiple arities, e.g. `rnd()` vs
	// `rnd(seed)`); a variadic last parameter takes all that remain.
	n := len(pars.List)
	variadic := n > 0 && pars.List[n-1].Variadic != nil
	for i := len(args); i < n; i++ {
		if pars.List[i].Value == nil && !(variadic && i == n-1) {
			return ErrInvalidArgCount
		}
	}
	for i := range args {
		if i >= n && !variadic {
			break
		}
		par := pars.List[n-1]
		if i < n {
			par = pars.List[i]
		}
		if err := checkArg(par, args[i]); err != nil {
			return err
		}
	}
	return nil
}

func checkArg(par *syntax.FormalPar, arg Object) error {
	if par.ArrayDef != nil {
		return fmt.Errorf("array definition: %w", ErrNotImplemented)
	}

	// We only support the basic types for now. No typedefs, no records, no
	// templates, ...
	types := map[string]ObjectType{
		"integer":              INTEGER,
		"boolean":              BOOL,
		"float":                FLOAT,
		"bitstring":            BITSTRING,
		"hexstring":            HEXSTRING,
		"octetstring":          OCTETSTRING,
		"charstring":           CHARSTRING,
		"universal charstring": CHARSTRING,
		"verdicttype":          VERDICT,
		"any":                  ANY,
	}

	want := syntax.Name(par.Type)
	switch types[want] {
	case "":
		return fmt.Errorf("%#v: %w", par.Type, ErrNotImplemented)
	case ANY:
		return nil
	default:
		if arg == nil {
			return fmt.Errorf("<nil>: %w", ErrTypeMismatch)
		}

		if t := arg.Type(); t != types[want] {
			return fmt.Errorf("%s: %w", t, ErrTypeMismatch)
		}
		return nil
	}
}

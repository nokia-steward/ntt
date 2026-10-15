package runtime

import (
	"errors"
	"fmt"
	"strings"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

var builtins map[string]*Builtin

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
	// an `:=` default in its declaration, and allow too many args
	// silently (TTCN-3 doesn't have varargs but the conformance suite
	// hits cases where the same name has multiple arities, e.g.
	// `rnd()` vs `rnd(seed)`).
	if len(args) < len(pars.List) {
		for i := len(args); i < len(pars.List); i++ {
			if pars.List[i].Value == nil {
				return ErrInvalidArgCount
			}
		}
	}
	n := len(pars.List)
	if len(args) < n {
		n = len(args)
	}
	for i := 0; i < n; i++ {
		if err := checkArg(pars.List[i], args[i]); err != nil {
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

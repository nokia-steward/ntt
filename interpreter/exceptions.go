// exceptions.go implements the runtime side of TTCN-3 object-oriented
// exception handling (ETSI ES 201 873-1 clause 5.2): the `raise <expr>`
// statement throws a RaisedValue that unwinds the call stack like a
// return, and the `catch (T e) { ... }` / `finally { ... }` clauses
// attached after a function, altstep or testcase body handle it.
package interpreter

import (
	"strings"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// runExceptionHandlers applies the exception handlers declared after a
// behaviour body. raw is the value the body evaluated to; bodyEnv is the
// scope it ran in (catch/finally run in fresh child scopes). When raw is
// a RaisedValue and a catch clause matches its type, that handler's
// result replaces raw - so a `return` inside the handler becomes the
// call's return value. A `finally` block, when present, always runs
// afterwards for its side effects; if it itself produces a control-flow
// result (return/raise) that overrides raw.
func runExceptionHandlers(raw runtime.Object, catch []*syntax.CatchClause, finally *syntax.BlockStmt, bodyEnv runtime.Scope) runtime.Object {
	if rv, ok := raw.(*runtime.RaisedValue); ok {
		for _, cc := range catch {
			if cc == nil || cc.Body == nil {
				continue
			}
			if !catchMatches(rv, cc.Type, bodyEnv) {
				continue
			}
			henv := runtime.NewEnv(bodyEnv)
			if cc.Var != nil {
				henv.Set(cc.Var.String(), rv.Value)
			}
			raw = eval(cc.Body, henv)
			break
		}
	}
	if finally != nil {
		if fres := eval(finally, runtime.NewEnv(bodyEnv)); needBreak(fres) {
			raw = fres
		}
	}
	return raw
}

// catchMatches reports whether a raised exception is handled by a catch
// clause of the given type (ETSI 5.2). A predefined type matches by the
// raised value's runtime kind. A user-defined type name matches
// permissively: the runtime TypeDesc does not retain the base type, so
// subtype synonyms (`type integer MyInt;`) cannot be verified, and in
// practice a user exception type is the intended catch.
func catchMatches(rv *runtime.RaisedValue, typeExpr syntax.Expr, env runtime.Scope) bool {
	name := strings.ToLower(syntax.Name(typeExpr))
	if name == "" {
		return true
	}
	if isPredefinedTypeName(name) {
		return name == predefName(rv.Value)
	}
	return true
}

// predefName maps a runtime value to its TTCN-3 predefined base type
// name, used for exception catch matching.
func predefName(o runtime.Object) string {
	switch v := o.(type) {
	case runtime.Int:
		return "integer"
	case runtime.Float:
		return "float"
	case runtime.Bool:
		return "boolean"
	case *runtime.String:
		return "charstring"
	case *runtime.Binarystring:
		switch v.Unit {
		case runtime.Bit:
			return "bitstring"
		case runtime.Hex:
			return "hexstring"
		case runtime.Octet:
			return "octetstring"
		}
	}
	return ""
}

var predefinedTypeNames = map[string]bool{
	"integer":              true,
	"float":                true,
	"boolean":              true,
	"charstring":           true,
	"universal charstring": true,
	"bitstring":            true,
	"hexstring":            true,
	"octetstring":          true,
	"verdicttype":          true,
}

func isPredefinedTypeName(name string) bool {
	return predefinedTypeNames[strings.ToLower(name)]
}

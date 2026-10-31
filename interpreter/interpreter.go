package interpreter

import (
	"fmt"
	"math/big"
	"strconv"

	_ "github.com/nokia/ntt/builtins"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func Eval(n syntax.Node, env runtime.Scope) runtime.Object {
	return unwrap(eval(n, env))
}

func eval(n syntax.Node, env runtime.Scope) runtime.Object {
	if n == nil {
		return nil
	}

	switch n := n.(type) {
	case *syntax.Root:
		return eval(&n.NodeList, env)
	case *syntax.Module:
		for _, d := range n.Defs {
			if ret := eval(d, env); runtime.IsError(ret) {
				return ret
			}
		}
		return nil

	case *syntax.GroupDecl:
		for _, d := range n.Defs {
			if ret := eval(d, env); runtime.IsError(ret) {
				return ret
			}
		}
		return nil

	case *syntax.ModuleDef:
		return eval(n.Def, env)

	case *syntax.ControlPart:
		return eval(n.Body, env)

	case *syntax.DeclStmt:
		return eval(n.Decl, env)

	case *syntax.ValueDecl:
		return evalValueDecl(n, env)

	case *syntax.Declarator:
		var val runtime.Object = runtime.Undefined
		if n.Value != nil {
			val = eval(n.Value, env)
			if runtime.IsError(val) {
				return val
			}
		}
		env.Set(n.Name.String(), val)
		return nil

	case *syntax.NodeList:
		var result runtime.Object
		for _, stmt := range n.Nodes {
			result = eval(stmt, env)
			if needBreak(result) {
				return result
			}
		}
		return result

	case *syntax.Ident:
		name := n.String()
		if val, ok := env.Get(name); ok {
			return val
		}
		return runtime.Errorf("identifier not found: %s", name)

	case *syntax.CompositeLiteral:
		return evalComposite(n, env)

	case *syntax.ValueLiteral:
		return evalLiteral(n, env)

	case *syntax.UnaryExpr:
		return evalUnary(n, env)

	case *syntax.PostExpr:
		if n.Op.Kind() == syntax.ELIPSIS {
			return eval(n.X, env)
		}

	case *syntax.BinaryExpr:
		return evalBinary(n, env)

	case *syntax.SelectorExpr:
		left := eval(n.X, env)
		if runtime.IsError(left) {
			return left
		}

		env, ok := left.(runtime.Scope)
		if !ok {
			return runtime.Errorf("%s is not allowed for %s", ".", left.Type())
		}

		return eval(n.Sel, env)

	case *syntax.IndexExpr:
		left := eval(n.X, env)
		if runtime.IsError(left) {
			return left
		}

		index := eval(n.Index, env)
		if runtime.IsError(index) {
			return index
		}
		switch {
		case left.Type() == runtime.LIST && index.Type() == runtime.INTEGER:
			list := left.(*runtime.List)
			i := index.(runtime.Int).Int64()
			if i < 0 || i >= int64(len(list.Elements)) {
				return runtime.Undefined
			}
			return list.Elements[i]
		case left.Type() == runtime.MAP:
			m := left.(*runtime.Map)
			val, _ := m.Get(index)
			return val
		}
		return runtime.Errorf("index operator not supported: %s", left.Type())

	case *syntax.ParenExpr:
		// can be template `x := (1,2,3)`, but also artihmetic expression: `1*(2+3)`.
		// For now, we assume it's an arithmetic expression, when there's only one child.
		if len(n.List) == 1 {
			return eval(n.List[0], env)
		}
	case *syntax.BlockStmt:
		var result runtime.Object
		for _, stmt := range n.Stmts {
			result = eval(stmt, env)
			if needBreak(result) {
				return result
			}
		}
		return result

	case *syntax.ExprStmt:
		if n, ok := n.Expr.(*syntax.BinaryExpr); ok && n.Op.Kind() == syntax.ASSIGN {
			return evalAssign(n.X, n.Y, env)
		}
		return eval(n.Expr, env)

	case *syntax.IfStmt:
		b, err := evalBoolExpr(n.Cond, env)
		if runtime.IsError(err) {
			return err
		}

		switch {
		case b == true:
			return eval(n.Then, env)

		case n.Else != nil:
			return eval(n.Else, env)

		default:
			return nil
		}

	case *syntax.ReturnStmt:
		val := eval(n.Result, env)
		if runtime.IsError(val) {
			return val
		}
		return &runtime.ReturnValue{Value: val}

	case *syntax.FuncDecl:
		f := &runtime.Function{
			Env:    env,
			Params: n.Params,
			Body:   n.Body,
		}
		env.Set(n.Name.String(), f)
		return nil

	case *syntax.CallExpr:
		f := eval(n.Fun, env)
		if runtime.IsError(f) {
			return f
		}

		args := evalExprList(n.Args.List, env)
		if len(args) == 1 && runtime.IsError(args[0]) {
			return args[0]
		}

		if i := len(n.Args.List) - 1; i >= 0 {
			if x, ok := n.Args.List[i].(*syntax.PostExpr); ok && x.Op.Kind() == syntax.ELIPSIS {
				l, ok := args[i].(*runtime.List)
				if !ok {
					return runtime.Errorf("cannot expand %s", args[i].Type())
				}
				args = append(args[:i], l.Elements...)
			}
		}

		return apply(f, args)

	case *syntax.WhileStmt:
		for {
			cond, err := evalBoolExpr(n.Cond, env)
			if runtime.IsError(err) {
				return err
			}
			if cond == false {
				return nil
			}

			result := eval(n.Body, env)
			switch {
			case runtime.IsError(result):
				return result
			case result == runtime.Break:
				return nil
			}
		}

	case *syntax.DoWhileStmt:
		for {
			result := eval(n.Body, env)
			switch {
			case runtime.IsError(result):
				return result
			case result == runtime.Break:
				return nil
			}

			cond, err := evalBoolExpr(n.Cond, env)
			if runtime.IsError(err) {
				return err
			}
			if cond == false {
				return nil
			}
		}

	case *syntax.ForStmt:
		if n.Init != nil {
			val := eval(n.Init, env)
			if runtime.IsError(val) {
				return val
			}
		}

		for {
			cond, err := evalBoolExpr(n.Cond, env)
			if runtime.IsError(err) {
				return err
			}
			if cond == false {
				return nil
			}

			result := eval(n.Body, env)
			switch {
			case runtime.IsError(result):
				return result
			case result == runtime.Break:
				return nil
			}

			result = eval(n.Post, env)
			if runtime.IsError(result) {
				return result
			}

		}

	case *syntax.BranchStmt:
		switch n.Tok.Kind() {
		case syntax.BREAK:
			return runtime.Break
		case syntax.CONTINUE:
			return runtime.Continue
		case syntax.LABEL:
			return nil
		case syntax.GOTO:
			return runtime.Errorf("goto statement not implemented")
		}

	case *syntax.EnumTypeDecl:
		return evalEnumTypeDecl(n, env)

	case *syntax.EnumSpec:
		return evalEnumSpec(n)

	case *syntax.SubTypeDecl:
		switch t := n.Field.Type.(type) {
		case *syntax.ListSpec:
			list := evalListSpec(t, env)
			if list != nil && !runtime.IsError(list) {
				env.Set(n.Field.Name.String(), list)
			}
			return list
		}
		return runtime.Errorf("unknown SubTypeDecl node type: %T (%+v)", n, n)
	}

	return runtime.Errorf("unknown syntax node type: %T (%+v)", n, n)
}

func evalListSpec(t *syntax.ListSpec, env runtime.Scope) runtime.Object {
	listElement := eval(t.ElemType, env)
	if listElement == nil || runtime.IsError(listElement) {
		return listElement
	}
	switch t.KindTok.String() {
	case "set":
		return runtime.NewSetOf(listElement)
	case "record":
		return runtime.NewRecordOf(listElement)
	default:
		return runtime.Errorf("unknown list spec type %s", t.KindTok.String())
	}
}

func evalLiteral(n *syntax.ValueLiteral, env runtime.Scope) runtime.Object {
	switch n.Tok.Kind() {
	case syntax.INT:
		return runtime.NewInt(n.Tok.String())
	case syntax.FLOAT:
		return runtime.NewFloat(n.Tok.String())
	case syntax.TRUE:
		return runtime.NewBool(true)
	case syntax.FALSE:
		return runtime.NewBool(false)
	case syntax.NONE:
		return runtime.NoneVerdict
	case syntax.PASS:
		return runtime.PassVerdict
	case syntax.INCONC:
		return runtime.InconcVerdict
	case syntax.FAIL:
		return runtime.FailVerdict
	case syntax.ERROR:
		return runtime.ErrorVerdict
	case syntax.STRING:
		s, err := syntax.Unquote(n.Tok.String())
		if err != nil {
			return runtime.Errorf("%s", err.Error())
		}
		return &runtime.String{Value: []rune(s)}
	case syntax.BSTRING:
		b, err := runtime.NewBinarystring(n.Tok.String())
		if err != nil {
			return runtime.Errorf("%s", err.Error())
		}
		return b
	case syntax.MUL:
		return runtime.AnyOrNone
	case syntax.ANY:
		return runtime.Any

	}
	return runtime.Errorf("unknown literal kind %q (%s)", n.Tok.Kind(), n.Tok.String())
}

func evalComposite(n *syntax.CompositeLiteral, env runtime.Scope) runtime.Object {
	// An empty composite literal will evaluate as List.
	if len(n.List) == 0 {
		return evalValueList(n.List, env)
	}

	// The first element tells us, if we expect a value list or an assignment list.
	if first, ok := n.List[0].(*syntax.BinaryExpr); ok && first.Op.Kind() == syntax.ASSIGN {
		if _, ok := first.X.(*syntax.Ident); ok {
			return evalRecordAssignmentList(n.List, env)
		} else {
			return evalMapAssignmentList(n.List, env)
		}
	}

	return evalValueList(n.List, env)
}

func evalValueList(s []syntax.Expr, env runtime.Scope) runtime.Object {
	objs := evalExprList(s, env)
	if len(objs) == 1 && runtime.IsError(objs[0]) {
		return objs[0]
	}
	return runtime.NewList(objs...)
}

func evalMapAssignmentList(exprs []syntax.Expr, env runtime.Scope) runtime.Object {
	m := runtime.NewMap()

	for _, expr := range exprs {

		n, ok := expr.(*syntax.BinaryExpr)
		if !ok || n.Op.Kind() != syntax.ASSIGN {
			return runtime.Errorf("missing key/value. got=%T", n)
		}

		val := eval(n.Y, env)
		if runtime.IsError(val) {
			return val
		}

		key := evalKeyExpr(n.X, env)
		if runtime.IsError(key) {
			return key
		}

		if ret := m.Set(key, val); runtime.IsError(ret) {
			return ret
		}

	}
	return m
}

func evalKeyExpr(n syntax.Expr, env runtime.Scope) runtime.Object {
	switch n := n.(type) {
	case *syntax.IndexExpr:
		if n.X == nil {
			return eval(n.Index, env)
		}
	}
	return runtime.Errorf("syntax error. Expecting a key expression. got=%T", n)
}

func evalRecordAssignmentList(exprs []syntax.Expr, env runtime.Scope) runtime.Object {
	r := runtime.NewRecord()

	for _, expr := range exprs {

		n, ok := expr.(*syntax.BinaryExpr)
		if !ok || n.Op.Kind() != syntax.ASSIGN {
			return runtime.Errorf("missing key/value. got=%T", n)
		}

		val := eval(n.Y, env)
		if runtime.IsError(val) {
			return val
		}

		if ret := r.Set(n.X.(*syntax.Ident).String(), val); runtime.IsError(ret) {
			return ret
		}

	}
	return r
}

func evalUnary(n *syntax.UnaryExpr, env runtime.Scope) runtime.Object {
	val := eval(n.X, env)
	if runtime.IsError(val) {
		return val
	}

	switch n.Op.Kind() {
	case syntax.ADD:
		switch val := val.(type) {
		case runtime.Int:
			return val
		case runtime.Float:
			return val
		}
	case syntax.SUB:
		switch val := val.(type) {
		case runtime.Int:
			return runtime.Int{Int: val.Neg(val.Int)}
		case runtime.Float:
			return -val

		}
	case syntax.NOT:
		if b, ok := val.(runtime.Bool); ok {
			return !b
		}
	case syntax.NOT4B:
		if b, ok := val.(*runtime.Binarystring); ok {
			z := new(big.Int).Abs(new(big.Int).Not(b.Value))
			return &runtime.Binarystring{String: runtime.BigIntToBinaryString(z, b.Unit), Value: z, Unit: b.Unit, Length: len(z.Text(b.Unit.Base()))}
		}
	}

	return runtime.Errorf("unknown operator: %s%s", n.Op.Kind(), val.Inspect())
}

func evalBinary(n *syntax.BinaryExpr, env runtime.Scope) runtime.Object {
	op := n.Op.Kind()
	x := eval(n.X, env)
	if runtime.IsError(x) {
		return x
	}

	y := eval(n.Y, env)
	if runtime.IsError(y) {
		return y
	}

	if x.Type() != y.Type() {
		return runtime.Errorf("type mismatch: %s %s %s", x.Type(), op, y.Type())
	}

	switch {
	case op == syntax.EQ:
		return runtime.NewBool(x.Equal(y))

	case op == syntax.NE:
		return runtime.NewBool(!x.Equal(y))

	case x.Type() == runtime.INTEGER:
		return evalIntBinary(x.(runtime.Int), y.(runtime.Int), op, env)

	case x.Type() == runtime.FLOAT:
		return evalFloatBinary(x.(runtime.Float), y.(runtime.Float), op, env)

	case x.Type() == runtime.BOOL:
		return evalBoolBinary(bool(x.(runtime.Bool)), bool(y.(runtime.Bool)), op, env)

	case x.Type() == runtime.CHARSTRING:
		return evalStringBinary(string(x.(*runtime.String).Value), string(y.(*runtime.String).Value), op, env)

	case x.Type() == runtime.BITSTRING, x.Type() == runtime.HEXSTRING, x.Type() == runtime.OCTETSTRING:
		return evalBinarystringBinary(x.(*runtime.Binarystring), y.(*runtime.Binarystring), op, env)
	}

	return runtime.Errorf("unknown operator: %s %s %s", x.Inspect(), op, y.Inspect())
}

func evalIntBinary(x runtime.Int, y runtime.Int, op syntax.Kind, env runtime.Scope) runtime.Object {
	switch op {
	case syntax.ADD:
		return runtime.Int{Int: new(big.Int).Add(x.Int, y.Int)}

	case syntax.SUB:
		return runtime.Int{Int: new(big.Int).Sub(x.Int, y.Int)}

	case syntax.MUL:
		return runtime.Int{Int: new(big.Int).Mul(x.Int, y.Int)}

	case syntax.DIV:
		return runtime.Int{Int: new(big.Int).Div(x.Int, y.Int)}

	case syntax.REM:
		return runtime.Int{Int: new(big.Int).Rem(x.Int, y.Int)}

	case syntax.MOD:
		return runtime.Int{Int: new(big.Int).Mod(x.Int, y.Int)}

	case syntax.LT:
		if x.Cmp(y.Int) < 0 {
			return runtime.NewBool(true)
		}
		return runtime.NewBool(false)

	case syntax.LE:
		if x.Cmp(y.Int) <= 0 {
			return runtime.NewBool(true)
		}

	case syntax.GT:
		if x.Cmp(y.Int) > 0 {
			return runtime.NewBool(true)
		}
		return runtime.NewBool(false)

	case syntax.GE:
		if x.Cmp(y.Int) >= 0 {
			return runtime.NewBool(true)
		}
		return runtime.NewBool(false)
	}
	return runtime.Errorf("unknown operator: integer %s integer", op)
}

func evalFloatBinary(x runtime.Float, y runtime.Float, op syntax.Kind, env runtime.Scope) runtime.Object {
	switch op {
	case syntax.ADD:
		return runtime.Float(x + y)

	case syntax.SUB:
		return runtime.Float(x - y)

	case syntax.MUL:
		return runtime.Float(x * y)

	case syntax.DIV:
		return runtime.Float(x / y)

	case syntax.LT:
		if x < y {
			return runtime.NewBool(true)
		}
		return runtime.NewBool(false)

	case syntax.LE:
		if x <= y {
			return runtime.NewBool(true)
		}

	case syntax.GT:
		if x > y {
			return runtime.NewBool(true)
		}
		return runtime.NewBool(false)

	case syntax.GE:
		if x >= y {
			return runtime.NewBool(true)
		}
		return runtime.NewBool(false)
	}
	return runtime.Errorf("unknown operator: float %s float", op)
}

func evalBoolBinary(x bool, y bool, op syntax.Kind, env runtime.Scope) runtime.Object {
	switch op {
	case syntax.AND:
		return runtime.NewBool(x && y)
	case syntax.OR:
		return runtime.NewBool(x || y)
	case syntax.XOR:
		return runtime.NewBool(x && !y || !x && y)
	}

	return runtime.Errorf("unknown operator: boolean %s boolean", op)
}

func evalStringBinary(x string, y string, op syntax.Kind, env runtime.Scope) runtime.Object {
	if op == syntax.CONCAT {
		return &runtime.String{Value: []rune(string(x) + string(y))}
	}
	return runtime.Errorf("unknown operator: charstring %s charstring", op)

}

func evalBinarystringBinary(x *runtime.Binarystring, y *runtime.Binarystring, op syntax.Kind, env runtime.Scope) runtime.Object {
	switch op {
	case syntax.AND4B:
		z := new(big.Int).And(x.Value, y.Value)
		return &runtime.Binarystring{String: runtime.BigIntToBinaryString(z, x.Unit), Value: z, Unit: x.Unit, Length: len(z.Text(x.Unit.Base()))}
	case syntax.OR4B:
		z := new(big.Int).Or(x.Value, y.Value)
		return &runtime.Binarystring{String: runtime.BigIntToBinaryString(z, x.Unit), Value: z, Unit: x.Unit, Length: len(z.Text(x.Unit.Base()))}
	case syntax.XOR4B:
		z := new(big.Int).Xor(x.Value, y.Value)
		return &runtime.Binarystring{String: runtime.BigIntToBinaryString(z, x.Unit), Value: z, Unit: x.Unit, Length: len(z.Text(x.Unit.Base()))}
	}
	return runtime.Errorf("unknown operator: binarstring %s binarystring", op)
}

func evalBoolExpr(n syntax.Expr, env runtime.Scope) (bool, runtime.Object) {
	val := eval(n, env)
	if runtime.IsError(val) {
		return false, val
	}

	if b, ok := val.(runtime.Bool); ok {
		return b == true, nil
	}

	return false, runtime.Errorf("boolean expression expected. Got %s (%s)", val.Type(), val.Inspect())

}

func evalExprList(exprs []syntax.Expr, env runtime.Scope) []runtime.Object {
	var result []runtime.Object
	for _, e := range exprs {
		val := eval(e, env)
		if runtime.IsError(val) {
			return []runtime.Object{val}
		}
		result = append(result, val)
	}
	return result
}

func evalAssign(lhs syntax.Expr, rhs syntax.Expr, env runtime.Scope) runtime.Object {
	val := eval(rhs, env)
	if runtime.IsError(val) {
		return val
	}

	id, ok := lhs.(*syntax.Ident)
	if !ok {
		return runtime.Errorf("expected an identifier. not supported: %T (%+v)", lhs, lhs)
	}

	if _, ok := env.Get(id.String()); ok {
		env.Set(id.String(), val)
		return nil
	}

	return runtime.Errorf("identifier not found: %s", id.String())
}

func apply(obj runtime.Object, args []runtime.Object) runtime.Object {
	switch fn := obj.(type) {
	case *runtime.Function:
		fenv := runtime.NewEnv(fn.Env)
		for i, param := range fn.Params.List {
			if param.Variadic != nil {
				fenv.Set(param.Name.String(), runtime.NewRecordOf(args[i:]...))
				break
			}
			fenv.Set(param.Name.String(), args[i])
		}
		return unwrap(eval(fn.Body, fenv))

	case *runtime.Builtin:
		return fn.Fn(args...)

	default:
		return runtime.Errorf("not a function: %s (%s)", obj.Type(), obj.Inspect())
	}

}

func needBreak(v interface{}) bool {
	switch v.(type) {
	case *runtime.ReturnValue:
		return true
	case *runtime.Error:
		return true
	default:
		return v == runtime.Break || v == runtime.Continue
	}
}

func unwrap(obj runtime.Object) runtime.Object {
	if ret, ok := obj.(*runtime.ReturnValue); ok {
		return ret.Value
	}

	if obj == runtime.Break || obj == runtime.Continue {
		return runtime.Errorf("break or continue statements not allowed outside loops")
	}
	return obj
}

func evalEnumTypeDeclRange(expr syntax.Expr) ([]runtime.EnumRange, error) {

	enumKeyRanges := []runtime.EnumRange{}

	switch t := expr.(type) {
	case *syntax.Ident:
		break

	case *syntax.CallExpr:
		for _, callExprArg := range t.Args.List {

			argRet := runtime.EnumRange{}
			var argErr error

			switch argT := callExprArg.(type) {
			case *syntax.ValueLiteral:
				argRet.First, argErr = evalInt(argT)
				argRet.Last = argRet.First
			case *syntax.UnaryExpr:
				argRet.First, argErr = evalInt(argT)
				argRet.Last = argRet.First
			case *syntax.BinaryExpr:
				if argT.Op.Kind() != syntax.RANGE {
					argErr = fmt.Errorf("BinaryExpr type %s", argT.Op.Kind())
					break
				}
				argRet.First, argErr = evalInt(argT.X)
				if argErr != nil {
					argErr = fmt.Errorf("BinaryExpr l-arguments %v", argErr)
					break
				}
				argRet.Last, argErr = evalInt(argT.Y)
				if argErr != nil {
					argErr = fmt.Errorf("BinaryExpr r-arguments %v", argErr)
				}
			default:
				argErr = fmt.Errorf("enum element has unexpected argument type %T", argT)
			}

			if argErr != nil {
				return enumKeyRanges, fmt.Errorf("enum element has unexpected CallExpr argument, %v", argErr)
			} else {
				enumKeyRanges = append(enumKeyRanges, argRet)
			}
		}
	default:
		return enumKeyRanges, runtime.Errorf("enum has unexpected element %v", t)
	}
	return enumKeyRanges, nil
}

func evalEnumElements(enums []syntax.Expr) (runtime.EnumElements, error) {
	ret := make(runtime.EnumElements)

	validateNewEnumKeyRanges := func(ranges []runtime.EnumRange) error {
		for eName, eRanges := range ret {
			for _, eR := range eRanges {
				for _, r := range ranges {
					if eR.Contains(r.First) || eR.Contains(r.Last) {
						return fmt.Errorf("range(%s) colides with ranges in key %s", r.ToString(), eName)
					}
				}
			}
		}
		return nil
	}

	enumKeyId := 0
	for _, e := range enums {

		eName := syntax.Name(e)
		if eName == "" {
			return ret, fmt.Errorf("can't add key without a name")
		}
		_, hasThisKey := ret[eName]
		if hasThisKey {
			return ret, fmt.Errorf("can't add key %s, key with this name aleady exists", eName)
		}

		eRanges, eErr := evalEnumTypeDeclRange(e)
		if eErr != nil {
			return ret, fmt.Errorf("can't add key %s, %v", eName, eErr)
		}
		if len(eRanges) == 0 {
			eRanges = []runtime.EnumRange{{First: enumKeyId, Last: enumKeyId}}
		}
		eRangesErr := validateNewEnumKeyRanges(eRanges)
		if eRangesErr != nil {
			return ret, fmt.Errorf("can't add key %s, %v", eName, eRangesErr)
		}

		ret[eName] = eRanges
		enumKeyId = eRanges[len(eRanges)-1].Last + 1
	}
	if len(ret) == 0 {
		return ret, runtime.Errorf("this enum has no elements")
	}
	return ret, nil
}

func evalEnumSpec(n *syntax.EnumSpec) runtime.Object {
	ret := runtime.NewEnumType("")
	var err error = nil
	ret.Elements, err = evalEnumElements(n.Enums)
	if err != nil {
		return runtime.Errorf("%s", err.Error())
	}
	return ret
}

func evalEnumTypeDecl(n *syntax.EnumTypeDecl, env runtime.Scope) runtime.Object {

	name := syntax.Name(n)
	ret := runtime.NewEnumType(name)

	var err error = nil
	ret.Elements, err = evalEnumElements(n.Enums)
	if err != nil {
		return runtime.Errorf("%s", err.Error())
	}
	env.Set(n.Name.String(), ret)
	return ret
}

func evalEnumValDecl(d *syntax.Declarator, enumType *runtime.EnumType) runtime.Object {

	var node syntax.Node = d.Value
	switch n := node.(type) {

	case *syntax.CallExpr:

		enumElementName := syntax.Name(n)

		if len(n.Args.List) != 1 {
			return runtime.Errorf("invalid enum value")
		}
		arg0 := n.Args.List[0]
		enumElementValue, err := evalInt(arg0)
		if err != nil {
			return runtime.Errorf("invalid enum value")
		}

		enumVal, err := runtime.NewEnumValue(enumType, enumElementName, enumElementValue)
		if err == nil {
			return enumVal
		}

	case *syntax.Ident:
		enumElementName := syntax.Name(n)
		enumVal, err := runtime.NewEnumValueByKey(enumType, enumElementName)
		if err == nil {
			return enumVal
		}
	}
	return runtime.Errorf("invalid enum declarator")
}

func evalInt(n syntax.Node) (int, error) {
	switch t := n.(type) {
	case *syntax.ValueLiteral:
		if t.Tok.Kind() != syntax.INT {
			return 0, fmt.Errorf("ValueLiteral unexpected %s", t.Tok.Kind())
		}
		val, err := strconv.Atoi(t.Tok.String())
		if err != nil {
			return 0, fmt.Errorf("ValueLiteral '%s' is not int, %v", t.Tok.String(), err)
		}
		return val, nil
	case *syntax.UnaryExpr:
		val, err := evalInt(t.X)
		if err != nil {
			return 0, fmt.Errorf("UnaryExpr '%v', %v", t, err)
		}
		switch t.Op.Kind() {
		case syntax.ADD:
			break
		case syntax.SUB:
			val = -val
		default:
			return 0, fmt.Errorf("UnaryExpr unexpected token type %v", t.Op.Kind())
		}
		return val, nil
	default:
		return 0, fmt.Errorf("unexpected expresiton %t", t)
	}
}

func evalValueDecl(vd *syntax.ValueDecl, env runtime.Scope) runtime.Object {
	if vd.Type != nil {
		if valueType, ok := env.Get(syntax.Name(vd.Type)); ok {
			switch n := valueType.(type) {
			case *runtime.EnumType:
				for _, decl := range vd.Decls {
					result := evalEnumValDecl(decl, n)
					if runtime.IsError(result) {
						return result
					}
					env.Set(decl.Name.String(), result)
				}
				return n
			}
		}
	}

	var result runtime.Object
	for _, decl := range vd.Decls {
		result = eval(decl, env)
		if runtime.IsError(result) {
			return result
		}
	}
	return result
}

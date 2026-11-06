// Package lower turns TTCN-3 AST nodes (`ttcn3/syntax`) into IR
// modules (`ir`). The lowering is intentionally conservative: it only
// recognises the subset of TTCN-3 the backends can currently compile
// (integer / boolean / string scalars, top-level functions and
// testcases, `if`/`while` control flow, `setverdict`, log). Anything
// outside that subset is reported via a Diagnostic and the offending
// definition is skipped, so a single unsupported helper does not
// stop the rest of the module from compiling.
//
// The lowering shares the interpreter's understanding of TTCN-3 (so
// e.g. verdict identifiers resolve the same way), but produces a
// data-flow representation suitable for compilation rather than tree-
// walking. The Go and C++ backends both consume the output.
package lower

import (
	"fmt"
	"strings"

	"github.com/nokia/ntt/ir"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// Diagnostic is a single problem encountered during lowering. The
// caller decides whether to surface diagnostics as warnings or hard
// errors; the lowering itself never fails - it just skips the
// offending node and moves on.
type Diagnostic struct {
	Message string
	Path    string
}

func (d Diagnostic) Error() string { return d.Message }

// Module lowers every TTCN-3 module found in tree into a single IR
// module named after the first module declaration. Diagnostics
// describe definitions that were skipped.
func Module(tree *ttcn3.Tree) (*ir.Module, []Diagnostic) {
	if tree == nil || tree.Root == nil {
		return nil, nil
	}
	var (
		mod   *ir.Module
		diags []Diagnostic
	)
	for _, m := range tree.Modules() {
		modNode, ok := m.Node.(*syntax.Module)
		if !ok {
			continue
		}
		if mod == nil {
			mod = &ir.Module{Name: syntax.Name(modNode.Name)}
		}
		ds := lowerModule(modNode, mod)
		diags = append(diags, ds...)
	}
	return mod, diags
}

func lowerModule(mod *syntax.Module, out *ir.Module) []Diagnostic {
	var diags []Diagnostic
	for _, def := range mod.Defs {
		fn, ok := def.Def.(*syntax.FuncDecl)
		if !ok {
			diags = append(diags, Diagnostic{
				Message: fmt.Sprintf("lower: skipping unsupported top-level decl %T", def.Def),
			})
			continue
		}
		if fn.Body == nil {
			continue
		}
		irFn, d := lowerFunction(fn)
		if irFn != nil {
			out.AddFunction(irFn)
		}
		diags = append(diags, d...)
	}
	return diags
}

// lowerFunction turns one FuncDecl into an ir.Function. Both regular
// functions and testcases go through this path; testcases get a void
// return because their value is the verdict, which the runtime
// captures separately.
func lowerFunction(fn *syntax.FuncDecl) (*ir.Function, []Diagnostic) {
	ret := ir.TypeVoid
	if !fn.IsTest() && fn.Return != nil && fn.Return.Type != nil {
		ret = typeFromIdent(syntax.Name(fn.Return.Type))
	}
	f := &ir.Function{
		Name:       syntax.Name(fn.Name),
		Return:     ret,
		IsTestcase: fn.IsTest(),
	}

	b := newBuilder(f)
	if fn.Params != nil {
		for _, p := range fn.Params.List {
			v := f.NewValue(typeFromIdent(syntax.Name(p.Type)))
			f.Params = append(f.Params, v)
			b.bind(p.Name.String(), v)
		}
	}

	if _, diags := b.lowerBlock(fn.Body); len(diags) > 0 {
		return f, diags
	}
	// Ensure every function ends with a return so the backend's
	// straight-line emission produces valid code.
	if !b.lastIsReturn() {
		b.emitReturn(nil)
	}
	return f, nil
}

// builder is the per-function lowering helper. It tracks the current
// block (where the next instruction lands), the active scope of
// identifier -> SSA value bindings, and unique label generation.
type builder struct {
	fn       *ir.Function
	current  *ir.Block
	scopes   []map[string]*ir.Value
	labelSeq int
}

func newBuilder(fn *ir.Function) *builder {
	b := &builder{fn: fn}
	b.pushScope()
	b.current = fn.NewBlock("entry")
	return b
}

func (b *builder) pushScope() { b.scopes = append(b.scopes, map[string]*ir.Value{}) }
func (b *builder) popScope()  { b.scopes = b.scopes[:len(b.scopes)-1] }

func (b *builder) bind(name string, v *ir.Value) { b.scopes[len(b.scopes)-1][name] = v }

func (b *builder) lookup(name string) (*ir.Value, bool) {
	for i := len(b.scopes) - 1; i >= 0; i-- {
		if v, ok := b.scopes[i][name]; ok {
			return v, true
		}
	}
	return nil, false
}

func (b *builder) emit(in *ir.Instr) {
	b.current.Instrs = append(b.current.Instrs, in)
}

func (b *builder) lastIsReturn() bool {
	if b.current == nil || len(b.current.Instrs) == 0 {
		return false
	}
	last := b.current.Instrs[len(b.current.Instrs)-1]
	return last.Op == ir.OpReturn
}

func (b *builder) freshBlock(prefix string) *ir.Block {
	b.labelSeq++
	return b.fn.NewBlock(fmt.Sprintf("%s%d", prefix, b.labelSeq))
}

func (b *builder) setBlock(blk *ir.Block) { b.current = blk }

func (b *builder) emitReturn(v *ir.Value) {
	in := &ir.Instr{Op: ir.OpReturn}
	if v != nil {
		in.Operands = []*ir.Value{v}
	}
	b.emit(in)
}

// lowerBlock lowers every statement of a BlockStmt in order. The
// returned value is nil for void blocks; it's the last expression's
// value when the block ends with one.
func (b *builder) lowerBlock(blk *syntax.BlockStmt) (*ir.Value, []Diagnostic) {
	var (
		last  *ir.Value
		diags []Diagnostic
	)
	b.pushScope()
	defer b.popScope()
	for _, stmt := range blk.Stmts {
		v, d := b.lowerStmt(stmt)
		diags = append(diags, d...)
		last = v
	}
	return last, diags
}

// lowerStmt dispatches over the recognised TTCN-3 statement shapes.
// Anything not recognised becomes a Diagnostic and the body falls
// through silently - the backend will still emit a syntactically
// valid (but semantically incomplete) function.
func (b *builder) lowerStmt(stmt syntax.Stmt) (*ir.Value, []Diagnostic) {
	switch s := stmt.(type) {
	case *syntax.DeclStmt:
		return b.lowerDecl(s.Decl)
	case *syntax.ExprStmt:
		// `x := y` parses as a BinaryExpr with the ASSIGN operator;
		// route it through the dedicated assignment lowering so the
		// LHS gets rebound to the RHS value instead of trying to
		// evaluate `:=` as a regular binary operator.
		if be, ok := s.Expr.(*syntax.BinaryExpr); ok && be.Op.Kind() == syntax.ASSIGN {
			return b.lowerAssign(be)
		}
		return b.lowerExpr(s.Expr)
	case *syntax.ReturnStmt:
		var (
			v     *ir.Value
			diags []Diagnostic
		)
		if s.Result != nil {
			v, diags = b.lowerExpr(s.Result)
		}
		b.emitReturn(v)
		return nil, diags
	case *syntax.IfStmt:
		return b.lowerIf(s)
	case *syntax.WhileStmt:
		return b.lowerWhile(s)
	case *syntax.ForStmt:
		return b.lowerFor(s)
	case *syntax.BlockStmt:
		return b.lowerBlock(s)
	}
	return nil, []Diagnostic{{
		Message: fmt.Sprintf("lower: skipping unsupported statement %T", stmt),
	}}
}

// lowerAssign lowers `lhs := rhs`. The LHS must be an identifier the
// scope already knows about; the RHS is evaluated and then copied into
// the same SSA slot via OpCopy so reads of `lhs` after the assignment
// observe the new value. The backends declare each Value as a mutable
// local, so reusing the destination is safe.
func (b *builder) lowerAssign(be *syntax.BinaryExpr) (*ir.Value, []Diagnostic) {
	rhs, diags := b.lowerExpr(be.Y)
	if id, ok := be.X.(*syntax.Ident); ok {
		if dst, ok := b.lookup(id.String()); ok {
			b.emit(&ir.Instr{Op: ir.OpCopy, Dst: dst, Operands: []*ir.Value{rhs}})
			return dst, diags
		}
		// Auto-declare a new SSA slot on first assignment so the
		// rest of the function can reference it; this mirrors the
		// interpreter's permissive scope rules.
		dst := b.fn.NewValue(rhs.Type)
		b.emit(&ir.Instr{Op: ir.OpCopy, Dst: dst, Operands: []*ir.Value{rhs}})
		b.bind(id.String(), dst)
		return dst, diags
	}
	return rhs, append(diags, Diagnostic{Message: "lower: assignment LHS must be an identifier"})
}

func (b *builder) lowerDecl(d syntax.Decl) (*ir.Value, []Diagnostic) {
	vd, ok := d.(*syntax.ValueDecl)
	if !ok {
		return nil, []Diagnostic{{Message: fmt.Sprintf("lower: skipping decl %T", d)}}
	}
	var (
		last  *ir.Value
		diags []Diagnostic
		typ   = typeFromIdent(syntax.Name(vd.Type))
	)
	for _, dec := range vd.Decls {
		var (
			v *ir.Value
			d []Diagnostic
		)
		if dec.Value != nil {
			v, d = b.lowerExpr(dec.Value)
			diags = append(diags, d...)
		} else {
			v = b.fn.NewValue(typ)
			// emit a zero-valued constant so the backend has
			// something to point the SSA name at.
			b.emit(&ir.Instr{Op: zeroConstOp(typ), Dst: v, Aux: zeroAux(typ)})
		}
		if dec.Name != nil {
			b.bind(dec.Name.String(), v)
		}
		last = v
	}
	return last, diags
}

// lowerExpr lowers any TTCN-3 expression. The returned Value is the
// SSA name carrying its result. Unrecognised shapes return a
// freshly-allocated zero value and a diagnostic.
func (b *builder) lowerExpr(expr syntax.Expr) (*ir.Value, []Diagnostic) {
	switch e := expr.(type) {
	case *syntax.ValueLiteral:
		return b.lowerLiteral(e), nil
	case *syntax.Ident:
		return b.lowerIdent(e)
	case *syntax.BinaryExpr:
		return b.lowerBinary(e)
	case *syntax.UnaryExpr:
		return b.lowerUnary(e)
	case *syntax.CallExpr:
		return b.lowerCall(e)
	case *syntax.ParenExpr:
		if len(e.List) != 1 {
			return b.zero(ir.TypeInt), []Diagnostic{{Message: "lower: empty/multi paren expr"}}
		}
		return b.lowerExpr(e.List[0])
	}
	return b.zero(ir.TypeInt), []Diagnostic{{Message: fmt.Sprintf("lower: skipping expression %T", expr)}}
}

func (b *builder) lowerLiteral(lit *syntax.ValueLiteral) *ir.Value {
	switch lit.Tok.Kind() {
	case syntax.INT:
		v := b.fn.NewValue(ir.TypeInt)
		// Use strconv-equivalent at backend layer; here we just stash
		// the literal string and the Go backend re-parses it.
		b.emit(&ir.Instr{Op: ir.OpConstInt, Dst: v, Aux: parseInt(lit.Tok.String())})
		return v
	case syntax.TRUE:
		v := b.fn.NewValue(ir.TypeBool)
		b.emit(&ir.Instr{Op: ir.OpConstBool, Dst: v, Aux: true})
		return v
	case syntax.FALSE:
		v := b.fn.NewValue(ir.TypeBool)
		b.emit(&ir.Instr{Op: ir.OpConstBool, Dst: v, Aux: false})
		return v
	case syntax.STRING:
		v := b.fn.NewValue(ir.TypeString)
		s, _ := syntax.Unquote(lit.Tok.String())
		b.emit(&ir.Instr{Op: ir.OpConstString, Dst: v, Aux: s})
		return v
	case syntax.PASS, syntax.FAIL, syntax.INCONC, syntax.NONE, syntax.ERROR:
		v := b.fn.NewValue(ir.TypeString)
		b.emit(&ir.Instr{Op: ir.OpConstString, Dst: v, Aux: strings.ToLower(lit.Tok.String())})
		return v
	}
	return b.zero(ir.TypeInt)
}

func (b *builder) lowerIdent(id *syntax.Ident) (*ir.Value, []Diagnostic) {
	name := id.String()
	// `getverdict` is a TTCN-3 predefined that can be referenced
	// either as a no-arg call or as a bare identifier.
	if name == "getverdict" {
		dst := b.fn.NewValue(ir.TypeVerdict)
		b.emit(&ir.Instr{Op: ir.OpGetVerdict, Dst: dst})
		return dst, nil
	}
	if v, ok := b.lookup(name); ok {
		return v, nil
	}
	// Predefined verdict identifiers compile down to const strings;
	// the backend's setverdict op turns them into runtime.Verdict.
	switch strings.ToLower(name) {
	case "pass", "fail", "inconc", "none", "error":
		v := b.fn.NewValue(ir.TypeString)
		b.emit(&ir.Instr{Op: ir.OpConstString, Dst: v, Aux: strings.ToLower(name)})
		return v, nil
	}
	return b.zero(ir.TypeInt), []Diagnostic{{Message: "lower: unknown identifier " + name}}
}

func (b *builder) lowerBinary(e *syntax.BinaryExpr) (*ir.Value, []Diagnostic) {
	lhs, d1 := b.lowerExpr(e.X)
	rhs, d2 := b.lowerExpr(e.Y)
	diags := append(d1, d2...)

	op, ok := binaryOp(e.Op.Kind())
	if !ok {
		return b.zero(lhs.Type), append(diags, Diagnostic{Message: "lower: unsupported binary op " + e.Op.Kind().String()})
	}
	dst := b.fn.NewValue(resultType(op, lhs.Type))
	b.emit(&ir.Instr{Op: op, Dst: dst, Operands: []*ir.Value{lhs, rhs}})
	return dst, diags
}

func (b *builder) lowerUnary(e *syntax.UnaryExpr) (*ir.Value, []Diagnostic) {
	val, diags := b.lowerExpr(e.X)
	switch e.Op.Kind() {
	case syntax.NOT:
		dst := b.fn.NewValue(ir.TypeBool)
		b.emit(&ir.Instr{Op: ir.OpNot, Dst: dst, Operands: []*ir.Value{val}})
		return dst, diags
	case syntax.SUB:
		// -x  ->  0 - x
		zero := b.zero(val.Type)
		dst := b.fn.NewValue(val.Type)
		b.emit(&ir.Instr{Op: ir.OpSub, Dst: dst, Operands: []*ir.Value{zero, val}})
		return dst, diags
	case syntax.ADD:
		return val, diags
	}
	return val, append(diags, Diagnostic{Message: "lower: unsupported unary op " + e.Op.Kind().String()})
}

func (b *builder) lowerCall(e *syntax.CallExpr) (*ir.Value, []Diagnostic) {
	ident, ok := e.Fun.(*syntax.Ident)
	if !ok {
		return b.zero(ir.TypeInt), []Diagnostic{{Message: "lower: callable must be an Ident in this pass"}}
	}
	name := ident.String()

	// setverdict gets its own opcode so the backend can dispatch to
	// runtime/report directly without going through a builtin lookup.
	if name == "setverdict" {
		if len(e.Args.List) == 0 {
			return nil, []Diagnostic{{Message: "lower: setverdict requires an argument"}}
		}
		arg := e.Args.List[0]
		var aux string
		if id, ok := arg.(*syntax.Ident); ok {
			aux = strings.ToLower(id.String())
		} else if lit, ok := arg.(*syntax.ValueLiteral); ok {
			aux = strings.ToLower(lit.Tok.String())
		}
		// Optional reason arguments are lowered as Operands; the
		// backend joins them with a space when it writes the
		// TestcaseExec call.
		var (
			operands []*ir.Value
			diags    []Diagnostic
		)
		for _, r := range e.Args.List[1:] {
			v, d := b.lowerExpr(r)
			operands = append(operands, v)
			diags = append(diags, d...)
		}
		b.emit(&ir.Instr{Op: ir.OpSetVerdict, Aux: aux, Operands: operands})
		return nil, diags
	}

	// log(...) emits its arguments to the TestcaseExec log buffer.
	if name == "log" {
		var (
			operands []*ir.Value
			diags    []Diagnostic
		)
		for _, r := range e.Args.List {
			v, d := b.lowerExpr(r)
			operands = append(operands, v)
			diags = append(diags, d...)
		}
		b.emit(&ir.Instr{Op: ir.OpLog, Operands: operands})
		return nil, diags
	}

	// getverdict reads back the current verdict from TestcaseExec.
	if name == "getverdict" {
		dst := b.fn.NewValue(ir.TypeVerdict)
		b.emit(&ir.Instr{Op: ir.OpGetVerdict, Dst: dst})
		return dst, nil
	}

	// lengthof / sizeof are unary builtins that map directly to an
	// IR opcode so the backends can emit a single runtime call.
	if name == "lengthof" || name == "sizeof" {
		if len(e.Args.List) != 1 {
			return b.zero(ir.TypeInt), []Diagnostic{{Message: "lower: " + name + " takes exactly one argument"}}
		}
		val, d := b.lowerExpr(e.Args.List[0])
		dst := b.fn.NewValue(ir.TypeInt)
		b.emit(&ir.Instr{Op: ir.OpLengthOf, Dst: dst, Operands: []*ir.Value{val}})
		return dst, d
	}

	args := make([]*ir.Value, 0, len(e.Args.List))
	var diags []Diagnostic
	for _, a := range e.Args.List {
		v, d := b.lowerExpr(a)
		args = append(args, v)
		diags = append(diags, d...)
	}
	dst := b.fn.NewValue(ir.TypeInt) // best-guess return type for now
	b.emit(&ir.Instr{Op: ir.OpCall, Dst: dst, Operands: args, Aux: name})
	return dst, diags
}

func (b *builder) lowerIf(s *syntax.IfStmt) (*ir.Value, []Diagnostic) {
	cond, diags := b.lowerExpr(s.Cond)
	thenBlk := b.freshBlock("then")
	elseBlk := b.freshBlock("else")
	endBlk := b.freshBlock("endif")

	b.emit(&ir.Instr{Op: ir.OpCondBranch, Operands: []*ir.Value{cond}, Targets: []*ir.Block{thenBlk, elseBlk}})

	b.setBlock(thenBlk)
	_, d := b.lowerStmt(s.Then)
	diags = append(diags, d...)
	if !b.lastIsReturn() {
		b.emit(&ir.Instr{Op: ir.OpBranch, Targets: []*ir.Block{endBlk}})
	}

	b.setBlock(elseBlk)
	if s.Else != nil {
		_, d := b.lowerStmt(s.Else)
		diags = append(diags, d...)
	}
	if !b.lastIsReturn() {
		b.emit(&ir.Instr{Op: ir.OpBranch, Targets: []*ir.Block{endBlk}})
	}

	b.setBlock(endBlk)
	return nil, diags
}

// lowerFor desugars a C-style `for (init; cond; post) body` into the
// same head/body/post/end block shape `lowerWhile` uses, with the
// init running once before the loop and the post statement running at
// the bottom of every body iteration.
func (b *builder) lowerFor(s *syntax.ForStmt) (*ir.Value, []Diagnostic) {
	var diags []Diagnostic
	if s.Init != nil {
		_, d := b.lowerStmt(s.Init)
		diags = append(diags, d...)
	}
	head := b.freshBlock("forhead")
	body := b.freshBlock("forbody")
	end := b.freshBlock("forend")

	b.emit(&ir.Instr{Op: ir.OpBranch, Targets: []*ir.Block{head}})

	b.setBlock(head)
	if s.Cond != nil {
		cond, d := b.lowerExpr(s.Cond)
		diags = append(diags, d...)
		b.emit(&ir.Instr{Op: ir.OpCondBranch, Operands: []*ir.Value{cond}, Targets: []*ir.Block{body, end}})
	} else {
		b.emit(&ir.Instr{Op: ir.OpBranch, Targets: []*ir.Block{body}})
	}

	b.setBlock(body)
	_, d := b.lowerStmt(s.Body)
	diags = append(diags, d...)
	if s.Post != nil {
		_, d := b.lowerStmt(s.Post)
		diags = append(diags, d...)
	}
	if !b.lastIsReturn() {
		b.emit(&ir.Instr{Op: ir.OpBranch, Targets: []*ir.Block{head}})
	}

	b.setBlock(end)
	return nil, diags
}

func (b *builder) lowerWhile(s *syntax.WhileStmt) (*ir.Value, []Diagnostic) {
	head := b.freshBlock("loophead")
	body := b.freshBlock("loopbody")
	end := b.freshBlock("loopend")

	b.emit(&ir.Instr{Op: ir.OpBranch, Targets: []*ir.Block{head}})

	b.setBlock(head)
	cond, diags := b.lowerExpr(s.Cond)
	b.emit(&ir.Instr{Op: ir.OpCondBranch, Operands: []*ir.Value{cond}, Targets: []*ir.Block{body, end}})

	b.setBlock(body)
	_, d := b.lowerStmt(s.Body)
	diags = append(diags, d...)
	if !b.lastIsReturn() {
		b.emit(&ir.Instr{Op: ir.OpBranch, Targets: []*ir.Block{head}})
	}

	b.setBlock(end)
	return nil, diags
}

// --- helpers ---

func (b *builder) zero(t ir.Type) *ir.Value {
	v := b.fn.NewValue(t)
	op := zeroConstOp(t)
	b.emit(&ir.Instr{Op: op, Dst: v, Aux: zeroAux(t)})
	return v
}

func zeroConstOp(t ir.Type) ir.Op {
	switch t {
	case ir.TypeInt:
		return ir.OpConstInt
	case ir.TypeBool:
		return ir.OpConstBool
	case ir.TypeString:
		return ir.OpConstString
	}
	return ir.OpConstInt
}

func zeroAux(t ir.Type) interface{} {
	switch t {
	case ir.TypeInt:
		return int64(0)
	case ir.TypeBool:
		return false
	case ir.TypeString:
		return ""
	}
	return int64(0)
}

func typeFromIdent(s string) ir.Type {
	switch strings.ToLower(s) {
	case "integer", "int":
		return ir.TypeInt
	case "boolean", "bool":
		return ir.TypeBool
	case "charstring", "universal charstring", "string":
		return ir.TypeString
	case "":
		return ir.TypeVoid
	}
	return ir.TypeInt
}

func binaryOp(k syntax.Kind) (ir.Op, bool) {
	switch k {
	case syntax.ADD:
		return ir.OpAdd, true
	case syntax.SUB:
		return ir.OpSub, true
	case syntax.MUL:
		return ir.OpMul, true
	case syntax.DIV:
		return ir.OpDiv, true
	case syntax.MOD:
		return ir.OpMod, true
	case syntax.REM:
		return ir.OpRem, true
	case syntax.CONCAT:
		return ir.OpConcat, true
	case syntax.EQ:
		return ir.OpEq, true
	case syntax.NE:
		return ir.OpNe, true
	case syntax.LT:
		return ir.OpLt, true
	case syntax.LE:
		return ir.OpLe, true
	case syntax.GT:
		return ir.OpGt, true
	case syntax.GE:
		return ir.OpGe, true
	case syntax.AND:
		return ir.OpAnd, true
	case syntax.OR:
		return ir.OpOr, true
	}
	return ir.OpNop, false
}

func resultType(op ir.Op, operandType ir.Type) ir.Type {
	switch op {
	case ir.OpEq, ir.OpNe, ir.OpLt, ir.OpLe, ir.OpGt, ir.OpGe, ir.OpAnd, ir.OpOr:
		return ir.TypeBool
	}
	return operandType
}

func parseInt(s string) int64 {
	// TTCN-3 integer literals are decimal by spec; strip optional `_`
	// grouping (rare in tests). We accept what strconv.ParseInt would.
	var n int64
	for _, r := range s {
		if r == '_' {
			continue
		}
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int64(r-'0')
	}
	return n
}

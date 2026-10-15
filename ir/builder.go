package ir

// Builder is a small fluent helper for constructing IR functions
// programmatically. Codegen passes can use it without going through
// the lowering pass for the cases where the IR is computed directly.
type Builder struct {
	fn      *Function
	current *Block
}

// NewBuilder starts emitting into fn's first block. If fn has no
// blocks yet, one named "entry" is created.
func NewBuilder(fn *Function) *Builder {
	if len(fn.Blocks) == 0 {
		fn.NewBlock("entry")
	}
	return &Builder{fn: fn, current: fn.Blocks[0]}
}

// SetBlock moves the cursor.
func (b *Builder) SetBlock(blk *Block) { b.current = blk }

// Current returns the block the next Emit will append to.
func (b *Builder) Current() *Block { return b.current }

// Function returns the function being built.
func (b *Builder) Function() *Function { return b.fn }

// Emit appends an instruction to the current block. dst is nil for
// void instructions; op and operands are the SSA form.
func (b *Builder) Emit(op Op, dst *Value, operands ...*Value) *Instr {
	in := &Instr{Op: op, Dst: dst, Operands: operands}
	b.current.Instrs = append(b.current.Instrs, in)
	return in
}

// EmitAux is Emit + an Aux payload.
func (b *Builder) EmitAux(op Op, dst *Value, aux interface{}, operands ...*Value) *Instr {
	in := b.Emit(op, dst, operands...)
	in.Aux = aux
	return in
}

// ConstInt loads a literal integer.
func (b *Builder) ConstInt(n int64) *Value {
	v := b.fn.NewValue(TypeInt)
	b.EmitAux(OpConstInt, v, n)
	return v
}

// ConstBool loads a literal boolean.
func (b *Builder) ConstBool(x bool) *Value {
	v := b.fn.NewValue(TypeBool)
	b.EmitAux(OpConstBool, v, x)
	return v
}

// ConstString loads a literal string.
func (b *Builder) ConstString(s string) *Value {
	v := b.fn.NewValue(TypeString)
	b.EmitAux(OpConstString, v, s)
	return v
}

// Add emits `dst = lhs + rhs`.
func (b *Builder) Add(lhs, rhs *Value) *Value {
	v := b.fn.NewValue(lhs.Type)
	b.Emit(OpAdd, v, lhs, rhs)
	return v
}

// Sub emits `dst = lhs - rhs`.
func (b *Builder) Sub(lhs, rhs *Value) *Value {
	v := b.fn.NewValue(lhs.Type)
	b.Emit(OpSub, v, lhs, rhs)
	return v
}

// Eq emits `dst = lhs == rhs`.
func (b *Builder) Eq(lhs, rhs *Value) *Value {
	v := b.fn.NewValue(TypeBool)
	b.Emit(OpEq, v, lhs, rhs)
	return v
}

// Return terminates the block with `return v` (v may be nil for void).
func (b *Builder) Return(v *Value) {
	if v == nil {
		b.Emit(OpReturn, nil)
		return
	}
	b.Emit(OpReturn, nil, v)
}

// Branch emits an unconditional jump.
func (b *Builder) Branch(target *Block) {
	in := b.Emit(OpBranch, nil)
	in.Targets = []*Block{target}
}

// CondBranch emits a conditional jump: if cond then thn else els.
func (b *Builder) CondBranch(cond *Value, thn, els *Block) {
	in := b.Emit(OpCondBranch, nil, cond)
	in.Targets = []*Block{thn, els}
}

// Call emits a function call with the given name and operands.
func (b *Builder) Call(name string, retType Type, operands ...*Value) *Value {
	var dst *Value
	if retType != TypeVoid {
		dst = b.fn.NewValue(retType)
	}
	b.EmitAux(OpCall, dst, name, operands...)
	return dst
}

// SetVerdict emits a setverdict op carrying the verdict name as aux.
func (b *Builder) SetVerdict(name string) {
	b.EmitAux(OpSetVerdict, nil, name)
}

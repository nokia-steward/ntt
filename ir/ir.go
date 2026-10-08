// Package ir defines a small, SSA-ish intermediate representation for
// TTCN-3 code. The IR sits between the schema-driven AST (`ttcn3/v2/
// syntax/nodes`) and the codegen backends in `backend/*`. Two goals
// drive the design:
//
//  1. Stay close to TTCN-3 semantics so backends can target the
//     `runtime/*` packages directly without a tower of abstractions:
//     templates, components, ports, timers and codecs are first-class
//     IR types, not opaque calls.
//
//  2. Stay easy to reason about so the Go backend (M7) and the C++
//     backend (M8) can share most of the lowering pass. The IR uses
//     SSA-style basic blocks with phi nodes; every value has a Type
//     and an owning Function.
//
// The current scope is intentionally a subset of TTCN-3: integer /
// boolean / string scalars, plus function declarations with
// `if`/`while` control flow. Records, ports, alts and so on are sketched
// in `Kind` so the surface stays stable while the lowering passes grow.
package ir

import (
	"fmt"
	"strings"
)

// Module is the top-level container. One TTCN-3 module lowers to one
// IR Module; cross-module references are by name and resolved at link
// time by the codegen backends.
type Module struct {
	Name      string
	Functions []*Function
}

// AddFunction appends fn to the module, returning a pointer for
// fluent construction.
func (m *Module) AddFunction(fn *Function) *Function {
	m.Functions = append(m.Functions, fn)
	return fn
}

// Function is one TTCN-3 function / testcase / altstep, with a list
// of parameters, a return type (Type{} when void), and a CFG of
// basic blocks. IsTestcase distinguishes testcases from regular
// functions so the backends can emit a runnable driver.
type Function struct {
	Name       string
	Params     []*Value
	Return     Type
	Blocks     []*Block
	IsTestcase bool
	nextID     int
}

// Block is a straight-line sequence of Instructions terminated by a
// branch / return.
type Block struct {
	Label    string
	Instrs   []*Instr
}

// NewBlock appends a fresh block to fn and returns it.
func (fn *Function) NewBlock(label string) *Block {
	b := &Block{Label: label}
	fn.Blocks = append(fn.Blocks, b)
	return b
}

// NewValue allocates an SSA value with a unique ID and the given Type.
// All values flow through the function so the codegen pass can emit
// stable names.
func (fn *Function) NewValue(typ Type) *Value {
	fn.nextID++
	return &Value{ID: fn.nextID, Type: typ, Function: fn}
}

// Type enumerates the IR-level types. They map 1:1 to TTCN-3 base
// types so backends can drop straight into runtime/object.
type Type int

const (
	TypeVoid Type = iota
	TypeInt
	TypeFloat
	TypeBool
	TypeString
	TypeBytes
	TypeRecord
	TypeList
	TypePort
	TypeTimer
	TypeComponent
	TypeVerdict
)

// String renders the type for debugging.
func (t Type) String() string {
	switch t {
	case TypeVoid:
		return "void"
	case TypeInt:
		return "int"
	case TypeFloat:
		return "float"
	case TypeBool:
		return "bool"
	case TypeString:
		return "string"
	case TypeBytes:
		return "bytes"
	case TypeRecord:
		return "record"
	case TypeList:
		return "list"
	case TypePort:
		return "port"
	case TypeTimer:
		return "timer"
	case TypeComponent:
		return "component"
	case TypeVerdict:
		return "verdict"
	}
	return "?"
}

// Value is an SSA value: an integer ID, a type, the function it
// belongs to.
type Value struct {
	ID       int
	Type     Type
	Function *Function
}

// String returns "%N" notation suitable for debug dumps.
func (v *Value) String() string {
	if v == nil {
		return "_"
	}
	return fmt.Sprintf("%%%d", v.ID)
}

// Op enumerates IR opcodes.
type Op int

const (
	OpNop Op = iota
	OpConstInt
	OpConstFloat
	OpConstBool
	OpConstString
	OpAdd
	OpSub
	OpMul
	OpDiv
	OpMod
	OpRem
	OpEq
	OpNe
	OpLt
	OpLe
	OpGt
	OpGe
	OpAnd
	OpOr
	OpNot
	OpNeg
	OpConcat
	OpLengthOf
	OpCopy
	OpCall
	OpReturn
	OpBranch
	OpCondBranch
	OpSetVerdict
	OpGetVerdict
	OpLog
	OpPortSend
	OpPortReceive
	OpTimerStart
	OpTimerTimeout
)

// String renders the op in lower-snake form.
func (o Op) String() string {
	switch o {
	case OpNop:
		return "nop"
	case OpConstInt:
		return "const.int"
	case OpConstFloat:
		return "const.float"
	case OpConstBool:
		return "const.bool"
	case OpConstString:
		return "const.string"
	case OpAdd:
		return "add"
	case OpSub:
		return "sub"
	case OpMul:
		return "mul"
	case OpDiv:
		return "div"
	case OpMod:
		return "mod"
	case OpRem:
		return "rem"
	case OpEq:
		return "eq"
	case OpNe:
		return "ne"
	case OpLt:
		return "lt"
	case OpLe:
		return "le"
	case OpGt:
		return "gt"
	case OpGe:
		return "ge"
	case OpAnd:
		return "and"
	case OpOr:
		return "or"
	case OpNot:
		return "not"
	case OpNeg:
		return "neg"
	case OpConcat:
		return "concat"
	case OpLengthOf:
		return "lengthof"
	case OpCopy:
		return "copy"
	case OpCall:
		return "call"
	case OpReturn:
		return "return"
	case OpBranch:
		return "br"
	case OpCondBranch:
		return "cond_br"
	case OpSetVerdict:
		return "setverdict"
	case OpGetVerdict:
		return "getverdict"
	case OpLog:
		return "log"
	case OpPortSend:
		return "port.send"
	case OpPortReceive:
		return "port.receive"
	case OpTimerStart:
		return "timer.start"
	case OpTimerTimeout:
		return "timer.timeout"
	}
	return "?"
}

// Instr is one IR instruction. Operands point to other Values; Dst is
// the SSA result (may be nil for void instructions like Return).
type Instr struct {
	Op       Op
	Dst      *Value
	Operands []*Value
	Targets  []*Block // for branches
	Aux      interface{} // op-specific extra data (int constants, function names, ...)
}

// Dump returns a human-readable text rendering of m. The output is
// stable so test fixtures can diff against it.
func Dump(m *Module) string {
	var b strings.Builder
	fmt.Fprintf(&b, "module %s\n", m.Name)
	for _, fn := range m.Functions {
		fmt.Fprintf(&b, "func %s(", fn.Name)
		for i, p := range fn.Params {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s: %s", p, p.Type)
		}
		fmt.Fprintf(&b, ") -> %s\n", fn.Return)
		for _, blk := range fn.Blocks {
			fmt.Fprintf(&b, "  %s:\n", blk.Label)
			for _, in := range blk.Instrs {
				b.WriteString("    ")
				if in.Dst != nil {
					fmt.Fprintf(&b, "%s = ", in.Dst)
				}
				b.WriteString(in.Op.String())
				for _, op := range in.Operands {
					fmt.Fprintf(&b, " %s", op)
				}
				for _, tgt := range in.Targets {
					fmt.Fprintf(&b, " @%s", tgt.Label)
				}
				if in.Aux != nil {
					fmt.Fprintf(&b, " <%v>", in.Aux)
				}
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}

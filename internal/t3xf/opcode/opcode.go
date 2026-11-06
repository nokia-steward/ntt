// Package opcode defines the opcodes used in T3XF.
package opcode

//go:generate go run gen.go

import "fmt"

type Opcode int

func (op Opcode) String() string {
	if s, ok := opcodeStrings[op]; ok {
		return s
	}
	return fmt.Sprintf("unknown_opcode(0x%x)", int(op))
}

func (op Opcode) Arg() Arg {
	switch op {
	case REF, GOTO, FROZEN_REF, CALL, ISCAN:
		return ArgRef
	case LINE:
		return ArgLine
	}
	if d, ok := Descriptions[op]; ok {
		return d.Arg
	}
	return ArgNone
}

func Unpack(x uint32) (Opcode, int) {
	if Opcode(x)&instrMask == instrMask {
		return Opcode(x & 0xffff), int(x >> 16)
	}

	op := Opcode(x & 0b11_0000000000000000000000000000_11)
	arg := int(x & 0b00_1111111111111111111111111111_00)

	if op == LINE {
		arg >>= 2
	}

	return op, arg
}

func Pack(op Opcode, arg int) uint32 {
	if op&instrMask == instrMask {
		return uint32(arg)<<16 | (uint32(op) & 0xffff)
	}

	if op == LINE {
		arg <<= 2
	}

	return uint32(op) | uint32(arg)&0b00_1111111111111111111111111111_00
}

func Parse(s string) (Opcode, error) {
	if op, ok := opcodeNames[s]; ok {
		return op, nil
	}
	return -1, fmt.Errorf("unknown opcode %q", s)
}

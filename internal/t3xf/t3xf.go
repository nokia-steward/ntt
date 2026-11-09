package t3xf

import (
	"errors"
	"github.com/nokia/ntt/internal/t3xf/opcode"
)

// Reference is a T3XF offset.
type Reference int

// String is a T3XF string.
type String struct {
	l int
	b []byte
}

func NewString(l int, b []byte) *String {
	return &String{l, b}
}

// Len returns the number of elements in the string. For a bitstring, this is
// the number of bits, for a charstring the number of characters.
func (s *String) Len() int {
	return s.l
}

// Bytes returns the byte slice for the string.
func (s *String) Bytes() []byte {
	return s.b
}

type Instruction struct {
	Op      opcode.Opcode
	Arg     any
	address int
	bytes   []byte
}

func Assemble(instrs []Instruction) ([]byte, error) {
	e := NewEncoder()
	for _, instr := range instrs {
		if err := e.Encode(instr.Op, instr.Arg); err != nil {
			return nil, err
		}
	}
	return e.Assemble()
}

func Disassemble(b []byte) ([]Instruction, error) {
	instrs := make([]Instruction, 0, len(b)/4)
	pc := 0
	for pc < len(b) {
		n, op, arg := Decode(b[pc:])
		instrs = append(instrs, Instruction{
			address: pc,
			bytes:   b[pc : pc+n],
			Op:      op,
			Arg:     arg,
		})

		if n == 0 {
			return instrs, errors.New("decoding error")
		}

		pc += n
	}
	return instrs, nil
}

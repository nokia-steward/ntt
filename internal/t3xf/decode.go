package t3xf

import (
	"encoding/binary"
	"math"

	"github.com/nokia/ntt/internal/t3xf/opcode"
)

// Decode decodes a T3XF instruction from a byte slice. It returns the number of
// bytes read, the opcode and its argument.
//
// The type of the argument depends on the opcode:
//   - LINE, NATLONG, SCAN, IFIELD, IGET and IDEF: the argument is an int.
//   - IEEE754DP: the argument is a float64.
//   - BITS, NIBBLES and OCTETS: the argument
//     is a *t3xf.String. The underlying byte slice is the given byte slice b.
//     Note, that the number of bytes read and the number of bytes in the String
//     slice may differ, because T3XF instructions are 4-byte aligned.
//   - NAME, UTF8, ISTR and FSTR: the argument is a string.
//   - REF, GOTO and FROZEN_REF: the argument is a t3xf.Reference.
func Decode(b []byte) (n int, op opcode.Opcode, arg interface{}) {

	op, i := opcode.Unpack(binary.LittleEndian.Uint32(b))

	switch op.Arg() {
	case opcode.ArgInt:
		arg = int(int32(binary.LittleEndian.Uint32(b[4:])))
		n = 4 + 4

	case opcode.ArgFloat:
		arg = math.Float64frombits(binary.LittleEndian.Uint64(b[4:]))
		n = 4 + 8

	case opcode.ArgString:
		l := int(binary.LittleEndian.Uint32(b[4:]))
		arg = string(b[8 : 8+l])
		n = 4 + 4 + ((l+3)/4)*4

	case opcode.ArgOctets:
		l := int(binary.LittleEndian.Uint32(b[4:]))
		arg = &String{l, b[8 : 8+l]}
		n = 4 + 4 + ((l+3)/4)*4

	case opcode.ArgNibbles:
		nNibbles := int(binary.LittleEndian.Uint32(b[4:]))
		l := (nNibbles + 1) / 2
		arg = &String{nNibbles, b[8 : 8+l]}
		n = 4 + 4 + ((l+3)/4)*4

	case opcode.ArgBits:
		nBits := int(binary.LittleEndian.Uint32(b[4:]))
		l := (nBits + 7) / 8
		arg = &String{nBits, b[8 : 8+l]}
		n = 4 + 4 + ((l+3)/4)*4

	case opcode.ArgRef:
		arg = Reference(i)
		n = 4

	case opcode.ArgLine, opcode.ArgImm:
		arg = int(i)
		n = 4

	default:
		if i != 0 {
			arg = i
		}
		n = 4
	}
	return
}

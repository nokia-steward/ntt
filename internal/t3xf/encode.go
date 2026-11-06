package t3xf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/nokia/ntt/internal/t3xf/opcode"
)

// ErrInvalidArg is returned when an argument is not of the expected type or
// size.
var ErrInvalidArg = errors.New("invalid argument")

// ErrUnknownReference is returned when a reference to an unknown instruction
// is encountered.
var ErrUnknownReference = errors.New("unknown reference")

type EncoderError struct {
	Err error         // Underlying error
	I   int           // Instruction index
	Op  opcode.Opcode // Current Opcode
}

func (e *EncoderError) Error() string {
	return fmt.Sprintf("t3xf.Encode: [0x%04x] %s: %s", e.I*4, e.Op.String(), e.Err.Error())
}

func (e *EncoderError) Unwrap() error {
	return e.Err
}

type Encoder struct {
	c       int    // Instruction count
	i       int    // Instruction index (== offset/4)
	b       []byte // T3XF byte code
	o       int
	op      opcode.Opcode // Current opcode
	offsets map[int]int   // Map instruction index to T3XF byte offset
	patches []patch       // List of unresolved references
}

func NewEncoder() *Encoder {
	return &Encoder{
		offsets: make(map[int]int),
	}
}

// patch represents a reference to an instruction that has not yet been
type patch struct {
	ref    int // Instruction index that needs to be resolved
	i      int // Instruction index of the instruction that needs to be patched
	offset int // Byte offset of the instruction that needs to be patched
	op     opcode.Opcode
}

func (e *Encoder) Len() int {
	return len(e.b)
}

// Reset resets the encoder to its initial state.
func (e *Encoder) Reset() {
	e.c = 0
	e.i = 0
	e.i = 0
	e.op = 0
	e.b = nil
	e.offsets = make(map[int]int)
	e.patches = nil
}

// Assemble returns the assembled T3XF byte code. References are resolved when
// possible, otherwise they are left unresolved and an error is returned.
//
// It is safe to call Assemble multiple times during the lifetime of an
// Encoder.
func (e *Encoder) Assemble() ([]byte, error) {
	var (
		err     error
		patches []patch
	)

	for _, p := range e.patches {
		e.i = p.i
		e.op = p.op

		if ref, ok := e.offsets[p.ref]; ok {
			binary.LittleEndian.PutUint32(e.b[p.offset:], opcode.Pack(p.op, ref))
		} else {
			patches = append(patches, p)
			err = errors.Join(err, e.errorf("%w: 0x04%x", ErrUnknownReference, p.ref*4))
		}
	}
	e.patches = patches
	return e.b, err
}

func (e *Encoder) Patch(offset int, op opcode.Opcode, arg any) error {
	binary.LittleEndian.PutUint32(e.b[offset:], opcode.Pack(op, int(arg.(Reference))))
	return nil
}

// Encode the given opcode and argument. If an argument is not of the expected
// type or size, an error is returned.
//
//   - References are given as the index of the Encode call that will be
//     referenced.
//   - String arguments are expected to implement the Len() and Bytes() methods.
//   - Integer arguments are expected to be of type int.
//   - Float arguments are expected to be of type float64.
func (e *Encoder) Encode(op opcode.Opcode, arg any) error {
	e.i = e.c
	e.op = op
	e.offsets[e.i] = len(e.b)

	switch op.Arg() {
	case opcode.ArgRef:
		return e.encodeRef(op, arg)
	case opcode.ArgFloat:
		return e.encodeFloat(op, arg)
	case opcode.ArgInt:
		return e.encodeInt(op, arg)
	case opcode.ArgBits, opcode.ArgNibbles, opcode.ArgOctets:
		return e.encodeBinaryString(op, arg)
	case opcode.ArgString:
		return e.encodeString(op, arg)
	case opcode.ArgLine:
		return e.encodeInstruction(op, arg, 0x0fffffff)
	default:
		return e.encodeInstruction(op, arg, 0xffff)
	}
}

func (e *Encoder) encodeRef(op opcode.Opcode, arg any) error {
	if ref, ok := arg.(Reference); ok {
		if ref < 0 || ref > math.MaxInt32 {
			return e.errorf("%w: argument too large: 0x%04x", ErrInvalidArg, ref)
		}
		e.appendUint32(opcode.Pack(op, int(ref)))
		return nil
	}

	refI, ok := arg.(int)
	if !ok {
		return e.errorf("%w: integer argument expected: %v", ErrInvalidArg, arg)
	}

	offs, ok := e.offsets[refI]
	if ok {
		e.appendUint32(opcode.Pack(op, offs))
	} else {
		if ref := refI * 4; ref < 0 || ref > math.MaxInt32 {
			return e.errorf("%w: argument too large: 0x%04x", ErrInvalidArg, ref)
		}
		e.patches = append(e.patches, patch{i: e.i, op: op, offset: len(e.b), ref: refI})
		e.appendUint32(opcode.Pack(op, 0xfffffffc))
	}
	e.c++
	return nil
}

func (e *Encoder) encodeFloat(op opcode.Opcode, arg any) error {
	if floatArg, ok := arg.(float64); ok {
		e.appendUint32(opcode.Pack(op, 0))
		e.appendUint64(math.Float64bits(floatArg))
		e.c++
		return nil
	}
	return e.errorf("%w: float64 argument expected: %v", ErrInvalidArg, arg)
}

func (e *Encoder) encodeInt(op opcode.Opcode, arg any) error {
	intArg, ok := arg.(int)
	if !ok {
		return e.errorf("%w: integer argument expected: %v", ErrInvalidArg, arg)
	}
	if intArg < math.MinInt32 || intArg > math.MaxInt32 {
		return e.errorf("%w: argument too large: %v", ErrInvalidArg, intArg)
	}
	e.appendUint32(opcode.Pack(op, 0))
	e.appendUint32(uint32(intArg))
	e.c++
	return nil
}

func (e *Encoder) encodeString(op opcode.Opcode, arg any) error {
	s, ok := arg.(string)
	if !ok {
		return e.errorf("%w: string argument expected: %v", ErrInvalidArg, arg)
	}
	length := len(s)
	if length < 0 || length > math.MaxUint32 {
		return e.errorf("%w: too long to encode: %v", ErrInvalidArg, length)
	}
	e.appendUint32(opcode.Pack(op, 0))
	e.appendUint32(uint32(length))
	e.b = append(e.b, []byte(s)...)
	if padding := 4 - (len(s) % 4); padding != 4 {
		for i := 0; i < padding; i++ {
			e.b = append(e.b, 0)
		}
	}
	e.c++
	return nil
}
func (e *Encoder) encodeBinaryString(op opcode.Opcode, arg any) error {
	s, ok := arg.(*String)
	if !ok {
		return e.errorf("%w: *t3xf.String argument expected: %T", ErrInvalidArg, arg)
	}

	l := s.Len()
	if l < 0 || l > math.MaxUint32 {
		return e.errorf("%w: too long to encode: %v", ErrInvalidArg, l)
	}

	b := s.Bytes()
	e.appendUint32(opcode.Pack(op, 0))
	e.appendUint32(uint32(l))
	e.b = append(e.b, b...)
	if padding := len(b) % 4; padding != 0 {
		e.b = append(e.b, make([]byte, 4-padding)...)
	}
	e.c++
	return nil
}

func (e *Encoder) encodeInstruction(op opcode.Opcode, arg any, max int) error {
	intArg, ok := arg.(int)
	if arg != nil {
		if !ok {
			return e.errorf("%w: integer argument expected: %v", ErrInvalidArg, arg)
		}
		if intArg < 0 || intArg > max {
			return e.errorf("%w: argument too large: %v", ErrInvalidArg, intArg)
		}
	}
	e.appendUint32(opcode.Pack(op, intArg))
	e.c++
	return nil
}

func (e *Encoder) errorf(format string, args ...any) error {
	return &EncoderError{
		Err: fmt.Errorf(format, args...),
		I:   e.i,
		Op:  e.op,
	}
}

func (e *Encoder) appendUint32(v uint32) {
	e.b = binary.LittleEndian.AppendUint32(e.b, v)
}

func (e *Encoder) appendUint64(v uint64) {
	e.b = binary.LittleEndian.AppendUint64(e.b, v)
}

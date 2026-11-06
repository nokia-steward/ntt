package vm

import (
	"encoding/binary"
	"github.com/nokia/ntt/internal/t3xf"
	"github.com/nokia/ntt/internal/t3xf/opcode"
	"math"
)

// patchBlocks replaces SCAN-blocks with ISCAN-blocks, it also lowers loops
// into flat GOTOs.
//
// In the future we might want to patch REFs to FROZEN_REFs and all special
// opcodes to FUNCTIONX opcodes.
func patchBlocks(b []byte) {
	type block struct {
		Parent *block
		Begin  int
		End    int

		Blocks   []*block
		Continue []int
		Break    []int
		NLocals  int
	}

	pc := 0
	end := len(b)
	curr := &block{}

	pop := func() *block {
		blk := curr.Blocks[len(curr.Blocks)-1]
		curr.Blocks = curr.Blocks[:len(curr.Blocks)-1]
		return blk
	}

	patch := func(b []byte, op opcode.Opcode, arg int) {
		binary.LittleEndian.PutUint32(b, opcode.Pack(op, arg))
	}

	replaceJumps := func(blk *block, begin, end int) {
		q := []*block{blk}
		for len(q) > 0 {
			blk, q = q[0], q[1:]
			q = append(q, blk.Blocks...)

			for _, addr := range blk.Continue {
				patch(b[addr:], opcode.GOTO, begin)
			}
			for _, addr := range blk.Break {
				patch(b[addr:], opcode.GOTO, end)
			}
		}
	}

	addReturn := func(blk *block) {
		if blk.Begin < blk.End {
			patch(b[blk.End:], opcode.RETURN, 0)
		}
	}

	for pc < end {
		n, op, _ := t3xf.Decode(b[pc:])
		switch op {
		case opcode.SCAN:
			curr = &block{
				Parent: curr,
				Begin:  pc,
			}

		case opcode.BLOCK:
			blk := curr
			patch(b[blk.Begin:], opcode.ISCAN, pc)
			blk.End = pc

			curr = curr.Parent
			curr.Blocks = append(curr.Blocks, blk)
			curr.NLocals += blk.NLocals

			// Clear stack from time to time, because we don't
			// always pop all the blocks.
			if len(curr.Blocks) > 1024 {
				curr.Blocks = curr.Blocks[len(curr.Blocks)-4:]
			}

		case opcode.SKIP:
			patch(b[pc:], opcode.ISCAN, pc)
			curr.Blocks = append(curr.Blocks, &block{
				Parent: curr,
				Begin:  pc,
				End:    pc,
			})

		case opcode.CONTINUE:
			curr.Continue = append(curr.Continue, pc)

		case opcode.BREAK:
			curr.Break = append(curr.Break, pc)

		case opcode.IF,
			opcode.EXECUTE,
			opcode.EXECUTED,
			opcode.EXECUTEL,
			opcode.EXECUTELD:
			if blk := pop(); blk.Begin < blk.End {
				patch(b[blk.End:], opcode.GOTO, pc+4)
			}

		case opcode.IFELSE:
			if blk := pop(); blk.Begin < blk.End {
				patch(b[blk.End:], opcode.GOTO, pc+4)
			}
			if blk := pop(); blk.Begin < blk.End {
				patch(b[blk.End:], opcode.GOTO, pc+4)
			}

		case opcode.WHILE:
			body := pop()
			cond := pop()

			patch(b[cond.Begin:], opcode.NOP, 0)
			patch(b[cond.End:], opcode.JT, 4)
			patch(b[body.Begin:], opcode.GOTO, pc+4)
			if body.Begin < body.End {
				patch(b[body.End:], opcode.NOP, 0)
			}
			patch(b[pc:], opcode.GOTO, cond.Begin)

			replaceJumps(body, cond.Begin, pc+4)

		case opcode.DOWHILE:
			cond := pop()
			body := pop()

			patch(b[body.Begin:], opcode.NOP, 0)
			patch(b[body.End:], opcode.NOP, 0)
			patch(b[cond.Begin:], opcode.NOP, 0)
			patch(b[cond.End:], opcode.JNT, 4)
			patch(b[pc:], opcode.GOTO, body.Begin)

			replaceJumps(body, body.Begin, pc+4)

		case opcode.FOR:
			incr := pop()
			body := pop()
			cond := pop()
			init := pop()

			patch(b[init.Begin:], opcode.NOP, 0)
			patch(b[init.End:], opcode.NOP, 0)
			patch(b[cond.Begin:], opcode.NOP, 0)
			patch(b[cond.End:], opcode.JT, 4)
			patch(b[body.Begin:], opcode.GOTO, pc+4)
			if body.Begin < body.End {
				patch(b[body.End:], opcode.NOP, 0)
			}
			patch(b[incr.Begin:], opcode.NOP, 0)
			patch(b[incr.End:], opcode.NOP, 0)
			patch(b[pc:], opcode.GOTO, cond.Begin)

			replaceJumps(body, body.Begin, pc+4)

		case opcode.VAR, opcode.VARDUP, opcode.MPAR:
			patch(b[pc:], op, curr.NLocals)
			curr.NLocals++

		case opcode.CONST, opcode.CONSTW, opcode.MPARD:
			patch(b[pc:], op, curr.NLocals)
			curr.NLocals++
			addReturn(pop())

		case opcode.TEMPLATE:
			patch(b[pc:], op, curr.NLocals)
			curr.NLocals++
			addReturn(pop())

		case opcode.CONTROL,
			opcode.FUNCTION,
			opcode.FUNCTIONB,
			opcode.FUNCTIONV,
			opcode.FUNCTIONVB,
			opcode.ALTSTEP,
			opcode.ALTSTEPB,
			opcode.ALTSTEPBW,
			opcode.ALTSTEPW,
			opcode.TESTCASE,
			opcode.TESTCASES:
			if curr.NLocals > math.MaxInt32 {
				panic("too many locals")
			}
			patch(b[pc:], op, curr.NLocals)
			addReturn(pop())
		}
		pc += n
	}
}

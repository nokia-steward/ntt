package t3xf

import (
	"encoding/binary"
	"github.com/nokia/ntt/internal/t3xf/opcode"
)

// PatchBlocks replaces SCAN-blocks with ISCAN-blocks, it also lowers loops
// into flat GOTOs.
//
// In the future we might want to patch REFs to FROZEN_REFs and all special
// opcodes to FUNCTIONX opcodes.
func PatchBlocks(b []byte) {
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

	// Blocks nested in a behaviour body share its local-slot counter, so a
	// nested VAR never reuses a slot. Blocks directly in the module start at 0.
	inBehaviour := func(blk *block) bool {
		return blk.Parent != nil && blk.Parent.Parent != nil
	}

	pop := func() *block {
		if len(curr.Blocks) == 0 {
			return &block{}
		}
		blk := curr.Blocks[len(curr.Blocks)-1]
		curr.Blocks = curr.Blocks[:len(curr.Blocks)-1]
		return blk
	}

	mergeJumps := func(into, from *block) {
		into.Continue = append(into.Continue, from.Continue...)
		into.Break = append(into.Break, from.Break...)
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
		n, op, _ := Decode(b[pc:])
		switch op {
		case opcode.SCAN:
			blk := &block{
				Parent: curr,
				Begin:  pc,
			}
			if inBehaviour(curr) {
				blk.NLocals = curr.NLocals
			}
			curr = blk

		case opcode.BLOCK:
			if curr.Parent == nil {
				return
			}
			blk := curr
			patch(b[blk.Begin:], opcode.ISCAN, pc)
			blk.End = pc

			curr = curr.Parent
			curr.Blocks = append(curr.Blocks, blk)
			if inBehaviour(curr) {
				curr.NLocals = blk.NLocals
			}

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
			blk := pop()
			if blk.Begin < blk.End {
				patch(b[blk.End:], opcode.GOTO, pc+4)
			}
			mergeJumps(curr, blk)

		case opcode.IFELSE:
			blk := pop()
			if blk.Begin < blk.End {
				patch(b[blk.End:], opcode.GOTO, pc+4)
			}
			mergeJumps(curr, blk)
			blk = pop()
			if blk.Begin < blk.End {
				patch(b[blk.End:], opcode.GOTO, pc+4)
			}
			mergeJumps(curr, blk)

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

			replaceJumps(body, incr.Begin, pc+4)

		case opcode.VAR, opcode.VARDUP, opcode.MPAR:
			patch(b[pc:], op, curr.NLocals)
			curr.NLocals++

		case opcode.CONST, opcode.CONSTW, opcode.MPARD:
			patch(b[pc:], op, curr.NLocals)
			curr.NLocals++
			addReturn(pop())

		case opcode.CONTROL,
			opcode.TEMPLATE,
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
			body := pop()
			patch(b[pc:], op, body.NLocals)
			addReturn(body)
		}
		pc += n
	}
}

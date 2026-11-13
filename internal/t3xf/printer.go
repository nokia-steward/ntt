package t3xf

import (
	"fmt"
	"github.com/nokia/ntt/internal/t3xf/opcode"
	"io"
	"strings"
)

type Printer struct {

	// Hide line number change instructions
	HideLines bool

	// Show the offset of each instruction
	ShowOffsets bool

	// Show the bytes of each instrution
	ShowBytes bool

	// Do not indent blocks
	NoIndent bool

	// Show label
	ShowLabel bool
}

func (p *Printer) Fprint(w io.Writer, code []byte) error {
	instrs, err := Disassemble(code)
	if err != nil {
		return err
	}
	indent := 0
	stack := []int{-1}
	for _, instr := range instrs {

		if p.ShowOffsets {
			if instr.Op != opcode.LINE {
				fmt.Fprintf(w, "[%04x] ", instr.address)
			} else if !p.HideLines {
				fmt.Fprintf(w, "       ")
			}
		}

		if p.ShowBytes {
			b := instr.bytes
			for i := 0; i < len(b); i += 4 {
				if i != 0 {
					fmt.Fprintf(w, "       ")
				}
				fmt.Fprintf(w, "% 2x", b[i:i+4])
			}
		}

		if !p.NoIndent {
			fmt.Fprintf(w, "%*s", indent, "\t")
		}

		if instr.Op == opcode.LINE {
			if !p.HideLines {
				fmt.Fprintf(w, "=%d\n", instr.Arg.(int))
			}
		} else {
			fmt.Fprint(w, strings.ToUpper(instr.Op.String()))
			p.printArg(w, instr, instrs)
			fmt.Fprintln(w)
		}

		switch {
		case instr.Op == opcode.SCAN:
			indent++
		case instr.Op == opcode.BLOCK:
			indent--
		case instr.Op == opcode.ISCAN:
			// non-empty ISCAN blocks
			if addr := int(instr.Arg.(Reference)); addr != instr.address {
				indent++
				stack = append(stack, addr)
			}
		// ISCAN block end?
		case instr.address == stack[len(stack)-1]:
			indent--
			stack = stack[:len(stack)-1]
		}
	}
	return nil
}

func (p *Printer) printArg(w io.Writer, instr Instruction, instrs []Instruction) {
	switch arg := instr.Arg.(type) {
	case nil:
	case Reference:
		fmt.Fprintf(w, " 0x%04x", arg)
	default:
		switch instr.Op {
		case opcode.NAME:
			fmt.Fprintf(w, " %s", arg)
		case opcode.UTF8:
			fmt.Fprintf(w, " %q", arg)
		default:
			fmt.Fprintf(w, " %v", arg)
		}
	}
}

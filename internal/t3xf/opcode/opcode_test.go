package opcode_test

import (
	"bytes"
	"os"
	"slices"
	"testing"

	"github.com/nokia/ntt/internal/t3xf/opcode"
	"github.com/nokia/ntt/internal/yaml"
)

func TestGeneratedTables(t *testing.T) {
	b, err := os.ReadFile("opcodes.yml")
	if err != nil {
		t.Fatal(err)
	}
	// A Windows checkout ends lines with CRLF; the generated tables do not.
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	var opcodes map[string]*opcode.Descriptor
	if err := yaml.Unmarshal(b, &opcodes); err != nil {
		t.Fatal(err)
	}
	if len(opcode.Descriptions) != len(opcodes) {
		t.Fatalf("len(Descriptions) = %d, want %d", len(opcode.Descriptions), len(opcodes))
	}
	for name, want := range opcodes {
		op, err := opcode.Parse(name)
		if err != nil {
			t.Errorf("Parse(%q): %v", name, err)
			continue
		}
		wantOp := opcode.Opcode(want.Opcode*16 + 3)
		if op != wantOp {
			t.Errorf("Parse(%q) = %#x, want %#x", name, int(op), int(wantOp))
		}
		if got := op.String(); got != name {
			t.Errorf("Opcode(%#x).String() = %q, want %q", int(op), got, name)
		}
		got, ok := opcode.Descriptions[op]
		if !ok {
			t.Errorf("Descriptions[%s] missing", name)
			continue
		}
		if got.Opcode != int(wantOp) ||
			got.Arg != want.Arg ||
			got.Description != want.Description ||
			!slices.Equal(got.Context, want.Context) ||
			!slices.Equal(got.Operations.Pre, want.Operations.Pre) ||
			!slices.Equal(got.Operations.Post, want.Operations.Post) {
			t.Errorf("Descriptions[%s] = %+v, want %+v (encoded %#x)", name, got, want, int(wantOp))
		}
		if gotArg := op.Arg(); gotArg != want.Arg {
			t.Errorf("Opcode(%s).Arg() = %q, want %q", name, gotArg, want.Arg)
		}
	}
	for op, wantArg := range map[opcode.Opcode]opcode.Arg{
		opcode.REF:        opcode.ArgRef,
		opcode.GOTO:       opcode.ArgRef,
		opcode.FROZEN_REF: opcode.ArgRef,
		opcode.CALL:       opcode.ArgRef,
		opcode.ISCAN:      opcode.ArgRef,
		opcode.LINE:       opcode.ArgLine,
	} {
		if got := op.Arg(); got != wantArg {
			t.Errorf("Opcode(%s).Arg() = %q, want %q", op, got, wantArg)
		}
	}
}

func TestUnpack(t *testing.T) {
	tests := []struct {
		input uint32
		op    opcode.Opcode
		arg   any
	}{
		{input: 0b00_0000000000000000000000000000_00, op: opcode.REF, arg: 0},
		{input: 0b00_0000000000000000000000000001_00, op: opcode.REF, arg: 4},
		{input: 0b00_0000001000000000000000000011_00, op: opcode.REF, arg: 8388620},
		{input: 0b00_0111111111111111111111111111_00, op: opcode.REF, arg: 536870908},
		{input: 0b10_0000000000000000000000000000_00, op: opcode.FROZEN_REF, arg: 0},
		{input: 0b10_0000000000000000000000000001_00, op: opcode.FROZEN_REF, arg: 4},
		{input: 0b10_0000001000000000000000000011_00, op: opcode.FROZEN_REF, arg: 8388620},
		{input: 0b10_0111111111111111111111111111_00, op: opcode.FROZEN_REF, arg: 536870908},
		{input: 0b00_0000000000000000000000000000_01, op: opcode.GOTO, arg: 0},
		{input: 0b00_0000000000000000000000000001_01, op: opcode.GOTO, arg: 4},
		{input: 0b00_0000001000000000000000000011_01, op: opcode.GOTO, arg: 8388620},
		{input: 0b00_0111111111111111111111111111_01, op: opcode.GOTO, arg: 536870908},
		{input: 0b01_0000001000000000000000000011_01, op: opcode.CALL, arg: 8388620},
		{input: 0b10_0000001000000000000000000011_01, op: opcode.ISCAN, arg: 8388620},
		{input: 0b00_0000000000000000000000000000_10, op: opcode.LINE, arg: 0},
		{input: 0b00_0000000000000000000000000001_10, op: opcode.LINE, arg: 1},
		{input: 0b00_0111111111111111111111111111_10, op: opcode.LINE, arg: 134217727},
	}

	for _, tt := range tests {
		op, arg := opcode.Unpack(tt.input)
		if op != tt.op || arg != tt.arg {
			t.Errorf("opcode.Unpack(%b) = %s,%v; want=%s,%v ", tt.input, op, arg, tt.op, tt.arg)
		}
	}
}

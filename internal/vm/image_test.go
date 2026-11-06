package vm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nokia/ntt/internal/runtime"
	"github.com/nokia/ntt/internal/t3xf"
	"github.com/nokia/ntt/internal/t3xf/opcode"

	"github.com/stretchr/testify/assert"
)

func TestPatchBlocks(t *testing.T) {
	tests := []struct {
		name string
		code []t3xf.Instruction
		want []t3xf.Instruction
		skip bool
	}{
		{name: "iscan", code: []t3xf.Instruction{
			{Op: opcode.SCAN},
			{Op: opcode.SCAN},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.BLOCK},
			{Op: opcode.SKIP},
			{Op: opcode.BLOCK},
		}, want: []t3xf.Instruction{
			{Op: opcode.ISCAN, Arg: t3xf.Reference(24)},
			{Op: opcode.ISCAN, Arg: t3xf.Reference(8)},
			{Op: opcode.BLOCK},
			{Op: opcode.ISCAN, Arg: t3xf.Reference(16)},
			{Op: opcode.BLOCK},
			{Op: opcode.ISCAN, Arg: t3xf.Reference(0x14)},
			{Op: opcode.BLOCK},
		}},

		{name: "if", code: []t3xf.Instruction{
			{Op: opcode.TRUE},
			{Op: opcode.SCAN},
			{Op: opcode.BLOCK},
			{Op: opcode.IF},
			{Op: opcode.TRUE},
			{Op: opcode.SKIP},
			{Op: opcode.IF},
		}, want: []t3xf.Instruction{
			{Op: opcode.TRUE},
			{Op: opcode.ISCAN, Arg: 2},
			{Op: opcode.GOTO, Arg: 4},
			{Op: opcode.IF},
			{Op: opcode.TRUE},
			{Op: opcode.ISCAN, Arg: t3xf.Reference(0x14)},
			{Op: opcode.IF},
		}},
		{name: "ifelse", code: []t3xf.Instruction{
			{Op: opcode.TRUE},
			{Op: opcode.SCAN},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.BLOCK},
			{Op: opcode.IFELSE},

			{Op: opcode.TRUE},
			{Op: opcode.SKIP},
			{Op: opcode.SKIP},
			{Op: opcode.IFELSE},

			{Op: opcode.TRUE},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 11},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 22},
			{Op: opcode.BLOCK},
			{Op: opcode.IFELSE},
		}, want: []t3xf.Instruction{
			{Op: opcode.TRUE},
			{Op: opcode.ISCAN, Arg: t3xf.Reference(8)},
			{Op: opcode.GOTO, Arg: t3xf.Reference(24)},
			{Op: opcode.ISCAN, Arg: t3xf.Reference(16)},
			{Op: opcode.GOTO, Arg: t3xf.Reference(24)},
			{Op: opcode.IFELSE},

			{Op: opcode.TRUE},
			{Op: opcode.ISCAN, Arg: t3xf.Reference(0x1c)},
			{Op: opcode.ISCAN, Arg: t3xf.Reference(0x20)},
			{Op: opcode.IFELSE},

			{Op: opcode.TRUE},
			{Op: opcode.ISCAN, Arg: t3xf.Reference(56)},
			{Op: opcode.NATLONG, Arg: 11},
			{Op: opcode.GOTO, Arg: t3xf.Reference(80)},
			{Op: opcode.ISCAN, Arg: t3xf.Reference(72)},
			{Op: opcode.NATLONG, Arg: 22},
			{Op: opcode.GOTO, Arg: t3xf.Reference(80)},
			{Op: opcode.IFELSE},
		}},

		{name: "while", code: []t3xf.Instruction{
			{Op: opcode.SCAN},
			{Op: opcode.TRUE},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.NOP},
			{Op: opcode.BLOCK},
			{Op: opcode.WHILE},
		}, want: []t3xf.Instruction{
			{Op: opcode.NOP},
			{Op: opcode.TRUE},
			{Op: opcode.JT, Arg: 4},
			{Op: opcode.GOTO, Arg: t3xf.Reference(28)},
			{Op: opcode.NOP},
			{Op: opcode.NOP},
			{Op: opcode.GOTO, Arg: t3xf.Reference(0)},
		}},

		{name: "while", code: []t3xf.Instruction{
			{Op: opcode.SCAN},
			{Op: opcode.TRUE},
			{Op: opcode.BLOCK},
			{Op: opcode.SKIP},
			{Op: opcode.WHILE},
		}, want: []t3xf.Instruction{
			{Op: opcode.NOP},
			{Op: opcode.TRUE},
			{Op: opcode.JT, Arg: 4},
			{Op: opcode.GOTO, Arg: t3xf.Reference(20)},
			{Op: opcode.GOTO, Arg: t3xf.Reference(0)},
		}},

		{name: "do-while", code: []t3xf.Instruction{
			{Op: opcode.SCAN},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.TRUE},
			{Op: opcode.BLOCK},
			{Op: opcode.DOWHILE},
		}, want: []t3xf.Instruction{
			{Op: opcode.NOP},
			{Op: opcode.NOP},
			{Op: opcode.NOP},
			{Op: opcode.TRUE},
			{Op: opcode.JNT, Arg: 4},
			{Op: opcode.GOTO, Arg: t3xf.Reference(0)},
		}},

		{name: "do-while", code: []t3xf.Instruction{
			{Op: opcode.SKIP},
			{Op: opcode.SCAN},
			{Op: opcode.TRUE},
			{Op: opcode.BLOCK},
			{Op: opcode.DOWHILE},
		}, want: []t3xf.Instruction{
			{Op: opcode.NOP},
			{Op: opcode.NOP},
			{Op: opcode.TRUE},
			{Op: opcode.JNT, Arg: 4},
			{Op: opcode.GOTO, Arg: t3xf.Reference(0)},
		}},

		{name: "continue", code: []t3xf.Instruction{
			{Op: opcode.SCAN},
			{Op: opcode.TRUE},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.SCAN},
			{Op: opcode.BREAK},
			{Op: opcode.CONTINUE},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.TRUE},
			{Op: opcode.BLOCK},
			{Op: opcode.DOWHILE},
			{Op: opcode.BREAK},
			{Op: opcode.BLOCK},
			{Op: opcode.WHILE},
		}, want: []t3xf.Instruction{
			{Op: opcode.NOP},
			{Op: opcode.TRUE},
			{Op: opcode.JT, Arg: 4},
			{Op: opcode.GOTO, Arg: t3xf.Reference(60)},
			{Op: opcode.NOP},
			{Op: opcode.GOTO, Arg: t3xf.Reference(48)},
			{Op: opcode.GOTO, Arg: t3xf.Reference(16)},
			{Op: opcode.NOP},
			{Op: opcode.NOP},
			{Op: opcode.TRUE},
			{Op: opcode.JNT, Arg: 4},
			{Op: opcode.GOTO, Arg: t3xf.Reference(16)},
			{Op: opcode.GOTO, Arg: t3xf.Reference(60)},
			{Op: opcode.NOP},
			{Op: opcode.GOTO, Arg: t3xf.Reference(0)},
		}},

		{name: "if-continue-in-while", code: []t3xf.Instruction{
			{Op: opcode.SCAN},
			{Op: opcode.TRUE},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.TRUE},
			{Op: opcode.SCAN},
			{Op: opcode.CONTINUE},
			{Op: opcode.BLOCK},
			{Op: opcode.IF},
			{Op: opcode.BLOCK},
			{Op: opcode.WHILE},
			{Op: opcode.NOP},
		}, want: []t3xf.Instruction{
			{Op: opcode.NOP},
			{Op: opcode.TRUE},
			{Op: opcode.JT, Arg: 4},
			{Op: opcode.GOTO, Arg: t3xf.Reference(0x2c)},
			{Op: opcode.TRUE},
			{Op: opcode.ISCAN, Arg: t3xf.Reference(0x1c)},
			{Op: opcode.GOTO, Arg: t3xf.Reference(0)},
			{Op: opcode.GOTO, Arg: t3xf.Reference(0x24)},
			{Op: opcode.IF},
			{Op: opcode.NOP},
			{Op: opcode.GOTO, Arg: t3xf.Reference(0)},
			{Op: opcode.NOP},
		}},

		{name: "for-continue", code: []t3xf.Instruction{
			{Op: opcode.SCAN},
			{Op: opcode.NOP},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.TRUE},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.CONTINUE},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.NOP},
			{Op: opcode.BLOCK},
			{Op: opcode.FOR},
			{Op: opcode.NOP},
		}, want: []t3xf.Instruction{
			{Op: opcode.NOP},
			{Op: opcode.NOP},
			{Op: opcode.NOP},
			{Op: opcode.NOP},
			{Op: opcode.TRUE},
			{Op: opcode.JT, Arg: 4},
			{Op: opcode.GOTO, Arg: t3xf.Reference(0x34)},
			{Op: opcode.GOTO, Arg: t3xf.Reference(0x24)},
			{Op: opcode.NOP},
			{Op: opcode.NOP},
			{Op: opcode.NOP},
			{Op: opcode.NOP},
			{Op: opcode.GOTO, Arg: t3xf.Reference(0xc)},
			{Op: opcode.NOP},
		}},

		{name: "const bodies", code: []t3xf.Instruction{
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 42},
			{Op: opcode.BLOCK},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "C"},
			{Op: opcode.CONST},
		}, want: []t3xf.Instruction{
			{Op: opcode.ISCAN, Arg: 2},
			{Op: opcode.NATLONG, Arg: 42},
			{Op: opcode.RETURN},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "C"},
			{Op: opcode.CONST},
		}},
	}

	t.Parallel()
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip()
			}
			want := mustAssemble(t, tt.want)
			got := mustAssemble(t, tt.code)
			t3xf.PatchBlocks(got)
			assert.Equal(t, disassemble(want), disassemble(got))
		})
	}
}

func TestLocations(t *testing.T) {

	load := func(t *testing.T, code []t3xf.Instruction) *Image {
		img, err := load(mustAssemble(t, code))
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Log(disassemble(img.Code))
		img.Path = "t3xf"
		return img
	}

	t.Run("sources", func(t *testing.T) {
		code := []t3xf.Instruction{
			{Op: opcode.ISCAN, Arg: 1},
			{Op: opcode.BLOCK},
			{Op: opcode.NAME, Arg: "src1"},
			{Op: opcode.SOURCE},
			{Op: opcode.ISCAN, Arg: 6},
			{Op: opcode.NOP},
			{Op: opcode.BLOCK},
			{Op: opcode.NAME, Arg: "src2"},
			{Op: opcode.SOURCE},
			{Op: opcode.NOP},
		}
		prog := load(t, code)
		assert.Equal(t, "src1", prog.Source(0x00))
		assert.Equal(t, "src1", prog.Source(0x04))
		assert.Equal(t, "src1", prog.Source(0x08))
		assert.Equal(t, "src1", prog.Source(0x10))
		assert.Equal(t, "src2", prog.Source(0x18))
		assert.Equal(t, "t3xf", prog.Source(0x34))
	})

	t.Run("modules", func(t *testing.T) {
		code := []t3xf.Instruction{
			{Op: opcode.ISCAN, Arg: 1},
			{Op: opcode.BLOCK},
			{Op: opcode.NAME, Arg: "mod1"},
			{Op: opcode.MODULE},
			{Op: opcode.ISCAN, Arg: 6},
			{Op: opcode.NOP},
			{Op: opcode.BLOCK},
			{Op: opcode.NAME, Arg: "mod2"},
			{Op: opcode.MODULE},
			{Op: opcode.NOP},
		}
		prog := load(t, code)
		assert.Equal(t, "mod1", prog.Module(0x00))
		assert.Equal(t, "mod1", prog.Module(0x04))
		assert.Equal(t, "mod1", prog.Module(0x08))
		assert.Equal(t, "mod1", prog.Module(0x10))
		assert.Equal(t, "mod2", prog.Module(0x18))
		assert.Equal(t, "", prog.Module(0x34))
	})

	t.Run("lines", func(t *testing.T) {
		code := []t3xf.Instruction{
			{Op: opcode.NOP},
			{Op: opcode.LINE, Arg: 11},
			{Op: opcode.NOP},
			{Op: opcode.LINE, Arg: 22},
			{Op: opcode.LINE, Arg: 22},
			{Op: opcode.LINE, Arg: 11},
		}
		prog := load(t, code)
		assert.Equal(t, 0, prog.Line(0x00), "addr=0x00")
		assert.Equal(t, 11, prog.Line(0x04), "addr=0x04")
		assert.Equal(t, 11, prog.Line(0x08), "addr=0x08")
		assert.Equal(t, 22, prog.Line(0x0c), "addr=0x0c")
		assert.Equal(t, 22, prog.Line(0x10), "addr=0x10")
		assert.Equal(t, 11, prog.Line(0x14), "addr=0x14")
		assert.Equal(t, 11, prog.Line(0x20), "addr=0x20")
	})
	t.Run("no lines", func(t *testing.T) {
		code := []t3xf.Instruction{
			{Op: opcode.NOP},
			{Op: opcode.NOP},
		}
		prog := load(t, code)
		assert.Equal(t, 0, prog.Line(0x00), "addr=0x00")
		assert.Equal(t, 0, prog.Line(0x04), "addr=0x04")
		assert.Equal(t, 0, prog.Line(0x08), "addr=0x08")
		assert.Equal(t, "t3xf:0x0004", prog.Pos(0x04))
	})

	t.Run("pos falls back to offset", func(t *testing.T) {
		img := NewImage(mustAssemble(t, []t3xf.Instruction{
			{Op: opcode.NOP},
			{Op: opcode.LINE, Arg: 9},
			{Op: opcode.NOP},
		}))
		img.Sources.Addr = []int{len(img.Code)}
		img.Sources.Name = []string{"f.ttcn3"}
		assert.Equal(t, "t3xf:0x0000", img.Pos(0))
		assert.Equal(t, "f.ttcn3:9", img.Pos(8))
	})
}

func TestT3XFHeader(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		img, err := Load(nil)
		assert.Nil(t, img)
		assert.Error(t, err)
	})

	t.Run("minimal header", func(t *testing.T) {
		b := mustAssemble(t, []t3xf.Instruction{
			{Op: opcode.NOP},
			{Op: opcode.NATLONG, Arg: 3},
			{Op: opcode.VERSION},
		})
		img, err := Load(b)
		assert.NotNil(t, img)
		assert.Nil(t, err)
	})
	t.Run("v2", func(t *testing.T) {
		b := mustAssemble(t, []t3xf.Instruction{
			{Op: opcode.NOP},
			{Op: opcode.NATLONG, Arg: 2},
			{Op: opcode.VERSION},
			{Op: opcode.SCAN},
			{Op: opcode.BLOCK},
		})
		img, err := Load(b)
		_, op, _ := t3xf.Decode(img.Code[16:])
		assert.Equal(t, "iscan", op.String())
		assert.Nil(t, err)
	})
	t.Run("minimal header", func(t *testing.T) {
		b := mustAssemble(t, []t3xf.Instruction{
			{Op: opcode.NOP},
			{Op: opcode.NATLONG, Arg: 3},
			{Op: opcode.VERSION},
		})
		img, err := Load(b)
		assert.NotNil(t, img)
		assert.Nil(t, err)
	})
}

func TestLoadEntities(t *testing.T) {

	t.Run("types", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.RECORDOF},
			{Op: opcode.NAME, Arg: "A"},
			{Op: opcode.TYPE},

			{Op: opcode.SCAN},
			{Op: opcode.UTF8, Arg: "encodet"},
			{Op: opcode.ENCODE},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "f"},
			{Op: opcode.FIELD},
			{Op: opcode.BLOCK},
			{Op: opcode.RECORD},
			{Op: opcode.NAME, Arg: "B"},
			{Op: opcode.TYPEW},
		}

		img := mustLoad(t, mustAssemble(t, input))

		assert.Equal(t, &runtime.NamedType{
			Name: "A",
			Type: runtime.RecordOfType{
				ElementType: runtime.IntegerType,
			},
		}, img.Tags[0x14>>2])

		assert.Equal(t, &runtime.NamedType{
			Name: "B",
			Type: runtime.RecordType{
				Fields: []*runtime.Var{
					{Name: "f", Type: runtime.IntegerType},
				},
			},
			Attributes: runtime.Attributes{
				Encode: runtime.Attribute{
					Value: "encodet",
				},
			},
		}, img.Tags[0x60>>2])

	})

	t.Run("consts", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 42},
			{Op: opcode.BLOCK},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "A"},
			{Op: opcode.CONST},

			{Op: opcode.SCAN},
			{Op: opcode.UTF8, Arg: "encodec"},
			{Op: opcode.ENCODE},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 32},
			{Op: opcode.BLOCK},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "B"},
			{Op: opcode.CONSTW},
		}
		img := mustLoad(t, mustAssemble(t, input))

		assert.Equal(t, &runtime.Var{
			Name:  "A",
			Type:  runtime.IntegerType,
			Value: Block{0x00, 0x0c},
		}, img.Tags[0x20>>2])

		assert.Equal(t, &runtime.Var{
			Name:  "B",
			Type:  runtime.IntegerType,
			Index: 1,
			Value: Block{0x40, 0x4c},
			Attributes: runtime.Attributes{
				Encode: runtime.Attribute{
					Value: "encodec",
				},
			},
		}, img.Tags[0x60>>2])
	})

	t.Run("mpars", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "A"},
			{Op: opcode.MPAR},

			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 42},
			{Op: opcode.BLOCK},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "B"},
			{Op: opcode.MPAR},
		}
		mustLoad(t, mustAssemble(t, input))
	})

	t.Run("var", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "A"},
			{Op: opcode.VAR},
		}
		mustLoad(t, mustAssemble(t, input))

		b := mustCompile(t, `module M { var integer x := 1 }`)
		t3xf.PatchBlocks(b)
		img, err := load(b)
		assert.NoError(t, err)
		var got *runtime.Var
		for _, obj := range img.Tags {
			if v, ok := obj.(*runtime.Var); ok {
				got = v
			}
		}
		assert.Equal(t, &runtime.Var{
			Name:  "M.x",
			Type:  runtime.IntegerType,
			Value: 1,
		}, got)
	})

	t.Run("template", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.SCAN},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "p"},
			{Op: opcode.INOUT},
			{Op: opcode.BLOCK},
			{Op: opcode.SKIP},
			{Op: opcode.NAME, Arg: "t"},
			{Op: opcode.TEMPLATE},
		}
		mustLoad(t, mustAssemble(t, input))

	})

	t.Run("control", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.SKIP},
			{Op: opcode.CONTROL},
		}
		mustLoad(t, mustAssemble(t, input))
	})

	t.Run("functions", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.SKIP},
			{Op: opcode.SCAN},
			{Op: opcode.BLOCK},
			{Op: opcode.NAME, Arg: "f"},
			{Op: opcode.FUNCTION},

			{Op: opcode.SKIP},
			{Op: opcode.COMPONENT},
			{Op: opcode.SCAN},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "pb"},
			{Op: opcode.INOUT},
			{Op: opcode.BLOCK},
			{Op: opcode.SKIP},
			{Op: opcode.NAME, Arg: "fb"},
			{Op: opcode.FUNCTIONB},

			{Op: opcode.INTEGER},
			{Op: opcode.SCAN},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "pv"},
			{Op: opcode.INOUT},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.BLOCK},
			{Op: opcode.NAME, Arg: "fv"},
			{Op: opcode.FUNCTIONV},

			{Op: opcode.SKIP},
			{Op: opcode.COMPONENT},
			{Op: opcode.INTEGER},
			{Op: opcode.SCAN},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "pvb"},
			{Op: opcode.INOUT},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.BLOCK},
			{Op: opcode.NAME, Arg: "fvb"},
			{Op: opcode.FUNCTIONV},
		}
		mustLoad(t, mustAssemble(t, input))
	})

	t.Run("parameter directions", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.SCAN},
			{Op: opcode.SKIP},
			{Op: opcode.SCAN},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "pi"},
			{Op: opcode.IN},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "po"},
			{Op: opcode.OUT},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "pio"},
			{Op: opcode.INOUT},
			{Op: opcode.BLOCK},
			{Op: opcode.SKIP},
			{Op: opcode.NAME, Arg: "f"},
			{Op: opcode.FUNCTION},
			{Op: opcode.BLOCK},
			{Op: opcode.NAME, Arg: "module"},
			{Op: opcode.MODULE},
		}
		l := mustLoad(t, mustAssemble(t, input))
		fn, ok := l.Names["module.f"].(*runtime.Function)
		assert.True(t, ok)
		if assert.NotNil(t, fn) && assert.Len(t, fn.Parameters, 3) {
			assert.Equal(t, runtime.Parameter, fn.Parameters[0].Flags)
			assert.Equal(t, runtime.Parameter|runtime.Out, fn.Parameters[1].Flags)
			assert.Equal(t, runtime.Parameter|runtime.Inout, fn.Parameters[2].Flags)
			assert.Equal(t, runtime.Flags(0), fn.Parameters[2].Flags&runtime.Out)
		}
	})

	t.Run("extfunc", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.SCAN},
			{Op: opcode.BLOCK},
			{Op: opcode.NAME, Arg: "fx"},
			{Op: opcode.FUNCTIONX},

			{Op: opcode.INTEGER},
			{Op: opcode.SCAN},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "pxv"},
			{Op: opcode.INOUT},
			{Op: opcode.BLOCK},
			{Op: opcode.NAME, Arg: "fxv"},
			{Op: opcode.FUNCTIONXV},

			{Op: opcode.SKIP},
			{Op: opcode.SCAN},
			{Op: opcode.BLOCK},
			{Op: opcode.NAME, Arg: "fwx"},
			{Op: opcode.FUNCTIONXW},

			{Op: opcode.INTEGER},
			{Op: opcode.SKIP},
			{Op: opcode.SCAN},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "pxwv"},
			{Op: opcode.INOUT},
			{Op: opcode.BLOCK},
			{Op: opcode.NAME, Arg: "fxwv"},
			{Op: opcode.FUNCTIONXVW},
		}
		mustLoad(t, mustAssemble(t, input))
	})

	t.Run("altsteps", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.SCAN},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "p"},
			{Op: opcode.INOUT},
			{Op: opcode.BLOCK},
			{Op: opcode.SKIP},
			{Op: opcode.NAME, Arg: "as"},
			{Op: opcode.ALTSTEP},

			{Op: opcode.SKIP},
			{Op: opcode.COMPONENT},
			{Op: opcode.SCAN},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "pb"},
			{Op: opcode.INOUT},
			{Op: opcode.BLOCK},
			{Op: opcode.SKIP},
			{Op: opcode.NAME, Arg: "asb"},
			{Op: opcode.ALTSTEPB},

			{Op: opcode.SKIP},
			{Op: opcode.COMPONENT},
			{Op: opcode.SKIP},
			{Op: opcode.SCAN},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "pbw"},
			{Op: opcode.INOUT},
			{Op: opcode.BLOCK},
			{Op: opcode.SKIP},
			{Op: opcode.NAME, Arg: "asbw"},
			{Op: opcode.ALTSTEPBW},

			{Op: opcode.SKIP},
			{Op: opcode.SCAN},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "pw"},
			{Op: opcode.INOUT},
			{Op: opcode.BLOCK},
			{Op: opcode.SKIP},
			{Op: opcode.NAME, Arg: "asw"},
			{Op: opcode.ALTSTEPW},
		}
		mustLoad(t, mustAssemble(t, input))
	})

	t.Run("testcases", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.SKIP},
			{Op: opcode.COMPONENT},
			{Op: opcode.SCAN},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "p1"},
			{Op: opcode.INOUT},
			{Op: opcode.BOOLEAN},
			{Op: opcode.NAME, Arg: "p2"},
			{Op: opcode.INOUT},
			{Op: opcode.BLOCK},
			{Op: opcode.SKIP},
			{Op: opcode.NAME, Arg: "tc"},
			{Op: opcode.TESTCASE},

			{Op: opcode.SKIP},
			{Op: opcode.COMPONENT},
			{Op: opcode.SKIP},
			{Op: opcode.COMPONENT},
			{Op: opcode.SCAN},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "ps1"},
			{Op: opcode.INOUT},
			{Op: opcode.BOOLEAN},
			{Op: opcode.NAME, Arg: "ps2"},
			{Op: opcode.INOUT},
			{Op: opcode.BLOCK},
			{Op: opcode.SKIP},
			{Op: opcode.NAME, Arg: "tcs"},
			{Op: opcode.TESTCASES},
		}
		mustLoad(t, mustAssemble(t, input))
	})
}

func TestCharstringU(t *testing.T) {
	t.Run("loader", func(t *testing.T) {
		img := mustLoad(t, mustAssemble(t, []t3xf.Instruction{
			{Op: opcode.CHARSTRINGU},
			{Op: opcode.NAME, Arg: "A"},
			{Op: opcode.TYPE},
		}))
		assert.Equal(t, &runtime.NamedType{Name: "A", Type: runtime.UniversalCharstringType}, img.Tags[0x10>>2])
	})

	t.Run("body", func(t *testing.T) {
		m := testMachine(t, []t3xf.Instruction{
			{Op: opcode.CHARSTRINGU},
			{Op: opcode.RETURN},
		}, &Machine{})
		assert.Same(t, runtime.UniversalCharstringType, popResult(m))
	})
}

func TestBehaviourOperandOrder(t *testing.T) {
	t.Run("functionvb", func(t *testing.T) {
		l := mustLoad(t, mustCompile(t, `module M {
			type component C {}
			function f() runs on C return integer {}
		}`))
		fn := l.Names["M.f"].(*runtime.Function)
		assert.Same(t, runtime.IntegerType, fn.ReturnType)
		runsOn := fn.RunsOn.(*runtime.NamedType)
		assert.Equal(t, "M.C", runsOn.Name)
		_, ok := runsOn.Type.(runtime.ComponentType)
		assert.True(t, ok)
	})

	t.Run("altstepbw", func(t *testing.T) {
		l := mustLoad(t, mustCompile(t, `module M {
			type component C {}
			altstep as() runs on C {} with { extension "e" }
		}`))
		var as *runtime.Function
		for _, obj := range l.Tags {
			fn, ok := obj.(*runtime.Function)
			if ok && fn.Name == "M.as" {
				as = fn
			}
		}
		if as == nil {
			t.Fatal("altstep not loaded")
		}
		runsOn := as.RunsOn.(*runtime.NamedType)
		assert.Equal(t, "M.C", runsOn.Name)
		if assert.Len(t, as.Attributes.Extensions, 1) {
			assert.Equal(t, "e", as.Attributes.Extensions[0].Value)
		}
	})
}

func TestFields(t *testing.T) {
	l := mustLoad(t, mustCompile(t, `module M {
		type union U1 { integer a, charstring b }
		type union U2 { charstring b, integer a, integer d }
		type record R { integer e, integer f optional }
		type union U3 { integer e }
	}`))
	types := map[string]runtime.Type{}
	for _, obj := range l.Tags {
		if typ, ok := obj.(*runtime.NamedType); ok {
			types[typ.Name] = typ.Type
		}
	}
	assert.Equal(t, runtime.UnionType{Fields: []*runtime.Var{
		{Name: "a", Type: runtime.IntegerType, Index: 9},
		{Name: "b", Type: runtime.CharstringType, Index: 10},
	}}, types["M.U1"])
	assert.Equal(t, runtime.UnionType{Fields: []*runtime.Var{
		{Name: "b", Type: runtime.CharstringType, Index: 10},
		{Name: "a", Type: runtime.IntegerType, Index: 9},
		{Name: "d", Type: runtime.IntegerType, Index: 11},
	}}, types["M.U2"])
	assert.Equal(t, runtime.RecordType{Fields: []*runtime.Var{
		{Name: "e", Type: runtime.IntegerType, Index: 0},
		{Name: "f", Type: runtime.IntegerType, Index: 1, Flags: runtime.Optional},
	}}, types["M.R"])
	assert.Equal(t, runtime.UnionType{Fields: []*runtime.Var{
		{Name: "e", Type: runtime.IntegerType, Index: 12},
	}}, types["M.U3"])
}

func TestReferences(t *testing.T) {
	t.Run("def before use", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "int"},
			{Op: opcode.TYPE},

			{Op: opcode.REF, Arg: 2},
			{Op: opcode.NAME, Arg: "A"},
			{Op: opcode.MPAR},
		}
		img := mustLoad(t, mustAssemble(t, input))
		typ := img.Tags[0x10>>2]
		mpar := img.Tags[0x24>>2]
		assert.Equal(t, &runtime.NamedType{
			Name: "int",
			Type: runtime.IntegerType,
		}, typ)

		if mpar := mpar.(*runtime.Var).Type; mpar != typ {
			t.Errorf("%v != %v", mpar, typ)
		}
	})

	t.Run("forward", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.REF, Arg: 5},
			{Op: opcode.NAME, Arg: "A"},
			{Op: opcode.MPAR},

			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "int"},
			{Op: opcode.TYPE},
		}
		img := mustLoad(t, mustAssemble(t, input))
		typ := img.Tags[0x24>>2]
		mpar := img.Tags[0x10>>2]
		assert.Equal(t, &runtime.NamedType{
			Name: "int",
			Type: runtime.IntegerType,
		}, typ)

		if mpar := mpar.(*runtime.Var).Type; mpar != typ {
			t.Errorf("%v != %v", mpar, typ)
		}
	})

	t.Run("forward", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.SCAN},
			{Op: opcode.REF, Arg: 10},
			{Op: opcode.NAME, Arg: "A"},
			{Op: opcode.MPAR},
			{Op: opcode.BLOCK},
			{Op: opcode.NAME, Arg: "module1"},
			{Op: opcode.MODULE},

			{Op: opcode.SCAN},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "int"},
			{Op: opcode.TYPE},
			{Op: opcode.BLOCK},
			{Op: opcode.NAME, Arg: "module2"},
			{Op: opcode.MODULE},
		}
		img := mustLoad(t, mustAssemble(t, input))
		typ := img.Tags[0x44>>2]
		mpar := img.Tags[0x14>>2].(*runtime.Var)
		assert.Equal(t, &runtime.NamedType{
			Name: "module2.int",
			Type: runtime.IntegerType,
		}, typ)

		if mpar.Type != typ {
			t.Errorf("%v != %v", mpar.Type, typ)
		}
	})

	t.Run("forward", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.MARK},
			{Op: opcode.REF, Arg: 14},
			{Op: opcode.COLLECT},
			{Op: opcode.SKIP},
			{Op: opcode.COMPONENTX},
			{Op: opcode.NAME, Arg: "D"},
			{Op: opcode.TYPE},

			{Op: opcode.SCAN},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "var"},
			{Op: opcode.VAR},
			{Op: opcode.BLOCK},
			{Op: opcode.COMPONENT},
			{Op: opcode.NAME, Arg: "C"},
			{Op: opcode.TYPE},
		}
		img := mustLoad(t, mustAssemble(t, input))
		wantC := &runtime.NamedType{
			Name: "C",
			Type: runtime.ComponentType{
				Body: Block{Begin: 0x24, End: 0x3c},
			},
		}
		wantD := &runtime.NamedType{
			Name: "D",
			Type: runtime.ComponentType{
				Extends: []runtime.Type{wantC},
				Body:    Block{Begin: 0x0c, End: 0x0c},
			},
		}

		assert.Equal(t, wantD, img.Tags[0x20>>2])
		assert.Equal(t, wantC, img.Tags[0x50>>2])

	})

	t.Run("var not a type", func(t *testing.T) {
		_, isType := any(&runtime.Var{}).(runtime.Type)
		assert.False(t, isType)

		b := mustCompile(t, `module M { const integer c := 1; var c x }`)
		t3xf.PatchBlocks(b)
		_, err := load(b)
		assert.ErrorContains(t, err, "type expected")
	})

	t.Run("signature not a type", func(t *testing.T) {
		_, isType := any(runtime.Signature{}).(runtime.Type)
		assert.False(t, isType)

		b := mustCompile(t, `module M { function f() {} var f x }`)
		t3xf.PatchBlocks(b)
		_, err := load(b)
		assert.ErrorContains(t, err, "type expected")
	})

	t.Run("unknown", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.REF, Arg: 0},
			{Op: opcode.NAME, Arg: "A"},
			{Op: opcode.MPAR},
		}
		b := mustAssemble(t, input)
		t3xf.PatchBlocks(b)
		_, err := load(b)
		assert.ErrorContains(t, err, "no constructor for opcode ref")
	})
}

func TestLoadCorruptImage(t *testing.T) {
	header := []t3xf.Instruction{
		{Op: opcode.NOP},
		{Op: opcode.NATLONG, Arg: 3},
		{Op: opcode.VERSION},
	}

	t.Run("bad attribute opcode", func(t *testing.T) {
		b := mustAssemble(t, append(append([]t3xf.Instruction{}, header...),
			t3xf.Instruction{Op: opcode.ISCAN, Arg: 5},
			t3xf.Instruction{Op: opcode.NOP},
			t3xf.Instruction{Op: opcode.BLOCK},
			t3xf.Instruction{Op: opcode.INTEGER},
			t3xf.Instruction{Op: opcode.NAME, Arg: "T"},
			t3xf.Instruction{Op: opcode.TYPEW},
		))
		img, err := Load(b)
		assert.Nil(t, img)
		assert.ErrorIs(t, err, ErrInvalidInstruction)
	})

	t.Run("recordof multiple types", func(t *testing.T) {
		b := mustAssemble(t, append(append([]t3xf.Instruction{}, header...),
			t3xf.Instruction{Op: opcode.ISCAN, Arg: 6},
			t3xf.Instruction{Op: opcode.INTEGER},
			t3xf.Instruction{Op: opcode.BOOLEAN},
			t3xf.Instruction{Op: opcode.BLOCK},
			t3xf.Instruction{Op: opcode.RECORDOF},
			t3xf.Instruction{Op: opcode.NAME, Arg: "T"},
			t3xf.Instruction{Op: opcode.TYPE},
		))
		img, err := Load(b)
		assert.Nil(t, img)
		assert.ErrorContains(t, err, "expected one type, got 2")
	})

	t.Run("record field wrong type", func(t *testing.T) {
		b := mustAssemble(t, append(append([]t3xf.Instruction{}, header...),
			t3xf.Instruction{Op: opcode.ISCAN, Arg: 5},
			t3xf.Instruction{Op: opcode.INTEGER},
			t3xf.Instruction{Op: opcode.BLOCK},
			t3xf.Instruction{Op: opcode.RECORD},
			t3xf.Instruction{Op: opcode.NAME, Arg: "T"},
			t3xf.Instruction{Op: opcode.TYPE},
		))
		img, err := Load(b)
		assert.Nil(t, img)
		assert.ErrorContains(t, err, "unexpected")
	})

	t.Run("unmatched block in v2", func(t *testing.T) {
		b := mustAssemble(t, []t3xf.Instruction{
			{Op: opcode.NOP},
			{Op: opcode.NATLONG, Arg: 2},
			{Op: opcode.VERSION},
			{Op: opcode.BLOCK},
		})
		img, err := Load(b)
		assert.Nil(t, img)
		assert.ErrorIs(t, err, ErrInvalidInstruction)
	})

	t.Run("stack underflow", func(t *testing.T) {
		b := mustAssemble(t, append(append([]t3xf.Instruction{}, header...),
			t3xf.Instruction{Op: opcode.DROP},
		))
		img, err := Load(b)
		assert.Nil(t, img)
		assert.Error(t, err)
	})
}

func TestLoadMark(t *testing.T) {
	input := []t3xf.Instruction{
		{Op: opcode.INTEGER},
		{Op: opcode.NAME, Arg: "A"},
		{Op: opcode.VAR},
		{Op: opcode.INTEGER},
		{Op: opcode.NAME, Arg: "B"},
		{Op: opcode.VAR},
		{Op: opcode.INTEGER},
		{Op: opcode.NAME, Arg: "C"},
		{Op: opcode.VAR},

		{Op: opcode.MARK},
		{Op: opcode.NATLONG, Arg: 1},
		{Op: opcode.NATLONG, Arg: 2},
		{Op: opcode.NATLONG, Arg: 3},
		{Op: opcode.VLIST},

		{Op: opcode.MARK},
		{Op: opcode.REF, Arg: 2},
		{Op: opcode.REF, Arg: 5},
		{Op: opcode.REF, Arg: 8},
		{Op: opcode.COLLECT},
	}
	l := mustLoad(t, mustAssemble(t, input))
	objA := l.Tags[0x10>>2].(*runtime.Var)
	objB := l.Tags[0x24>>2].(*runtime.Var)
	objC := l.Tags[0x38>>2].(*runtime.Var)

	varA := &runtime.Var{
		Name:  "A",
		Type:  runtime.IntegerType,
		Index: 0,
	}

	varB := &runtime.Var{
		Name:  "B",
		Type:  runtime.IntegerType,
		Index: 1,
	}
	varC := &runtime.Var{
		Name:  "C",
		Type:  runtime.IntegerType,
		Index: 2,
	}
	assert.Equal(t, varA, objA)
	assert.Equal(t, varB, objB)
	assert.Equal(t, varC, objC)

	assert.Equal(t, []any{objA, objB, objC}, l.pop())
	assert.Equal(t, []any{1, 2, 3}, l.pop())
}

func TestLoadFileInvalid(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		img, err := LoadFile(filepath.Join(t.TempDir(), "missing.t3xf"))
		assert.Nil(t, img)
		assert.Error(t, err)
	})
	t.Run("garbage", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "garbage.t3xf")
		if err := os.WriteFile(path, []byte("garbage"), 0o644); err != nil {
			t.Fatal(err)
		}
		img, err := LoadFile(path)
		assert.Nil(t, img)
		assert.Error(t, err)
	})
}

package vm

import (
	"bytes"
	"fmt"
	"github.com/nokia/ntt/internal/t3xf"
	"testing"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func disassemble(b []byte) string {
	buf := bytes.Buffer{}
	p := t3xf.Printer{}
	if err := p.Fprint(&buf, b); err != nil {
		panic(err)
	}
	return buf.String()
}

func tags(l *loader) string {
	buf := bytes.Buffer{}
	for addr, obj := range l.Tags {
		if obj != nil {
			fmt.Fprintf(&buf, "[0x%04x] := %#v\n", addr<<2, obj)
		}
	}
	return buf.String()
}

func mustParse(t *testing.T, src string) *syntax.Root {
	root := syntax.Parse([]byte(src))
	if err := root.Err(); err != nil {
		t.Fatalf("syntax.Parse() failed: %s", err.Error())
	}
	return root
}

func mustCompile(t *testing.T, src string) []byte {
	ast := mustParse(t, src)
	comp := t3xf.NewCompiler()
	if err := comp.Compile(ast); err != nil {
		t.Fatalf("Compiler.Compile() failed: %s", err.Error())
	}
	b, err := comp.Assemble()
	if err != nil {
		t.Fatalf("Compiler.Assemble() failed: %s", err.Error())
	}
	return b
}

func mustAssemble(t *testing.T, instrs []t3xf.Instruction) []byte {
	b, err := t3xf.Assemble(instrs)
	if err != nil {
		t.Fatalf("t3xf.Assemble() failed: %s", err.Error())
	}
	return b
}

func mustLoad(t *testing.T, b []byte) *loader {
	t3xf.PatchBlocks(b)
	l := loader{Image: NewImage(b)}
	err := l.load(Block{Begin: -4, End: len(b)})
	if err != nil {
		buf := bytes.Buffer{}
		fmt.Fprintf(&buf, "%s\n", err.Error())
		fmt.Fprintf(&buf, "%s\n", disassemble(b))
		fmt.Fprintf(&buf, "Tags:\n%s\n", tags(&l))
		t.Fatal(buf.String())
	}
	return &l
}

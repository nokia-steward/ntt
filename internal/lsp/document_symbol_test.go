package lsp_test

import (
	"testing"

	"github.com/nokia/ntt/internal/lsp"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/stretchr/testify/assert"
)

func testDocumentSymbol(t *testing.T, text string) []Symbol {
	tree := ttcn3.Parse(text)
	var (
		ret        []Symbol
		makeSymbol func(sym protocol.DocumentSymbol) Symbol
	)

	makeSymbol = func(sym protocol.DocumentSymbol) Symbol {
		begin := tree.PosFor(int(sym.Range.Start.Line)+1, int(sym.Range.Start.Character)+1)
		end := tree.PosFor(int(sym.Range.End.Line)+1, int(sym.Range.End.Character)+1)
		s := Symbol{
			Kind:   sym.Kind,
			Name:   sym.Name,
			Detail: sym.Detail,
			Text:   string(text[begin:end]),
		}
		for _, c := range sym.Children {
			s.Children = append(s.Children, makeSymbol(c))
		}
		return s
	}

	for _, ds := range lsp.DocumentSymbols(tree) {
		ret = append(ret, makeSymbol(ds))
	}
	return ret
}

type Symbol struct {
	Kind     protocol.SymbolKind
	Name     string
	Detail   string
	Text     string
	Children []Symbol
}

func TestFunctionDefWithModuleDotRunsOn(t *testing.T) {
	input := `
	module Test
	{
		type component B0 extends C0, C1 {
			var integer i := 1;
			timer t1 := 2.0;
			port P p;
		}
		function f() runs on TestFunctionDefWithModuleDotRunsOn_Module_1.C0 system B0 return integer {}
	}
	module TestFunctionDefWithModuleDotRunsOn_Module_1
	{
		type component C0 {}
		type component C1 {}
	}`

	want := []Symbol{
		{Kind: protocol.Class, Name: "B0", Detail: "component type",
			Text: "type component B0 extends C0, C1 {\n\t\t\tvar integer i := 1;\n\t\t\ttimer t1 := 2.0;\n\t\t\tport P p;\n\t\t}",
			Children: []Symbol{
				{Kind: protocol.Array, Name: "extends", Detail: "",
					Text: "C0, C1",
					Children: []Symbol{
						{Kind: protocol.Class, Name: "C0", Detail: "", Text: "C0"},
						{Kind: protocol.Class, Name: "C1", Detail: "", Text: "C1"}}},
				{Kind: protocol.Variable, Name: "i", Detail: "var integer", Text: "var integer i := 1"},
				{Kind: protocol.Event, Name: "t1", Detail: "timer", Text: "timer t1 := 2.0"},
				{Kind: protocol.Interface, Name: "p", Detail: "port P", Text: "port P p"}}},
		{Kind: protocol.Method, Name: "f", Detail: "function definition",
			Text: "function f() runs on TestFunctionDefWithModuleDotRunsOn_Module_1.C0 system B0 return integer {}",
			Children: []Symbol{
				{Kind: protocol.Class, Name: "runs on", Detail: "TestFunctionDefWithModuleDotRunsOn_Module_1.C0", Text: "TestFunctionDefWithModuleDotRunsOn_Module_1.C0"},
				{Kind: protocol.Class, Name: "system", Detail: "B0", Text: "B0"},
				{Kind: protocol.Struct, Name: "return", Detail: "integer", Text: "integer"}}},

		{Kind: protocol.Class, Name: "C0", Detail: "component type", Text: "type component C0 {}"},
		{Kind: protocol.Class, Name: "C1", Detail: "component type", Text: "type component C1 {}"}}
	assert.Equal(t, want, testDocumentSymbol(t, input))
}

func TestRecordOfTypeDefWithTypeRef(t *testing.T) {
	input := `
	module Test
	{
		type integer Byte(0..255)
		type record of Byte Octets
	}`

	want := []Symbol{
		{Kind: protocol.Struct, Name: "Byte", Detail: "subtype",
			Text: "type integer Byte(0..255)",
		},
		{Kind: protocol.Array, Name: "Octets", Detail: "record of type",
			Text: "type record of Byte Octets",
			Children: []Symbol{
				{Kind: protocol.Struct, Name: "Byte", Detail: "element type", Text: "Byte"}}}}
	assert.Equal(t, want, testDocumentSymbol(t, input))
}

func TestConstTemplModulePar(t *testing.T) {
	input := `
	module Test
	{
		const R c_r := {10, true}
		modulepar R m_r := {10, true}
		template R t_r1 := ?;
		template R t_r2 := {10, true}
		template R t_r3(integer pi:=10) := {pi, true}
		template R t_r4 modifies t_r1 := {f2:= true}
	}`

	want := []Symbol{
		{Kind: protocol.Constant, Name: "c_r", Detail: "const R",
			Text: "const R c_r := {10, true}",
		},
		{Kind: protocol.Constant, Name: "m_r", Detail: "modulepar R",
			Text: "modulepar R m_r := {10, true}",
		},
		{Kind: protocol.Constant, Name: "t_r1", Detail: "template R",
			Text: "template R t_r1 := ?",
		},
		{Kind: protocol.Constant, Name: "t_r2", Detail: "template R",
			Text: "template R t_r2 := {10, true}",
		},
		{Kind: protocol.Constant, Name: "t_r3", Detail: "template R",
			Text: "template R t_r3(integer pi:=10) := {pi, true}",
		},
		{Kind: protocol.Constant, Name: "t_r4", Detail: "template R",
			Text: "template R t_r4 modifies t_r1 := {f2:= true}",
			Children: []Symbol{
				{Kind: protocol.Constant, Name: "t_r1", Detail: "template", Text: "t_r1"}}}}
	assert.Equal(t, want, testDocumentSymbol(t, input))
}

func TestPortTypeDecl(t *testing.T) {
	input := `
	module Test
	{
		type port Pmessage message {
			address AddressType;
			in aModule.Msg1, integer, Msg2;
			out Msg3, Msg4;
			inout Msg5;
			map param(PmessageMapType1 p1, PmessageMapType2 p2[]);
			unmap(PmessageUnmapType1 p1);
		}
	}`

	want := []Symbol{
		{Kind: protocol.Interface, Name: "Pmessage", Detail: "message port type",
			Text: "type port Pmessage message {\n\t\t\taddress AddressType;\n\t\t\tin aModule.Msg1, integer, Msg2;\n\t\t\tout Msg3, Msg4;\n\t\t\tinout Msg5;\n\t\t\tmap param(PmessageMapType1 p1, PmessageMapType2 p2[]);\n\t\t\tunmap(PmessageUnmapType1 p1);\n\t\t}",
			Children: []Symbol{
				{Kind: protocol.Struct, Name: "address", Detail: "AddressType type", Text: "address AddressType"},
				{Kind: protocol.Array, Name: "in", Detail: "",
					Text: "in aModule.Msg1, integer, Msg2",
					Children: []Symbol{
						{Kind: protocol.Struct, Name: "aModule.Msg1", Detail: "type", Text: "aModule.Msg1"},
						{Kind: protocol.Struct, Name: "integer", Detail: "type", Text: "integer"},
						{Kind: protocol.Struct, Name: "Msg2", Detail: "type", Text: "Msg2"}}},
				{Kind: protocol.Array, Name: "out", Detail: "",
					Text: "out Msg3, Msg4",
					Children: []Symbol{
						{Kind: protocol.Struct, Name: "Msg3", Detail: "type", Text: "Msg3"},
						{Kind: protocol.Struct, Name: "Msg4", Detail: "type", Text: "Msg4"}}},
				{Kind: protocol.Array, Name: "inout", Detail: "",
					Text: "inout Msg5",
					Children: []Symbol{
						{Kind: protocol.Struct, Name: "Msg5", Detail: "type", Text: "Msg5"}}}}}}
	assert.Equal(t, want, testDocumentSymbol(t, input))
}

func TestSignatureDecl(t *testing.T) {
	input := `
	module Test
	{
		signature MyRemoteProcOne ();
		signature MyRemoteProcTwo () noblock;
		signature MyRemoteProcThree (in integer Par1, out float Par2, inout integer Par3);
		signature MyRemoteProcFour (in integer Par1) return integer;
		signature MyRemoteProcFive (inout float Par1) return integer
			exception (ExceptionType1, ExceptionType2);
		signature MyRemoteProcSix (in integer Par1) noblock
			exception (integer, float);
	}`

	want := []Symbol{
		{Kind: protocol.Function, Name: "MyRemoteProcOne", Detail: "blocking signature",
			Text: "signature MyRemoteProcOne ()",
		},
		{Kind: protocol.Function, Name: "MyRemoteProcTwo", Detail: "non-blocking signature",
			Text: "signature MyRemoteProcTwo () noblock",
		},
		{Kind: protocol.Function, Name: "MyRemoteProcThree", Detail: "blocking signature",
			Text: "signature MyRemoteProcThree (in integer Par1, out float Par2, inout integer Par3)",
		},
		{Kind: protocol.Function, Name: "MyRemoteProcFour", Detail: "blocking signature",
			Text: "signature MyRemoteProcFour (in integer Par1) return integer",
			Children: []Symbol{
				{Kind: protocol.Struct, Name: "integer", Detail: "return type", Text: "integer"}}},
		{Kind: protocol.Function, Name: "MyRemoteProcFive", Detail: "blocking signature",
			Text: "signature MyRemoteProcFive (inout float Par1) return integer\n\t\t\texception (ExceptionType1, ExceptionType2)",
			Children: []Symbol{
				{Kind: protocol.Struct, Name: "integer", Detail: "return type", Text: "integer"},
				{Kind: protocol.Array, Name: "Exceptions", Detail: "",
					Text: "exception (ExceptionType1, ExceptionType2)",
					Children: []Symbol{
						{Kind: protocol.Struct, Name: "ExceptionType1", Detail: "type", Text: "ExceptionType1"},
						{Kind: protocol.Struct, Name: "ExceptionType2", Detail: "type", Text: "ExceptionType2"}}}}},
		{Kind: protocol.Function, Name: "MyRemoteProcSix", Detail: "non-blocking signature",
			Text: "signature MyRemoteProcSix (in integer Par1) noblock\n\t\t\texception (integer, float)",
			Children: []Symbol{
				{Kind: protocol.Array, Name: "Exceptions", Detail: "",
					Text: "exception (integer, float)",
					Children: []Symbol{
						{Kind: protocol.Struct, Name: "integer", Detail: "type", Text: "integer"},
						{Kind: protocol.Struct, Name: "float", Detail: "type", Text: "float"}}}}}}
	assert.Equal(t, want, testDocumentSymbol(t, input))
}

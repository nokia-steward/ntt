package lsp_test

import (
	"fmt"
	"testing"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/lsp"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/stretchr/testify/assert"
)

func TestModule(t *testing.T) {
	actual := testTokens(t, nil, `
        module Test {
               function f(integer x) {
	               return x;
	       }
	}`)
	assert.Equal(t, []Token{
		{Line: 2, Text: "Test", Type: lsp.Namespace, Mod: lsp.Definition},
		{Line: 3, Text: "f", Type: lsp.Function, Mod: lsp.Definition},
		{Line: 3, Text: "integer", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 3, Text: "x", Type: lsp.Parameter, Mod: lsp.Declaration},
		{Line: 4, Text: "x", Type: lsp.Parameter},
	}, actual)
}

func TestEnum(t *testing.T) {
	actual := testTokens(t, nil, `
        module Test {
	        type enumerated Enum { E1 }
	        type record R { enumerated { E2 } E /* E not colored*/ }
		control { log(E1, E2, R.E /* E not colored */) }
	}`)
	assert.Equal(t, []Token{
		{Line: 2, Text: "Test", Type: lsp.Namespace, Mod: lsp.Definition},
		{Line: 3, Text: "Enum", Type: lsp.Enum, Mod: lsp.Definition},
		{Line: 3, Text: "E1", Type: lsp.EnumMember, Mod: lsp.Definition},
		{Line: 4, Text: "R", Type: lsp.Struct, Mod: lsp.Definition},
		{Line: 4, Text: "E2", Type: lsp.EnumMember, Mod: lsp.Definition},
		{Line: 5, Text: "log", Type: lsp.Function, Mod: lsp.DefaultLibrary},
		{Line: 5, Text: "E1", Type: lsp.EnumMember},
		{Line: 5, Text: "E2", Type: lsp.EnumMember},
		{Line: 5, Text: "R", Type: lsp.Struct},
	}, actual)
}

func TestEmbeddedFields(t *testing.T) {
	actual := testTokens(t, nil, `
        module Test {
	        type record R { B, integer }
	        type record B { integer x }
	}`)
	assert.Equal(t, []Token{
		{Line: 2, Text: "Test", Type: lsp.Namespace, Mod: lsp.Definition},
		{Line: 3, Text: "R", Type: lsp.Struct, Mod: lsp.Definition},
		{Line: 3, Text: "B", Type: lsp.Struct},
		{Line: 3, Text: "integer", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 4, Text: "B", Type: lsp.Struct, Mod: lsp.Definition},
		{Line: 4, Text: "integer", Type: lsp.Type, Mod: lsp.DefaultLibrary},
	}, actual)
}

func TestComponentDef(t *testing.T) {
	actual := testTokens(t, nil, `
        module Test
        {
                import from TestComponentDef_Module_1 all;
                type component B0 extends C0, TestComponentDef_Module_1.C1 {
                        var integer i := 1;
                        timer t1 := 2.0;
                        port P p;
                }
                function f() runs on TestComponentDef_Module_1.C0 system B0 return integer {}
        }
        module TestComponentDef_Module_1
        {
                type component C0 {}
                type component C1 {}
                type port P message {
                    inout charstring
                }
        }`)
	assert.Equal(t, []Token{
		{Line: 2, Text: "Test", Type: lsp.Namespace, Mod: lsp.Definition},
		{Line: 4, Text: "TestComponentDef_Module_1", Type: lsp.Namespace},
		{Line: 5, Text: "B0", Type: lsp.Class, Mod: lsp.Definition},
		{Line: 5, Text: "C0", Type: lsp.Class},
		{Line: 5, Text: "TestComponentDef_Module_1", Type: lsp.Namespace},
		{Line: 5, Text: "C1", Type: lsp.Class},
		{Line: 6, Text: "integer", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 6, Text: "i", Type: lsp.Property, Mod: lsp.Declaration},
		{Line: 7, Text: "t1", Type: lsp.Property, Mod: lsp.Declaration},
		{Line: 8, Text: "P", Type: lsp.Interface},
		{Line: 8, Text: "p", Type: lsp.Interface, Mod: lsp.Declaration},
		{Line: 10, Text: "f", Type: lsp.Function, Mod: lsp.Definition},
		{Line: 10, Text: "TestComponentDef_Module_1", Type: lsp.Namespace},
		{Line: 10, Text: "C0", Type: lsp.Class},
		{Line: 10, Text: "B0", Type: lsp.Class},
		{Line: 10, Text: "integer", Type: lsp.Type, Mod: lsp.DefaultLibrary},

		{Line: 12, Text: "TestComponentDef_Module_1", Type: lsp.Namespace, Mod: lsp.Definition},
		{Line: 14, Text: "C0", Type: lsp.Class, Mod: lsp.Definition},
		{Line: 15, Text: "C1", Type: lsp.Class, Mod: lsp.Definition},
		{Line: 16, Text: "P", Type: lsp.Interface, Mod: lsp.Definition},
		{Line: 17, Text: "charstring", Type: lsp.Type, Mod: lsp.DefaultLibrary},
	}, actual)
}

func TestFullModuleKwTypeId(t *testing.T) {
	actual := testTokens(t, nil, `
        module Test
        {
                type record MyRec {
                        integer i
                }
                type record length(0..2) of integer RoI;
                type set MySet {
                        integer i
                }
                function f() return integer {}
        }`)

	assert.Equal(t, []Token{
		{Line: 2, Text: "Test", Type: lsp.Namespace, Mod: lsp.Definition},
		{Line: 4, Text: "MyRec", Type: lsp.Struct, Mod: lsp.Definition},
		{Line: 5, Text: "integer", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 7, Text: "integer", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 7, Text: "RoI", Type: lsp.Type, Mod: lsp.Definition},
		{Line: 8, Text: "MySet", Type: lsp.Struct, Mod: lsp.Definition},
		{Line: 9, Text: "integer", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 11, Text: "f", Type: lsp.Function, Mod: lsp.Definition},
		{Line: 11, Text: "integer", Type: lsp.Type, Mod: lsp.DefaultLibrary},
	}, actual)
}

func TestConstAndTemplDecl(t *testing.T) {
	actual := testTokens(t, nil, `
        module Test
        {
                type integer X;
                type X Y;
                type record MyRec {
                        integer i
                }
                const float pi := 3.14;
                template MyRec a_rec1 := *;
                template (value) MyRec a_rec2(template integer p_i) := {
                        i := p_i
                }
        }`)

	assert.Equal(t, []Token{
		{Line: 2, Text: "Test", Type: lsp.Namespace, Mod: lsp.Definition},
		{Line: 4, Text: "integer", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 4, Text: "X", Type: lsp.Type, Mod: lsp.Definition},
		{Line: 5, Text: "X", Type: lsp.Type},
		{Line: 5, Text: "Y", Type: lsp.Type, Mod: lsp.Definition},
		{Line: 6, Text: "MyRec", Type: lsp.Struct, Mod: lsp.Definition},
		{Line: 7, Text: "integer", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 9, Text: "float", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 9, Text: "pi", Type: lsp.Variable, Mod: lsp.Declaration | lsp.Readonly},
		{Line: 10, Text: "MyRec", Type: lsp.Struct},
		{Line: 10, Text: "a_rec1", Type: lsp.Variable, Mod: lsp.Declaration | lsp.Readonly},
		{Line: 11, Text: "MyRec", Type: lsp.Struct},
		{Line: 11, Text: "a_rec2", Type: lsp.Variable, Mod: lsp.Declaration | lsp.Readonly},
		{Line: 11, Text: "integer", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 11, Text: "p_i", Type: lsp.Parameter, Mod: lsp.Declaration},
		{Line: 12, Text: "p_i", Type: lsp.Parameter},
	}, actual)
}

func TestModuleIds(t *testing.T) {
	actual := testTokens(t, nil, `
        module Test
        {
                import from TestModuleIds_Module_1 all;
                type TestModuleIds_Module_1.Byte MyByte;
                function f(TestModuleIds_Module_1.Byte pb) {
                        var TestModuleIds_Module_1.MyRec r;
                        r.i := pb;
                }
        }
        module TestModuleIds_Module_1
        {
                type integer Byte;
                type record MyRec {integer i};
        }`)

	assert.Equal(t, []Token{
		{Line: 2, Text: "Test", Type: lsp.Namespace, Mod: lsp.Definition},
		{Line: 4, Text: "TestModuleIds_Module_1", Type: lsp.Namespace},
		{Line: 5, Text: "TestModuleIds_Module_1", Type: lsp.Namespace},
		{Line: 5, Text: "Byte", Type: lsp.Type},
		{Line: 5, Text: "MyByte", Type: lsp.Type, Mod: lsp.Definition},
		{Line: 6, Text: "f", Type: lsp.Function, Mod: lsp.Definition},
		{Line: 6, Text: "TestModuleIds_Module_1", Type: lsp.Namespace},
		{Line: 6, Text: "Byte", Type: lsp.Type},
		{Line: 6, Text: "pb", Type: lsp.Parameter, Mod: lsp.Declaration},
		{Line: 7, Text: "TestModuleIds_Module_1", Type: lsp.Namespace},
		{Line: 7, Text: "MyRec", Type: lsp.Struct},
		{Line: 7, Text: "r", Type: lsp.Variable, Mod: lsp.Declaration},
		{Line: 8, Text: "r", Type: lsp.Variable},
		{Line: 8, Text: "pb", Type: lsp.Parameter},

		{Line: 11, Text: "TestModuleIds_Module_1", Type: lsp.Namespace, Mod: lsp.Definition},
		{Line: 13, Text: "integer", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 13, Text: "Byte", Type: lsp.Type, Mod: lsp.Definition},
		{Line: 14, Text: "MyRec", Type: lsp.Struct, Mod: lsp.Definition},
		{Line: 14, Text: "integer", Type: lsp.Type, Mod: lsp.DefaultLibrary},
	}, actual)
}

func TestModuleAlias(t *testing.T) {
	actual := testTokens(t, nil, `
        module Test
        {
                import from TestModuleAlias_Module_1 -> M1 all;
                type M1.Byte MyByte;
                const universal charstring c := M1.Byte.encode;
        }
        module TestModuleAlias_Module_1
        {
                type integer Byte;
        }`)

	assert.Equal(t, []Token{
		{Line: 2, Text: "Test", Type: lsp.Namespace, Mod: lsp.Definition},
		{Line: 4, Text: "TestModuleAlias_Module_1", Type: lsp.Namespace},
		{Line: 4, Text: "M1", Type: lsp.Namespace, Mod: lsp.Declaration},
		{Line: 5, Text: "M1", Type: lsp.Namespace},
		{Line: 5, Text: "Byte", Type: lsp.Type},
		{Line: 5, Text: "MyByte", Type: lsp.Type, Mod: lsp.Definition},
		{Line: 6, Text: "universal", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 6, Text: "charstring", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 6, Text: "c", Type: lsp.Variable, Mod: lsp.Declaration | lsp.Readonly},
		{Line: 6, Text: "M1", Type: lsp.Namespace},
		{Line: 6, Text: "Byte", Type: lsp.Type},

		{Line: 8, Text: "TestModuleAlias_Module_1", Type: lsp.Namespace, Mod: lsp.Definition},
		{Line: 10, Text: "integer", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 10, Text: "Byte", Type: lsp.Type, Mod: lsp.Definition},
	}, actual)
}

func TestDeltaModuleKwTypeId(t *testing.T) {
	actual := testTokens(t, SetRange(7, 1, 10, 17), `
        module Test
        {
                type record MyRec {
                        integer i
                }
                type record length(0..2) of integer RoI;
                type set MySet {
                        integer i
                }
                function f() return integer {}
        }`)

	assert.Equal(t, []Token{
		{Line: 7, Text: "integer", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 7, Text: "RoI", Type: lsp.Type, Mod: lsp.Definition},
		{Line: 8, Text: "MySet", Type: lsp.Struct, Mod: lsp.Definition},
		{Line: 9, Text: "integer", Type: lsp.Type, Mod: lsp.DefaultLibrary},
	}, actual)
}

func TestUniversalCharstring(t *testing.T) {
	actual := testTokens(t, nil, `
        module Test
        {
                type record MyRec {
                        universal /* foo */ charstring  uch,
                        charstring ch
                }
                type universal charstring UcharT;
                function f() return universal charstring {}
        }`)

	assert.Equal(t, []Token{
		{Line: 2, Text: "Test", Type: lsp.Namespace, Mod: lsp.Definition},
		{Line: 4, Text: "MyRec", Type: lsp.Struct, Mod: lsp.Definition},
		{Line: 5, Text: "universal", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 5, Text: "charstring", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 6, Text: "charstring", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 8, Text: "universal", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 8, Text: "charstring", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 8, Text: "UcharT", Type: lsp.Type, Mod: lsp.Definition},
		{Line: 9, Text: "f", Type: lsp.Function, Mod: lsp.Definition},
		{Line: 9, Text: "universal", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 9, Text: "charstring", Type: lsp.Type, Mod: lsp.DefaultLibrary},
	}, actual)
}

func TestPortTypeDeclSemTok(t *testing.T) {
	actual := testTokens(t, nil, `
        module Test
        {
                import from TestPortTypeDeclSemTok_Module_1 all;
                type charstring AddrType;
                type boolean Type1;
                type port MyPort message {
                        in integer, universal charstring
                        out Type1, TestPortTypeDeclSemTok_Module_1.Type2
                        map param (in integer pi, boolean pb);
                        address AddrType
                }
        }
        module TestPortTypeDeclSemTok_Module_1
        {
                type integer Type2;
        }`)

	assert.Equal(t, []Token{
		{Line: 2, Text: "Test", Type: lsp.Namespace, Mod: lsp.Definition},
		{Line: 4, Text: "TestPortTypeDeclSemTok_Module_1", Type: lsp.Namespace},
		{Line: 5, Text: "charstring", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 5, Text: "AddrType", Type: lsp.Type, Mod: lsp.Definition},
		{Line: 6, Text: "boolean", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 6, Text: "Type1", Type: lsp.Type, Mod: lsp.Definition},
		{Line: 7, Text: "MyPort", Type: lsp.Interface, Mod: lsp.Definition},
		{Line: 8, Text: "integer", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 8, Text: "universal", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 8, Text: "charstring", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 9, Text: "Type1", Type: lsp.Type},
		{Line: 9, Text: "TestPortTypeDeclSemTok_Module_1", Type: lsp.Namespace},
		{Line: 9, Text: "Type2", Type: lsp.Type},
		{Line: 10, Text: "integer", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 10, Text: "pi", Type: lsp.Parameter, Mod: lsp.Declaration},
		{Line: 10, Text: "boolean", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 10, Text: "pb", Type: lsp.Parameter, Mod: lsp.Declaration},
		{Line: 11, Text: "AddrType", Type: lsp.Type},

		{Line: 14, Text: "TestPortTypeDeclSemTok_Module_1", Type: lsp.Namespace, Mod: lsp.Definition},
		{Line: 16, Text: "integer", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 16, Text: "Type2", Type: lsp.Type, Mod: lsp.Definition},
	}, actual)
}

func TestFunctionParams(t *testing.T) {
	actual := testTokens(t, nil, `
        module Test
        {
                import from TestFunctionParams_Module_1 all;

                function f(charstring p_ch) return template charstring {
                        var R v_r1 := {p_ch};
                        var template R t_r2 := {ch1 := p_ch};
                        var charstring ch := p_ch;
                        ch := p_ch;
                        v_r1.ch1 := p_ch;
                        t_r2 := {p_ch};
                        t_r2 := {ch1 := p_ch};
                        f(p_ch := p_ch);
                        return R:{p_ch}
                }
        }
        module TestFunctionParams_Module_1
        {
                type record R {
                        charstring ch1
                };
        }`)

	assert.Equal(t, []Token{
		{Line: 2, Text: "Test", Type: lsp.Namespace, Mod: lsp.Definition},
		{Line: 4, Text: "TestFunctionParams_Module_1", Type: lsp.Namespace},
		{Line: 6, Text: "f", Type: lsp.Function, Mod: lsp.Definition},
		{Line: 6, Text: "charstring", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 6, Text: "p_ch", Type: lsp.Parameter, Mod: lsp.Declaration},
		{Line: 6, Text: "charstring", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 7, Text: "R", Type: lsp.Struct},
		{Line: 7, Text: "v_r1", Type: lsp.Variable, Mod: lsp.Declaration},
		{Line: 7, Text: "p_ch", Type: lsp.Parameter},
		{Line: 8, Text: "R", Type: lsp.Struct},
		{Line: 8, Text: "t_r2", Type: lsp.Variable, Mod: lsp.Declaration},
		{Line: 8, Text: "p_ch", Type: lsp.Parameter},
		{Line: 9, Text: "charstring", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 9, Text: "ch", Type: lsp.Variable, Mod: lsp.Declaration},
		{Line: 9, Text: "p_ch", Type: lsp.Parameter},
		{Line: 10, Text: "ch", Type: lsp.Variable},
		{Line: 10, Text: "p_ch", Type: lsp.Parameter},
		{Line: 11, Text: "v_r1", Type: lsp.Variable},
		{Line: 11, Text: "p_ch", Type: lsp.Parameter},
		{Line: 12, Text: "t_r2", Type: lsp.Variable},
		{Line: 12, Text: "p_ch", Type: lsp.Parameter},
		{Line: 13, Text: "t_r2", Type: lsp.Variable},
		{Line: 13, Text: "p_ch", Type: lsp.Parameter},
		{Line: 14, Text: "f", Type: lsp.Function},
		{Line: 14, Text: "p_ch", Type: lsp.Parameter},
		{Line: 14, Text: "p_ch", Type: lsp.Parameter},
		{Line: 15, Text: "R", Type: lsp.Struct},
		{Line: 15, Text: "p_ch", Type: lsp.Parameter},

		{Line: 18, Text: "TestFunctionParams_Module_1", Type: lsp.Namespace, Mod: lsp.Definition},
		{Line: 20, Text: "R", Type: lsp.Struct, Mod: lsp.Definition},
		{Line: 21, Text: "charstring", Type: lsp.Type, Mod: lsp.DefaultLibrary},
	}, actual)
}

func TestTemplateAndConst(t *testing.T) {
	actual := testTokens(t, nil, `
        module Test
        {
                import from TestTemplateAndConst_Module_1 all;

                const charstring c_ch2 := c_ch;
                template charstring a_ch2 := a_ch;
                template R a_r2(template charstring p_ch := a_ch) modifies a_r := {
                        ch1 := c_ch,
                        ch2 := p_ch
                }
                template R a_r3 := a_r(p_ch := a_ch2);
        }
        module TestTemplateAndConst_Module_1
        {
                type record R {
                        charstring ch1,
                        charstring ch2 optional
                };
                const charstring c_ch := "xx";
                template charstring a_ch := ?;
                template (omit) R a_r(template charstring p_ch) := {
                        ch1 := p_ch,
                        ch2 := omit
                }
        }`)

	assert.Equal(t, []Token{
		{Line: 2, Text: "Test", Type: lsp.Namespace, Mod: lsp.Definition},
		{Line: 4, Text: "TestTemplateAndConst_Module_1", Type: lsp.Namespace},
		{Line: 6, Text: "charstring", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 6, Text: "c_ch2", Type: lsp.Variable, Mod: lsp.Declaration | lsp.Readonly},
		{Line: 6, Text: "c_ch", Type: lsp.Variable, Mod: lsp.Readonly},
		{Line: 7, Text: "charstring", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 7, Text: "a_ch2", Type: lsp.Variable, Mod: lsp.Declaration | lsp.Readonly},
		{Line: 7, Text: "a_ch", Type: lsp.Variable, Mod: lsp.Readonly},
		{Line: 8, Text: "R", Type: lsp.Struct},
		{Line: 8, Text: "a_r2", Type: lsp.Variable, Mod: lsp.Declaration | lsp.Readonly},
		{Line: 8, Text: "charstring", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 8, Text: "p_ch", Type: lsp.Parameter, Mod: lsp.Declaration},
		{Line: 8, Text: "a_ch", Type: lsp.Variable, Mod: lsp.Readonly},
		{Line: 8, Text: "a_r", Type: lsp.Variable, Mod: lsp.Readonly},
		{Line: 9, Text: "c_ch", Type: lsp.Variable, Mod: lsp.Readonly},
		{Line: 10, Text: "p_ch", Type: lsp.Parameter},
		{Line: 12, Text: "R", Type: lsp.Struct},
		{Line: 12, Text: "a_r3", Type: lsp.Variable, Mod: lsp.Declaration | lsp.Readonly},
		{Line: 12, Text: "a_r", Type: lsp.Variable, Mod: lsp.Readonly},
		{Line: 12, Text: "p_ch", Type: lsp.Parameter},
		{Line: 12, Text: "a_ch2", Type: lsp.Variable, Mod: lsp.Readonly},

		{Line: 14, Text: "TestTemplateAndConst_Module_1", Type: lsp.Namespace, Mod: lsp.Definition},
		{Line: 16, Text: "R", Type: lsp.Struct, Mod: lsp.Definition},
		{Line: 17, Text: "charstring", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 18, Text: "charstring", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 20, Text: "charstring", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 20, Text: "c_ch", Type: lsp.Variable, Mod: lsp.Declaration | lsp.Readonly},
		{Line: 21, Text: "charstring", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 21, Text: "a_ch", Type: lsp.Variable, Mod: lsp.Declaration | lsp.Readonly},
		{Line: 22, Text: "R", Type: lsp.Struct},
		{Line: 22, Text: "a_r", Type: lsp.Variable, Mod: lsp.Declaration | lsp.Readonly},
		{Line: 22, Text: "charstring", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 22, Text: "p_ch", Type: lsp.Parameter, Mod: lsp.Declaration},
		{Line: 23, Text: "p_ch", Type: lsp.Parameter},
	}, actual)
}

func TestComponentVars(t *testing.T) {
	actual := testTokens(t, nil, `
        module Test
        {
                import from TestComponentVars_Module_1 all;
                testcase tc() runs on C3 system C0 {
                        vC0_C1 := C0.create();
                        vi_C2 := 1;
                        log(vi_C2);
                        p0_C0.send("hello");
                        vC0_C1.start(f(p_i := vi_C2));
                }
        }
        module TestComponentVars_Module_1
        {
                type component C0 {
                        port P p0_C0;
                }
                type component C1 {
                        var C0 vC0_C1;
                }
                type component C2 extends C1 {
                        var integer vi_C2;
                }
                type component C3 extends C2, C0 {}
                type port P message {
                        inout charstring
                }
                function f(integer p_i) runs on C0 {}
        }`)

	assert.Equal(t, []Token{
		{Line: 2, Text: "Test", Type: lsp.Namespace, Mod: lsp.Definition},
		{Line: 4, Text: "TestComponentVars_Module_1", Type: lsp.Namespace},
		{Line: 5, Text: "tc", Type: lsp.Function, Mod: lsp.Definition},
		{Line: 5, Text: "C3", Type: lsp.Class},
		{Line: 5, Text: "C0", Type: lsp.Class},
		{Line: 6, Text: "vC0_C1", Type: lsp.Property},
		{Line: 6, Text: "C0", Type: lsp.Class},
		{Line: 7, Text: "vi_C2", Type: lsp.Property},
		{Line: 8, Text: "log", Type: lsp.Function, Mod: lsp.DefaultLibrary},
		{Line: 8, Text: "vi_C2", Type: lsp.Property},
		{Line: 9, Text: "p0_C0", Type: lsp.Interface},
		{Line: 10, Text: "vC0_C1", Type: lsp.Property},
		{Line: 10, Text: "f", Type: lsp.Function},
		{Line: 10, Text: "p_i", Type: lsp.Parameter},
		{Line: 10, Text: "vi_C2", Type: lsp.Property},

		{Line: 13, Text: "TestComponentVars_Module_1", Type: lsp.Namespace, Mod: lsp.Definition},
		{Line: 15, Text: "C0", Type: lsp.Class, Mod: lsp.Definition},
		{Line: 16, Text: "P", Type: lsp.Interface},
		{Line: 16, Text: "p0_C0", Type: lsp.Interface, Mod: lsp.Declaration},
		{Line: 18, Text: "C1", Type: lsp.Class, Mod: lsp.Definition},
		{Line: 19, Text: "C0", Type: lsp.Class},
		{Line: 19, Text: "vC0_C1", Type: lsp.Property, Mod: lsp.Declaration},
		{Line: 21, Text: "C2", Type: lsp.Class, Mod: lsp.Definition},
		{Line: 21, Text: "C1", Type: lsp.Class},
		{Line: 22, Text: "integer", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 22, Text: "vi_C2", Type: lsp.Property, Mod: lsp.Declaration},
		{Line: 24, Text: "C3", Type: lsp.Class, Mod: lsp.Definition},
		{Line: 24, Text: "C2", Type: lsp.Class},
		{Line: 24, Text: "C0", Type: lsp.Class},
		{Line: 25, Text: "P", Type: lsp.Interface, Mod: lsp.Definition},
		{Line: 26, Text: "charstring", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 28, Text: "f", Type: lsp.Function, Mod: lsp.Definition},
		{Line: 28, Text: "integer", Type: lsp.Type, Mod: lsp.DefaultLibrary},
		{Line: 28, Text: "p_i", Type: lsp.Parameter, Mod: lsp.Declaration},
		{Line: 28, Text: "C0", Type: lsp.Class},
	}, actual)
}

type Token struct {
	Line int
	Text string
	Type lsp.SemanticTokenType
	Mod  lsp.SemanticTokenModifiers
}

func testTokens(t *testing.T, rng *protocol.Range, text string) []Token {
	t.Helper()

	file := fmt.Sprintf("%s.ttcn3", t.Name())
	fs.SetContent(file, []byte(text))
	tree := ttcn3.ParseFile(file)
	if tree.Err != nil {
		t.Fatal(tree.Err)
	}

	// Build index to for tree.Lookup to resolve imported symbols.
	db := &ttcn3.DB{}
	db.Index(file)

	begin := tree.Pos()
	end := tree.End()
	if rng != nil {
		begin = tree.PosFor(int(rng.Start.Line), int(rng.Start.Character))
		end = tree.PosFor(int(rng.End.Line), int(rng.End.Character))
	}

	var (
		toks []Token
		line = 1
		col  = 1
	)
	list := lsp.SemanticTokens(tree, db, begin, end)
	for i := 0; i < len(list); i += 5 {
		line += int(list[i])
		if list[i] != 0 {
			col = 1
		}
		col += int(list[i+1])
		pos := tree.PosFor(line, col)
		toks = append(toks, Token{
			Line: line,
			Text: Substr(text, pos, pos+int(list[i+2])),
			Type: lsp.SemanticTokenType(list[i+3]),
			Mod:  lsp.SemanticTokenModifiers(list[i+4]),
		})
	}
	return toks
}

func Substr(s string, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end > len(s) {
		end = len(s)
	}
	return s[start:end]
}

func SetRange(startLine uint32, startCol uint32, endLine uint32, endCol uint32) *protocol.Range {
	return &protocol.Range{
		Start: protocol.Position{Line: startLine, Character: startCol},
		End:   protocol.Position{Line: endLine, Character: endCol}}
}

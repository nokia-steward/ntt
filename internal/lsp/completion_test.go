package lsp_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/lsp"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/internal/ntttest"
	"github.com/nokia/ntt/project"
	"github.com/nokia/ntt/ttcn3"
	"github.com/stretchr/testify/assert"
)

var (
	predefMap map[string]bool
)

func init() {
	predefMap = make(map[string]bool)
	for _, def := range lsp.PredefinedFunctions {
		predefMap[def.Label] = true
	}

}

func testCompletion(t *testing.T, input string, imports ...string) []protocol.CompletionItem {
	name := func(i int) string {
		return fmt.Sprintf("%s_Module_%d.ttcn3", t.Name(), i)
	}

	suite := &lsp.Suite{
		Config: &project.Config{},
		DB:     &ttcn3.DB{},
	}

	str, cursor := ntttest.CutCursor(input)

	fs.SetContent(name(0), []byte(str))
	suite.Config.Sources = append(suite.Config.Sources, name(0))
	for i, str := range imports {
		fs.SetContent(name(i+1), []byte(str))
		suite.Config.Sources = append(suite.Config.Sources, name(i+1))
	}
	suite.DB.Index(suite.Config.Sources...)

	syntax := ttcn3.ParseFile(name(0))
	nodeStack := lsp.LastNonWsToken(syntax.Root, cursor)
	basename := name(0)
	basename = basename[:len(basename)-len(filepath.Ext(basename))]

	var items []protocol.CompletionItem
	for _, item := range lsp.Complete(suite, cursor, nodeStack, basename) {
		if predefMap[item.Label] {
			continue
		}
		items = append(items, item)
	}

	return items
}

func TestLastNonWsToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"module M {}", nil},

		// {"¶", []string{
		// 	"*syntax.Root",
		// }},

		// {"¶ module A {}", []string{
		// 	"*syntax.Root",
		// }},

		{"module ¶ module A {}", []string{
			"*syntax.Root",
			"*syntax.Module",
		}},

		{"import from ¶", []string{
			"*syntax.Root",
			"*syntax.ModuleDef",
			"*syntax.ImportDecl",
		}},

		{"import from ¶ all", []string{
			"*syntax.Root",
			"*syntax.ModuleDef",
			"*syntax.ImportDecl",
		}},

		{"import from ¶all", []string{
			"*syntax.Root",
			"*syntax.ModuleDef",
			"*syntax.ImportDecl",
		}},

		{"import from ¶ const", []string{
			"*syntax.Root",
			"*syntax.ModuleDef",
			"*syntax.ImportDecl",
		}},

		{"import from ¶const", []string{
			"*syntax.Root",
			"*syntax.ModuleDef",
			"*syntax.ImportDecl",
		}},

		{"import from  ¶ foo", []string{
			"*syntax.Root",
			"*syntax.ModuleDef",
			"*syntax.ImportDecl",
		}},

		{"import from  ¶foo", []string{
			"*syntax.Root",
			"*syntax.ModuleDef",
			"*syntax.ImportDecl",
			"*syntax.Ident(foo)",
		}},

		{"import ¶ from", []string{
			"*syntax.Root",
			"*syntax.ModuleDef",
			"*syntax.ImportDecl",
		}},

		{"import from A { modulepar ¶}", []string{
			"*syntax.Root",
			"*syntax.ModuleDef",
			"*syntax.ImportDecl",
			"*syntax.DefKindExpr",
			"*syntax.ErrorNode",
		}},

		{"import from A { modulepar ¶", []string{
			"*syntax.Root",
			"*syntax.ModuleDef",
			"*syntax.ImportDecl",
			"*syntax.DefKindExpr",
			"*syntax.ErrorNode",
		}},
	}

	for _, tt := range tests {
		input, cursor := ntttest.CutCursor(tt.input)
		tree := ttcn3.Parse(input)
		var got []string
		for _, n := range lsp.LastNonWsToken(tree.Root, cursor) {
			got = append(got, ntttest.NodeString(n))
		}
		assert.Equal(t, tt.want, got, fmt.Sprintf("%q", tt.input))
	}
}

// Completion within Import statement.
// TODO: func TestImportTypes(t *testing.T) {}
// TODO: func TestImportTypesCtrlSpc(t *testing.T) {}

func TestImportModulenamesCtrlSpc(t *testing.T) {
	list := testCompletion(t, `module Test
	{
		import from ¶
		import from A all;
	  }`, `module A
	  {}`, `module B
	  {}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "TestImportModulenamesCtrlSpc_Module_1", Kind: protocol.ModuleCompletion, SortText: " TestImportModulenamesCtrlSpc_Module_1"},
		{Label: "TestImportModulenamesCtrlSpc_Module_2", Kind: protocol.ModuleCompletion, SortText: " TestImportModulenamesCtrlSpc_Module_2"}}, list)
}

func TestImportModulenames(t *testing.T) {
	list := testCompletion(t, `module Test
	{
		import from Tes¶
		import from A all;
	  }`, `module A
	  {}`, `module B
	  {}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "TestImportModulenames_Module_1", Kind: protocol.ModuleCompletion, SortText: " TestImportModulenames_Module_1"},
		{Label: "TestImportModulenames_Module_2", Kind: protocol.ModuleCompletion, SortText: " TestImportModulenames_Module_2"}}, list)

}

func TestImportModuleAlias(t *testing.T) {
	list := testCompletion(t, `module Test
	{
		import from A -> Tes¶
		import from A all;
	  }`, `module A
	  {}`, `module B
	  {}`)

	assert.Nil(t, list)
}

func TestImportBehavioursCtrlSpc(t *testing.T) {
	list := testCompletion(t, `module Test
    {
        import from TestImportBehavioursCtrlSpc_Module_1 {
            altstep ¶  }
		import from A all;
	  }`, `module TestImportBehavioursCtrlSpc_Module_1
      {
		  altstep a1() {}
		  altstep a2() {}
	  }`, `module B
	  {}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "a1", Kind: protocol.FunctionCompletion},
		{Label: "a2", Kind: protocol.FunctionCompletion},
		{Label: "all;", Kind: protocol.KeywordCompletion}}, list)
}

func TestImportBehaviours(t *testing.T) {
	list := testCompletion(t, `module Test
    {
        import from TestImportBehaviours_Module_1 {
            testcase ¶t}
		import from A all;
	  }`, `module TestImportBehaviours_Module_1
      {
		  testcase tc1() runs on C0 {}
		  testcase tc2() runs on C0 {}
	  }`, `module B
	  {}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "tc1", Kind: protocol.FunctionCompletion},
		{Label: "tc2", Kind: protocol.FunctionCompletion},
		{Label: "all;", Kind: protocol.KeywordCompletion}}, list)
}

func TestImportTemplates(t *testing.T) {
	list := testCompletion(t, `module Test
    {
        import from TestImportTemplates_Module_1 {
            template a¶_}
		import from A all;
	  }`, `module TestImportTemplates_Module_1
      {
		  template charstring a_name := "Justus"
		  type record R1 {integer f1, boolean f2}
		  template R1 a_r1(boolean b) := {f1 := 10, f2 := b}
		  function f1() {template integer a_local_int := 0;}
	  }`, `module B
	  {}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "a_name", Kind: protocol.ConstantCompletion},
		{Label: "a_r1", Kind: protocol.ConstantCompletion},
		{Label: "all;", Kind: protocol.KeywordCompletion}}, list)
}

func TestImportTemplatesCtrlSpc(t *testing.T) {
	list := testCompletion(t, `module Test
    {
        import from TestImportTemplatesCtrlSpc_Module_1 {
            template¶ }
		import from A all;
	  }`, `module TestImportTemplatesCtrlSpc_Module_1
      {
		  template charstring a_name := "Justus"
		  type record R1 {integer f1, boolean f2}
		  template R1 a_r1(boolean b) := {f1 := 10, f2 := b}
	  }`, `module B
	  {}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "a_name", Kind: protocol.ConstantCompletion},
		{Label: "a_r1", Kind: protocol.ConstantCompletion},
		{Label: "all;", Kind: protocol.KeywordCompletion}}, list)
}

func TestImportConstantsCtrlSpc(t *testing.T) {
	list := testCompletion(t, `module Test
    {
        import from TestImportConstantsCtrlSpc_Module_1 {
            const¶ }
		import from A all;
	  }`, `module TestImportConstantsCtrlSpc_Module_1
      {
		  const charstring c_name := "Justus"
		  type record R1 {integer f1, boolean f2}
		  const R1 c_r1 := {f1 := 10, f2 := false}
		  function f1() { const integer c_localInt := 0;}
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "c_name", Kind: protocol.ConstantCompletion},
		{Label: "c_r1", Kind: protocol.ConstantCompletion},
		{Label: "all;", Kind: protocol.KeywordCompletion}}, list)
}

func TestImportExceptConstantsCtrlSpc(t *testing.T) {
	list := testCompletion(t, `module Test
    {
        import from TestImportExceptConstantsCtrlSpc_Module_1 all except {
            const¶ }
		import from A all;
	  }`, `module TestImportExceptConstantsCtrlSpc_Module_1
      {
		  const charstring c_name := "Justus"
		  type record R1 {integer f1, boolean f2}
		  const R1 c_r1 := {f1 := 10, f2 := false}
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "c_name", Kind: protocol.ConstantCompletion},
		{Label: "c_r1", Kind: protocol.ConstantCompletion},
		{Label: "all;", Kind: protocol.KeywordCompletion}}, list)
}

func TestImportExceptConstants(t *testing.T) {
	list := testCompletion(t, `module Test
    {
        import from TestImportExceptConstants_Module_1 all except {
            const c¶_}
		import from A all;
	  }`, `module TestImportExceptConstants_Module_1
      {
		  const charstring c_name := "Justus"
		  type record R1 {integer f1, boolean f2}
		  const R1 c_r1 := {f1 := 10, f2 := false}
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "c_name", Kind: protocol.ConstantCompletion},
		{Label: "c_r1", Kind: protocol.ConstantCompletion},
		{Label: "all;", Kind: protocol.KeywordCompletion}}, list)
}

func TestImportModuleparsCtrlSpc(t *testing.T) {
	list := testCompletion(t, `module Test
    {
        import from TestImportModuleparsCtrlSpc_Module_1 {
            modulepar }¶
		import from A all;
	  }`, `module TestImportModuleparsCtrlSpc_Module_1
      {
		  modulepar charstring m_name := "Justus"
		  type record R1 {integer f1, boolean f2}
		  modulepar R1 m_r1 := {f1 := 10, f2 := true}
		  const integer c_int := 2;
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "m_name", Kind: protocol.ConstantCompletion},
		{Label: "m_r1", Kind: protocol.ConstantCompletion},
		{Label: "all;", Kind: protocol.KeywordCompletion}}, list)
}

func TestImportExceptModuleparsCtrlSpc(t *testing.T) {
	list := testCompletion(t, `module Test
    {
        import from TestImportExceptModuleparsCtrlSpc_Module_1 all except {
            modulepar¶ }
		import from A all;
	  }`, `module TestImportExceptModuleparsCtrlSpc_Module_1
      {
		  modulepar charstring m_name := "Justus"
		  type record R1 {integer f1, boolean f2}
		  modulepar R1 m_r1 := {f1 := 10, f2 := false}
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "m_name", Kind: protocol.ConstantCompletion},
		{Label: "m_r1", Kind: protocol.ConstantCompletion},
		{Label: "all;", Kind: protocol.KeywordCompletion}}, list)
}

func TestImportExceptModulepars(t *testing.T) {
	list := testCompletion(t, `module Test
    {
        import from TestImportExceptModulepars_Module_1 all except {
            modulepar m¶_}
		import from A all;
	  }`, `module TestImportExceptModulepars_Module_1
      {
		  modulepar charstring m_name := "Justus"
		  type record R1 {integer f1, boolean f2}
		  modulepar R1 m_r1 := {f1 := 10, f2 := false}
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "m_name", Kind: protocol.ConstantCompletion},
		{Label: "m_r1", Kind: protocol.ConstantCompletion},
		{Label: "all;", Kind: protocol.KeywordCompletion}}, list)
}

func TestImportTypesCtrlSpc(t *testing.T) {
	list := testCompletion(t, `module Test
    {
        import from TestImportTypesCtrlSpc_Module_1 {
            type¶ }
		import from A all;
	  }`, `module TestImportTypesCtrlSpc_Module_1
      {
		  type charstring String;
		  type record R1 {integer f1, boolean f2}
		  type set S1 {integer f1, boolean f2}
		  type union U1 {integer f1, boolean f2}
		  type record of integer RoI1;
		  type record length(2..10) of integer RoI2;
		  type record of T MyList <in type T>;
		  type set of integer SoI;
		  type function MyBehavType() return integer;
		  type enumerated E1 {red, green, blue};
		  type port MyPort message {inout E1}
		  type component C0 {}
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "String", Kind: protocol.StructCompletion},
		{Label: "R1", Kind: protocol.StructCompletion},
		{Label: "S1", Kind: protocol.StructCompletion},
		{Label: "U1", Kind: protocol.StructCompletion},
		{Label: "RoI1", Kind: protocol.StructCompletion},
		{Label: "RoI2", Kind: protocol.StructCompletion},
		{Label: "MyList", Kind: protocol.StructCompletion},
		{Label: "SoI", Kind: protocol.StructCompletion},
		{Label: "MyBehavType", Kind: protocol.StructCompletion},
		{Label: "E1", Kind: protocol.StructCompletion},
		{Label: "MyPort", Kind: protocol.StructCompletion},
		{Label: "C0", Kind: protocol.StructCompletion},
		{Label: "all;", Kind: protocol.KeywordCompletion}}, list)
}

func TestImportTypes(t *testing.T) {
	list := testCompletion(t, `module Test
    {
        import from TestImportTypes_Module_1 {
            type ¶E}
		import from A all;
	  }`, `module TestImportTypes_Module_1
      {
		  type charstring String;
		  type record R1 {integer f1, boolean f2}
		  type set S1 {integer f1, boolean f2}
		  type union U1 {integer f1, boolean f2}
		  type record of integer RoI1;
		  type record length(2..10) of integer RoI2;
		  type record of T MyList <in type T>;
		  type set of integer SoI;
		  type function MyBehavType() return integer;
		  type enumerated E1 {red, green, blue};
		  type port MyPort message {inout E1}
		  type component C0 {}
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "String", Kind: protocol.StructCompletion},
		{Label: "R1", Kind: protocol.StructCompletion},
		{Label: "S1", Kind: protocol.StructCompletion},
		{Label: "U1", Kind: protocol.StructCompletion},
		{Label: "RoI1", Kind: protocol.StructCompletion},
		{Label: "RoI2", Kind: protocol.StructCompletion},
		{Label: "MyList", Kind: protocol.StructCompletion},
		{Label: "SoI", Kind: protocol.StructCompletion},
		{Label: "MyBehavType", Kind: protocol.StructCompletion},
		{Label: "E1", Kind: protocol.StructCompletion},
		{Label: "MyPort", Kind: protocol.StructCompletion},
		{Label: "C0", Kind: protocol.StructCompletion},
		{Label: "all;", Kind: protocol.KeywordCompletion}}, list)
}

func TestRunsOnTypesCtrlSpc(t *testing.T) {
	list := testCompletion(t, `module Test
    {
        type component B0 {}
		type component B1 {}
		function f() runs on¶ //
	  }`, `module TestRunsOnTypesCtrlSpc_Module_1
      {
		  type component C0 {}
	  }`, `module TestRunsOnTypesCtrlSpc_Module_2
      {
		  type component A0 {}
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "B0", Kind: protocol.StructCompletion, SortText: " 1B0", Detail: "Test.B0"},
		{Label: "B1", Kind: protocol.StructCompletion, SortText: " 1B1", Detail: "Test.B1"},
		{Label: "C0", Kind: protocol.StructCompletion, SortText: " 1C0", Detail: "TestRunsOnTypesCtrlSpc_Module_1.C0"},
		{Label: "A0", Kind: protocol.StructCompletion, SortText: " 1A0", Detail: "TestRunsOnTypesCtrlSpc_Module_2.A0"},
		{Label: "TestRunsOnTypesCtrlSpc_Module_1", Kind: protocol.ModuleCompletion, SortText: " 2TestRunsOnTypesCtrlSpc_Module_1"},
		{Label: "TestRunsOnTypesCtrlSpc_Module_2", Kind: protocol.ModuleCompletion, SortText: " 2TestRunsOnTypesCtrlSpc_Module_2"}}, list)
}

func TestRunsOnTypes(t *testing.T) {
	list := testCompletion(t, `module Test
    {
        type component B0 {}
		type component B1 {}
		function f() runs on ¶A//
	  }`, `module TestRunsOnTypes_Module_1
      {
		  type component C0 {}
	  }`, `module TestRunsOnTypes_Module_2
      {
		  type component A0 {}
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "B0", Kind: protocol.StructCompletion, SortText: " 1B0", Detail: "Test.B0"},
		{Label: "B1", Kind: protocol.StructCompletion, SortText: " 1B1", Detail: "Test.B1"},
		{Label: "C0", Kind: protocol.StructCompletion, SortText: " 1C0", Detail: "TestRunsOnTypes_Module_1.C0"},
		{Label: "A0", Kind: protocol.StructCompletion, SortText: " 1A0", Detail: "TestRunsOnTypes_Module_2.A0"},
		{Label: "TestRunsOnTypes_Module_1", Kind: protocol.ModuleCompletion, SortText: " 2TestRunsOnTypes_Module_1"},
		{Label: "TestRunsOnTypes_Module_2", Kind: protocol.ModuleCompletion, SortText: " 2TestRunsOnTypes_Module_2"}}, list)
}

func TestRunsOnModuleDotTypesCtrlSpc(t *testing.T) {
	list := testCompletion(t, `module Test
    {
        type component B0 {}
		type component B1 {}
		function f() runs on TestRunsOnModuleDotTypesCtrlSpc_Module_1.¶//
	  }`, `module TestRunsOnModuleDotTypesCtrlSpc_Module_1
      {
		  type component C0 {}
	  }`, `module TestRunsOnModuleDotTypesCtrlSpc_Module_2
      {
		  type component A0 {}
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "C0", Kind: protocol.StructCompletion, SortText: " 1C0", Detail: "TestRunsOnModuleDotTypesCtrlSpc_Module_1.C0"}}, list)
}

func TestRunsOnModuleDotTypes(t *testing.T) {
	list := testCompletion(t, `module Test
    {
        type component B0 {}
		type component B1 {}
		function f() runs on TestRunsOnModuleDotTypes_Module_1.¶C//
	  }`, `module TestRunsOnModuleDotTypes_Module_1
      {
		  type component C0 {}
	  }`, `module TestRunsOnModuleDotTypes_Module_2
      {
		  type component A0 {}
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "C0", Kind: protocol.StructCompletion, SortText: " 1C0", Detail: "TestRunsOnModuleDotTypes_Module_1.C0"}}, list)
}

func TestSystemTypesCtrlSpc(t *testing.T) {
	list := testCompletion(t, `module Test
    {
        type component B0 {}
		type component B1 {}
		testcase f() runs on C0 system¶ //
	  }`, `module TestSystemTypesCtrlSpc_Module_1
      {
		  type component C0 {}
	  }`, `module TestSystemTypesCtrlSpc_Module_2
      {
		  type component A0 {}
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "B0", Kind: protocol.StructCompletion, SortText: " 1B0", Detail: "Test.B0"},
		{Label: "B1", Kind: protocol.StructCompletion, SortText: " 1B1", Detail: "Test.B1"},
		{Label: "C0", Kind: protocol.StructCompletion, SortText: " 1C0", Detail: "TestSystemTypesCtrlSpc_Module_1.C0"},
		{Label: "A0", Kind: protocol.StructCompletion, SortText: " 1A0", Detail: "TestSystemTypesCtrlSpc_Module_2.A0"},
		{Label: "TestSystemTypesCtrlSpc_Module_1", Kind: protocol.ModuleCompletion, SortText: " 2TestSystemTypesCtrlSpc_Module_1"},
		{Label: "TestSystemTypesCtrlSpc_Module_2", Kind: protocol.ModuleCompletion, SortText: " 2TestSystemTypesCtrlSpc_Module_2"}}, list)
}

func TestSystemModuleDotTypes(t *testing.T) {
	list := testCompletion(t, `module Test
    {
        type component B0 {}
		type component B1 {}
		function f() runs on TestSystemModuleDotTypes_Module_1.C0 system TestSystemModuleDotTypes_Module_1.¶C//
	  }`, `module TestSystemModuleDotTypes_Module_1
      {
		  type component C0 {}
	  }`, `module TestSystemModuleDotTypes_Module_2
      {
		  type component A0 {}
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "C0", Kind: protocol.StructCompletion, SortText: " 1C0", Detail: "TestSystemModuleDotTypes_Module_1.C0"}}, list)
}

func TestExtendsTypesCtrlSpc(t *testing.T) {
	list := testCompletion(t, `module Test
    {
        type component B0 {}
		type component B1 {}
		type component B2 extends¶ //
	  }`, `module TestExtendsTypesCtrlSpc_Module_1
      {
		  type component C0 {}
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "B0", Kind: protocol.StructCompletion, SortText: " 1B0", Detail: "Test.B0"},
		{Label: "B1", Kind: protocol.StructCompletion, SortText: " 1B1", Detail: "Test.B1"},
		{Label: "B2", Kind: protocol.StructCompletion, SortText: " 1B2", Detail: "Test.B2"}, // TODO: filter 'self' out
		{Label: "C0", Kind: protocol.StructCompletion, SortText: " 1C0", Detail: "TestExtendsTypesCtrlSpc_Module_1.C0"},
		{Label: "TestExtendsTypesCtrlSpc_Module_1", Kind: protocol.ModuleCompletion, SortText: " 2TestExtendsTypesCtrlSpc_Module_1"}}, list)
}

func TestExtendsTypes(t *testing.T) {
	list := testCompletion(t, `module Test
    {
        type component B0 {}
		type component B1 {}
		type component B2 extends ¶B//
	  }`, `module TestExtendsTypes_Module_1
      {
		  type component C0 {}
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "B0", Kind: protocol.StructCompletion, SortText: " 1B0", Detail: "Test.B0"},
		{Label: "B1", Kind: protocol.StructCompletion, SortText: " 1B1", Detail: "Test.B1"},
		{Label: "B2", Kind: protocol.StructCompletion, SortText: " 1B2", Detail: "Test.B2"},
		{Label: "C0", Kind: protocol.StructCompletion, SortText: " 1C0", Detail: "TestExtendsTypes_Module_1.C0"},
		{Label: "TestExtendsTypes_Module_1", Kind: protocol.ModuleCompletion, SortText: " 2TestExtendsTypes_Module_1"}}, list)
}

func TestExtendsModuleDotTypes(t *testing.T) {
	list := testCompletion(t, `module Test
    {
        type component B0 {}
		type component B1 {}
		type component B2 extends TestExtendsModuleDotTypes_Module_1¶.//
	  }`, `module TestExtendsModuleDotTypes_Module_1
      {
		  type component C0 {}
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "C0", Kind: protocol.StructCompletion, SortText: " 1C0", Detail: "TestExtendsModuleDotTypes_Module_1.C0"}}, list)
}

func TestModifiesCtrlSpc(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		import from TestModifiesCtrlSpc_Module_1 all;
		template R t_r := *;
		template integer t_i := ?;
		template (omit) R t_rmod modifies ¶//
	  }`, `module TestModifiesCtrlSpc_Module_1
      {
		  type record R {integer f1, boolean f2 optional}
		  template (value) R t_r2 := {10, omit}
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "t_r", Kind: protocol.ConstantCompletion, Detail: "Test.t_r"},
		{Label: "t_i", Kind: protocol.ConstantCompletion, Detail: "Test.t_i"},       // TODO: implement filter on Compatible Type
		{Label: "t_rmod", Kind: protocol.ConstantCompletion, Detail: "Test.t_rmod"}, // TODO: implement filter for self
		{Label: "t_r2", Kind: protocol.ConstantCompletion, Detail: "TestModifiesCtrlSpc_Module_1.t_r2"},
		{Label: "TestModifiesCtrlSpc_Module_1", Kind: protocol.ModuleCompletion}}, list)
}

func TestModifiesParseErrorCtrlSpc(t *testing.T) {
	list := testCompletion(t, `module Test {
		   template integer t_base := ?
		   template in¶//

		   function setup_ports()
			runs on CpMctMain
		{
			map(system:ifF1Sctp[0], mtc:ifF1Sctp[0]) param(t_f1SctpMapParam1);
			map(system:ifF1Sctp[1], mtc:ifF1Sctp[1]) param(t_f1SctpMapParam2);
			map(system:ifF1Sctp[2], mtc:ifF1Sctp[2]) param(t_f1SctpMapParam3);
			map(system:ifF1Sctp[3], mtc:ifF1Sctp[3]) param(t_f1SctpMapParam4);
			map(system:ifF1Sctp[4], mtc:ifF1Sctp[4]) param(t_f1SctpMapParam5);
			map(system:ifF1Sctp[5], mtc:ifF1Sctp[5]) param(t_f1SctpMapParam6);
			map(system:ifF1Sctp[6], mtc:ifF1Sctp[6]) param(t_f1SctpMapParam7);
			map(system:ifX2Sctp[0], mtc:ifX2Sctp[0]) param(t_x2SctpMapParam);
			map(system:ifXNSctp[0], mtc:ifXNSctp[0]) param(t_xnSctpMapParam);
		}
	}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "anytype ", Kind: protocol.KeywordCompletion},
		{Label: "bitstring ", Kind: protocol.KeywordCompletion},
		{Label: "boolean ", Kind: protocol.KeywordCompletion},
		{Label: "charstring ", Kind: protocol.KeywordCompletion},
		{Label: "default ", Kind: protocol.KeywordCompletion},
		{Label: "float ", Kind: protocol.KeywordCompletion},
		{Label: "hexstring ", Kind: protocol.KeywordCompletion},
		{Label: "integer ", Kind: protocol.KeywordCompletion},
		{Label: "octetstring ", Kind: protocol.KeywordCompletion},
		{Label: "universal charstring ", Kind: protocol.KeywordCompletion},
		{Label: "verdicttype ", Kind: protocol.KeywordCompletion}}, list)
}

func TestTemplateTypeCtrlSpc(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		type integer Byte(0..255);
		template ¶//
		template integer a_i := ?;
	}`, `module TestTemplateTypeCtrlSpc_Module_1
      {
		  type record R {integer f1, boolean f2 optional}
		  template (value) R t_r2 := {10, omit}
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "Byte ", Kind: protocol.StructCompletion, SortText: " 1Byte", Detail: "Test.Byte"},
		{Label: "R ", Kind: protocol.StructCompletion, SortText: " 2R", Detail: "TestTemplateTypeCtrlSpc_Module_1.R"},
		{Label: "anytype ", Kind: protocol.KeywordCompletion},
		{Label: "bitstring ", Kind: protocol.KeywordCompletion},
		{Label: "boolean ", Kind: protocol.KeywordCompletion},
		{Label: "charstring ", Kind: protocol.KeywordCompletion},
		{Label: "default ", Kind: protocol.KeywordCompletion},
		{Label: "float ", Kind: protocol.KeywordCompletion},
		{Label: "hexstring ", Kind: protocol.KeywordCompletion},
		{Label: "integer ", Kind: protocol.KeywordCompletion},
		{Label: "octetstring ", Kind: protocol.KeywordCompletion},
		{Label: "universal charstring ", Kind: protocol.KeywordCompletion},
		{Label: "verdicttype ", Kind: protocol.KeywordCompletion},
		{Label: "TestTemplateTypeCtrlSpc_Module_1", Kind: protocol.ModuleCompletion, SortText: " 3TestTemplateTypeCtrlSpc_Module_1"}}, list)
}

func TestTemplateType(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		type integer Byte(0..255);
		template h¶//
		template integer a_i := ?;
	}`, `module TestTemplateType_Module_1
      {
		  type record R {integer f1, boolean f2 optional}
		  template (value) R t_r2 := {10, omit}
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "Byte ", Kind: protocol.StructCompletion, SortText: " 1Byte", Detail: "Test.Byte"},
		{Label: "R ", Kind: protocol.StructCompletion, SortText: " 2R", Detail: "TestTemplateType_Module_1.R"},
		{Label: "anytype ", Kind: protocol.KeywordCompletion},
		{Label: "bitstring ", Kind: protocol.KeywordCompletion},
		{Label: "boolean ", Kind: protocol.KeywordCompletion},
		{Label: "charstring ", Kind: protocol.KeywordCompletion},
		{Label: "default ", Kind: protocol.KeywordCompletion},
		{Label: "float ", Kind: protocol.KeywordCompletion},
		{Label: "hexstring ", Kind: protocol.KeywordCompletion},
		{Label: "integer ", Kind: protocol.KeywordCompletion},
		{Label: "octetstring ", Kind: protocol.KeywordCompletion},
		{Label: "universal charstring ", Kind: protocol.KeywordCompletion},
		{Label: "verdicttype ", Kind: protocol.KeywordCompletion},
		{Label: "TestTemplateType_Module_1", Kind: protocol.ModuleCompletion, SortText: " 3TestTemplateType_Module_1"}}, list)
}

func TestTemplateModuleDotType(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		type integer Byte(0..255);
		template TestTemplateModuleDotType_Module_1¶.//
		template integer a_i := ?;  // NOTE: this template kw is apparently consumed by the parser leading to integer being interpreted as Name!!!
	}`, `module TestTemplateModuleDotType_Module_1
      {
		  type record R {integer f1, boolean f2 optional}
		  template (value) R t_r2 := {10, omit}
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "R ", Kind: protocol.StructCompletion, Detail: "TestTemplateModuleDotType_Module_1.R"}}, list)
}

func TestModifiesModuleDot(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		import from TestModifiesModuleDot_Module_1 all;
		template R t_r := *;
		template integer t_i := ?;
		template (omit) R t_rmod modifies TestModifiesModuleDot_Module_1.¶//
	  }`, `module TestModifiesModuleDot_Module_1
      {
		  type record R {integer f1, boolean f2 optional}
		  template (value) R t_r2 := {10, omit}
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "t_r2", Kind: protocol.ConstantCompletion}}, list)
}

func TestSubTypeDefSegv(t *testing.T) {
	list := testCompletion(t, `module Test
    {
        type ¶r
	  }`)

	assert.Empty(t, list)
}

func TestNewModuleDef(t *testing.T) {
	list := testCompletion(t, `module Test
    {
        ¶t
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "import from ", Kind: protocol.KeywordCompletion},
		{Label: "type ", Kind: protocol.KeywordCompletion},
		{Label: "const ", Kind: protocol.KeywordCompletion},
		{Label: "modulepar ", Kind: protocol.KeywordCompletion},
		{Label: "template ", Kind: protocol.KeywordCompletion},
		{Label: "function ", Kind: protocol.KeywordCompletion},
		{Label: "external function ", Kind: protocol.KeywordCompletion},
		{Label: "altstep ", Kind: protocol.KeywordCompletion},
		{Label: "testcase ", Kind: protocol.KeywordCompletion},
		{Label: "control ", Kind: protocol.KeywordCompletion},
		{Label: "signature ", Kind: protocol.KeywordCompletion}}, list)
}

func TestPortTypeInsideComponent(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		type port P1 message {
			inout charstring
		}
        type component B0 {
			port ¶P//
		}
	  }`, `module TestPortTypeInsideComponent_Module_1
      {
		  type port P2 message {
			  in integer
			  out float
		  }
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "P1", Kind: protocol.InterfaceCompletion, SortText: " 1P1", Detail: "Test.P1"},
		{Label: "P2", Kind: protocol.InterfaceCompletion, SortText: " 2P2", Detail: "TestPortTypeInsideComponent_Module_1.P2"},
		{Label: "TestPortTypeInsideComponent_Module_1", Kind: protocol.ModuleCompletion, SortText: " 3TestPortTypeInsideComponent_Module_1"}}, list)
}

func TestInsideBehavBody(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		function f1() {}
		function f2() {
			f//
¶		};
	}`, `module TestInsideBehavBody_Module_1
      {
		  function f3() {}
		  altstep a1() runs on C0 {}
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "f1()", Kind: protocol.FunctionCompletion, SortText: " 1f1", InsertText: "f1()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "function Test.f1()", Documentation: ""},
		{Label: "f2()", Kind: protocol.FunctionCompletion, SortText: " 1f2", InsertText: "f2()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "function Test.f2()", Documentation: ""},
		{Label: "f3()", Kind: protocol.FunctionCompletion, SortText: " 2f3", InsertText: "f3()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "function TestInsideBehavBody_Module_1.f3()", Documentation: ""},
		{Label: "a1()", Kind: protocol.FunctionCompletion, SortText: " 2a1", InsertText: "a1()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "altstep TestInsideBehavBody_Module_1.a1()\n  runs on C0", Documentation: ""},
		{Label: "TestInsideBehavBody_Module_1", Kind: protocol.ModuleCompletion, SortText: " 4TestInsideBehavBody_Module_1"}},
		list)
}

func TestInsideTcBody(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		function f1() {}
		testcase tc1() {
			f//
¶		};
	}`, `module TestInsideTcBody_Module_1
      {
		  function f3() {}
		  altstep a1() runs on C0 {}
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "f1()", Kind: protocol.FunctionCompletion, SortText: " 1f1", InsertText: "f1()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "function Test.f1()", Documentation: ""},
		{Label: "f3()", Kind: protocol.FunctionCompletion, SortText: " 2f3", InsertText: "f3()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "function TestInsideTcBody_Module_1.f3()", Documentation: ""},
		{Label: "a1()", Kind: protocol.FunctionCompletion, SortText: " 2a1", InsertText: "a1()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "altstep TestInsideTcBody_Module_1.a1()\n  runs on C0", Documentation: ""},
		{Label: "TestInsideTcBody_Module_1", Kind: protocol.ModuleCompletion, SortText: " 4TestInsideTcBody_Module_1"}},
		list)
}

func TestInsideTcBodyCtrlSpc(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		function f1() {}
		testcase tc1() {
			//
¶		};
	}`, `module TestInsideTcBodyCtrlSpc_Module_1
      {
		  function f3() {}
		  altstep a1() runs on C0 {}
	  }`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "f1()", Kind: protocol.FunctionCompletion, SortText: " 1f1", InsertText: "f1()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "function Test.f1()", Documentation: ""},
		{Label: "f3()", Kind: protocol.FunctionCompletion, SortText: " 2f3", InsertText: "f3()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "function TestInsideTcBodyCtrlSpc_Module_1.f3()", Documentation: ""},
		{Label: "a1()", Kind: protocol.FunctionCompletion, SortText: " 2a1", InsertText: "a1()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "altstep TestInsideTcBodyCtrlSpc_Module_1.a1()\n  runs on C0", Documentation: ""},
		{Label: "TestInsideTcBodyCtrlSpc_Module_1", Kind: protocol.ModuleCompletion, SortText: " 4TestInsideTcBodyCtrlSpc_Module_1"}},
		list)
}

func TestInsideTcBodyInsideIf(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		function f1() {}
		function f2() return boolean {}
		testcase tc1() {
			if(¶f/**/)
		};
	}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "f2()", Kind: protocol.FunctionCompletion, SortText: " 1f2", InsertText: "f2()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "function Test.f2()\n  return boolean", Documentation: ""}},
		list)
}

func TestInsideTcBodyModuleDotInsideIf(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		testcase tc1() {
			if(TestInsideTcBodyModuleDotInsideIf_Module_1.¶f/**/)
		};
	}`, `module TestInsideTcBodyModuleDotInsideIf_Module_1
	{
		function f1() {}
		function f2() return boolean {}
		function f3() runs on C0 return integer {}
		function f4() runs on C0 {}
		altstep a1() runs on C0 {}
	}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "f2()", Kind: protocol.FunctionCompletion, SortText: " 1f2", InsertText: "f2()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "function TestInsideTcBodyModuleDotInsideIf_Module_1.f2()\n  return boolean", Documentation: ""},
		{Label: "f3()", Kind: protocol.FunctionCompletion, SortText: " 1f3", InsertText: "f3()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "function TestInsideTcBodyModuleDotInsideIf_Module_1.f3()\n  runs on C0\n  return integer", Documentation: ""}},
		list)
}

func TestInsideTcBodyInsideExpr(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		// f1:
		// function without return value
		function f1() {}
		function f2() return boolean {}
		testcase tc1() {
			var integer i := 2 * ¶f//
		};
	}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "i", Kind: protocol.VariableCompletion, SortText: " 2i"},
		{Label: "f2()", Kind: protocol.FunctionCompletion, SortText: " 1f2", InsertText: "f2()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "function Test.f2()\n  return boolean", Documentation: ""}},
		list)
}

func TestInsideTcBodyInsideSend(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		function f1() {}
		function f2(integer pi:=314) return boolean {}
		testcase tc1() {
			p.send(¶f/**/)
		};
	}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "f2()", Kind: protocol.FunctionCompletion, SortText: " 1f2", InsertText: "f2($1)$0", InsertTextFormat: protocol.SnippetTextFormat,
			Detail: "function Test.f2( integer pi := 314)\n  return boolean", Documentation: ""}},
		list)
}

func TestInsideTcBodyAsFuncParam(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		function f1() {}
		function f2(integer pi) return boolean {}
		testcase tc1() {
			f1(¶f//);
		};
	}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "f2()", Kind: protocol.FunctionCompletion, SortText: " 1f2", InsertText: "f2($1)$0", InsertTextFormat: protocol.SnippetTextFormat,
			Detail: "function Test.f2( integer pi)\n  return boolean", Documentation: ""}},
		list)
}

func TestInsideTcBodyModuleDotInsideStart(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		testcase tc1() {
			// allow only funcs with runs on(behaviour)
			// or return value (accepts float for timer)
			ptcOrTimer.start(TestInsideTcBodyModuleDotInsideStart_Module_1.¶f/**/)
		};
	}`, `module TestInsideTcBodyModuleDotInsideStart_Module_1
	{
		function f1() {}
		function f2() return boolean {}
		function f3() runs on C0 return integer {}
		function f4() runs on C0 {}
		altstep a1() runs on C0 {}
	}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "f2()", Kind: protocol.FunctionCompletion, SortText: " 1f2", InsertText: "f2()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "function TestInsideTcBodyModuleDotInsideStart_Module_1.f2()\n  return boolean", Documentation: ""},
		{Label: "f3()", Kind: protocol.FunctionCompletion, SortText: " 1f3", InsertText: "f3()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "function TestInsideTcBodyModuleDotInsideStart_Module_1.f3()\n  runs on C0\n  return integer", Documentation: ""},
		{Label: "f4()", Kind: protocol.FunctionCompletion, SortText: " 1f4", InsertText: "f4()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "function TestInsideTcBodyModuleDotInsideStart_Module_1.f4()\n  runs on C0", Documentation: ""}},
		list)
}

func TestInsideTcBodyInsideStart(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		function f1() {}
		function f2() runs on C1 {}
		function f3() return boolean {}
		function f4() return float {}
		testcase tc1() {
			// allow only funcs with runs on(behaviour)
			// or return value (accepts float for timer)
			ptcOrTimer.start(¶f/**/)
		};
	}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "f2()", Kind: protocol.FunctionCompletion, SortText: " 1f2", InsertText: "f2()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "function Test.f2()\n  runs on C1", Documentation: ""},
		{Label: "f3()", Kind: protocol.FunctionCompletion, SortText: " 1f3", InsertText: "f3()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "function Test.f3()\n  return boolean", Documentation: ""},
		{Label: "f4()", Kind: protocol.FunctionCompletion, SortText: " 1f4", InsertText: "f4()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "function Test.f4()\n  return float", Documentation: ""}},
		list)
}

func TestInsideTcBodyNestedInsideStart(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		function f1() {}
		function f2() runs on C1 {}
		function f3() return boolean {}
		function f4() runs on C1 return boolean {}
		testcase tc1() {
			// allow only funcs with return value (accepts float for timer)
			ptcOrTimer.start(somefunc(¶f/**/))
		};
	}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "f3()", Kind: protocol.FunctionCompletion, SortText: " 1f3", InsertText: "f3()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "function Test.f3()\n  return boolean", Documentation: ""},
		{Label: "f4()", Kind: protocol.FunctionCompletion, SortText: " 1f4", InsertText: "f4()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "function Test.f4()\n  runs on C1\n  return boolean", Documentation: ""}},
		list)
}

func TestFuncComplInsideConstDecl(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		function f1() {}
		function f2() runs on C1 {}
		function f3() return boolean {}
		function f4() runs on C1 return boolean {}
		const integer ci :=¶ //
		testcase tc1() {
		};
	}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "f3()", Kind: protocol.FunctionCompletion, SortText: " 1f3", InsertText: "f3()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "function Test.f3()\n  return boolean", Documentation: ""},
		{Label: "f4()", Kind: protocol.FunctionCompletion, SortText: " 1f4", InsertText: "f4()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "function Test.f4()\n  runs on C1\n  return boolean", Documentation: ""}},
		list)
}

func TestFuncComplInsideConstDeclBody(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		function f1() {}
		function f2() runs on C1 {}
		function f3() return boolean {}
		function f4() runs on C1 return boolean {}
		const R ci := {f1 :=¶ /**/}
	}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "f3()", Kind: protocol.FunctionCompletion, SortText: " 1f3", InsertText: "f3()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "function Test.f3()\n  return boolean", Documentation: ""},
		{Label: "f4()", Kind: protocol.FunctionCompletion, SortText: " 1f4", InsertText: "f4()", InsertTextFormat: protocol.PlainTextTextFormat,
			Detail: "function Test.f4()\n  runs on C1\n  return boolean", Documentation: ""}},
		list)
}

// TODO: fixing this issue requires more effort.
/*
func TestSyntaxErrorProvokingInvalidPos(t *testing.T) {
	suite := buildSuite(t, `module Test
    {
    type component Ptc {}
    type component Sys {}
    function setColor(integer p_color) runs on {
        log(p_color);
    }
    testcase tc1() runs on test.Ptc system Sys { }
    c
	  }`)
	name := fmt.Sprintf("%s_Module_0.ttcn3", t.Name())
	syntax := suite.ParseWithAllErrors(name)
	pos := syntax.Pos(9, 6)
	assert.Equal(t, pos, 203)
}
*/

func TestSelectorComplWithStructTypeDecl(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		type record Structure
		{
			integer firstField,
			AnotherStructure secondField,
			YetAnotherOne thirdField
		}

		function f1() {
			var template Structure st := {};
			st.¶
		}
	}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "firstField", Kind: protocol.FieldCompletion, Detail: "integer", Documentation: ""},
		{Label: "secondField", Kind: protocol.FieldCompletion, Detail: "AnotherStructure", Documentation: ""},
		{Label: "thirdField", Kind: protocol.FieldCompletion, Detail: "YetAnotherOne", Documentation: ""}},
		list)
}

func TestDeepSelectorComplWithStructTypeDecl(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		type record YetAnotherOne
		{
			integer i
		}

		type record AnotherStructure
		{
			YetAnotherOne singleField
		}

		type record Structure
		{
			integer firstField,
			AnotherStructure secondField,
			YetAnotherOne thirdField
		}

		function f1() {
			var template Structure st := {};
			st.secondField.¶
		}
	}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "singleField", Kind: protocol.FieldCompletion, Detail: "YetAnotherOne", Documentation: ""}},
		list)
}

func TestSelectorComplWithStructTypeDeclPartialInput(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		type record Structure
		{
			integer firstField,
			AnotherStructure secondField,
			YetAnotherOne thirdField
		}

		function f1() {
			var template Structure st := {};
			st.f¶
		}
	}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "firstField", Kind: protocol.FieldCompletion, Detail: "integer", Documentation: ""},
		{Label: "secondField", Kind: protocol.FieldCompletion, Detail: "AnotherStructure", Documentation: ""},
		{Label: "thirdField", Kind: protocol.FieldCompletion, Detail: "YetAnotherOne", Documentation: ""}},
		list)
}

func TestNestedSelectorComplWithStructTypeDeclPartialInput(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		type record YetAnotherOne
		{
			integer i
		}

		type record AnotherStructure
		{
			YetAnotherOne singleField
		}

		type record Structure
		{
			integer firstField,
			AnotherStructure secondField,
			YetAnotherOne thirdField
		}

		function f1() {
			var template Structure st := {};
			st.secondField.s¶
		}
	}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "singleField", Kind: protocol.FieldCompletion, Detail: "YetAnotherOne", Documentation: ""}},
		list)
}

func TestNamedParamsCompletion(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		function fn(integer intParam, Structure structParam, integer intParam2) {}

		function f1() {
			fn(i¶)
		}
	}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "intParam", Kind: protocol.EnumMemberCompletion, Detail: "integer", Documentation: "", InsertText: "intParam := ${1:}",
			InsertTextFormat: protocol.SnippetTextFormat, SortText: " 1intParam"},
		{Label: "structParam", Kind: protocol.EnumMemberCompletion, Detail: "Structure", Documentation: "", InsertText: "structParam := ${1:}",
			InsertTextFormat: protocol.SnippetTextFormat, SortText: " 1structParam"},
		{Label: "intParam2", Kind: protocol.EnumMemberCompletion, Detail: "integer", Documentation: "", InsertText: "intParam2 := ${1:}",
			InsertTextFormat: protocol.SnippetTextFormat, SortText: " 1intParam2"}},
		list)
}

func TestNamedParamsInsideAssignCompletion(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		function fn(integer intParam, Structure structParam, integer intParam2) {}

		function f1() {
			fn(i¶ := 42)
		}
	}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "intParam", Kind: protocol.EnumMemberCompletion, Detail: "integer", Documentation: "", InsertText: "intParam",
			InsertTextFormat: protocol.SnippetTextFormat, SortText: " 1intParam"},
		{Label: "structParam", Kind: protocol.EnumMemberCompletion, Detail: "Structure", Documentation: "", InsertText: "structParam",
			InsertTextFormat: protocol.SnippetTextFormat, SortText: " 1structParam"},
		{Label: "intParam2", Kind: protocol.EnumMemberCompletion, Detail: "integer", Documentation: "", InsertText: "intParam2",
			InsertTextFormat: protocol.SnippetTextFormat, SortText: " 1intParam2"}},
		list)
}

func TestNamedParamsAfterAssignCompletion(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		function fn(integer intParam, Structure structParam, integer intParam2) {}

		function f1() {
			fn(intParam := ¶)
		}
	}`)

	assert.Equal(t, []protocol.CompletionItem(nil),
		list)
}

func TestNamedParamsInCompositeLiteralCompletion(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		type record Structure
		{
			integer firstField,
			AnotherStructure secondField,
			YetAnotherOne thirdField
		}

		function f1() {
			var template Structure st := {
				s¶
			};
		}
	}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "firstField", Kind: protocol.EnumMemberCompletion, Detail: "integer", Documentation: "", InsertText: "firstField := ${1:}",
			InsertTextFormat: protocol.SnippetTextFormat, SortText: " 1firstField"},
		{Label: "secondField", Kind: protocol.EnumMemberCompletion, Detail: "AnotherStructure", Documentation: "", InsertText: "secondField := ${1:}",
			InsertTextFormat: protocol.SnippetTextFormat, SortText: " 1secondField"},
		{Label: "thirdField", Kind: protocol.EnumMemberCompletion, Detail: "YetAnotherOne", Documentation: "", InsertText: "thirdField := ${1:}",
			InsertTextFormat: protocol.SnippetTextFormat, SortText: " 1thirdField"},
		{Label: "st", Kind: protocol.VariableCompletion, SortText: " 2st"}},
		list)
}

func TestNamedParamsInCompositeLiteralInsideAssignCompletion(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		type record Structure
		{
			integer firstField,
			AnotherStructure secondField,
			YetAnotherOne thirdField
		}

		function f1() {
			var template Structure st := {
				s¶ := 42
			};
		}
	}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "firstField", Kind: protocol.EnumMemberCompletion, Detail: "integer", Documentation: "", InsertText: "firstField",
			InsertTextFormat: protocol.SnippetTextFormat, SortText: " 1firstField"},
		{Label: "secondField", Kind: protocol.EnumMemberCompletion, Detail: "AnotherStructure", Documentation: "", InsertText: "secondField",
			InsertTextFormat: protocol.SnippetTextFormat, SortText: " 1secondField"},
		{Label: "thirdField", Kind: protocol.EnumMemberCompletion, Detail: "YetAnotherOne", Documentation: "", InsertText: "thirdField",
			InsertTextFormat: protocol.SnippetTextFormat, SortText: " 1thirdField"},
		{Label: "st", Kind: protocol.VariableCompletion, SortText: " 2st"}},
		list)
}

func TestNamedParamsInCompositeLiteralAfterAssignCompletion(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		type record Structure
		{
			integer firstField,
			AnotherStructure secondField,
			YetAnotherOne thirdField
		}

		function f1() {
			var template Structure st := {
				firstField := s¶
			};
		}
	}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "st", Kind: protocol.VariableCompletion, SortText: " 2st"}},
		list)
}

func TestRecordItemSelectorCompletion(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		type record Structure
		{
			integer firstField,
			AnotherStructure secondField,
			YetAnotherOne thirdField
		}

		type record of Structure RoStructure;

		function f1() {
			var template RoStructure roSt := {};
			roSt[i].f¶;
		}
	}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "firstField", Kind: protocol.FieldCompletion, Detail: "integer", Documentation: ""},
		{Label: "secondField", Kind: protocol.FieldCompletion, Detail: "AnotherStructure", Documentation: ""},
		{Label: "thirdField", Kind: protocol.FieldCompletion, Detail: "YetAnotherOne", Documentation: ""}},
		list)
}

func TestRecordItemSelectorNotDeepEnoughCompletion(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		type record Structure
		{
			integer firstField,
			AnotherStructure secondField,
			YetAnotherOne thirdField
		}

		type record of Structure RoStructure;

		function f1() {
			var template RoStructure roSt := {};
			roSt.f¶;
		}
	}`)

	assert.Equal(t, []protocol.CompletionItem(nil), list)
}

func TestRecordItemSelectorTooDeepCompletion(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		type record Structure
		{
			integer firstField,
			AnotherStructure secondField,
			YetAnotherOne thirdField
		}

		type record of Structure RoStructure;

		function f1() {
			var template RoStructure roSt := {};
			roSt[i][j].f¶;
		}
	}`)

	assert.Equal(t, []protocol.CompletionItem(nil), list)
}

func TestRecordItemSelectorMultidimensionalCompletion(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		type record Structure
		{
			integer firstField,
			AnotherStructure secondField,
			YetAnotherOne thirdField
		}

		type record of Structure RoStructure;
		type record of RoStructure RoRoStructure;
		type record of RoRoStructure RoRoRoStructure;

		function f1() {
			var template RoRoRoStructure roRoRoSt := {};
			roRoRoSt[i][j][k].f¶;
		}
	}`)

	assert.Equal(t, []protocol.CompletionItem{
		{Label: "firstField", Kind: protocol.FieldCompletion, Detail: "integer", Documentation: ""},
		{Label: "secondField", Kind: protocol.FieldCompletion, Detail: "AnotherStructure", Documentation: ""},
		{Label: "thirdField", Kind: protocol.FieldCompletion, Detail: "YetAnotherOne", Documentation: ""}},
		list)
}

func TestRecordItemSelectorMultidimensionalNotDeepEnoughCompletion(t *testing.T) {
	list := testCompletion(t, `module Test
    {
		type record Structure
		{
			integer firstField,
			AnotherStructure secondField,
			YetAnotherOne thirdField
		}

		type record of Structure RoStructure;
		type record of RoStructure RoRoStructure;
		type record of RoRoStructure RoRoRoStructure;

		function f1() {
			var template RoRoRoStructure roRoRoSt := {};
			roRoRoSt[i][j].f¶;
		}
	}`)

	assert.Equal(t, []protocol.CompletionItem(nil), list)
}

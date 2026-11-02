package lsp_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/lsp"
	"github.com/nokia/ntt/internal/ntttest"
	"github.com/nokia/ntt/ttcn3"
	"github.com/stretchr/testify/assert"
)

// testReferences indexes inputs as files named "<test>_<i>" and returns the
// references of the identifier marked by the cursor in the first input as
// "<i>:<line>:<column>" strings (1-based).
func testReferences(t *testing.T, includeDecl bool, inputs ...string) []string {
	t.Helper()

	name := func(i int) string {
		return fmt.Sprintf("test://%s_%d", t.Name(), i)
	}

	input, cursor := ntttest.CutCursor(inputs[0])
	inputs[0] = input

	var files []string
	for i, input := range inputs {
		fs.SetContent(name(i), []byte(input))
		files = append(files, name(i))
	}

	db := &ttcn3.DB{}
	db.Index(files...)

	tree := ttcn3.ParseFile(name(0))
	pos := tree.Position(cursor)

	var result []string
	for _, loc := range lsp.FindReferences(db, tree, pos.Line, pos.Column, includeDecl) {
		file := strings.TrimPrefix(string(loc.URI), fmt.Sprintf("test://%s_", t.Name()))
		result = append(result, fmt.Sprintf("%s:%d:%d", file, loc.Range.Start.Line+1, loc.Range.Start.Character+1))
	}
	return result
}

func TestFindAllTypeDefs(t *testing.T) {
	actual := testReferences(t, true, `
	module A {
		type integer ¶Byte(0..255);
		function f() return Byte {
		var Byte ret := 100;
		}
	}
	module B {
		import from A all;
		template Byte a_byte := ?;
	}`, `
	module C {
		import from A {type Byte}
		type Byte AliasByte;
	}`)

	assert.Equal(t, []string{
		"0:3:16",
		"0:4:23",
		"0:5:7",
		"0:10:12",
		"1:3:23",
		"1:4:8",
	}, actual)
}

func TestReferencesLocalVariables(t *testing.T) {
	actual := testReferences(t, true, `
	module M {
		function f() {
			var integer x := 1;
			¶x := x + 1;
		}
		function g() {
			var integer x := 2;
			log(x);
		}
	}`)

	assert.Equal(t, []string{
		"0:4:16",
		"0:5:4",
		"0:5:9",
	}, actual)
}

func TestReferencesImportedConst(t *testing.T) {
	inputs := func() []string {
		return []string{`
	module B {
		import from A all;
		function g() { log(¶c) }
	}`, `
	module A {
		const integer c := 1;
		function f() { log(c) }
	}`, `
	module D {
		const integer c := 2;
		function h() { log(c) }
	}`}
	}

	t.Run("IncludeDeclaration", func(t *testing.T) {
		assert.Equal(t, []string{
			"0:4:22",
			"1:3:17",
			"1:4:22",
		}, testReferences(t, true, inputs()...))
	})

	t.Run("ExcludeDeclaration", func(t *testing.T) {
		assert.Equal(t, []string{
			"0:4:22",
			"1:4:22",
		}, testReferences(t, false, inputs()...))
	})
}

func TestReferencesRecordField(t *testing.T) {
	input := `
	module R {
		type record Rec { integer f }
		function k() {
			var Rec r := { f := 2 };
			r.¶f := 1;
			log(r.f);
			var integer f := 3;
			log(f);
		}
	}`

	assert.Equal(t, []string{
		"0:3:29",
		"0:5:19",
		"0:6:6",
		"0:7:10",
	}, testReferences(t, true, input))

	assert.Equal(t, []string{
		"0:5:19",
		"0:6:6",
		"0:7:10",
	}, testReferences(t, false, input))
}

func TestReferencesNoIdentifier(t *testing.T) {
	assert.Nil(t, testReferences(t, true, `
	module M {
		function f() {
			var integer x := 1 ¶+ 2;
		}
	}`))
}

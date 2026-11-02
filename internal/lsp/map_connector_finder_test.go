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

func TestFindAllMaps(t *testing.T) {

	name1 := fmt.Sprintf("test://%s_input1", t.Name())
	name2 := fmt.Sprintf("test://%s_input2", t.Name())
	name3 := fmt.Sprintf("test://%s_input3", t.Name())

	input1 := `module Test
	{
		type port Pt1 message {inout integer};
		type port Pt2 message {inout charstring};
		type component C1 {
			port Pt1 p
		}
		type component C2 {
			port Pt2 p,
			port Pt1, pt1
		}
		function f() runs on C1 {
			map(mtc:p, system:p)
		}
	  }`
	input2 := `module A
	  {
			import from TestFindAllMaps_Module_0 all;
			function fA() runs on C2 {
				map(mtc:p, system:p)
				map(mtc:pt1, system:pt1)
			}
	  }`
	input3 := `module B
	  {
			function fB() runs on C2 {
				map(system:p, mtc:p)	// w/o import statement, this occurence shouldn't be found
			}
	  }`

	fs.SetContent(name1, []byte(input1))
	fs.SetContent(name2, []byte(input2))
	fs.SetContent(name3, []byte(input3))

	db := &ttcn3.DB{}
	db.Index(name1, name2, name3)

	// Lookup `Msg`
	list := lsp.FindMapConnectStatementForPortIdMatchingTheName(db, "p")
	assert.Equal(t, []protocol.Location{
		{URI: "test://TestFindAllMaps_input1", Range: protocol.Range{Start: protocol.Position{Line: 12, Character: 3}, End: protocol.Position{Line: 12, Character: 23}}},
		{URI: "test://TestFindAllMaps_input2", Range: protocol.Range{Start: protocol.Position{Line: 4, Character: 4}, End: protocol.Position{Line: 4, Character: 24}}},
		{URI: "test://TestFindAllMaps_input3", Range: protocol.Range{Start: protocol.Position{Line: 3, Character: 4}, End: protocol.Position{Line: 3, Character: 24}}},
	}, list)
}

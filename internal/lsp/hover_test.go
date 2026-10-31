package lsp_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/lsp"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/project"
	"github.com/nokia/ntt/ttcn3"
	"github.com/stretchr/testify/assert"
)

func TestPlainTextHoverForFunction(t *testing.T) {
	actual := testHover(t, `
        module Test {
            function myfunc(integer x)
                runs on Component
                system System
                return integer
            {
                return x;
            }
        }`,
		protocol.Position{Line: 2, Character: 25},
		&lsp.PlainTextHover{})

	expected :=
		"function myfunc(integer x)\n" +
			"  runs on Component\n" +
			"  system System\n" +
			"  return integer\n"

	assert.Equal(t, expected, actual.Contents.Value)
}

func TestMarkdownHoverForFunction(t *testing.T) {
	actual := testHover(t, `
        module Test {
            function myfunc(integer x)
                runs on Component
                system System
                return integer
            {
                return x;
            }
        }`,
		protocol.Position{Line: 2, Character: 25},
		&lsp.MarkdownHover{})

	expected :=
		"```ttcn3\n" +
			"function myfunc(integer x)\n" +
			"  runs on Component\n" +
			"  system System\n" +
			"  return integer\n" +
			"```\n"

	assert.Equal(t, expected, actual.Contents.Value)
}

func TestPlainTextHoverForPortDefFromDecl(t *testing.T) {
	actual := testHover(t, `
        module Test {
            type component Component {
                port P p1;
            }
            function myfunc(integer x)
                runs on Component
            {
                map(system:p1, mtc:p1);
                p1.receive;
            }
        }`,
		protocol.Position{Line: 3, Character: 24},
		&lsp.PlainTextHover{})

	expected :=
		"port P p1\n" +
			"possible map / connect statements\n" +
			"_________________________________\n" +
			filepath.FromSlash("/TestPlainTextHoverForPortDefFromDecl.ttcn3") + ":9\n"

	assert.Equal(t, expected, actual.Contents.Value)
}

func TestPlainTextHoverForPortDefFromUsage(t *testing.T) {
	actual := testHover(t, `
        module Test {
            type component Component {
                port P p1;
            }
            function myfunc(integer x)
                runs on Component
            {
                map(system:p1, mtc:p1);
                p1.receive;
            }
        }`,
		protocol.Position{Line: 9, Character: 16},
		&lsp.PlainTextHover{})

	expected :=
		"port P p1\n" +
			"possible map / connect statements\n" +
			"_________________________________\n" +
			filepath.FromSlash("/TestPlainTextHoverForPortDefFromUsage.ttcn3") + ":9\n"

	assert.Equal(t, expected, actual.Contents.Value)
}

func testHover(t *testing.T, text string, position protocol.Position, capability lsp.HoverContentProvider) *protocol.Hover {
	t.Helper()
	suite := &lsp.Suite{
		Config: &project.Config{},
		DB:     &ttcn3.DB{},
	}
	file := fmt.Sprintf("file://%s.ttcn3", t.Name())
	fs.SetContent(file, []byte(text))
	suite.Config.Sources = append(suite.Config.Sources, file)
	suite.DB.Index(suite.Config.Sources...)
	params := protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			Position: position,
			TextDocument: protocol.TextDocumentIdentifier{
				URI: protocol.DocumentURI(file),
			},
		},
	}

	hover, err := lsp.ProcessHover(&params, suite.DB, capability)
	assert.Equal(t, err, nil)
	return hover
}

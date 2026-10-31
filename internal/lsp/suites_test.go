package lsp

import (
	"testing"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/project"
	"github.com/nokia/ntt/ttcn3"
	"github.com/stretchr/testify/assert"
)

func newTestSuite(t *testing.T, files map[string]string) *Suite {
	t.Helper()
	suite := &Suite{
		Config: &project.Config{},
		DB:     &ttcn3.DB{},
	}
	for name, text := range files {
		fs.SetContent(name, []byte(text))
		suite.Config.Sources = append(suite.Config.Sources, name)
	}
	return suite
}

func TestSuiteContainsFile(t *testing.T) {
	suite := newTestSuite(t, map[string]string{
		"TestSuiteContainsFile_a.ttcn3": `module A {}`,
	})

	assert.True(t, suite.ContainsFile(string(fs.URI("TestSuiteContainsFile_a.ttcn3"))))
	assert.False(t, suite.ContainsFile("TestSuiteContainsFile_a.ttcn3"))
	assert.False(t, suite.ContainsFile(string(fs.URI("TestSuiteContainsFile_x.ttcn3"))))
}

func TestSuiteFindModule(t *testing.T) {
	suite := newTestSuite(t, map[string]string{
		"TestSuiteFindModule_a.ttcn3": `module Dup {}`,
		"TestSuiteFindModule_b.ttcn3": `module Dup {}`,
		"TestSuiteFindModule_c.ttcn3": `module Other {}`,
	})
	suite.DB.Index(suite.Files()...)

	file, err := suite.FindModule("Other")
	assert.Nil(t, err)
	assert.Equal(t, string(fs.URI("TestSuiteFindModule_c.ttcn3")), file)

	for i := 0; i < 10; i++ {
		file, err = suite.FindModule("Dup")
		assert.Nil(t, err)
		assert.Equal(t, string(fs.URI("TestSuiteFindModule_a.ttcn3")), file)
	}

	_, err = suite.FindModule("TestSuiteFindModule_NoSuchModule")
	assert.NotNil(t, err)
}

func TestSuitesOwners(t *testing.T) {
	suite := newTestSuite(t, map[string]string{
		"TestSuitesOwners_a.ttcn3": `module A {}`,
	})
	suites := Suites{roots: map[string]*Suite{"root": suite}}

	owners := suites.Owners(protocol.DocumentURI(fs.URI("TestSuitesOwners_a.ttcn3")))
	assert.Equal(t, []*Suite{suite}, owners)

	owners = suites.Owners(protocol.DocumentURI(fs.URI("TestSuitesOwners_x.ttcn3")))
	assert.Empty(t, owners)
}

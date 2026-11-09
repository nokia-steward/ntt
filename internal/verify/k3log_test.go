package verify

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeK3logLine(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"leading datetime stripped", `20260626T120800.582542|vrsn|k3r|12.0`, `vrsn|k3r|12.0`},
		{"exit line", `20260626T120800.999999|exit|0`, `exit|0`},
		{"plain ulog untouched", `ulog|k3r|"Hello, TTCN-3 world"`, `ulog|k3r|"Hello, TTCN-3 world"`},
		{"timestamp masked", `foo|timestamp=12.345678|bar`, `foo|timestamp=x.xxxxxx|bar`},
		{"t3xf offset removed", `tcst=t3xf:1234|m.tc`, `tcst|m.tc`},
		{"hex address removed", `entr|$1a2b3c=|x`, `entr||x`},
		{"error description removed", `abcd->some failure detail|tail`, `abcd|tail`},
		{"ttcn3 dir stripped", `ulog|MTC=/a/b/verd.ttcn3:7|none`, `ulog|MTC=verd.ttcn3:7|none`},
		{"t3xf dir stripped", `load|/home/x/y/hello.t3xf|ok`, `load|hello.t3xf|ok`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NormalizeK3logLine(tt.in))
		})
	}
}

func TestNormalizeK3logEmpty(t *testing.T) {
	assert.Equal(t, "", NormalizeK3log(nil))
	assert.Equal(t, "vrsn|k3r|12.0\nexit|0\n", NormalizeK3log([]string{
		`20260626T120800.582542|vrsn|k3r|12.0`,
		`20260626T120800.999999|exit|0`,
	}))
}

func TestDiffK3log(t *testing.T) {
	dir := t.TempDir()
	golden := filepath.Join(dir, "hello.log-expected")
	require.NoError(t, os.WriteFile(golden, []byte("ulog|k3r|\"Hello, TTCN-3 world\"\nexit|0\n"), 0644))

	// Raw actual log with variable content that must normalize to the golden.
	actual := []string{
		`20260626T120800.583918|ulog|k3r|"Hello, TTCN-3 world"`,
		`20260626T120800.999999|exit|0`,
	}

	t.Run("match", func(t *testing.T) {
		diff, err := DiffK3log(golden, actual)
		require.NoError(t, err)
		assert.Empty(t, diff, "normalized actual equals golden")
	})

	t.Run("mismatch", func(t *testing.T) {
		diff, err := DiffK3log(golden, []string{
			`20260626T120800.583918|ulog|k3r|"Goodbye"`,
			`20260626T120800.999999|exit|0`,
		})
		require.NoError(t, err)
		assert.NotEmpty(t, diff, "differing log yields a diff")
	})

	t.Run("missing golden", func(t *testing.T) {
		_, err := DiffK3log(filepath.Join(dir, "nope.log-expected"), actual)
		assert.Error(t, err)
	})
}

func TestWriteGoldenK3log(t *testing.T) {
	dir := t.TempDir()
	golden := filepath.Join(dir, "new.log-expected")
	actual := []string{
		`20260626T120800.583918|ulog|k3r|"Hello, TTCN-3 world"`,
		`20260626T120800.999999|exit|0`,
	}
	require.NoError(t, WriteGoldenK3log(golden, actual))

	// A freshly written golden must compare clean against the same run.
	diff, err := DiffK3log(golden, actual)
	require.NoError(t, err)
	assert.Empty(t, diff)

	b, err := os.ReadFile(golden)
	require.NoError(t, err)
	assert.Equal(t, "ulog|k3r|\"Hello, TTCN-3 world\"\nexit|0\n", string(b))
}

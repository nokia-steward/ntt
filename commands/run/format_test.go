package run

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/nokia/ntt/internal/k3log"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestColorFormatPaintsLogEvents(t *testing.T) {
	const stamp = "20210506T092317.322187"
	f := colorFormat{enabled: true}

	tests := []struct {
		name string
		line string
		code string
	}{
		{name: "error event", line: stamp + "|DOME|MTC=a.ttcn3:1|x", code: "1;37;41"},
		{name: "verdict mismatch", line: stamp + "|VMIS->Verdict mismatch|k3=a.ttcn3:3|T|pass|fail", code: "1;37;41"},
		{name: "tcfi error", line: stamp + "|tcfi|k3r=(unknown):0|T|error", code: "31"},
		{name: "cofi error", line: stamp + "|cofi|MTC|error", code: "31"},
		{name: "setv error", line: stamp + "|setv|MTC=a.ttcn3:1|fail|error|boom", code: "31"},
		{name: "tcfi inconc", line: stamp + "|tcfi|k3r=(unknown):0|T|inconc", code: "33"},
		{name: "cofi inconc", line: stamp + "|cofi|MTC|inconc", code: "33"},
		{name: "setv inconc", line: stamp + "|setv|MTC=a.ttcn3:1|none|inconc", code: "33"},
		{name: "tcfi pass", line: stamp + "|tcfi|k3r=(unknown):0|T|pass", code: "32"},
		{name: "cofi pass", line: stamp + "|cofi|MTC|pass", code: "32"},
		{name: "setv pass", line: stamp + "|setv|MTC=a.ttcn3:1|none|pass", code: "32"},
		{name: "tcfi fail", line: stamp + "|tcfi|k3r=(unknown):0|T|fail", code: "31"},
		{name: "cofi fail", line: stamp + "|cofi|MTC|fail", code: "31"},
		{name: "setv fail", line: stamp + "|setv|MTC=a.ttcn3:1|pass|fail|boom", code: "31"},
		{name: "setv uses the new verdict", line: stamp + "|setv|MTC=a.ttcn3:1|inconc|fail", code: "31"},
		{name: "tcfi none", line: stamp + "|tcfi|k3r=(unknown):0|T|none"},
		{name: "name containing pass", line: stamp + "|tcst|k3r=(unknown):0|Verdict.tc_pass|444.0"},
		{name: "plod", line: stamp + "|plod|k3|plugin.so|math|runtime", code: "2;90"},
		{name: "perf", line: stamp + "|perf|k3|load|1.2", code: "2;90"},
		{name: "vrsn", line: stamp + "|vrsn|k3|1.2.3", code: "2;90"},
		{name: "vach", line: stamp + "|vach|MTC=a.ttcn3:1|integer|x|1", code: "2;90"},
		{name: "bctr", line: stamp + "|bctr|MTC|frame", code: "2;90"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := k3log.ParseEvent(tt.line)
			require.NoError(t, err)
			got := f.Format(event)
			if tt.code == "" {
				assert.Equal(t, event.String(), got)
				return
			}
			assert.Equal(t, "\x1b["+tt.code+"m"+event.String()+resetFor(tt.code), got)
		})
	}

	t.Run("disabled color format stays plain", func(t *testing.T) {
		event, err := k3log.ParseEvent(stamp + "|tcfi|k3r=(unknown):0|T|pass")
		require.NoError(t, err)
		assert.Equal(t, event.String(), colorFormat{enabled: false}.Format(event))
	})
}

func TestFormatLineColorsAVerdictMismatch(t *testing.T) {
	line := "20210506T092317.322187|VMIS->Verdict mismatch|k3=a.ttcn3:3|Verdict.tc_error|pass|error"
	got := formatLine(colorFormat{enabled: true}, line)
	assert.Equal(t, "\x1b[1;37;41m"+line+"\x1b[22;0;0m", got)
	assert.Equal(t, "not a log event", formatLine(colorFormat{enabled: true}, "not a log event"))
}

func TestPlainFormatPrintsTheRawEvent(t *testing.T) {
	event, err := k3log.ParseEvent("20210506T092317.322187|tcfi|k3r=(unknown):0|T|fail")
	require.NoError(t, err)
	f, err := newFormatter("plain", os.Stdout)
	require.NoError(t, err)
	assert.Equal(t, event.String(), f.Format(event))
}

func TestNewFormatter(t *testing.T) {
	event, err := k3log.ParseEvent("20210506T092317.322187|tcfi|k3r=(unknown):0|T|pass")
	require.NoError(t, err)

	t.Run("color always paints", func(t *testing.T) {
		f, err := newFormatter("color", &bytes.Buffer{})
		require.NoError(t, err)
		assert.Equal(t, "\x1b[32m"+event.String()+"\x1b[0m", f.Format(event))
	})

	t.Run("auto leaves a pipe uncolored", func(t *testing.T) {
		f, err := newFormatter("auto", &bytes.Buffer{})
		require.NoError(t, err)
		assert.Equal(t, event.String(), f.Format(event))
	})

	t.Run("unknown format", func(t *testing.T) {
		_, err := newFormatter("tap", &bytes.Buffer{})
		require.Error(t, err)
		assert.ErrorContains(t, err, `"tap"`)
	})
}

func TestColorsFollowTheEnvironment(t *testing.T) {
	clear := func(t *testing.T) {
		t.Helper()
		t.Setenv("NO_COLOR", "")
		t.Setenv("K3_COLORS", "")
		t.Setenv("TERM", "xterm")
	}

	t.Run("NO_COLOR", func(t *testing.T) {
		clear(t)
		t.Setenv("NO_COLOR", "1")
		assert.False(t, colorsWanted(os.Stdout))
	})

	t.Run("TERM dumb", func(t *testing.T) {
		clear(t)
		t.Setenv("TERM", "dumb")
		assert.False(t, colorsWanted(os.Stdout))
	})

	t.Run("K3_COLORS never", func(t *testing.T) {
		clear(t)
		t.Setenv("K3_COLORS", "never")
		assert.False(t, colorsWanted(os.Stdout))
	})

	t.Run("a pipe is not a terminal", func(t *testing.T) {
		clear(t)
		assert.False(t, colorsWanted(&bytes.Buffer{}))
	})

	t.Run("a terminal is colored", func(t *testing.T) {
		clear(t)
		if !isTerminal(os.Stdout) {
			t.Skip("standard output is not a terminal")
		}
		assert.True(t, colorsWanted(os.Stdout))
	})
}

// resetFor is the SGR reset fatih/color emits for the given opening sequence.
func resetFor(code string) string {
	parts := strings.Split(code, ";")
	resets := make([]string, len(parts))
	for i, part := range parts {
		switch part {
		case "1", "2":
			resets[i] = "22"
		default:
			resets[i] = "0"
		}
	}
	return "\x1b[" + strings.Join(resets, ";") + "m"
}

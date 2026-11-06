package run

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nokia/ntt/internal/k3log"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutputCheck(t *testing.T) {
	const stamp = "20210506T092317.322187"
	outputs := []Output{
		{File: "/src/suite.ttcn3", Line: 5, Text: "1", Loc: "suite.ttcn3:5", Testcase: "Suite.A"},
		{File: "/src/suite.ttcn3", Line: 6, Text: `"a|b""c"`, Loc: "suite.ttcn3:6", Testcase: "Suite.A"},
		{File: "/src/suite.ttcn3", Line: 7, Text: `"value received: "123`, Loc: "suite.ttcn3:7", Testcase: "Suite.A"},
		{File: "/other/suite.ttcn3", Line: 9, Text: "x", Loc: "suite.ttcn3:9"},
	}
	event := func(line string) k3log.Event {
		t.Helper()
		e, err := k3log.ParseEvent(stamp + "|" + line)
		require.NoError(t, err)
		return e
	}

	tests := []struct {
		name   string
		event  k3log.Event
		lines  []string
		failed []outputFailure
	}{
		{
			name:  "absolute path matches",
			event: event("ulog|MTC=/src/suite.ttcn3:5|1"),
		},
		{
			name:  "stripped file name matches",
			event: event("ulog|MTC=suite.ttcn3:5|1"),
		},
		{
			name:  "relative path matches",
			event: event("ulog|worker=src/suite.ttcn3:5|1"),
		},
		{
			name:   "text differs",
			event:  event("ulog|MTC=/src/suite.ttcn3:5|2"),
			lines:  []string{outputMismatchLine(stamp, "suite.ttcn3:5", "1", "2")},
			failed: []outputFailure{{loc: "suite.ttcn3:5", expected: "1", actual: "2"}},
		},
		{
			name:  "unescaped bar matches",
			event: event(`ulog|MTC=/src/suite.ttcn3:6|"a|b""c"`),
		},
		{
			name:   "escaped bar does not match",
			event:  event(`ulog|MTC=/src/suite.ttcn3:6|"a\u007cb""c"`),
			lines:  []string{outputMismatchLine(stamp, "suite.ttcn3:6", `"a|b""c"`, `"a\u007cb""c"`)},
			failed: []outputFailure{{loc: "suite.ttcn3:6", expected: `"a|b""c"`, actual: `"a\u007cb""c"`}},
		},
		{
			name:  "several log arguments match",
			event: event(`ulog|MTC=/src/suite.ttcn3:7|"value received: "123`),
		},
		{
			name:  "unannotated line is ignored",
			event: event("ulog|MTC=/src/suite.ttcn3:8|anything"),
		},
		{
			name:  "other absolute file is ignored",
			event: event("ulog|MTC=/elsewhere/suite.ttcn3:5|2"),
		},
		{
			name:  "other base name is ignored",
			event: event("ulog|MTC=other.ttcn3:5|2"),
		},
		{
			name:  "partial base name is ignored",
			event: event("ulog|MTC=ite.ttcn3:5|2"),
		},
		{
			name:  "ulog without location is ignored",
			event: event("ulog|k3r|fail"),
		},
		{
			name:  "other event is ignored",
			event: event("setv|MTC=/src/suite.ttcn3:5|none|pass"),
		},
		{
			name:  "stripped name matches every file with that name",
			event: event("ulog|MTC=suite.ttcn3:9|x"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := newOutputCheck(outputs, true, nil)
			assert.Equal(t, tt.lines, check.observe(tt.event))
			assert.Equal(t, tt.failed, check.failed)
		})
	}

	t.Run("path relative to the working directory matches", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o755))
		t.Chdir(filepath.Join(dir, "sub"))
		check := newOutputCheck([]Output{{File: filepath.Join(dir, "suite.ttcn3"), Line: 5, Text: "1", Loc: "suite.ttcn3:5"}}, true, nil)
		assert.Empty(t, check.observe(event("ulog|MTC=../suite.ttcn3:5|1")))
		assert.Empty(t, check.finish(stamp))
	})

	t.Run("each logging statement on a line is checked", func(t *testing.T) {
		check := newOutputCheck(outputs, true, nil)
		assert.Empty(t, check.observe(event("ulog|MTC=/src/suite.ttcn3:5|1")))
		assert.Equal(t, []string{outputMismatchLine(stamp, "suite.ttcn3:5", "1", "3")}, check.observe(event("ulog|MTC=/src/suite.ttcn3:5|3")))
		assert.Equal(t, []outputFailure{{loc: "suite.ttcn3:5", expected: "1", actual: "3"}}, check.failed)
	})

	t.Run("complete run reports every line that logged nothing", func(t *testing.T) {
		check := newOutputCheck(outputs, true, nil)
		check.observe(event("ulog|MTC=/src/suite.ttcn3:5|1"))
		check.observe(event(`ulog|MTC=/src/suite.ttcn3:7|"value received: "123`))

		lines := check.finish(stamp)

		assert.Equal(t, []string{
			outputMismatchLine(stamp, "suite.ttcn3:6", `"a|b""c"`, ""),
			outputMismatchLine(stamp, "suite.ttcn3:9", "x", ""),
		}, lines)
		assert.Equal(t, []outputFailure{
			{loc: "suite.ttcn3:6", expected: `"a|b""c"`},
			{loc: "suite.ttcn3:9", expected: "x"},
		}, check.failed)
	})

	t.Run("mismatched line is not reported again as missing", func(t *testing.T) {
		check := newOutputCheck(outputs[:1], true, nil)
		check.observe(event("ulog|MTC=/src/suite.ttcn3:5|2"))
		assert.Empty(t, check.finish(stamp))
		assert.Len(t, check.failed, 1)
	})

	t.Run("filtered run reports only testcases that started", func(t *testing.T) {
		check := newOutputCheck(outputs, false, []Job{{Name: "Suite.B"}})
		assert.Empty(t, check.finish(stamp))

		check = newOutputCheck(outputs, false, []Job{{Name: "Suite.A"}})
		lines := check.finish(stamp)
		assert.Len(t, lines, 3)
		assert.Equal(t, outputMismatchLine(stamp, "suite.ttcn3:5", "1", ""), lines[0])
	})

	t.Run("mismatch line keeps one field per text", func(t *testing.T) {
		line := outputMismatchLine(stamp, "a.ttcn3:3", `"x|y"`, "")
		assert.Equal(t, stamp+`|OMIS->Output mismatch|k3=a.ttcn3:3|"x\u007cy"|`, line)
		e, err := k3log.ParseEvent(line)
		require.NoError(t, err)
		assert.True(t, e.Category.IsError())
		assert.Equal(t, "OMIS", e.ID())
		assert.Equal(t, "k3=a.ttcn3:3", e.Field(2))
		assert.Equal(t, `"x\u007cy"`, e.Field(3))
		assert.Empty(t, e.Field(4))
		assert.Empty(t, e.Field(5))
	})
}

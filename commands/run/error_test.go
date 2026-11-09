package run

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/nokia/ntt/internal/k3log"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// exitError returns the error of a process that exited with status code.
func exitError(t *testing.T, code int) error {
	t.Helper()
	err := exec.Command("sh", "-c", fmt.Sprintf("exit %d", code)).Run()
	require.Error(t, err)
	return fmt.Errorf("k3r: %w", err)
}

func TestErrorCheck(t *testing.T) {
	const stamp = "20210506T092317.322187"
	expected := []ExpectedError{
		{File: "/src/suite.ttcn3", Line: 5, Category: "CONV", Loc: "suite.ttcn3:5"},
		{File: "/src/suite.ttcn3", Line: 6, Loc: "suite.ttcn3:6"},
		{File: "/src/suite.ttcn3", Line: 9, Category: "DIV0", Loc: "suite.ttcn3:9", Testcase: "Suite.A"},
	}
	event := func(line string) k3log.Event {
		t.Helper()
		e, err := k3log.ParseEvent(stamp + "|" + line)
		require.NoError(t, err)
		return e
	}

	tests := []struct {
		name       string
		event      k3log.Event
		lines      []string
		failed     []errorFailure
		expected   int
		unexpected int
	}{
		{
			name:     "absolute path and category match",
			event:    event("CONV->Constraint violation|k3r=/src/suite.ttcn3:5|Suite.A|boolean(true)|(anon)|boolean(true,false)"),
			expected: 1,
		},
		{
			name:     "stripped file name matches",
			event:    event("CONV->Constraint violation|k3r=suite.ttcn3:5|Suite.A|x|y|z"),
			expected: 1,
		},
		{
			name:     "relative path matches",
			event:    event("CONV->Constraint violation|k3r=src/suite.ttcn3:5|Suite.A|x|y|z"),
			expected: 1,
		},
		{
			name:     "debug location after the code is ignored",
			event:    event("DIV0=/build/suite.t3xf:344(div)->Divide by zero|MTC=/src/suite.ttcn3:9|(anon)"),
			expected: 1,
		},
		{
			name:     "bare tag matches any category",
			event:    event("DOME->Domain error|k3r=/src/suite.ttcn3:6|x"),
			expected: 1,
		},
		{
			name:       "category differs",
			event:      event("DOME->Domain error|k3r=/src/suite.ttcn3:5|x"),
			lines:      []string{errorMismatchLine(stamp, "suite.ttcn3:5", "CONV", "DOME")},
			failed:     []errorFailure{{loc: "suite.ttcn3:5", expected: "CONV", actual: "DOME"}},
			unexpected: 1,
		},
		{
			name:       "untagged line is unexpected",
			event:      event("CONV->Constraint violation|k3r=/src/suite.ttcn3:4|x|y|z|w"),
			unexpected: 1,
		},
		{
			name:       "other absolute file is unexpected",
			event:      event("CONV->Constraint violation|k3r=/elsewhere/suite.ttcn3:5|x|y|z|w"),
			unexpected: 1,
		},
		{
			name:       "error without location is unexpected",
			event:      event("DIV0=/build/suite.t3xf:228(div)->Divide by zero|k3r|(anon)"),
			unexpected: 1,
		},
		{
			name:  "ordinary event is ignored",
			event: event("tcfi|k3r|Suite.A|error|DIV0->Divide by zero|MTC=/src/suite.ttcn3:9|(anon)"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := newErrorCheck(expected, true, nil)
			assert.Equal(t, tt.lines, check.observe(tt.event))
			assert.Equal(t, tt.failed, check.failed)
			assert.Equal(t, tt.expected, check.expected)
			assert.Equal(t, tt.unexpected, check.unexpected)
		})
	}

	t.Run("path relative to the working directory matches", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o755))
		t.Chdir(filepath.Join(dir, "sub"))
		check := newErrorCheck([]ExpectedError{{File: filepath.Join(dir, "suite.ttcn3"), Line: 5, Category: "CONV", Loc: "suite.ttcn3:5"}}, true, nil)
		assert.Empty(t, check.observe(event("CONV->Constraint violation|k3r=../suite.ttcn3:5|x|y|z|w")))
		assert.Empty(t, check.finish(stamp))
		assert.Equal(t, 1, check.expected)
	})

	t.Run("load rejection is accepted when every error was expected", func(t *testing.T) {
		check := newErrorCheck(expected[:1], true, nil)
		check.observe(event("CONV->Constraint violation|k3r=suite.ttcn3:5|x|y|z|w"))
		assert.Empty(t, check.finish(stamp))
		assert.True(t, check.accepts(exitError(t, loadFailure)))
	})

	t.Run("unexpected error keeps the rejection failing", func(t *testing.T) {
		check := newErrorCheck(expected[:1], true, nil)
		check.observe(event("CONV->Constraint violation|k3r=suite.ttcn3:5|x|y|z|w"))
		check.observe(event("CONV->Constraint violation|k3r=suite.ttcn3:7|x|y|z|w"))
		assert.False(t, check.accepts(exitError(t, loadFailure)))
	})

	t.Run("only exit status 2 is accepted", func(t *testing.T) {
		check := newErrorCheck(expected[:1], true, nil)
		check.observe(event("CONV->Constraint violation|k3r=suite.ttcn3:5|x|y|z|w"))
		assert.False(t, check.accepts(exitError(t, 3)))
		assert.False(t, check.accepts(errors.New("k3r: signal: killed")))
	})

	t.Run("rejection without an error event is not accepted", func(t *testing.T) {
		check := newErrorCheck(expected[:1], true, nil)
		assert.False(t, check.accepts(exitError(t, loadFailure)))
	})

	t.Run("suite without @error never accepts a rejection", func(t *testing.T) {
		check := newErrorCheck(nil, true, nil)
		check.observe(event("CONV->Constraint violation|k3r=suite.ttcn3:5|x|y|z|w"))
		assert.False(t, check.accepts(exitError(t, loadFailure)))
	})

	t.Run("complete run reports every line that reported no error", func(t *testing.T) {
		check := newErrorCheck(expected, true, nil)
		check.observe(event("CONV->Constraint violation|k3r=suite.ttcn3:5|x|y|z|w"))

		lines := check.finish(stamp)

		assert.Equal(t, []string{
			errorMismatchLine(stamp, "suite.ttcn3:6", "", ""),
			errorMismatchLine(stamp, "suite.ttcn3:9", "DIV0", ""),
		}, lines)
		assert.Equal(t, []errorFailure{
			{loc: "suite.ttcn3:6"},
			{loc: "suite.ttcn3:9", expected: "DIV0"},
		}, check.failed)
	})

	t.Run("mismatched line is not reported again as missing", func(t *testing.T) {
		check := newErrorCheck(expected[:1], true, nil)
		check.observe(event("DOME->Domain error|k3r=suite.ttcn3:5|x"))
		assert.Empty(t, check.finish(stamp))
		assert.Len(t, check.failed, 1)
	})

	t.Run("filtered run reports only testcases that started", func(t *testing.T) {
		check := newErrorCheck(expected, false, []Job{{Name: "Suite.B"}})
		assert.Empty(t, check.finish(stamp))

		check = newErrorCheck(expected, false, []Job{{Name: "Suite.A"}})
		assert.Equal(t, []string{errorMismatchLine(stamp, "suite.ttcn3:9", "DIV0", "")}, check.finish(stamp))
	})

	t.Run("mismatch line has the expected and actual category", func(t *testing.T) {
		line := errorMismatchLine(stamp, "a.ttcn3:3", "CONV", "DOME")
		assert.Equal(t, stamp+"|EMIS->Error mismatch|k3=a.ttcn3:3|CONV|DOME", line)
		e, err := k3log.ParseEvent(line)
		require.NoError(t, err)
		assert.True(t, e.Category.IsError())
		assert.Equal(t, "EMIS", e.ID())
		assert.Equal(t, "k3=a.ttcn3:3", e.Field(2))
	})
}

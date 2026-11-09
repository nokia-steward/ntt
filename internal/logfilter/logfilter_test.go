package logfilter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFiltersCombineInOrder(t *testing.T) {
	modules := []string{
		"UeProcedures",
		"UeHandling",
		"F1APControlMessages",
		"OamMessagesCpRt",
		"RadParams",
		"TimeForwarding",
	}
	args := []string{
		"--activate=fnen|fnlv",
		"--deactivate=UeProcedures:paon|rvon",
		"--deactivate=UeHandling:paon|rvon",
		"--deactivate=F1APControlMessages:paon|rvon",
		"--deactivate=OamMessagesCpRt:paon|rvon",
		"--deactivate=RadParams:f.en|f.lv",
		"--deactivate=TimeForwarding:tmto|else",
	}

	fs := Flags()
	require.NoError(t, fs.Parse(args))
	f, err := NewWithFlags(fs, modules)
	require.NoError(t, err)
	assert.Equal(t, args, f.Args())

	assert.True(t, f.Allows("fnen", "UeProcedures"))
	assert.False(t, f.Allows("paon", "UeProcedures"))
	assert.True(t, f.Allows("paon", "Other"))
	assert.False(t, f.Allows("fnen", "RadParams"))
	assert.False(t, f.Allows("tmto", "TimeForwarding"))
	assert.True(t, f.Allows("tmto", "UeProcedures"))
}

func TestShorthandFiltersKeepOrder(t *testing.T) {
	fs := Flags()
	require.NoError(t, fs.Parse([]string{"-D", "-a", "fnen|fnlv", "-A"}))
	f, err := NewWithFlags(fs, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"--deactivate-all",
		"--activate=fnen|fnlv",
		"--activate-all",
	}, f.Args())
}

func TestNoFiltersLeavesDefault(t *testing.T) {
	fs := Flags()
	require.NoError(t, fs.Parse(nil))
	f, err := NewWithFlags(fs, nil)
	require.NoError(t, err)
	assert.Empty(t, f.Args())
}

func TestUnknownModule(t *testing.T) {
	fs := Flags()
	require.NoError(t, fs.Parse([]string{"--deactivate=Missing:paon"}))
	_, err := NewWithFlags(fs, []string{"Suite"})
	require.Error(t, err)
	assert.ErrorContains(t, err, `"Missing"`)
}

func TestEventPatternMustMatch(t *testing.T) {
	tests := []struct {
		name string
		arg  string
	}{
		{name: "unknown event", arg: "--activate=not-an-event"},
		{name: "bad regexp", arg: "--deactivate=["},
		{name: "missing pattern", arg: "--activate="},
		{name: "missing module", arg: "--deactivate=:fnen"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := Flags()
			err := fs.Parse([]string{tt.arg})
			require.Error(t, err)
		})
	}
}

func TestAllowsStartsFromEveryEventEnabled(t *testing.T) {
	fs := Flags()
	require.NoError(t, fs.Parse([]string{"--deactivate=tcst"}))
	f, err := NewWithFlags(fs, nil)
	require.NoError(t, err)

	assert.False(t, f.Allows("tcst", ""))
	assert.True(t, f.Allows("tcfi", ""))
}

func TestDeactivateAllThenActivate(t *testing.T) {
	fs := Flags()
	require.NoError(t, fs.Parse([]string{"--deactivate-all", "--activate=fnen|fnlv"}))
	f, err := NewWithFlags(fs, nil)
	require.NoError(t, err)

	assert.True(t, f.Allows("fnen", "Suite"))
	assert.True(t, f.Allows("fnlv", "Suite"))
	assert.False(t, f.Allows("tcst", "Suite"))
}

func TestEventPatternMayBeARegularExpression(t *testing.T) {
	fs := Flags()
	require.NoError(t, fs.Parse([]string{"--deactivate=f.en|f.lv"}))
	f, err := NewWithFlags(fs, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"--deactivate=f.en|f.lv"}, f.Args())
}

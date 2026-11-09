package vm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nokia/ntt/internal/backend"
	"github.com/nokia/ntt/internal/k3log"
	"github.com/nokia/ntt/internal/t3xf"

	"github.com/nokia/ntt/ttcn3/syntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// compileModule compiles source with the in-process compiler and writes the
// T3XF into a temporary directory.
func compileModule(t *testing.T, name, source string) string {
	t.Helper()
	root := syntax.Parse([]byte(source))
	root.Filename = name + ".ttcn3"
	require.NoError(t, root.Err())
	c := t3xf.NewImageCompiler()
	require.NoError(t, c.Compile(root))
	code, err := c.Assemble()
	require.NoError(t, err)

	file := filepath.Join(t.TempDir(), name+".t3xf")
	require.NoError(t, os.WriteFile(file, code, 0o644))
	return file
}

func testRuntime(t *testing.T, source string, opts ...Option) *Runtime {
	t.Helper()
	r, err := NewRuntime(compileModule(t, "example", source), opts...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func moduleSource(body string) string {
	return "module example {\n\ttype component C {}\n" + body + "\n}\n"
}

// collect reads all events until the channel is closed.
func collect(t *testing.T, r *Runtime) []k3log.Event {
	t.Helper()
	var events []k3log.Event
	timeout := time.After(20 * time.Second)
	for {
		select {
		case e, ok := <-r.Events():
			if !ok {
				return events
			}
			events = append(events, e)
		case <-timeout:
			t.Fatal("timeout waiting for events")
		}
	}
}

// eventIDs returns the ids of events whose id is one of ids, in order.
func eventIDs(events []k3log.Event, ids ...string) []string {
	var got []string
	for _, e := range events {
		for _, id := range ids {
			if e.ID() == id {
				got = append(got, id)
			}
		}
	}
	return got
}

// jobNames returns the job names of the tcst and cpen events.
func jobNames(events []k3log.Event) []string {
	var names []string
	for _, e := range events {
		if e.ID() == "tcst" || e.ID() == "cpen" {
			names = append(names, strings.TrimSuffix(e.Field(3), "()"))
		}
	}
	return names
}

func TestRuntimeRunsRequestsInOrder(t *testing.T) {
	r := testRuntime(t, moduleSource(`
	testcase A() runs on C { setverdict(pass); }
	testcase B() runs on C { setverdict(fail); }
	control {}`))

	require.NoError(t, r.Start("example.A"))
	require.NoError(t, r.Start("example.control"))
	require.NoError(t, r.Start("example.B"))
	go func() { _ = r.CloseRequests() }()
	events := collect(t, r)

	assert.Equal(t, []string{"tcst", "tcfi", "cpen", "cplv", "tcst", "tcfi"}, eventIDs(events, "tcst", "tcfi", "cpen", "cplv"))
	assert.Equal(t, []string{"example.A", "example.control", "example.B"}, jobNames(events))
	var verdicts []string
	for _, e := range events {
		if e.ID() == "tcfi" {
			verdicts = append(verdicts, e.Field(4))
		}
	}
	assert.Equal(t, []string{"pass", "fail"}, verdicts)
	assert.NoError(t, r.Err())
}

func TestRuntimeUsesModuleParameterDefaults(t *testing.T) {
	r := testRuntime(t, moduleSource(`
	modulepar integer P := 7;
	testcase A() runs on C { log(P); setverdict(pass); }`))

	require.NoError(t, r.Start("example.A"))
	require.NoError(t, r.CloseRequests())
	events := collect(t, r)

	var logs []string
	for _, e := range events {
		if e.ID() == "ulog" {
			logs = append(logs, e.Field(3))
		}
	}
	assert.Equal(t, []string{"7"}, logs)
	assert.NoError(t, r.Err())
}

func TestRuntimeFaultYieldsErrorVerdict(t *testing.T) {
	r := testRuntime(t, moduleSource(`
	testcase Fail() runs on C { var integer x := 0; log(1 / x); }
	testcase Pass() runs on C { setverdict(pass); }`))

	require.NoError(t, r.Start("example.Fail"))
	require.NoError(t, r.Start("example.Pass"))
	require.NoError(t, r.CloseRequests())
	events := collect(t, r)

	assert.Equal(t, []string{"DIV0", "tcfi", "tcfi"}, eventIDs(events, "DIV0", "tcfi"))
	assert.NoError(t, r.Err())
}

func TestRuntimeStopsAtUnknownRequest(t *testing.T) {
	r := testRuntime(t, moduleSource(`
	testcase A() runs on C { setverdict(pass); }`))

	require.NoError(t, r.Start("example.Missing"))
	require.NoError(t, r.Start("example.A"))
	events := collect(t, r)

	assert.Empty(t, eventIDs(events, "tcst"))
	assert.ErrorContains(t, r.Err(), "example.Missing")
	assert.ErrorIs(t, r.Start("example.A"), backend.ErrClosed)
}

func TestRuntimeCloseClosesEvents(t *testing.T) {
	r := testRuntime(t, moduleSource(`
	testcase A() runs on C { setverdict(pass); }`))
	require.NoError(t, r.Start("example.A"))

	// Nobody consumes the events: Close must not hang.
	require.NoError(t, r.Close())

	select {
	case <-r.Done():
	default:
		t.Fatal("Done not closed after Close")
	}
	_, ok := <-r.Events()
	assert.False(t, ok)
	assert.ErrorIs(t, r.Start("example.A"), backend.ErrClosed)
}

func TestRuntimeContextStopsRuntime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := testRuntime(t, moduleSource(``), WithContext(ctx))

	cancel()
	collect(t, r)
	assert.ErrorIs(t, r.Err(), context.Canceled)
}

func TestRuntimeRejectsModuleParameters(t *testing.T) {
	file := compileModule(t, "example", moduleSource(`
	modulepar integer P := 7;`))

	_, err := NewRuntime(file, WithModuleParameters(backend.ParameterMap(map[string]string{"example.P": "1"})))
	assert.ErrorContains(t, err, "example.P")

	r, err := NewRuntime(file, WithModuleParameters(backend.ParameterMap(map[string]string{"other.P": "1"})))
	require.NoError(t, err)
	assert.NoError(t, r.Close())
}

func TestRuntimeFailsToLoadMissingFile(t *testing.T) {
	_, err := NewRuntime(filepath.Join(t.TempDir(), "missing.t3xf"))
	assert.Error(t, err)
}

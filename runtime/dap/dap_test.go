package dap_test

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nokia/ntt/runtime/dap"
)

type fakeHandler struct {
	tests   []string
	verdict string
	reason  string
}

func (f *fakeHandler) ListTests() []string { return f.tests }
func (f *fakeHandler) Launch(name string, emit func(string)) (string, string, error) {
	emit("running " + name)
	return f.verdict, f.reason, nil
}

func encodeRequest(seq int, cmd string, args any) []byte {
	body, _ := json.Marshal(args)
	m := dap.Message{
		Seq:     seq,
		Type:    "request",
		Command: cmd,
		Args:    body,
	}
	data, _ := json.Marshal(m)
	return []byte("Content-Length: " + itoa(len(data)) + "\r\n\r\n" + string(data))
}

func itoa(n int) string { return string([]byte{'0' + byte(n/100), '0' + byte((n/10)%10), '0' + byte(n%10)}) }

// syncBuffer is a concurrency-safe sink for the DAP server's output.
// The `launch` command emits its `output`/`terminated` events from a
// background goroutine (see dap.dispatch), so a test that polls the
// captured output races the writer unless both sides share a lock. It
// deliberately does not embed bytes.Buffer (which would promote an
// unlocked WriteString that io.WriteString would prefer over Write).
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestServe_InitializeAndThreads(t *testing.T) {
	h := &fakeHandler{tests: []string{"M.tc_a", "M.tc_b"}, verdict: "pass"}

	in := &bytes.Buffer{}
	in.Write(encodeRequest(1, "initialize", map[string]any{}))
	in.Write(encodeRequest(2, "threads", map[string]any{}))
	in.Write(encodeRequest(3, "disconnect", map[string]any{}))

	var out bytes.Buffer
	if err := dap.Serve(in, &out, h); err != nil && err != io.EOF {
		t.Fatalf("Serve: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		`"event":"initialized"`,
		`"command":"initialize"`,
		`"command":"threads"`,
		`"M.tc_a"`,
		`"M.tc_b"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in output:\n%s", want, got)
		}
	}
}

func TestServe_LaunchEmitsTerminated(t *testing.T) {
	h := &fakeHandler{tests: []string{"M.tc_a"}, verdict: "fail", reason: "boom"}

	in := &bytes.Buffer{}
	in.Write(encodeRequest(1, "launch", map[string]any{"test": "M.tc_a"}))
	in.Write(encodeRequest(2, "disconnect", map[string]any{}))

	out := &syncBuffer{}
	if err := dap.Serve(in, out, h); err != nil && err != io.EOF {
		t.Fatalf("Serve: %v", err)
	}

	// Launch is async - wait briefly for the terminated event to land.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(out.String(), `"event":"terminated"`) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	got := out.String()
	for _, want := range []string{`"event":"output"`, `"running M.tc_a\n"`, `"event":"terminated"`, `"fail"`, `"boom"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in output:\n%s", want, got)
		}
	}
}

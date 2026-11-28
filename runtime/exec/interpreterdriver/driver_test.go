package interpreterdriver

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nokia/ntt/runtime/report"
)

func TestRunKeepsLogs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "M.ttcn3")
	src := `module M {
  type component C {}
  function f() runs on C { log("ptc ", 2) }
  testcase tc() runs on C system C {
    log("heapAlloc=", 1234);
    var C ptc := C.create;
    ptc.start(f());
    ptc.done;
    setverdict(pass, "done");
  }
  testcase tc_quiet() runs on C { setverdict(pass) }
}
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	d := New([]string{path})
	v, reason, err := d.Run(context.Background(), "M.tc")
	if err != nil || v != report.Pass || reason != "done" {
		t.Fatalf("Run = %v, %q, %v", v, reason, err)
	}
	logs := d.LastLogs()
	if len(logs) != 2 || logs[0] != `"heapAlloc=" 1234` || logs[1] != `"ptc " 2` {
		t.Errorf("LastLogs = %q", logs)
	}
	if _, _, err := d.Run(context.Background(), "M.tc_quiet"); err != nil {
		t.Fatal(err)
	}
	if logs := d.LastLogs(); len(logs) != 0 {
		t.Errorf("LastLogs after a quiet run = %q", logs)
	}
}

package vm

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nokia/ntt/internal/runtime"

	"github.com/stretchr/testify/assert"
)

func runPortModule(t *testing.T, src string, adapter runtime.PortAdapter) string {
	t.Helper()
	image := mustLoad(t, mustCompile(t, src)).Image
	image.PortAdapter = adapter
	var log bytes.Buffer
	if err := Run(image, &log); err != nil {
		t.Fatalf("Run() failed: %v\n%s", err, log.String())
	}
	return log.String()
}

func assertPortPass(t *testing.T, log string) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(log), "\n") {
		if strings.Contains(line, "|tcfi|") {
			assert.True(t, strings.HasSuffix(line, "|pass"), log)
			return
		}
	}
	t.Fatalf("missing tcfi event:\n%s", log)
}

func TestMessagePortLoopback(t *testing.T) {
	log := runPortModule(t, `module M {
type port P message { inout integer }
type component C { port P p }
testcase tc() runs on C {
  p.send(2);
  alt {
    [] p.receive(1) { setverdict(fail) }
    [] p.receive(2) { setverdict(pass) }
  }
}
control { execute(tc()) }
}`, runtime.LoopbackAdapter)
	assertPortPass(t, log)
	assert.Contains(t, log, "|ptsd|")
	assert.Contains(t, log, "|ptrx|")
}

func TestMessagePortConnection(t *testing.T) {
	log := runPortModule(t, `module M {
type port P message { inout integer }
type component C { port P p }
function peer() runs on C {
  p.receive(7);
  setverdict(pass);
}
testcase tc() runs on C {
  var C c := C.create;
  connect(self:p, c:p);
  c.start(peer());
  p.send(7);
  c.done;
}
control { execute(tc()) }
}`, runtime.LoopbackAdapter)
	assertPortPass(t, log)
	assert.Contains(t, log, "|ptcn|")
	assert.Contains(t, log, "|ptqu|")
}

func TestMessagePortCheckAndSender(t *testing.T) {
	log := runPortModule(t, `module M {
type integer address;
type port P message { inout integer }
type component C { port P p }
testcase tc() runs on C {
  var address v_sender;
  p.send(1) to 80;
  any port.check(receive -> sender v_sender);
  any port.receive;
  if (v_sender == 80 and p.checkstate("Started")) {
    setverdict(pass);
  } else {
    setverdict(fail);
  }
}
control { execute(tc()) }
}`, runtime.LoopbackAdapter)
	assertPortPass(t, log)
	assert.Contains(t, log, "|ptck|")
}

func TestMessagePortBroadcastAdapter(t *testing.T) {
	log := runPortModule(t, `module M {
type port P message { inout integer }
type component C { port P a, b }
testcase tc() runs on C {
  a.send(3);
  b.receive(3);
  setverdict(pass);
}
control { execute(tc()) }
}`, runtime.BroadcastAdapter)
	assertPortPass(t, log)
}

func TestMessagePortTriggerArray(t *testing.T) {
	log := runPortModule(t, `module M {
type port P message { inout integer }
type component C { port P p[2], q[2] }
testcase tc() runs on C {
  var integer i;
  connect(self:p[0], self:q[0]);
  connect(self:p[1], self:q[1]);
  q[0].send(9);
  q[1].send(5);
  any from p.trigger(integer:5) -> @index value i;
  if (i == 1) { setverdict(pass) } else { setverdict(fail) }
}
control { execute(tc()) }
}`, runtime.LoopbackAdapter)
	assertPortPass(t, log)
	assert.Contains(t, log, "|ptds|")
	assert.Contains(t, log, "|alrp|")
}

func TestMessagePortLifecycle(t *testing.T) {
	log := runPortModule(t, `module M {
type port P message { inout integer }
type component C { port P p }
testcase tc() runs on C {
  p.stop;
  if (not p.checkstate("Stopped")) { setverdict(fail) }
  p.start;
  p.clear;
  p.halt;
  if (p.checkstate("Halted")) { setverdict(pass) } else { setverdict(fail) }
}
control { execute(tc()) }
}`, runtime.LoopbackAdapter)
	assertPortPass(t, log)
	assert.Contains(t, log, "|ptsp|")
	assert.Contains(t, log, "|ptst|")
	assert.Contains(t, log, "|ptcl|")
	assert.Contains(t, log, "|ptha|")
}

package vm

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const mtcPath = "/usr/local/libexec/mtc"
const k3rPath = "/usr/local/libexec/k3r"

var (
	stampRE = regexp.MustCompile(`\d{8}T\d{6}\.\d{6}`)
	errRE   = regexp.MustCompile(`^([A-Z]{4})=[^|]*->[^|]*\|`)
)

// TestReferenceEvents runs modules compiled by mtc and compares the events
// with the output of the reference runtime.
func TestReferenceEvents(t *testing.T) {
	if _, err := os.Stat(mtcPath); err != nil {
		t.Skip(err)
	}
	tests := []struct{ name, src, want string }{
		{"timers", `module timers {
type component C { timer ct := 0.01; }
testcase tc() runs on C {
  timer t := 0.02;
  log(t.running, t);
  t.start;
  log(t.running);
  t.stop;
  log(t.running);
  ct.start;
  ct.timeout;
  t.start(-1.0);
}
control { execute(tc()); }
}`, `cpen|k3r|timers.control()
tcst|k3r|timers.tc|unlimited
vach|k3r|timer|MTC.ct|{inactive,0.0100000000000000002082}
cocr|k3r|MTC|once
cost|MTC=timers.ttcn3:3|timers.tc
tcen|MTC|timers.tc()
vach|MTC=timers.ttcn3:4|timer|timers.tc.t|{inactive,0.0200000000000000004163}
tmru|MTC=timers.ttcn3:5|timers.tc.t|false
ulog|MTC=timers.ttcn3:5|false{inactive,0.0200000000000000004163}
tmst|MTC=timers.ttcn3:6|timers.tc.t|0.0200000000000000004163s
tmru|MTC=timers.ttcn3:7|timers.tc.t|true
ulog|MTC=timers.ttcn3:7|true
tmsp|MTC=timers.ttcn3:8|timers.tc.t
tmru|MTC=timers.ttcn3:9|timers.tc.t|false
ulog|MTC=timers.ttcn3:9|false
tmst|MTC=timers.ttcn3:10|MTC.ct|0.0100000000000000002082s
alen|MTC=timers.ttcn3:11
tmto|MTC=timers.ttcn3:11|MTC.ct|wait(TS)
alwt|MTC=timers.ttcn3:11|TS
tmto|MTC=timers.ttcn3:11|MTC.ct|ready
allv|MTC=timers.ttcn3:11
CONV|MTC=timers.ttcn3:12|timer|timer|timeout value|-1.0
bctr|MTC|timers.ttcn3:14:tc()
tcfi|k3r|timers.tc|error
cplv|k3r|timers.control()`},
		{"calls", `module calls {
type component C {}
function f(in integer a, inout integer b, out integer c) return integer {
  b := b + a; c := a * 2; return b + c;
}
function g() return integer { if (false) { return 1; } }
testcase tc() runs on C {
  var integer x := 1, y := 2, z;
  x := f(x, y, z);
  y := y;
  setverdict(pass);
}
control { execute(tc()); }
}`, `cpen|k3r|calls.control()
tcst|k3r|calls.tc|unlimited
cocr|k3r|MTC|once
cost|MTC=calls.ttcn3:7|calls.tc
tcen|MTC|calls.tc()
vach|MTC=calls.ttcn3:8|integer|calls.tc.x|1
vach|MTC=calls.ttcn3:8|integer|calls.tc.y|2
fnen|MTC=calls.ttcn3:9|calls.f(a=1,b=2,c=-)
vach|MTC=calls.ttcn3:4|integer|calls.f.b|3
vach|MTC=calls.ttcn3:4|integer|calls.f.c|2
fnlv|MTC=calls.ttcn3:9|calls.f(a=-,b=3,c=2)->5
vach|MTC=calls.ttcn3:9|integer|calls.tc.x|5
setv|MTC=calls.ttcn3:11|none|pass
tclv|MTC|calls.tc()
cofi|MTC|pass
tcfi|k3r|calls.tc|pass
cplv|k3r|calls.control()`},
		{"values", `module values {
type record R { integer a, integer b optional }
control {
  var float f := 1.5 * 2.0 - 0.25;
  var charstring s := "ab" & "c";
  var bitstring b := '1100'B and4b '1010'B;
  var octetstring o := 'A0'O xor4b 'FF'O;
  log(f, s, b, o, not4b '0F'O, '1100'B << 1, '1100'B <@ 1, "abc" @> 1);
  log(lengthof(s), s[1], b[0]);
  s[1] := "x";
  var R r := { 1, omit };
  r.a := 2;
  log(r, int2str(42), str2int("-7"), int2bit(5, 4), oct2int('FF'O), char2int("A"), float2int(2.7));
}
}`, `cpen|k3r|values.control()
vach|k3r|float|values.control.f|2.75
vach|k3r|charstring|values.control.s|"abc"
vach|k3r|bitstring|values.control.b|'1000'B
vach|k3r|octetstring|values.control.o|'5F'O
ulog|k3r|2.75"abc"'1000'B'5F'O'F0'O'1000'B'1001'B"cab"
ulog|k3r|3"b"'1'B
vach|k3r|charstring|values.control.s[1]|"x"
vach|k3r|values.R|values.control.r|{1,omit}
vach|k3r|values.R.a|values.control.r.a|2
ulog|k3r|{2,omit}"42"-7'0101'B255652
cplv|k3r|values.control()`},
		{"ctl", `module ctl {
type component C { timer t := 0.01; }
altstep as() runs on C { [] t.timeout { log("as"); } }
testcase tc() runs on C {
  var integer i := 0;
  for (i := 0; i < 2; i := i + 1) { log(i); }
  alt { [else] { log("else"); } }
  var default d := activate(as());
  t.start;
  alt { [false] t.timeout {} }
  deactivate(d);
  interleave { [] t.timeout {} [] t.timeout {} }
  setverdict(pass);
  log(getverdict);
  action("act");
}
control { execute(tc()); }
}`, `cpen|k3r|ctl.control()
tcst|k3r|ctl.tc|unlimited
vach|k3r|timer|MTC.t|{inactive,0.0100000000000000002082}
cocr|k3r|MTC|once
cost|MTC=ctl.ttcn3:4|ctl.tc
tcen|MTC|ctl.tc()
vach|MTC=ctl.ttcn3:5|integer|ctl.tc.i|0
vach|MTC=ctl.ttcn3:6|integer|ctl.tc.i|0
ulog|MTC=ctl.ttcn3:6|0
vach|MTC=ctl.ttcn3:6|integer|ctl.tc.i|1
ulog|MTC=ctl.ttcn3:6|1
vach|MTC=ctl.ttcn3:6|integer|ctl.tc.i|2
alen|MTC=ctl.ttcn3:7
else|MTC=ctl.ttcn3:7|ready
ulog|MTC=ctl.ttcn3:7|"else"
allv|MTC=ctl.ttcn3:7
dtac|MTC=ctl.ttcn3:8|ctl.as()
vach|MTC=ctl.ttcn3:8|default|ctl.tc.d|active
tmst|MTC=ctl.ttcn3:9|MTC.t|0.0100000000000000002082s
alen|MTC=ctl.ttcn3:10
dten|MTC=ctl.ttcn3:10|ctl.as()
tmto|MTC=ctl.ttcn3:3|MTC.t|wait(TS)
dtlv|MTC=ctl.ttcn3:10|ctl.as()
alwt|MTC=ctl.ttcn3:10|TS
dten|MTC=ctl.ttcn3:10|ctl.as()
tmto|MTC=ctl.ttcn3:3|MTC.t|ready
ulog|MTC=ctl.ttcn3:3|"as"
dtlv|MTC=ctl.ttcn3:10|ctl.as()
allv|MTC=ctl.ttcn3:10
dtde|MTC=ctl.ttcn3:11|ctl.as()
ilen|MTC=ctl.ttcn3:12
tmto|MTC=ctl.ttcn3:12|MTC.t|never(inactive)
tmto|MTC=ctl.ttcn3:12|MTC.t|never(inactive)
illv|MTC=ctl.ttcn3:12
DEAD|MTC=ctl.ttcn3:12
bctr|MTC|ctl.ttcn3:17:tc()
tcfi|k3r|ctl.tc|error
cplv|k3r|ctl.control()`},
		{"checks", `module checks {
type record R { integer a, integer b optional }
type union U { integer i, charstring s }
type enumerated E { e0, e1(5) }
control {
  var R r;
  log(isbound(r), isbound(r.a), ispresent(r.b));
  r := { 1, omit };
  log(isbound(r), ispresent(r.b), isvalue(r));
  var U u := { s := "x" };
  log(ischosen(u.s), ischosen(u.i), enum2int(E:e1));
}
}`, `cpen|k3r|checks.control()
ulog|k3r|falsefalsefalse
vach|k3r|checks.R|checks.control.r|{1,omit}
ulog|k3r|truefalsetrue
vach|k3r|checks.U|checks.control.u|{s:="x"}
ulog|k3r|truefalse5
cplv|k3r|checks.control()`},
		{"patterns", `module patterns {
const charstring cs := "b+";
template charstring tp := pattern "a{cs}c";
control {
  log(match("abbc", tp), match("ac", tp));
  log(match("a1", pattern "a\d"), match("aZZ", pattern "a[A-Z]#(2)"), match("x", pattern "[^x]"));
  log(regexp("key=val", charstring:"(\w+)=(\w+)", 1));
}
}`, `cpen|k3r|patterns.control()
matc|k3r|"abbc"|"a{cs}c"|
matc|k3r|"ac"|"a{cs}c"|value:"ac"~"a{cs}c"
ulog|k3r|truefalse
matc|k3r|"a1"|"a\d"|
matc|k3r|"aZZ"|"a[A-Z]#(2)"|
matc|k3r|"x"|"[^x]"|value:"x"~"[^x]"
ulog|k3r|truetruefalse
ulog|k3r|"val"
cplv|k3r|patterns.control()`},
		{"faults", `module faults {
type component C {}
type record R { integer a, R2 b optional }
type record R2 { integer x }
type record of integer RI;
testcase t1() runs on C { var integer a; var integer b := a; }
testcase t2() runs on C { var R r := { 1, omit }; log(r.b.x); }
testcase t3() runs on C { var RI l := {1}; log(isbound(l[3])); log(l[3]); }
testcase t4() runs on C { var R r; log(r.a); }
control { execute(t1()); execute(t2()); execute(t3()); execute(t4()); }
}`, `cpen|k3r|faults.control()
tcst|k3r|faults.t1|unlimited
cocr|k3r|MTC|once
cost|MTC=faults.ttcn3:6|faults.t1
tcen|MTC|faults.t1()
UNDF|MTC=faults.ttcn3:6|faults.t1.a
bctr|MTC|faults.ttcn3:10:t1()
tcfi|k3r|faults.t1|error
tcst|k3r|faults.t2|unlimited
cocr|k3r|MTC|once
cost|MTC=faults.ttcn3:7|faults.t2
tcen|MTC|faults.t2()
vach|MTC=faults.ttcn3:7|faults.R|faults.t2.r|{1,omit}
OMIT|MTC=faults.ttcn3:7|faults.t2.r.b
bctr|MTC|faults.ttcn3:10:t2()
tcfi|k3r|faults.t2|error
tcst|k3r|faults.t3|unlimited
cocr|k3r|MTC|once
cost|MTC=faults.ttcn3:8|faults.t3
tcen|MTC|faults.t3()
vach|MTC=faults.ttcn3:8|faults.RI|faults.t3.l|{1}
ulog|MTC=faults.ttcn3:8|false
ARRV|MTC=faults.ttcn3:8|faults.t3.l|3
bctr|MTC|faults.ttcn3:10:t3()
tcfi|k3r|faults.t3|error
tcst|k3r|faults.t4|unlimited
cocr|k3r|MTC|once
cost|MTC=faults.ttcn3:9|faults.t4
tcen|MTC|faults.t4()
UNDF|MTC=faults.ttcn3:9|faults.t4.r
bctr|MTC|faults.ttcn3:10:t4()
tcfi|k3r|faults.t4|error
cplv|k3r|faults.control()`},
		{"maps", `module maps {
type map from charstring to integer T;
control {
  var T m;
  m["a"] := 1;
  log(m["a"], m.from, m.to);
}
}`, `cpen|k3r|maps.control()
vach|k3r|maps.T.to[-]|maps.control.m["a"]|1
ulog|k3r|1{"a"}{1}
cplv|k3r|maps.control()`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, tt.name+".ttcn3")
			obj := filepath.Join(dir, tt.name+".t3xf")
			if err := os.WriteFile(src, []byte(tt.src), 0o644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(mtcPath, "-o", obj, filepath.Base(src))
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("mtc: %v\n%s", err, out)
			}
			b, err := os.ReadFile(obj)
			if err != nil {
				t.Fatal(err)
			}
			img, err := Load(b)
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := Run(img, &out); err != nil {
				t.Fatalf("run: %v\n%s", err, out.String())
			}
			var got []string
			for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
				line = line[strings.Index(line, "|")+1:]
				if strings.HasPrefix(line, "plod|") || strings.HasPrefix(line, "perf|") || strings.HasPrefix(line, "vrsn|") {
					continue
				}
				got = append(got, errRE.ReplaceAllString(stampRE.ReplaceAllString(line, "TS"), "$1|"))
			}
			assert.Equal(t, strings.Split(tt.want, "\n"), got)
		})
	}
}

func TestReviewedReferenceEvents(t *testing.T) {
	if _, err := os.Stat(mtcPath); err != nil {
		t.Skip(err)
	}
	if _, err := os.Stat(k3rPath); err != nil {
		t.Skip(err)
	}
	tests := []struct{ name, src string }{
		{"position", `module position {
type component C {}
function id(integer x) return integer { return x + 10; }
control { log(id(1)); }
}`},
		{"localtimer", `module localtimer {
type component C {}
function f() runs on C { timer t := 0.01; t.start; }
testcase tc() runs on C { f(); any timer.timeout; }
control { execute(tc()); }
}`},
		{"timerloop", `module timerloop {
type component C {}
testcase tc() runs on C {
  var integer i := 0;
  while (i < 2) { timer x := 0.01; if (i == 0) { x.start; } i := i + 1; }
  timer y := 0.02; y.start;
  any timer.timeout;
}
control { execute(tc()); }
}`},
		{"stopped", `module stopped {
type component C {}
testcase tc() runs on C { setverdict(pass); stop; }
control { execute(tc()); }
}`},
		{"cow", `module cow {
type record of integer RI;
type record of RI RRI;
type record R { RI l }
type component C {}
function f(inout integer e) runs on C { e := 5; }
testcase tc() runs on C {
  var RRI n := { { 1, 2 } }; n[0][1] := 5;
  var R s := { { 1 } }; s.l[0] := 5;
  f(s.l[0]);
}
control { execute(tc()); }
}`},
		{"anytimer", `module anytimer {
type component C {}
testcase tc() runs on C { timer t; any timer.timeout; }
control { execute(tc()); }
}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, tt.name+".ttcn3")
			obj := filepath.Join(dir, tt.name+".t3xf")
			if err := os.WriteFile(src, []byte(tt.src), 0o644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(mtcPath, "-o", obj, filepath.Base(src))
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("mtc: %v\n%s", err, out)
			}
			want, err := exec.Command(k3rPath, obj).CombinedOutput()
			if err != nil {
				t.Fatalf("reference: %v\n%s", err, want)
			}
			b, err := os.ReadFile(obj)
			if err != nil {
				t.Fatal(err)
			}
			img, err := Load(b)
			if err != nil {
				t.Fatal(err)
			}
			var got bytes.Buffer
			if err := Run(img, &got); err != nil {
				t.Fatalf("run: %v\n%s", err, got.String())
			}
			assert.Equal(t, referenceEvents(string(want)), referenceEvents(got.String()))
		})
	}
}

func referenceEvents(out string) []string {
	var events []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = line[strings.Index(line, "|")+1:]
		if strings.HasPrefix(line, "plod|") || strings.HasPrefix(line, "perf|") || strings.HasPrefix(line, "vrsn|") {
			continue
		}
		events = append(events, errRE.ReplaceAllString(stampRE.ReplaceAllString(line, "TS"), "$1|"))
	}
	return events
}

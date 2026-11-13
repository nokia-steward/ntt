package interpreter_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

const castClasses = `
		type component C {}
		type class Super {}
		type class Sub extends Super {}
		type class Other {}
`

// `objRef => ClassType` re-types a reference the object satisfies, and
// `objRef of ClassType` tests its runtime class against that class and
// its bases (ETSI 5.1.2.5 / 5.1.2.6, Sem_5010206_Casting_001).
func TestObjectCast_DowncastThenTypeTest(t *testing.T) {
	runPass(t, "M.tc", `module M {`+castClasses+`
		testcase tc() runs on C {
			var Super v_a := Sub.create();
			var Sub v_b := v_a => Sub;
			if (v_b of Sub and v_b of Super and v_a of Sub) { setverdict(pass); }
			else { setverdict(fail); }
		}
	}`)
}

// The `of` test is false for a class the object is unrelated to, and for
// a null reference, which is of no class at all.
func TestObjectCast_OfIsFalseForUnrelatedAndNull(t *testing.T) {
	runPass(t, "M.tc", `module M {`+castClasses+`
		testcase tc() runs on C {
			var Super v_a := Sub.create();
			var Super v_null := null;
			if (not (v_a of Other) and not (v_null of Super)) { setverdict(pass); }
			else { setverdict(fail); }
		}
	}`)
}

// A cast the object does not satisfy is a dynamic error, so the testcase
// reports the bad cast rather than failing an unrelated assertion later.
func TestObjectCast_UnrelatedCastIsAnError(t *testing.T) {
	src := `module M {` + castClasses + `
		testcase tc() runs on C {
			var Super v_a := Super.create();
			var Sub v_b := v_a => Sub;
			setverdict(pass);
		}
	}`
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{parse(t, src)}, "M.tc")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	if v != runtime.ErrorVerdict || !strings.Contains(reason, "is not a Sub") {
		t.Fatalf("verdict = %s (%s), want error naming Sub", v, reason)
	}
}

// Casting a null object reference is an error too: there is no object to
// re-type.
func TestObjectCast_NullCastIsAnError(t *testing.T) {
	src := `module M {` + castClasses + `
		testcase tc() runs on C {
			var Super v_a := null;
			var Sub v_b := v_a => Sub;
			setverdict(pass);
		}
	}`
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{parse(t, src)}, "M.tc")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	if v != runtime.ErrorVerdict || !strings.Contains(reason, "null object reference") {
		t.Fatalf("verdict = %s (%s), want error about the null reference", v, reason)
	}
}

// Inside a class, its members are named without `this.` (ES 203 790
// 5.1.1, 5.1.1.9): a method's parameters and locals hide a member, a
// member hides a definition of the module, and a method called by its
// name is the most specific one of the object's class.
func TestClassMembersByName(t *testing.T) {
	runPass(t, "M.tc", `module M {
		const integer a := 1;
		function name() return charstring { return "module" }
		type component C {}
		type class K {
			const integer a := 2;
			var integer n := 0;
			public function name() return charstring { return "base" }
			public function f(integer a := 3) return integer { return a * 100 + this.a * 10 + M.a }
			public function g() return integer { return a }
			public function who() return charstring { return name() }
			public function local() return charstring { var charstring name := "local"; return name }
			public function inc() return integer { n := n + 1; return n }
		}
		type class K2 extends K {
			public function name() return charstring { return "sub" }
		}
		testcase tc() runs on C {
			var K k := K.create();
			var K2 k2 := K2.create();
			k.inc();
			if (k.f() == 321 and k.g() == 2 and k.who() == "base" and k2.who() == "sub" and
			    k.local() == "local" and name() == "module" and k.inc() == 2) {
				setverdict(pass)
			} else {
				setverdict(fail, k.f(), k.g(), k.who(), k2.who(), k.local())
			}
		}
	}`)
}

// TestClassScopeOfAMethod: a method sees the members of its own class —
// its own and those it inherits, not a private one of a base class, nor
// one of a subclass (ES 203 790 5.1.1.1); a method it names is the
// object's most specific one, through super as well (5.1.1.8); and a
// field's initialiser may name a field declared after it (5.1.1.6).
func TestClassScopeOfAMethod(t *testing.T) {
	for name, src := range map[string]string{
		"an inherited method calls the object's override, through super too": `module M {
  type component C {}
  type class K {
    public function vg() return integer { return 1 }
    public function callvg() return integer { return this.vg() }
    public function callbare() return integer { return vg() }
  }
  type class K2 extends K {
    public function vg() return integer { return 2 }
    public function viaSuper() return integer { return super.callvg() }
    public function viaSuperBare() return integer { return super.callbare() }
  }
  testcase tc() runs on C {
    var K2 k2 := K2.create();
    log("this.vg via super=", k2.viaSuper(), " bare via super=", k2.viaSuperBare(), " direct=", k2.callbare());
    if (k2.viaSuper() == 2 and k2.viaSuperBare() == 2) { setverdict(pass) } else { setverdict(fail) }
  }
}`,
		"an abstract method called by its name, through super": `module M {
  function vg() return integer { return 100 }
  type component C {}
  type class @abstract K {
    public function @abstract vg() return integer;
    public function callvg() return integer { return vg() }
  }
  type class K2 extends K {
    public function vg() return integer { return 2 }
    public function callvg() return integer { return super.callvg() + 1000 }
  }
  testcase tc() runs on C {
    var K2 k2 := K2.create();
    if (k2.callvg() == 1002) { setverdict(pass) } else { setverdict(fail, k2.callvg()) }
  }
}`,
		"a base class's private field is not a subclass's": `module M {
  const integer x := 100;
  type component C {}
  type class K {
    private var integer x := 1;
    var integer n := 0;
    public function inc() return integer { n := n + 1; return n }
    public function other(K o) return integer { var integer r := o.inc(); return r * 10 + n }
    public function rec(integer d) return integer { if (d == 0) { return n } n := n + 1; return rec(d - 1) }
  }
  type class K2 extends K {
    public function getx() return integer { return x }
  }
  testcase tc() runs on C {
    var K a := K.create(), b := K.create();
    b.inc(); b.inc();
    var integer r := a.other(b);
    var integer q := a.rec(3);
    var K2 k2 := K2.create();
    var integer gx := k2.getx();
    log("other=", r, " rec=", q, " getx=", gx);
    if (r == 30 and q == 3 and gx == 100) { setverdict(pass) } else { setverdict(fail) }
  }
}`,
		"a subclass's field is not its base class's": `module M {
  const integer w := 5;
  type component C {}
  type class K {
    public function getw() return integer { return w }
  }
  type class K2 extends K {
    var integer w := 9;
  }
  testcase tc() runs on C {
    var K2 k2 := K2.create();
    var K k := K.create();
    log("k.getw=", k.getw(), " k2.getw=", k2.getw());
    if (k2.getw() == 5) { setverdict(pass) } else { setverdict(fail, k2.getw()) }
  }
}`,
		"a subclass's method is not its base class's": `module M {
  function helper() return charstring { return "module" }
  type component C {}
  type class K {
    public function run() return charstring { return helper() }
  }
  type class K2 extends K {
    public function helper() return charstring { return "subclass" }
  }
  testcase tc() runs on C {
    var K2 k2 := K2.create();
    if (k2.run() == "module") { setverdict(pass) } else { setverdict(fail, k2.run()) }
  }
}`,
		"a private method is its class's own": `module M {
			type component C {}
			type class A {
				private function p() return integer { return 1 }
				public function callp() return integer { return p() }
			}
			type class B extends A {
				public function p(integer a, integer b) return integer { return a + b }
			}
			testcase tc() runs on C {
				var B b := B.create();
				if (b.callp() == 1) { setverdict(pass) } else { setverdict(fail, b.callp()) }
			}
		}`,
		"this.p() with p private is the class's own": `module M {
  type component C {}
  type class K {
    private function p() return integer { return 1 }
    public function viaThis() return integer { return this.p() }
  }
  type class K2 extends K {
    private function p() return integer { return 2 }
  }
  testcase tc() runs on C {
    var K2 o := K2.create();
    log("viaThis=", o.viaThis());
    if (o.viaThis() == 1) { setverdict(pass) } else { setverdict(fail) }
  }
}`,
		"a class extending itself": `module M {
  type component C {}
  type class A extends B { public function f() return integer { return g() } }
  type class B extends A { }
  function g() return integer { return 1 }
  testcase tc() runs on C {
    var A o := A.create();
    log("f=", o.f());
    setverdict(pass)
  }
}`,
		"a field initialised from a later one": `module M {
			const integer y := 7;
			type component C {}
			type class K {
				var integer z := y;
				var integer y := 3;
				public function get() return integer { return z * 10 + y }
			}
			testcase tc() runs on C {
				var K k := K.create();
				if (k.get() == 33) { setverdict(pass) } else { setverdict(fail, k.get()) }
			}
		}`,
	} {
		t.Run(name, func(t *testing.T) { runPass(t, "M.tc", src) })
	}
}

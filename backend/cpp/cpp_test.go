package cpp_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/nokia/ntt/backend/cpp"
	"github.com/nokia/ntt/ir"
)

// buildSampleModule mirrors the Go-backend smoke test so we keep both
// backends honest against the same IR. See backend/golang/golang_test.go.
func buildSampleModule() *ir.Module {
	m := &ir.Module{Name: "Sample"}

	add := m.AddFunction(&ir.Function{Name: "add", Return: ir.TypeInt})
	pa := add.NewValue(ir.TypeInt)
	pb := add.NewValue(ir.TypeInt)
	add.Params = []*ir.Value{pa, pb}
	ab := ir.NewBuilder(add)
	ab.Return(ab.Add(pa, pb))

	tc := m.AddFunction(&ir.Function{Name: "tc_smoke", Return: ir.TypeVoid, IsTestcase: true})
	tb := ir.NewBuilder(tc)
	five := tb.ConstInt(5)
	r := tb.Call("add", ir.TypeInt, tb.ConstInt(2), tb.ConstInt(3))
	cond := tb.Eq(r, five)

	pass := tc.NewBlock("pass")
	fail := tc.NewBlock("fail")
	tb.CondBranch(cond, pass, fail)

	tb.SetBlock(pass)
	tb.SetVerdict("pass")
	tb.Return(nil)

	tb.SetBlock(fail)
	tb.SetVerdict("fail")
	tb.Return(nil)

	return m
}

func TestGenerate_HeaderAndNamespace(t *testing.T) {
	var buf bytes.Buffer
	if err := cpp.Generate(&buf, buildSampleModule()); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	src := buf.String()
	for _, want := range []string{
		`#include "ntt/runtime.hpp"`,
		"namespace ntt_tests {",
		"add(",
		"tc_smoke(",
		"goto block_pass",
		"::ntt::Verdict::Pass",
		"::ntt::Verdict::Fail",
		"run_suite",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q in:\n%s", want, src)
		}
	}
}

func TestGenerate_CompilesWithGpp(t *testing.T) {
	// Skip if g++ isn't available or doesn't support -std=c++17.
	if _, err := exec.LookPath("g++"); err != nil {
		t.Skip("no g++ on PATH")
	}
	dir := t.TempDir()
	var buf bytes.Buffer
	if err := cpp.Generate(&buf, buildSampleModule()); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	src := buf.String()

	// A minimal stub `ntt/runtime.hpp` so the generated file compiles
	// to object code without the real runtime headers. The point of
	// this test is to catch syntax errors in the codegen, not to
	// validate the runtime - the e2e test in e2e_test.go exercises
	// the real runtime header.
	stub := `#pragma once
#include <cstdint>
#include <string>
#include <utility>
#include <vector>
namespace ntt {
enum class Verdict { None, Pass, Inconc, Fail, Error };
struct Context {};
class TestcaseExec {
public:
    explicit TestcaseExec(std::string) {}
    void setVerdict(Verdict, std::string = {}) {}
    Verdict getVerdict() const { return Verdict::None; }
    const std::string& reason() const { static std::string s; return s; }
    void log(std::string) {}
    const std::vector<std::string>& logs() const { static std::vector<std::string> s; return s; }
};
}`

	if err := writeFile(dir+"/ntt/runtime.hpp", stub); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(dir+"/gen.cpp", src); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("g++", "-std=c++17", "-I", dir, "-c", "-o", dir+"/gen.o", dir+"/gen.cpp")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("g++ failed: %v\n%s\n---\n%s", err, out, src)
	}
}

func writeFile(path, content string) error {
	if err := mkdirAll(path); err != nil {
		return err
	}
	return writeBytes(path, []byte(content))
}

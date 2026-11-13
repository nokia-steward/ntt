package framework_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nokia/ntt/internal/framework"
)

func TestNewPlugin(t *testing.T) {
	t.Skip("C++ plugin fixture not part of ntt (requires k3 toolchain)")
}

func TestNewT3XF(t *testing.T) {
	srcdir, _ := initStage(t)

	b := framework.NewT3XF(framework.DefaultEnv, "suite.t3xf", []string{filepath.Join(srcdir, "testdata/suite/test.ttcn3")})[0]
	err := b.Run()
	if err != nil {
		t.Errorf("Run() = %v", err)
	}

	out := b.Outputs()
	if len(out) != 1 {
		t.Fatalf("Unexpected numberof build artifacts: %v", out)
	}

	_, err = os.Stat(out[0])
	if err != nil {
		t.Error(err)
	}
}

func initStage(t *testing.T) (string, string) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	// The source directory, absolute: on Windows the temporary
	// directory can be on another drive, which no relative path reaches.
	t.Cleanup(func() {
		_ = os.Chdir(old)
	})

	return old, dir
}

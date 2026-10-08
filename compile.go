package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nokia/ntt/backend/cpp"
	"github.com/nokia/ntt/backend/golang"
	"github.com/nokia/ntt/ir"
	"github.com/nokia/ntt/ir/lower"
	"github.com/nokia/ntt/ttcn3"
	"github.com/spf13/cobra"
)

var (
	compileTarget string
	compileOut    string

	// CompileCommand runs the AST -> IR -> source pipeline. It is the
	// command-line surface of `ir/lower` + `backend/{golang,cpp}`. The
	// output is source code (Go or C++) that links against runtime/*
	// and can be compiled into a standalone test binary with the
	// usual toolchain (`go build`, `g++ -std=c++17`).
	CompileCommand = &cobra.Command{
		Use:   "compile [file.ttcn3]",
		Short: "Compile a TTCN-3 module to Go or C++ source",
		Long: `compile lowers a single TTCN-3 file through the IR (ir/lower) and
emits source code for the target backend. The generated code links
against the ntt runtime packages and can be compiled into a standalone
test binary with the usual toolchain.

The current pipeline supports the subset of TTCN-3 the interpreter
also handles: integer / boolean / string scalars, top-level functions
and testcases, if / while control flow, setverdict, log. Anything
outside that subset produces a "skip" diagnostic on stderr; the rest
of the module still lowers successfully.`,
		Args: cobra.ExactArgs(1),
		RunE: runCompile,
	}
)

func init() {
	RootCommand.AddCommand(CompileCommand)
	CompileCommand.Flags().StringVar(&compileTarget, "target", "go",
		"output target: 'go' or 'cpp'")
	CompileCommand.Flags().StringVar(&compileOut, "out", "",
		"output file (default: stdout)")
}

func runCompile(cmd *cobra.Command, args []string) error {
	path := args[0]
	tree := ttcn3.ParseFile(path)
	if tree == nil {
		return fmt.Errorf("compile: parse failed for %s", path)
	}
	if tree.Err != nil {
		return fmt.Errorf("compile: %s: %w", path, tree.Err)
	}

	m, diags := lower.Module(tree)
	if m == nil {
		return fmt.Errorf("compile: no module found in %s", path)
	}
	for _, d := range diags {
		fmt.Fprintf(os.Stderr, "lower: %s\n", d.Message)
	}

	out := io.Writer(os.Stdout)
	if compileOut != "" {
		f, err := os.Create(compileOut)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}

	switch strings.ToLower(compileTarget) {
	case "go", "golang":
		return golang.Generate(out, m)
	case "cpp", "c++":
		return cpp.Generate(out, m)
	case "ir":
		_, err := io.WriteString(out, ir.Dump(m))
		return err
	}
	return fmt.Errorf("compile: unknown target %q (want go|cpp|ir)", compileTarget)
}

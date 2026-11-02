package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/nokia/ntt/backend/cpp"
	"github.com/nokia/ntt/backend/golang"
	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/ir"
	"github.com/nokia/ntt/ir/lower"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
	"github.com/spf13/cobra"
)

var (
	CompileCommand = &cobra.Command{
		Use:   "compile [file.ttcn3]",
		Short: "Compile TTCN-3 sources and generate output for other tools",
		Long: `Compile TTCN-3 sources and generate output for other tools.

With --target, compile lowers a single TTCN-3 file through the IR
(ir/lower) and emits source code for the target backend: Go or C++
that links against the ntt runtime packages and can be compiled into a
standalone test binary with the usual toolchain, or the IR itself. The
pipeline supports the subset of TTCN-3 the interpreter also handles:
integer / boolean / string scalars, top-level functions and testcases,
if / while control flow, setverdict, log. Anything outside that subset
produces a "skip" diagnostic on stderr; the rest of the module still
lowers successfully.`,

		RunE: compile,
	}

	format        string
	compileTarget string
	compileOut    string
)

func init() {
	CompileCommand.Flags().StringVarP(&format, "generator", "G", "stdout", "generator to use (default stdout)")
	CompileCommand.Flags().StringVar(&compileTarget, "target", "",
		"lower one file and emit it for a backend: 'go', 'cpp' or 'ir'")
	CompileCommand.Flags().StringVar(&compileOut, "out", "",
		"with --target, the output file (default: stdout)")
}

func compile(cmd *cobra.Command, args []string) error {
	if compileTarget != "" {
		if len(args) != 1 {
			return fmt.Errorf("compile: --target takes one file")
		}
		return compileToTarget(args[0])
	}

	srcs, err := fs.TTCN3Files(Project.Sources...)
	if err != nil {
		return err
	}

	imports, err := fs.TTCN3Files(Project.Imports...)
	if err != nil {
		return err
	}

	files := append(srcs, imports...)

	if format == "stdout" {
		writeSource(os.Stdout, files...)
		return nil
	}

	generator, err := exec.LookPath(fmt.Sprintf("ntt-gen-%s", format))
	if err != nil {
		return fmt.Errorf("could not find generator %q", format)
	}
	proc := exec.Command(generator)
	proc.Stdout = os.Stdout
	proc.Stderr = os.Stderr
	stdin, err := proc.StdinPipe()
	if err != nil {
		return err
	}

	go func() {
		defer stdin.Close()
		writeSource(stdin, files...)
	}()

	if err := proc.Run(); err != nil {
		return err
	}
	return nil
}

// compileToTarget runs the AST -> IR -> source pipeline of ir/lower and
// backend/{golang,cpp} on the file at path.
func compileToTarget(path string) error {
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

func writeSource(w io.Writer, files ...string) {
	for _, file := range files {
		src := buildSource(file)
		b, err := json.MarshalIndent(src, "", "  ")
		if err != nil {
			fatal(err)
		}
		if _, err := w.Write(b); err != nil {
			fatal(err)
		}
	}
}

func buildSource(file string) ttcn3.Source {
	src := ttcn3.Source{
		Filename: file,
	}
	tree := ttcn3.ParseFile(file)
	if tree.Root == nil {
		fatal(tree.Err)
	}
	var stack []int
	for _, e := range syntax.Serialize(tree.Root) {
		switch e.Kind {
		case syntax.TokenEvent:
			src.Events = append(src.Events, ttcn3.NodeEvent{
				Kind: "add",
				Text: e.Token().String(),
				Offs: e.Node.Pos(),
			})
		case syntax.OpenEvent:
			k := strings.TrimPrefix(strings.TrimPrefix(fmt.Sprintf("%T", e.Node), "*"), "syntax.")
			stack = append(stack, len(src.Events))
			src.Events = append(src.Events, ttcn3.NodeEvent{
				Kind: "open",
				Node: k,
			})
		case syntax.CloseEvent:
			idx := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			src.Events = append(src.Events, ttcn3.NodeEvent{
				Kind: "close",
			})
			src.Events[idx].End = len(src.Events) - 1
		}
	}
	return src
}

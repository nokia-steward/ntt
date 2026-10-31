package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
	"github.com/spf13/cobra"
)

var (
	CompileCommand = &cobra.Command{
		Use:   "compile",
		Short: "Compile TTCN-3 sources and generate output for other tools",
		Long:  `Compile TTCN-3 sources and generate output for other tools.`,

		RunE: compile,
	}

	format string
)

func init() {
	CompileCommand.Flags().StringVarP(&format, "generator", "G", "stdout", "generator to use (default stdout)")
}

func compile(cmd *cobra.Command, args []string) error {
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

func writeSource(w io.Writer, files ...string) {
	for _, file := range files {
		src := buildSource(file)
		b, err := json.MarshalIndent(src, "", "  ")
		if err != nil {
			fatal(err)
		}
		w.Write(b)
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
				Offs: int(e.Node.Pos()),
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

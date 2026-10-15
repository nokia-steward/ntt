package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/nokia/ntt/runtime/dap"
	"github.com/nokia/ntt/runtime/explorer"
	rreport "github.com/nokia/ntt/runtime/report"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
	"github.com/spf13/cobra"
)

var (
	// ExplorerCommand groups the JSON-stream commands the VS Code
	// extension drives. Two subcommands exist: `list` for the static
	// tree, `dap` to expose a Debug Adapter Protocol server over stdio.
	ExplorerCommand = &cobra.Command{
		Use:   "explorer",
		Short: "IDE integration: JSON tree and DAP server",
	}

	ExplorerListCommand = &cobra.Command{
		Use:   "list [path...]",
		Short: "Print the testcase tree as a JSON document",
		RunE:  runExplorerList,
	}

	ExplorerDAPCommand = &cobra.Command{
		Use:   "dap",
		Short: "Run a Debug Adapter Protocol server over stdio",
		RunE:  runExplorerDAP,
	}
)

func init() {
	RootCommand.AddCommand(ExplorerCommand)
	ExplorerCommand.AddCommand(ExplorerListCommand)
	ExplorerCommand.AddCommand(ExplorerDAPCommand)
}

func runExplorerList(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		args = []string{"."}
	}
	files := collectTTCN3Files(args)
	tree := explorer.Tree{Project: "ntt"}
	if Project != nil {
		tree.Project = Project.Name
	}
	mods := map[string]*explorer.Module{}
	for _, p := range files {
		t := ttcn3.ParseFile(p)
		if t == nil || t.Root == nil {
			continue
		}
		for _, modNode := range t.Modules() {
			mod, ok := modNode.Node.(*syntax.Module)
			if !ok {
				continue
			}
			modName := syntax.Name(mod.Name)
			m := mods[modName]
			if m == nil {
				m = &explorer.Module{Name: modName, File: p}
				mods[modName] = m
				tree.Modules = append(tree.Modules, *m)
			}
			mod.Inspect(func(n syntax.Node) bool {
				fn, ok := n.(*syntax.FuncDecl)
				if !ok {
					return true
				}
				if !fn.IsTest() {
					return true
				}
				tc := explorer.Testcase{
					Name:     syntax.Name(fn.Name),
					FullName: modName + "." + syntax.Name(fn.Name),
					File:     p,
				}
				for i := range tree.Modules {
					if tree.Modules[i].Name == modName {
						tree.Modules[i].Testcases = append(tree.Modules[i].Testcases, tc)
					}
				}
				return true
			})
		}
	}
	return explorer.WriteTree(os.Stdout, tree)
}

func runExplorerDAP(cmd *cobra.Command, args []string) error {
	// The DAP server reuses the same staticDriver as `ntt exec`, so
	// the IDE story stays consistent with the CLI story until the
	// interpreter rewire lands.
	if len(args) == 0 {
		args = []string{"."}
	}
	files := collectTTCN3Files(args)
	driver := newStaticDriver(files)
	h := dapDriverAdapter{driver: driver}
	return dap.Serve(os.Stdin, os.Stdout, h)
}

type dapDriverAdapter struct {
	driver *staticDriver
}

func (a dapDriverAdapter) ListTests() []string { return a.driver.List() }

func (a dapDriverAdapter) Launch(name string, emit func(string)) (string, string, error) {
	emit(fmt.Sprintf("launching %s", name))
	v, reason, err := a.driver.Run(context.Background(), name)
	verdict := rreport.Verdict(v).String()
	if err != nil {
		return verdict, reason, err
	}
	emit(fmt.Sprintf("verdict=%s", verdict))
	return verdict, reason, nil
}

var _ io.Writer = (*os.File)(nil)

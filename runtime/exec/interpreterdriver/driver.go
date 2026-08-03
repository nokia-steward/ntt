// Package interpreterdriver adapts TTCN-3 source files to the generic
// runtime/exec scheduler using the tree-walking interpreter.
package interpreterdriver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/runtime/report"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// Driver discovers testcases by walking TTCN-3 syntax trees, then
// dispatches each testcase through interpreter.RunTestcaseWith.
type Driver struct {
	cases    []string
	owner    map[string]string      // testcase name -> file path
	trees    map[string]*ttcn3.Tree // file path -> parsed tree
	modParam map[string]string      // cfg [MODULE_PARAMETERS]
}

// CollectTTCN3Files walks each argument and returns every TTCN-3 source
// file under it. Hidden directories are skipped to avoid recursing into
// .git and local tool caches.
func CollectTTCN3Files(args []string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, a := range args {
		info, err := os.Stat(a)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			if isTTCN3(a) && !seen[a] {
				seen[a] = true
				out = append(out, a)
			}
			continue
		}
		_ = filepath.Walk(a, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				base := filepath.Base(p)
				if strings.HasPrefix(base, ".") && p != a {
					return filepath.SkipDir
				}
				return nil
			}
			if isTTCN3(p) && !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
			return nil
		})
	}
	sort.Strings(out)
	return out
}

func isTTCN3(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ttcn", ".ttcn3", ".ttcnpp":
		return true
	}
	return false
}

// New returns an interpreter-backed exec.Driver over the provided files.
func New(files []string) *Driver {
	d := &Driver{
		owner: map[string]string{},
		trees: map[string]*ttcn3.Tree{},
	}
	seen := map[string]bool{}
	for _, p := range files {
		tree := ttcn3.ParseFile(p)
		if tree == nil || tree.Root == nil {
			continue
		}
		d.trees[p] = tree
		for _, modNode := range tree.Modules() {
			mod, ok := modNode.Node.(*syntax.Module)
			if !ok {
				continue
			}
			modName := syntax.Name(mod.Name)
			mod.Inspect(func(n syntax.Node) bool {
				fn, ok := n.(*syntax.FuncDecl)
				if !ok || !fn.IsTest() {
					return true
				}
				qn := modName + "." + syntax.Name(fn.Name)
				if seen[qn] {
					return true
				}
				seen[qn] = true
				d.cases = append(d.cases, qn)
				d.owner[qn] = p
				return true
			})
		}
	}
	sort.Strings(d.cases)
	return d
}

// SetModuleParameters records cfg [MODULE_PARAMETERS] for each testcase run.
func (d *Driver) SetModuleParameters(params map[string]string) error {
	if len(params) == 0 {
		d.modParam = nil
		return nil
	}
	cp := make(map[string]string, len(params))
	for k, v := range params {
		cp[k] = v
	}
	d.modParam = cp
	return nil
}

// List returns every discovered testcase as a fully-qualified name.
func (d *Driver) List() []string {
	out := append([]string(nil), d.cases...)
	sort.Strings(out)
	return out
}

// Run executes one testcase through the tree-walking interpreter.
func (d *Driver) Run(ctx context.Context, name string) (report.Verdict, string, error) {
	_ = ctx
	path := d.owner[name]
	if path == "" {
		return report.Error, "testcase not found", nil
	}
	tree := d.trees[path]
	if tree == nil {
		tree = ttcn3.ParseFile(path)
	}
	if tree == nil || tree.Err != nil {
		reason := "parse error"
		if tree != nil && tree.Err != nil {
			reason = tree.Err.Error()
		}
		return report.Error, reason, nil
	}

	trees := make([]*ttcn3.Tree, 0, len(d.trees))
	trees = append(trees, tree)
	for p, t := range d.trees {
		if p == path || t == nil {
			continue
		}
		trees = append(trees, t)
	}
	// Deliberately leaves DeterministicClock and DeterministicScheduler
	// off: this driver executes real test suites, so timers must pace real
	// I/O rather than jump a virtual clock. Semantics default to strict.
	opts := interpreter.TestcaseOptions{
		ModuleParameters: d.modParam,
		ModuleParamWarning: func(msg string) {
			fmt.Fprintf(os.Stderr, "module parameter: %s\n", msg)
		},
	}
	v, reason, err := interpreter.RunTestcaseWith(trees, name, opts)
	if err != nil {
		return report.Error, err.Error(), nil
	}
	return mapVerdict(v), reason, nil
}

func mapVerdict(v runtime.Verdict) report.Verdict {
	switch v {
	case runtime.PassVerdict:
		return report.Pass
	case runtime.InconcVerdict:
		return report.Inconc
	case runtime.FailVerdict:
		return report.Fail
	case runtime.ErrorVerdict:
		return report.Error
	case runtime.NoneVerdict:
		return report.None
	default:
		return report.None
	}
}

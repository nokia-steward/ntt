package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/project"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/semantic"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// runFiles are the files of the suite: its sources and its imports.
func runFiles(conf *project.Config) ([]string, error) {
	srcs, err := fs.TTCN3Files(conf.Sources...)
	if err != nil {
		return nil, err
	}
	imports, err := fs.TTCN3Files(conf.Imports...)
	if err != nil {
		return nil, err
	}
	return append(srcs, imports...), nil
}

// buildSuite is what building the suite takes: every file parses, and, when
// analyze is set (a module expects to be rejected), the suite is checked
// as ntt check does.
func buildSuite(files []string, analyze bool) error {
	var errs []error
	var db *ttcn3.DB
	if analyze {
		db = &ttcn3.DB{}
		db.Index(files...)
	}
	for _, f := range files {
		tree := ttcn3.ParseFile(f)
		if tree == nil {
			errs = append(errs, fmt.Errorf("%s: cannot be read", f))
			continue
		}
		if tree.Err != nil {
			errs = append(errs, tree.Err)
			continue
		}
		if !analyze {
			continue
		}
		for _, d := range semantic.NewAnalyzer(db).Analyze(tree) {
			if d.Severity == semantic.SeverityError {
				errs = append(errs, fmt.Errorf("%s: %s", f, d.Message))
			}
		}
	}
	return errors.Join(errs...)
}

// moduleParameters are the module parameters of the project's parameters
// file; a YAML block scalar's final newline is not part of the value, and an
// empty value keeps the declared default.
func moduleParameters(conf *project.Config) map[string]string {
	if conf == nil {
		return nil
	}
	m := map[string]string{}
	for k, v := range conf.Parameters.Parameters {
		if v = strings.TrimRight(v, "\n"); v != "" {
			m[k] = v
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// hasJob reports whether the suite has the testcase or the control part
// ("Module.control") name.
func (d *staticDriver) hasJob(name string) bool {
	if _, ok := d.owner[name]; ok {
		return true
	}
	mod, ok := strings.CutSuffix(name, ".control")
	if !ok {
		return false
	}
	for _, p := range d.files {
		tree := d.trees[p]
		if tree == nil {
			continue
		}
		for _, m := range tree.Modules() {
			node, ok := m.Node.(*syntax.Module)
			if !ok || syntax.Name(node.Name) != mod {
				continue
			}
			for _, def := range node.Defs {
				if _, ok := def.Def.(*syntax.ControlPart); ok {
					return true
				}
			}
		}
	}
	return false
}

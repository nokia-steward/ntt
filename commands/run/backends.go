package run

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/nokia/ntt/internal/backend"
	"github.com/nokia/ntt/internal/backend/vm"
	"github.com/nokia/ntt/internal/cache"
	"github.com/nokia/ntt/internal/framework"
	"github.com/nokia/ntt/project"
)

// A Backend is a runtime ntt run executes a suite with, named by --backend.
type Backend struct {
	// Build makes the suite ready to run, and returns why it does not
	// build. analyze asks for the static checks too, as a module expects
	// to be rejected.
	Build func(conf *project.Config, analyze bool) error

	// Runtime starts the backend on the suite, with the module parameters
	// of the parameters file.
	Runtime func(ctx context.Context, conf *project.Config, params backend.ModuleParFunc) (backend.Runtime, error)
}

// backends are the backends --backend names, by name.
var backends = map[string]Backend{}

// RegisterBackend makes b available to --backend as name.
func RegisterBackend(name string, b Backend) {
	backends[name] = b
}

// SetDefaultBackend makes name the backend ntt run uses without --backend.
func SetDefaultBackend(name string) {
	backendName = name
	if f := Command.Flags().Lookup("backend"); f != nil {
		f.DefValue = name
	}
}

// backendNames lists the registered backends, for messages.
func backendNames() string {
	names := make([]string, 0, len(backends))
	for n := range backends {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// t3xfBackend compiles the suite to T3XF and runs it on the virtual
// machine in process.
var t3xfBackend = Backend{
	Build: func(conf *project.Config, _ bool) error {
		tasks, err := framework.BuildTasks(conf)
		if err != nil {
			return err
		}
		for _, t := range tasks {
			if err := t.Run(); err != nil {
				return fmt.Errorf("+ %s\n%w", t.String(), err)
			}
		}
		return nil
	},
	Runtime: func(ctx context.Context, conf *project.Config, params backend.ModuleParFunc) (backend.Runtime, error) {
		t3xf := cache.Lookup(fmt.Sprintf("%s.t3xf", conf.Name))
		return vm.NewRuntime(t3xf, vm.WithContext(ctx), vm.WithModuleParameters(params))
	},
}

func init() {
	RegisterBackend("t3xf", t3xfBackend)
	RegisterBackend("vm", t3xfBackend) // its former name
}

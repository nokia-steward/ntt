// Package cli implements a helper libraries for building command line
// interfaces consistent across the k3 project.
package cli

import (
	"github.com/nokia/ntt/internal/framework"

	"github.com/nokia/ntt/project"
)

// Parse parses the command line arguments and returns a project configuration
// using the arguments before the first "--". The arguments after the "--" are
// returned as is.
func Parse(pos int, args ...string) (*project.Config, []string, error) {
	var files []string
	if pos < 0 {
		files, args = args[0:], nil
	} else {
		files, args = args[0:pos], args[pos:]
	}
	conf, err := project.Open(files...)
	if err == nil {
		err = framework.WithK3()(conf)
	}

	return conf, args, err
}

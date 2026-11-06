package proc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/nokia/ntt/internal/env"
	"github.com/nokia/ntt/internal/log"
)

// Command returns a exec.Cmd struct to execute the named program with the given arguments.
// The parent death signal is set to SIGKILL for platforms that support it.
func Command(name string, args ...string) *exec.Cmd {
	return CommandContext(context.Background(), name, args...)
}

// CommandContext is like Command but includes a context.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	setPdeathsig(cmd, syscall.SIGKILL)
	return cmd
}

// Exec executes the named program with the given arguments.
// On Unix the parent process is replaced by the new process; on Windows a child runs.
func Exec(name string, args ...string) error {
	return execute(name, args...)
}

// Task returns a Cmd struct which implements the project.Task interface.
func Task(s string) *Cmd {
	return &Cmd{
		Line: s,
	}
}

type Cmd struct {

	// Before is called before the command is executed.
	Before func(*Cmd) error

	// Line is the command line to execute.
	Line string

	// Sources are used to substitute the $srcs in the command line.
	Sources []string

	// Sources are used to substitute the $tgts in the command line.
	Targets []string

	// Env is the environment to use for the command.
	Env map[string]string
}

func (c *Cmd) Inputs() []string {
	return c.Sources
}

func (c *Cmd) Outputs() []string {
	return c.Targets
}

func (c *Cmd) String() string {
	return strings.Join(c.args(), " ")
}

func (c *Cmd) Run() error {
	args := c.args()
	if len(args) == 0 {
		return fmt.Errorf("empty command")
	}
	for _, output := range c.Targets {
		older, err := IsOlder(output, c.Sources...)
		if err != nil {
			return err
		}
		if older {
			if c.Before != nil {
				if err := c.Before(c); err != nil {
					return err
				}
			}
			log.Verboseln("+", c.String())
			cmd := Command(args[0], args[1:]...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Env = append(env.Env(c.Env).Slice(), os.Environ()...)
			return cmd.Run()
		}
	}
	return nil
}

func (c *Cmd) args() []string {
	expand := func(s string) string {
		if s == "srcs" || s == "tgts" {
			return fmt.Sprintf(" ${%s} ", s)
		}
		if s, ok := env.LookupEnv(s); ok {
			return s
		}
		return c.Env[s]
	}

	var args []string
	for _, f := range strings.Fields(os.Expand(c.Line, expand)) {
		f = strings.TrimSpace(f)
		switch {
		case f == "${srcs}":
			args = append(args, c.Sources...)
		case f == "${tgts}":
			args = append(args, c.Targets...)
		case f != "":
			args = append(args, f)
		}
	}
	return args
}

// IsOlder returns true if the target is older than any of the source files.
//
// If the target does not exist, IsOlder returns true.
// It returns an error if any of the sources does not exist.
func IsOlder(target string, sources ...string) (bool, error) {
	stat, err := os.Stat(os.ExpandEnv(target))
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	for _, src := range sources {
		src = os.ExpandEnv(src)
		sstat, err := os.Stat(src)
		if err != nil {
			return false, err
		}
		if sstat.ModTime().After(stat.ModTime()) {
			return true, nil
		}
	}
	return false, nil
}

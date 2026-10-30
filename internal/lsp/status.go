package lsp

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"text/template"

	"github.com/dustin/go-humanize"
	"github.com/nokia/ntt/project"
)

const statusTemplate = `

=== Language Server Status ===

Executable   : {{ .Executable }}
Version      : {{ .Version }}
Process ID   : {{ .PID }}
Memory cache : {{ memory }}


=== Session ===

{{range .Suites}}
Root Folder: {{ .Root }}
Known Files: {{ range files .}}
	- {{.}}{{end}}

{{end}}
`

type Status struct {
	Executable string
	Version    string
	PID        int
	Suites     []*Suite
}

var funcMap = template.FuncMap{
	"files": func(suite *Suite) []string {
		f, _ := project.Files(suite.Config)
		return f
	},
	"memory": func() string {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return humanize.Bytes(m.Alloc)
	},
}

func NewStatus(suites []*Suite) *Status {
	s := Status{
		PID:    os.Getpid(),
		Suites: suites,
	}

	if path, err := os.Executable(); err == nil {
		s.Executable = path
		if out, err := exec.Command(path, "version").Output(); err == nil {
			s.Version = strings.TrimSpace(string(out))
		}
	}

	return &s
}

func (s *Server) status(ctx context.Context) (interface{}, error) {
	t := template.Must(template.New("ntt.status").Funcs(funcMap).Parse(statusTemplate))
	var suites []*Suite
	for _, s := range s.roots {
		suites = append(suites, s)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, NewStatus(suites)); err != nil {
		s.Log(context.TODO(), "An error occurred during collecting status information. This is probably a ntt bug:")
		s.Log(context.TODO(), err.Error())
	}
	s.Log(ctx, buf.String())
	return nil, nil
}

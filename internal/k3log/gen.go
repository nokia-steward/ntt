//go:build ignore

package main

import (
	"github.com/nokia/ntt/internal/k3log"
	"github.com/nokia/ntt/internal/yaml"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"text/template"

	"github.com/5nord/templates"
)

var funcs = template.FuncMap{
	"uc":  strings.ToUpper,
	"now": time.Now,
	"line": func(id string, d *k3log.Descriptor) string {
		parts := []string{id}
		if d != nil {
			parts = append(parts, d.Description)
		}
		parts = append(parts, d.Fields...)
		return strconv.Quote(strings.Join(parts, "|"))
	},
	"quote": strconv.Quote,
}

func main() {

	events := make(map[string]*k3log.Descriptor)

	b, err := os.ReadFile("events.yaml")
	if err != nil {
		log.Fatal(err)
	}
	if err := yaml.Unmarshal(b, &events); err != nil {
		log.Fatal(err)
	}

	if err := templates.Generate(".", funcs, events); err != nil {
		log.Fatal(err)
	}
}

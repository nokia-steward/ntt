//go:build ignore

package main

import (
	"bytes"
	"fmt"
	"go/format"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/nokia/ntt/internal/t3xf/opcode"
	"github.com/nokia/ntt/internal/yaml"
)

var funcs = template.FuncMap{
	"uc": strings.ToUpper,
	"encode": func(op int) string {
		return fmt.Sprintf("0x%04x", op*16+3)
	},
	"comment": func(s string) string {
		s = strings.TrimSpace(s)
		if s == "" {
			return ""
		}

		return "// " + strings.ReplaceAll(s, "\n", "\n// ")
	},
	"quote": func(s string) string {
		return fmt.Sprintf("%q", s)
	},
	"join": func(a []string, sep string) string {
		return strings.Join(a, sep)
	},
	"arg": func(a opcode.Arg) string {
		switch a {
		case opcode.ArgImm:
			return "ArgImm"
		case opcode.ArgInt:
			return "ArgInt"
		case opcode.ArgFloat:
			return "ArgFloat"
		case opcode.ArgString:
			return "ArgString"
		case opcode.ArgBits:
			return "ArgBits"
		case opcode.ArgNibbles:
			return "ArgNibbles"
		case opcode.ArgOctets:
			return "ArgOctets"
		default:
			log.Fatalf("unknown arg kind %q", a)
			return ""
		}
	},
}

func main() {
	opcodes := make(map[string]*opcode.Descriptor)

	b, err := os.ReadFile("opcodes.yml")
	if err != nil {
		log.Fatal(err)
	}
	if err := yaml.Unmarshal(b, &opcodes); err != nil {
		log.Fatal(err)
	}

	files, err := filepath.Glob("*.tmpl")
	if err != nil {
		log.Fatal(err)
	}
	for _, file := range files {
		var buf bytes.Buffer
		t := template.Must(template.New(file).Funcs(funcs).ParseFiles(file))
		if err := t.Execute(&buf, opcodes); err != nil {
			log.Fatal(err)
		}
		src, err := format.Source(buf.Bytes())
		if err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(strings.TrimSuffix(file, ".tmpl"), src, 0644); err != nil {
			log.Fatal(err)
		}
	}
}

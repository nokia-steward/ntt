package main

import (
	"bufio"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/fatih/color"
	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/ttcn3"
	printer "github.com/nokia/ntt/ttcn3/format"
	"github.com/nokia/ntt/ttcn3/syntax"
	syntax2 "github.com/nokia/ntt/ttcn3/v2/syntax"
	"github.com/spf13/cobra"
)

var (
	DumpCommand = &cobra.Command{
		Use:   "dump",
		Short: "Dump TTCN-3 syntax trees",
		RunE: func(cmd *cobra.Command, args []string) error {
			srcs, _ := fs.TTCN3Files(Project.Sources...)
			imports, _ := fs.TTCN3Files(Project.Imports...)
			files := append(srcs, imports...)
			for _, file := range files {
				tree := ttcn3.ParseFile(file)
				switch format := Format(); format {
				case "ttcn3":
					b, err := fs.Content(file)
					if err != nil {
						fatal(err)
					}
					if err := printer.Fprint(os.Stdout, b); err != nil {
						fatal(err)
					}
				case "dot":
					dot(tree.Root)
				case "text":
					dumpAST(0, "Root", reflect.ValueOf(tree.Root.NodeList.Nodes))
					w.Flush()
				case "plain":
					if !onlyTokens {
						fatal(fmt.Errorf("parsing syntax not implemented. Please use option --only-tokens"))
					}
					b, err := fs.Content(file)
					if err != nil {
						fatal(err)
					}
					w := bufio.NewWriter(os.Stdout)
					syntax2.Tokenize(b).Inspect(func(n syntax2.Node) bool {
						if !n.IsValid() || !n.IsToken() {
							return true
						}

						if !withTrivia && n.Kind().IsTrivia() {
							return true
						}

						if withPosition {
							fmt.Fprintf(w, "%d	%d	", n.Pos(), n.End())
						}

						if n.Kind().IsKeyword() {
							fmt.Fprintf(w, "KEYWORD")
						} else {
							fmt.Fprint(w, strings.ToUpper(n.Kind().String()))
						}

						if withValue {
							fmt.Fprintf(w, "	%s", strings.ReplaceAll(n.Text(), "\n", "\\n"))
						}

						fmt.Fprintln(w)

						return true
					})
					w.Flush()
				default:
					fatal(fmt.Errorf("format not supported: %s", Format()))
				}
			}

			return nil
		},
	}

	outputTTCN3 = false
	outputDot   = false

	onlyTokens   = false
	withTrivia   = false
	withPosition = false
	withValue    = false

	bold  = color.New(color.Bold)
	faint = color.New(color.Faint)
	token = color.New(color.FgMagenta)
)

func init() {
	DumpCommand.PersistentFlags().BoolVarP(&outputTTCN3, "ttcn3", "", false, "formatted TTCN-3 output")
	DumpCommand.PersistentFlags().BoolVarP(&outputDot, "dot", "", false, "graphviz output")

	DumpCommand.PersistentFlags().BoolVarP(&onlyTokens, "only-tokens", "", false, "only dump tokens")
	DumpCommand.PersistentFlags().BoolVarP(&withTrivia, "with-trivia", "", false, "dump tokens with trivia")
	DumpCommand.PersistentFlags().BoolVarP(&withPosition, "with-position", "", false, "dump tokens with position")
	DumpCommand.PersistentFlags().BoolVarP(&withValue, "with-value", "", false, "dump token with value")
}

func dumpAST(indent int, name string, v reflect.Value) {
	if !v.IsValid() || v.IsZero() {
		return
	}

	span := ""
	if v.CanInterface() {
		if n, ok := v.Interface().(syntax.Node); ok {
			span = fmt.Sprintf("[%d:%d)", n.Pos(), n.End())
		}
	}
	fmt.Fprintf(w, "%-15s", span)

	for i := 0; i < indent; i++ {
		faint.Fprint(w, "·   ")
	}

	bold.Fprintf(w, "%s:", name)

	if v.CanInterface() {
		switch n := v.Interface().(type) {
		case syntax.Token:
			token.Fprintf(w, " %s\n", n.String())
			return // do not recurse any further
		case syntax.Node:
			fmt.Fprintf(w, " %s", strings.TrimPrefix(reflect.TypeOf(n).String(), "*syntax."))
		}
	}
	fmt.Fprintln(w)

	if v.Kind() == reflect.Interface {
		v = v.Elem()
	}

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			tf := v.Type().Field(i)
			dumpAST(indent+1, tf.Name, f)
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			dumpAST(indent+1, fmt.Sprintf("[%d]", i), v.Index(i))
		}
	}
}

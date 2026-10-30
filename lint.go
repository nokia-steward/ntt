package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/log"
	"github.com/nokia/ntt/internal/yaml"
	"github.com/nokia/ntt/project"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/doc"
	"github.com/nokia/ntt/ttcn3/syntax"
	"github.com/spf13/cobra"
)

var (
	LintCommand = &cobra.Command{
		Use:   "lint",
		Short: "Check a test suite for suspicious or invalid code",
		Long: `lint examines TTCN-3 source files and reports suspicious code.
It may find problems not caught by the compiler, but also constructs
considered "bad style".

Lint's exit code is non-zero for erroneous invocation of the tool or if a
problem was reported.


Formatting Checks

    max_lines          Number of lines a behaviour body must not exceed.
    aligned_braces     Braces must be in the same column or same line.
    require_case_else  Every select-statement must have one case-else.


Cyclomatic Complexity Checks

    complexity.max           Cyclomatic complexity muss not exceed.
    complexity.ignore_guards Ignore complexity of alt- and interleave guards


Naming Convention Checks

    naming.modules            Checks for module identifiers.
    naming.tests              Checks for test-case identifier.
    naming.functions          Checks for function identifiers.
    naming.altsteps           Checks for altstep identifiers.
    naming.parameters         Checks for parameter identifiers.
    naming.component_vars     Checks for component variable identifiers.
    naming.component_var_templates
                              Checks for var templates that are component variables.
    naming.var_templates      Checks for variable template identifiers.
    naming.port_types         Checks for port type identifiers.
    naming.ports              Checks for port instance identifiers.
    naming.global_consts      Checks for global constant identifiers.
    naming.component_consts   Checks for component scoped constant identifiers.
    naming.templates          Checks for constant template identifiers.
    naming.locals             Checks for local variable identifiers.
    naming.record             Checks for record identifiers.
    naming.record_fields      Checks for identifiers of record fields.
    naming.record_of          Checks for record-of identifiers.
    naming.set                Checks for set identifiers.
    naming.set_fields         Checks for identifiers of set fields.
    naming.set_of             Checks for set-of identifiers.
    naming.union              Checks for union identifiers.
    naming.union_fields       Checks for identifiers of union fields.
    naming.enum               Checks for enum identifiers
    naming.enum_label         Checks for labels of enumerated types.

    tags.modules              Checks for module tags.
    tags.tests                Checks for test-case tags.


White-Listing

    ignore.modules    Ignore modules
    ignore.files      Ignore files


Refactoring

When TTCN-3 code is refactored incrementally, it happens that references to
legacy code are faster added than one can remove them. This check helps with a
warning, as soon as the usage of a symbol exceed a defined limit.


Unused Symbols

    unused.modules    Checks for unused modules


Example configuration file:

	aligned_braces: true
	require_case_else: true
	max_lines: 40

	usage:
	  "foo":
	    limit: 12
	    text: Use "bar" instead.

	unused:
	  modules: true

	complexity:
	  max: 15
	  ignore_guards: true

	tags:
	  tests:
	    "@author": "testcases must have a @author tag"

	naming:
	  tests:
	    # An exlamation mark inverts the match.
	    "!.{130,}": "testcase identifiers must not be longer than 130 characters"

	  functions:
	    "^[a-z]"      : "function identifiers must begin with a lower case letter"
	    "!^(f|func)_" : "function identifiers must not begin with f_ or func_"

	  global_consts:
	    "^[A-Z0-9_]+$": "global constants must be UPPER_CASE"

	ignore:
	  modules:
	    # Ignore generated modules
	    - "^Protobuf_.+$"

	  files:
	    # Ignore all files from generated folders
	    - "generated/"

For information on writing new checks, see <TBD>.
`,

		RunE: lint,
	}

	config  string
	regexes = make(map[string]*regexp.Regexp)
	issues  = 0

	usedModules  = make(map[string]Import)
	usedModuleMu sync.Mutex

	style = struct {
		MaxLines        int  `yaml:"max_lines"`
		AlignedBraces   bool `yaml:"aligned_braces"`
		RequireCaseElse bool `yaml:"require_case_else"`
		Complexity      struct {
			Max          int
			IgnoreGuards bool `yaml:"ignore_guards"`
		}
		Naming struct {
			Modules               map[string]string
			Tests                 map[string]string
			Functions             map[string]string
			Altsteps              map[string]string
			Parameters            map[string]string
			ComponentVars         map[string]string `yaml:"component_vars"`
			ComponentVarTemplates map[string]string `yaml:"component_var_templates"`
			VarTemplates          map[string]string `yaml:"var_templates"`
			PortTypes             map[string]string `yaml:"port_types"`
			Ports                 map[string]string
			GlobalConsts          map[string]string `yaml:"global_consts"`
			ComponentConsts       map[string]string `yaml:"component_consts"`
			Templates             map[string]string
			Locals                map[string]string
			Record                map[string]string
			RecordFields          map[string]string `yaml:"record_fields"`
			RecordOf              map[string]string `yaml:"record_of"`
			Set                   map[string]string
			SetFields             map[string]string `yaml:"set_fields"`
			SetOf                 map[string]string `yaml:"set_of"`
			Union                 map[string]string
			UnionFields           map[string]string `yaml:"union_fields"`
			Enum                  map[string]string
			EnumLabels            map[string]string `yaml:"enum_labels"`
		}
		Tags struct {
			Modules map[string]string
			Tests   map[string]string
		}
		Ignore struct {
			Modules []string
			Files   []string
		}
		Usage map[string]*struct {
			Text  string
			Limit int
			count int
		}
		Unused struct {
			Modules bool
		}
	}{}
)

type Import struct {
	Node     *syntax.ImportDecl
	Tree     *ttcn3.Tree
	From     string
	Imported string
}

func init() {
	LintCommand.PersistentFlags().StringVarP(&config, "config", "c", ".ntt-lint.yml", "path to YAML formatted file containing linter configuration")
}

func lint(cmd *cobra.Command, args []string) error {
	c := fs.Open(config)
	b, err := c.Bytes()
	if err != nil {
		log.Verbose(err.Error())
		return nil
	}

	if err := yaml.Unmarshal(b, &style); err != nil {
		return err
	}

	if err := buildRegexCache(); err != nil {
		return err
	}

	files, err := project.Files(Project)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(len(files))

	for i := range files {
		go func(i int) {
			defer wg.Done()

			if isWhiteListed(style.Ignore.Files, files[i]) {
				return
			}

			tree := ttcn3.ParseFile(files[i])
			if err := tree.Err; err != nil {
				reportError(err)
			}

			for _, def := range tree.Modules() {
				mod := def.Node.(*syntax.Module)
				if isWhiteListed(style.Ignore.Modules, syntax.Name(mod.Name)) {
					continue
				}

				stack := make([]syntax.Node, 1, 64)
				cc := make(map[syntax.Node]int)
				ccID := syntax.Node(mod)
				caseElse := make(map[syntax.Node]int)

				checkNaming(mod, style.Naming.Modules)
				checkBraces(mod.LBrace, mod.RBrace)
				checkTags(mod, style.Tags.Modules)
				mod.Inspect(func(n syntax.Node) bool {
					if n == nil {
						stack = stack[:len(stack)-1]
						return false
					}

					stack = append(stack, n)

					switch n := n.(type) {

					case *syntax.Ident:
						checkUsage(n)

					case *syntax.FuncDecl:
						ccID = n
						cc[ccID] = 1 // Initial McCabe value

						switch n.KindTok.Kind() {
						case syntax.TESTCASE:
							checkNaming(n, style.Naming.Tests)
							checkTags(n, style.Tags.Tests)
						case syntax.FUNCTION:
							checkNaming(n, style.Naming.Functions)
						case syntax.ALTSTEP:
							checkNaming(n, style.Naming.Altsteps)
						}

						checkLines(n)

					case *syntax.FormalPar:
						checkNaming(n, style.Naming.Parameters)

						// We do not descent any further,
						// because we do not want to count
						// cyclomatic complexity for default
						// values.
						return false

					case *syntax.PortTypeDecl:
						checkNaming(n, style.Naming.PortTypes)
						checkBraces(n.LBrace, n.RBrace)
					case *syntax.Declarator:
						if len(stack) <= 2 {
							return true
						}

						// The parent of a declarator should
						// always be a ValueDecl.  If not, we
						// have some internal issues, it's okay
						// to panic then.
						parent := stack[len(stack)-2].(*syntax.ValueDecl)
						scope := stack[:len(stack)-2]

						switch {
						case isPort(parent):
							checkNaming(n, style.Naming.Ports)
						case isConst(parent):
							switch {
							case inGlobalScope(scope):
								checkNaming(n, style.Naming.GlobalConsts)
							case inComponentScope(scope):
								checkNaming(n, style.Naming.ComponentConsts)
							}
						case isVarTemplate(parent):
							switch {
							case inComponentScope(scope):
								checkNaming(n, style.Naming.ComponentVarTemplates)
							default:
								checkNaming(n, style.Naming.VarTemplates)
							}
						case isVar(parent):
							switch {
							case inComponentScope(scope):
								checkNaming(n, style.Naming.ComponentVars)
							default:
								checkNaming(n, style.Naming.Locals)
							}
						}

						return true

					case *syntax.TemplateDecl:
						checkNaming(n, style.Naming.Templates)

					case *syntax.BlockStmt:
						checkBraces(n.LBrace, n.RBrace)
					case *syntax.CompositeLiteral:
						checkBraces(n.LBrace, n.RBrace)
					case *syntax.ExceptExpr:
						checkBraces(n.LBrace, n.RBrace)
					case *syntax.SelectStmt:
						caseElse[n] = 0
						checkBraces(n.LBrace, n.RBrace)
					case *syntax.StructSpec:
						checkBraces(n.LBrace, n.RBrace)
					case *syntax.EnumSpec:
						checkBraces(n.LBrace, n.RBrace)
						for _, x := range n.Enums {
							checkNaming(x, style.Naming.EnumLabels)
						}
					case *syntax.ModuleParameterGroup:
						checkBraces(n.LBrace, n.RBrace)
					case *syntax.StructTypeDecl:
						checkBraces(n.LBrace, n.RBrace)
						var nameRules, fieldRules map[string]string
						switch n.KindTok.Kind() {
						case syntax.RECORD:
							nameRules = style.Naming.Record
							fieldRules = style.Naming.RecordFields
						case syntax.SET:
							nameRules = style.Naming.Set
							fieldRules = style.Naming.SetFields
						case syntax.UNION:
							nameRules = style.Naming.Union
							fieldRules = style.Naming.UnionFields
						default:
							log.Verbosef("unknown struct type %q. Ignoring\n", n.KindTok.Kind())
							return true

						}
						checkNaming(n, nameRules)
						for _, f := range n.Fields {
							checkNaming(f, fieldRules)
						}
					case *syntax.EnumTypeDecl:
						checkBraces(n.LBrace, n.RBrace)
						checkNaming(n, style.Naming.Enum)
						for _, x := range n.Enums {
							checkNaming(x, style.Naming.EnumLabels)
						}
					case *syntax.SubTypeDecl:
						if n.Field == nil {
							break
						}

						if lt, ok := n.Field.Type.(*syntax.ListSpec); ok {
							switch lt.KindTok.Kind() {
							case syntax.RECORD:
								checkNaming(n, style.Naming.RecordOf)
							case syntax.SET:
								checkNaming(n, style.Naming.SetOf)
							}
						}

					case *syntax.ImportDecl:
						checkBraces(n.LBrace, n.RBrace)
						checkImport(n, mod, tree)
					case *syntax.GroupDecl:
						checkBraces(n.LBrace, n.RBrace)
					case *syntax.WithSpec:
						checkBraces(n.LBrace, n.RBrace)
					case *syntax.ParenExpr:
						if n.LParen.Kind() == syntax.LBRACE {
							checkBraces(n.LParen, n.RParen)
						}

					case *syntax.ModuleDef:
						// Reset ID for counting cyclomatic complexity.
						ccID = mod

					case *syntax.BinaryExpr:
						if n.Op.Kind() == syntax.AND || n.Op.Kind() == syntax.OR {
							cc[ccID]++
						}

					case *syntax.IfStmt:
						cc[ccID]++

					case *syntax.CaseClause:
						if p, ok := tree.ParentOf(n).(*syntax.SelectStmt); ok && isCaseElse(n) {
							caseElse[p]++
						} else {
							// Do not count case else for complexity
							cc[ccID]++
						}

					case *syntax.CommClause:
						if style.Complexity.IgnoreGuards {
							return true
						}

						// Do not count else-guards
						if n.Else != nil {
							return true
						}
						// Every AltGuard increases cyclomatic complexity.
						cc[ccID]++

						// Every AltGuard expressions also increases complexity.
						if n.X != nil {
							cc[ccID]++
						}

					}
					return true
				})

				checkComplexity(cc)
				checkCaseElse(caseElse)
			}
		}(i)
	}

	wg.Wait()

	checkConf(Project)

	switch issues {
	case 0:
		return nil
	case 1:
		return fmt.Errorf("1 issue found")
	default:
		return fmt.Errorf("%d issues found", issues)
	}

}

func checkNaming(n syntax.Node, patterns map[string]string) {
	checkPatterns(n, patterns, syntax.Name(n))
}

func checkTags(n syntax.Node, patterns map[string]string) {
	if len(patterns) == 0 {
		return
	}

	var tags []string
	for _, t := range doc.FindAllTags(syntax.Doc(n)) {
		tags = append(tags, strings.Join(t, ":"))
	}

	checkPatterns(n, patterns, tags...)
}

func checkPatterns(n syntax.Node, patterns map[string]string, ss ...string) {
next:
	for p, msg := range patterns {
		expect := true
		if strings.HasPrefix(p, "!") {
			expect = false
			p = p[1:]
		}

		// Match any.
		for _, s := range ss {
			if regexes[p].MatchString(s) == expect {
				continue next
			}
		}

		// If we could not match any, we report an error
		reportError(&errPattern{node: n, msg: msg})
	}
}

func checkLines(n syntax.Node) {
	if style.MaxLines == 0 {
		return
	}

	begin := syntax.Begin(n)
	end := syntax.End(n)
	lines := end.Line - begin.Line
	if lines > style.MaxLines {
		reportError(&errLines{node: n, lines: lines})
	}

}

func checkBraces(left syntax.Node, right syntax.Node) {
	if !style.AlignedBraces {
		return
	}

	if left == nil || right == nil {
		return
	}

	p1 := syntax.Begin(left)
	p2 := syntax.Begin(right)
	if p1.Line != p2.Line && p1.Column != p2.Column {
		reportError(&errBraces{left: left, right: right})
	}
}

func checkComplexity(cc map[syntax.Node]int) {
	if style.Complexity.Max == 0 {
		return
	}

	for n, v := range cc {
		if v > style.Complexity.Max {
			reportError(&errComplexity{node: n, complexity: v})
		}

	}
}

func checkCaseElse(caseElse map[syntax.Node]int) {
	if !style.RequireCaseElse {
		return
	}
	for n, v := range caseElse {
		if v == 0 {
			reportError(&errMissingCaseElse{node: n})
		}
	}
}

func checkUsage(n *syntax.Ident) {

	if style.Usage == nil {
		return
	}
	id := n.String()
	u, ok := style.Usage[id]
	if !ok {
		return
	}
	u.count++
	if u.count >= u.Limit {
		reportError(&errUsageExceedsLimit{
			node:  n,
			usage: u.count,
			limit: u.Limit,
			text:  u.Text})
	}
}

func checkImport(n *syntax.ImportDecl, mod *syntax.Module, tree *ttcn3.Tree) {
	if !style.Unused.Modules {
		return
	}

	imported := syntax.Name(n.Module)
	importing := syntax.Name(mod.Name)

	usedModuleMu.Lock()
	usedModules[imported] = Import{
		Node:     n,
		Tree:     tree,
		From:     importing,
		Imported: imported,
	}
	usedModuleMu.Unlock()

}

func checkConf(conf *project.Config) {

	if !style.Unused.Modules {
		return
	}

	for _, pkg := range conf.Imports {
		files, _ := filepath.Glob(pkg + "/*.ttcn3")
		for _, file := range files {
			if isWhiteListed(style.Ignore.Files, file) {
				continue
			}

			for _, def := range ttcn3.ParseFile(file).Modules() {
				mod := syntax.Name(def.Node.(*syntax.Module))

				if isWhiteListed(style.Ignore.Modules, mod) {
					return
				}

				if _, found := usedModules[mod]; !found {
					reportError(&errUnusedModule{file: file, name: mod})
				}
			}
		}
	}
}

func matchAny(patterns []string, s string) bool {
	for _, p := range patterns {

		expect := true
		if strings.HasPrefix(p, "!") {
			expect = false
			p = p[1:]
		}

		if regexes[p].MatchString(s) == expect {
			return true
		}
	}
	return false
}

func reportError(e error) {

	// Check if this error is silenced (with a NOLINT-directive for example).
	type silencer interface {
		IsSilent() bool
	}
	if e, ok := e.(silencer); ok && e.IsSilent() {
		return
	}

	issues++
	fmt.Println(e.Error())
}

func isWhiteListed(list []string, s string) bool {
	if len(list) == 0 {
		return false
	}
	return matchAny(list, s)
}

func inComponentScope(stack []syntax.Node) bool {
	for _, n := range stack {
		if _, ok := n.(*syntax.ComponentTypeDecl); ok {
			return true
		}
	}
	return false
}

func inGlobalScope(stack []syntax.Node) bool {
	for _, n := range stack {
		switch n.(type) {
		case *syntax.Module, *syntax.ModuleDef, *syntax.GroupDecl, *syntax.ModuleParameterGroup:
		default:
			return false
		}
	}

	return true
}

func isPort(d *syntax.ValueDecl) bool {
	return d.KindTok != nil && d.KindTok.Kind() == syntax.PORT
}

func isConst(d *syntax.ValueDecl) bool {
	return d.KindTok != nil && d.KindTok.Kind() == syntax.CONST
}

func isVar(d *syntax.ValueDecl) bool {
	return !isVarTemplate(d) && d.KindTok != nil && d.KindTok.Kind() == syntax.VAR
}

func isVarTemplate(d *syntax.ValueDecl) bool {
	return d.TemplateRestriction != nil
}

func isCaseElse(n *syntax.CaseClause) bool {
	return n.Case == nil
}

func buildRegexCache() error {

	for p := range style.Naming.Modules {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Naming.Tests {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Naming.Functions {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Naming.Altsteps {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Naming.Parameters {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Naming.ComponentVars {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Naming.ComponentVarTemplates {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Naming.PortTypes {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Naming.Ports {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Naming.GlobalConsts {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Naming.ComponentConsts {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Naming.Templates {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Naming.VarTemplates {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Naming.Locals {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Naming.Record {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Naming.RecordFields {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Naming.RecordOf {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Naming.Set {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Naming.SetFields {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Naming.SetOf {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Naming.Union {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Naming.UnionFields {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Naming.Enum {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Naming.EnumLabels {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Tags.Tests {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for p := range style.Tags.Modules {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for _, p := range style.Ignore.Modules {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	for _, p := range style.Ignore.Files {
		if err := cacheRegex(p); err != nil {
			return err
		}
	}
	return nil
}

func cacheRegex(p string) error {
	p = strings.TrimPrefix(p, "!")

	if _, ok := regexes[p]; !ok {
		r, err := regexp.Compile(p)
		if err != nil {
			return err
		}
		regexes[p] = r
	}
	return nil
}

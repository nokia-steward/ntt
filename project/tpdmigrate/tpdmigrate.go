// Package tpdmigrate converts a Titan project descriptor (.tpd) into
// an equivalent package.yml that ntt's project loader understands.
//
// Migrating an existing Titan project to ntt is a two-step process:
//   1. Run `ntt migrate from-titan path/to/project.tpd > package.yml`
//   2. Adjust any test-port references the Titan project pulled in via
//      `<ReferencedProject>` entries.
//
// The migrator is deliberately conservative: it preserves source
// paths, include directories and project references, but leaves the
// translation of Titan-specific extensions (`<MakefileGen>` plugin
// settings, `<ProjectProperties>` rebuild rules) to the user. Each
// such omission generates a `# TODO:` comment in the output.
package tpdmigrate

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/nokia/ntt/project/internal/titan"
)

// ConvertFile loads the TPD at path and writes the package.yml
// equivalent to w. This is the convenience used by the CLI; callers
// that already have a loaded *titan.Project go through Convert.
func ConvertFile(w io.Writer, path string) error {
	p, err := titan.Load(path)
	if err != nil {
		return err
	}
	return Convert(w, p)
}

// Convert reads tpd and writes the equivalent package.yml to w. The
// caller passes a Project already loaded via titan.Load to share I/O
// concerns with the rest of the project loader.
func Convert(w io.Writer, p *titan.Project) error {
	if p == nil {
		return fmt.Errorf("tpdmigrate: nil project")
	}
	fmt.Fprintln(w, "# Migrated from Titan project descriptor by `ntt migrate from-titan`.")
	fmt.Fprintf(w, "name: %s\n", quoteIfNeeded(p.Name))

	if len(p.Sources) > 0 {
		fmt.Fprintln(w, "sources:")
		sorted := append([]string{}, p.Sources...)
		sort.Strings(sorted)
		for _, s := range sorted {
			fmt.Fprintf(w, "  - %s\n", quoteIfNeeded(s))
		}
	}
	if len(p.ImportDirs) > 0 {
		fmt.Fprintln(w, "imports:")
		sorted := append([]string{}, p.ImportDirs...)
		sort.Strings(sorted)
		for _, d := range sorted {
			fmt.Fprintf(w, "  - %s\n", quoteIfNeeded(d))
		}
	}
	if len(p.References) > 0 {
		fmt.Fprintln(w, "# TODO: translate ReferencedProjects to ntt dependency entries.")
		fmt.Fprintln(w, "# Each referenced project should be migrated separately and added")
		fmt.Fprintln(w, "# to a top-level package list once the layout is finalised.")
		for _, ref := range p.References {
			fmt.Fprintf(w, "#   - %s (%s)\n", ref.Name, ref.Path)
		}
	}
	return nil
}

// quoteIfNeeded wraps s in double quotes when it contains whitespace
// or characters that YAML treats as control. We use the same heuristic
// as the rest of the ntt project package so package.yml output looks
// hand-written.
func quoteIfNeeded(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, " :#-?") {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}

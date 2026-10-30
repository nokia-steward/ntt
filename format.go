package main

import (
	"bytes"
	"fmt"
	"os"
	"runtime"

	"github.com/hashicorp/go-multierror"
	"github.com/nokia/ntt/internal/fs"
	printer "github.com/nokia/ntt/ttcn3/format"
	"github.com/pmezard/go-difflib/difflib"
	"github.com/spf13/cobra"
)

var (
	FormatCommand = &cobra.Command{
		Hidden: true,
		Use:    "format",
		Short:  "format files according to the canonical TTCN-3 style",
		Long: `Format files according to the canonical TTCN-3 style.

The format command reads the given source files and formats them according to
the standard TTCN-3 style. Files from imports or generated files are not
formatted.

If a directory is given as argument, the command scans the directory for
manifest files (package.yml) and potential TTCN-3 source files to format.

Without any arguments, the command reads scans the current directory for
manifest files (package.yml) and potential TTCN-3 source files.


CANONICAL STYLE

Formatting style is a matter of taste and a subject of debate. A canonical
style settles such debates and makes code easier to read due to a consistent
style across projects and teams.

For this reason, the format command does not support custom confirguration like
maximum line length or indentation style.


EXIT STATUS

Exit status is zero if no errors were encountered, and non-zero otherwise.

The --diff or --list flags will cause the command to exit with non-zero status,
if any files need to be formatted.


WARNING

The canonical style is still developing and will change in the future as is
this tool. Do not use it in production yet.

`,

		RunE: func(cmd *cobra.Command, args []string) error {
			srcs, err := fs.TTCN3Files(Project.Sources...)
			if err != nil {
				return err
			}

			var merr *multierror.Error
			for _, src := range srcs {
				if err := processFile(src); err != nil {
					merr = multierror.Append(merr, err)
				}
			}

			if formattedFiles > 0 && (listFiles || diff) {
				return fmt.Errorf("%d files with format differences", formattedFiles)
			}

			return merr.ErrorOrNil()
		},
	}

	listFiles, diff, inplace bool
	formattedFiles           int
	spaces                   int
)

func init() {
	FormatCommand.Flags().BoolVarP(&inplace, "in-place", "i", false, "format files in place")
	FormatCommand.Flags().BoolVarP(&diff, "diff", "d", false, "display diff instead of rewriting files. Exit with non-zero status if any files need to be formatted")
	FormatCommand.Flags().BoolVarP(&listFiles, "list", "l", false, "list files whose formatting differs. Exit with non-zero status if any files need to be formatted")
	FormatCommand.Flags().IntVarP(&spaces, "tabs-to-spaces", "s", 0, "convert each tab to N spaces")
}

func processFile(path string) error {
	src, err := fs.Content(path)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	p := printer.NewCanonicalPrinter(&buf)
	p.Indent = -1
	if spaces > 0 {
		p.UseSpaces = true
		p.TabWidth = spaces
	}
	if err := p.Fprint(src); err != nil {
		return err
	}
	res := buf.Bytes()

	if !bytes.Equal(src, res) {

		formattedFiles++

		if listFiles {
			fmt.Fprintln(os.Stdout, path)
		}

		if diff {
			d, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
				A:        difflib.SplitLines(string(src)),
				B:        difflib.SplitLines(string(res)),
				FromFile: fmt.Sprintf("%s.orig", path),
				ToFile:   path,
				Context:  1,
			})
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stdout, d)
		}

		if inplace {
			info, err := os.Stat(path)
			if err != nil {
				return err
			}
			perm := info.Mode().Perm()
			backup, err := backupFile(src, perm)
			if err != nil {
				return err
			}
			if err := os.WriteFile(path, res, perm); err != nil {
				if rerr := os.Rename(backup, path); rerr != nil {
					return fmt.Errorf("%w; restoring %s from %s: %s", err, path, backup, rerr.Error())
				}
				return err
			}
			if err := os.Remove(backup); err != nil {
				return err
			}
		}
	}

	if !inplace && !diff && !listFiles {
		fmt.Fprint(os.Stdout, string(res))
	}

	return nil
}

func backupFile(b []byte, perm os.FileMode) (string, error) {
	f, err := os.CreateTemp("", "ntt-format-")
	if err != nil {
		return "", err
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(f.Name(), perm); err != nil {
			f.Close()
			os.Remove(f.Name())
			return "", err
		}
	}

	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}

	return f.Name(), nil
}

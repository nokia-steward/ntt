package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/nokia/ntt/build/cfgvalidate"
	"github.com/nokia/ntt/build/makefilegen"
	"github.com/nokia/ntt/project"
	"github.com/nokia/ntt/project/tpdmigrate"
	"github.com/nokia/ntt/runtime/cfg"
	"github.com/spf13/cobra"
)

var (
	// MigrateCommand is the umbrella for `ntt migrate from-titan` and
	// future `ntt migrate from-*` subcommands.
	MigrateCommand = &cobra.Command{
		Use:   "migrate",
		Short: "Migrate projects from other TTCN-3 toolchains",
	}

	MigrateFromTitanCommand = &cobra.Command{
		Use:   "from-titan [path/to/project.tpd]",
		Short: "Convert a Titan project descriptor to package.yml",
		Args:  cobra.ExactArgs(1),
		RunE:  runMigrateFromTitan,
	}

	MakefilegenCommand = &cobra.Command{
		Use:   "makefilegen",
		Short: "Generate a Titan-compatible Makefile for the current project",
		RunE:  runMakefilegen,
	}

	CfgValidateCommand = &cobra.Command{
		Use:   "cfgvalidate [file...]",
		Short: "Validate one or more TTCN-3 .cfg files",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runCfgValidate,
	}

	migrateOutDir string

	makefileLegacy bool
	makefileOut    string
)

func init() {
	RootCommand.AddCommand(MigrateCommand)
	MigrateCommand.AddCommand(MigrateFromTitanCommand)
	RootCommand.AddCommand(MakefilegenCommand)
	RootCommand.AddCommand(CfgValidateCommand)

	MigrateFromTitanCommand.Flags().StringVar(&migrateOutDir, "out", "",
		"directory to write package.yml into (default stdout)")
	MakefilegenCommand.Flags().BoolVar(&makefileLegacy, "legacy", false,
		"emit Titan-compatible targets (compile, run, ...)")
	MakefilegenCommand.Flags().StringVar(&makefileOut, "out", "",
		"file to write the Makefile to (default stdout)")
}

func runMigrateFromTitan(cmd *cobra.Command, args []string) error {
	out := io.Writer(os.Stdout)
	if migrateOutDir != "" {
		if err := os.MkdirAll(migrateOutDir, 0o755); err != nil {
			return err
		}
		f, err := os.Create(filepath.Join(migrateOutDir, "package.yml"))
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}
	if err := tpdmigrate.ConvertFile(out, args[0]); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

func runMakefilegen(cmd *cobra.Command, args []string) error {
	if Project == nil {
		return fmt.Errorf("makefilegen: no project loaded; run inside a directory with package.yml or use --chdir")
	}
	out := io.Writer(os.Stdout)
	if makefileOut != "" {
		f, err := os.Create(makefileOut)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}
	return makefilegen.Generate(out, Project, makefilegen.Options{LegacyTargets: makefileLegacy})
}

func runCfgValidate(cmd *cobra.Command, args []string) error {
	exit := 0
	for _, path := range args {
		f, diags, err := cfg.Load(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			exit = 1
			continue
		}
		for _, d := range diags {
			fmt.Printf("%s:%d: parse: %s\n", path, d.Line, d.Message)
			exit = 1
		}
		issues := cfgvalidate.Validate(f)
		for _, i := range issues {
			fmt.Printf("%s:%s\n", path, i)
			exit = 1
		}
	}
	if exit != 0 {
		os.Exit(exit)
	}
	_ = project.Config{}
	return nil
}

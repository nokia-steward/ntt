package main

import (
	"fmt"
	"strings"

	"runtime/debug"

	"github.com/spf13/cobra"
)

var (
	VersionCommand = &cobra.Command{
		Use:   "version",
		Short: "Show version.",
		Run:   versionInfo,
	}
)

func init() {
	RootCommand.AddCommand(VersionCommand)
	// Convenience: `ntt --version` is what most CLI users try first.
	// Cobra reserves the flag for us when SetVersionTemplate is set.
	RootCommand.Version = versionString()
	RootCommand.SetVersionTemplate("{{.Version}}\n")

	if version == "devel" {
		info, ok := debug.ReadBuildInfo()
		if ok && strings.HasPrefix(info.Main.Version, "v") {
			version = info.Main.Version
		}
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.time":
				date = s.Value
			case "vcs.revision":
				commit = s.Value
			case "vcs.modified":
				if s.Value == "true" {
					commit += "-dirty"
				}
			}
		}
	}

}

func versionInfo(cmd *cobra.Command, args []string) {
	fmt.Println(versionString())
}

func versionString() string {
	return fmt.Sprintf("ntt %v, commit %s, built at %s", version, commit, date)
}

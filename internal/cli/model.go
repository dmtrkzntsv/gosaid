package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/models"
	"github.com/dmtrkzntsv/gosaid/internal/platform"
)

const modelUsage = "usage: gosaid model download <hf-repo> <file> [--name <name>] [--endpoint <id>] [--force]"

// RunModel handles `gosaid model ...` subcommands.
func RunModel(args []string) int {
	if len(args) == 0 || args[0] != "download" {
		fmt.Fprintln(os.Stderr, modelUsage)
		return 2
	}
	fs := flag.NewFlagSet("model download", flag.ContinueOnError)
	name := fs.String("name", "", "model name to register (default: derived from file name)")
	endpoint := fs.String("endpoint", "local", "whisper_cpp endpoint id to register under")
	force := fs.Bool("force", false, "overwrite an existing file and config entry")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	rest := fs.Args()
	// Allow flags after the positionals too (flag stops at the first non-flag).
	if len(rest) > 2 {
		if err := fs.Parse(rest[2:]); err != nil {
			return 2
		}
		rest = rest[:2]
	}
	if len(rest) != 2 || rest[0] == "" || rest[1] == "" || strings.HasPrefix(rest[0], "-") || strings.HasPrefix(rest[1], "-") {
		fmt.Fprintln(os.Stderr, modelUsage)
		return 2
	}
	if *name == "" {
		*name = models.DeriveName(rest[1])
	}

	cfgPath, err := config.Path()
	if err == nil {
		var modelsDir string
		modelsDir, err = platform.ModelsDir()
		if err == nil {
			err = models.Download(models.DownloadOpts{
				Repo: rest[0], File: rest[1], Name: *name, EndpointID: *endpoint,
				CfgPath: cfgPath, ModelsDir: modelsDir,
				BaseURL: models.HuggingFaceBase, Force: *force,
			})
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

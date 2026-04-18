package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/platform"
)

// OpenConfig ensures the config file exists (writing defaults if missing),
// opens it in the user's editor, and re-validates the result.
// It returns a non-nil error if the edit produced an invalid config, but the
// file is kept so the user can fix it.
func OpenConfig() error {
	path, err := config.Path()
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	if _, err := config.Load(path); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "opening %s\n", path)
	if err := runEditor(path); err != nil {
		return err
	}

	cfg, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("reload after edit: %w", err)
	}
	if err := config.Validate(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "warning: config is invalid: %v\n", err)
		return err
	}
	fmt.Fprintln(os.Stderr, "config ok")
	return nil
}

func runEditor(path string) error {
	editor := firstNonEmpty(os.Getenv("VISUAL"), os.Getenv("EDITOR"))
	if editor == "" {
		return platform.OpenInDefaultApp(path)
	}
	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

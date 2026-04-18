package main

import (
	"fmt"
	"os"

	"github.com/dmtrkzntsv/gosaid/internal/cli"
	"github.com/dmtrkzntsv/gosaid/internal/daemon"
	"github.com/dmtrkzntsv/gosaid/internal/inject"

	"golang.design/x/hotkey/mainthread"
)

const version = "0.0.1-dev"

func main() {
	args := os.Args[1:]

	// Non-daemon sub-commands exit without touching the hotkey runloop.
	if handled, code := cli.RunDebug(args); handled {
		os.Exit(code)
	}
	if len(args) > 0 {
		os.Exit(cli.Dispatch(version, args))
	}

	daemon.Version = version
	// Daemon mode: must stay on the main thread on macOS for global hotkeys.
	mainthread.Init(runDaemon)
}

func runDaemon() {
	injector, err := inject.NewPasteInjector()
	if err != nil {
		fmt.Fprintf(os.Stderr, "init injector: %v\n", err)
		os.Exit(1)
	}
	if err := daemon.Run(injector); err != nil {
		fmt.Fprintf(os.Stderr, "daemon: %v\n", err)
		os.Exit(1)
	}
}

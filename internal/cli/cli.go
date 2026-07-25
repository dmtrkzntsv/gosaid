package cli

import (
	"fmt"
	"os"

	"github.com/dmtrkzntsv/gosaid/internal/setup"
)

// Dispatch handles a single argv invocation and returns an exit code.
// Run() in the daemon package is wired in later steps.
func Dispatch(version string, args []string) int {
	if handled, code := RunDebug(args); handled {
		return code
	}
	if len(args) == 0 {
		fmt.Println("daemon stub")
		return 0
	}
	switch args[0] {
	case "config":
		if err := OpenConfig(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		return 0
	case "model":
		return RunModel(args[1:])
	case "mic":
		return RunMic(args[1:])
	case "vocab":
		return RunVocab(args[1:])
	case "setup":
		return setup.Run(args[1:])
	case "version", "-v", "--version":
		fmt.Println(version)
		return 0
	case "help", "-h", "--help":
		Usage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		Usage()
		return 2
	}
}

func Usage() {
	fmt.Println(`gosaid - headless push-to-talk voice dictation daemon

usage:
  gosaid           run the daemon
  gosaid setup     local setup wizard — transcription, a hotkey, optional local
                   chat stages. Edit config.json directly for cloud providers.
  gosaid config    open the config file in $EDITOR
  gosaid mic       select the default microphone interactively
  gosaid vocab <word> [--delete]
                   add or remove a custom-vocabulary word (hinted to
                   transcription and the text stages so it's spelled right);
                   run with no word to list
  gosaid model download <hf-repo> <file>
                   download a model from Hugging Face and register it
  gosaid version   print version
  gosaid help      print this message`)
}

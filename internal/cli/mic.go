package cli

import "github.com/dmtrkzntsv/gosaid/internal/setup"

// RunMic opens the interactive default-microphone picker.
func RunMic(args []string) int {
	return setup.RunMicrophone(args)
}

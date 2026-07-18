package cli

import (
	"fmt"
	"os"

	"github.com/dmtrkzntsv/gosaid/internal/audio"
)

// RunMic handles the `gosaid mic` sub-commands.
func RunMic(args []string) int {
	if len(args) == 0 || args[0] == "list" {
		return micList()
	}
	fmt.Fprintf(os.Stderr, "unknown mic command: %s\n\nusage:\n  gosaid mic list    list available microphones\n", args[0])
	return 2
}

func micList() int {
	devices, err := audio.ListCaptureDevices()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if len(devices) == 0 {
		fmt.Fprintln(os.Stderr, "no microphones found")
		return 1
	}
	for _, d := range devices {
		marker := " "
		if d.Default {
			marker = "*"
		}
		fmt.Printf("%s %s\n", marker, d.Name)
	}
	fmt.Fprintln(os.Stderr, "\n* = system default. Pin a hotkey to a device with its \"microphone\" field (name or substring, case-insensitive).")
	return 0
}

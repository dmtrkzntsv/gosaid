package setup

import (
	"errors"
	"fmt"
	"os"

	"charm.land/huh/v2"
	"golang.org/x/term"

	"github.com/dmtrkzntsv/gosaid/internal/audio"
	"github.com/dmtrkzntsv/gosaid/internal/config"
)

const micUsage = `usage:
  gosaid mic         select the default microphone interactively
  gosaid mic list    select the default microphone interactively`

var microphoneTheme = huh.ThemeFunc(func(isDark bool) *huh.Styles {
	styles := huh.ThemeCharm(isDark)
	accent := huh.ThemeBase16(isDark).Focused.Title.GetForeground()
	styles.Focused.Title = styles.Focused.Title.Foreground(accent)
	styles.Blurred.Title = styles.Blurred.Title.Foreground(accent)
	return styles
})

// RunMicrophone is the standalone interactive microphone picker. The "list"
// alias is retained for compatibility with the earlier non-interactive command.
func RunMicrophone(args []string) int {
	if len(args) == 1 {
		switch args[0] {
		case "-h", "--help", "help":
			fmt.Println(micUsage)
			return 0
		case "list":
			// Continue into the picker.
		default:
			fmt.Fprintf(os.Stderr, "unknown mic command: %s\n\n%s\n", args[0], micUsage)
			return 2
		}
	} else if len(args) > 1 {
		fmt.Fprintf(os.Stderr, "gosaid mic takes no arguments\n\n%s\n", micUsage)
		return 2
	}

	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintln(os.Stderr, "error: gosaid mic requires an interactive terminal")
		return 1
	}
	s, err := LoadSession()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	mic, err := askMicrophone(s.Cfg.Microphone)
	if errors.Is(err, errCancelStep) || errors.Is(err, huh.ErrUserAborted) {
		return 0
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if mic == s.Cfg.Microphone {
		fmt.Println("No changes.")
		return 0
	}

	s.Cfg.Microphone = mic
	// A microphone value is valid independently of the provider/hotkey config.
	// Save it even when setup is otherwise incomplete, which is common when
	// this focused command is used before the full setup wizard.
	if err := config.Save(s.Path, s.Cfg); err != nil {
		fmt.Fprintf(os.Stderr, "config not saved: %v\n", err)
		return 1
	}
	fmt.Printf("Saved %s\n", s.Path)
	offerRestart()
	return 0
}

// microphoneOptions builds the device picker: "System default" first, then
// every capture device (the current system default labeled).
func microphoneOptions(devices []audio.CaptureDevice) []huh.Option[string] {
	opts := []huh.Option[string]{huh.NewOption("System default", "")}
	for _, d := range devices {
		label := d.Name
		if d.Default {
			label += " (system default)"
		}
		opts = append(opts, huh.NewOption(label, d.Name))
	}
	return opts
}

// selectedMicrophone resolves a configured substring to the concrete device
// row that should be highlighted. An unavailable configured value is retained
// so opening and accepting the picker does not silently clear it.
func selectedMicrophone(current string, devices []audio.CaptureDevice) (string, bool) {
	if current == "" {
		return "", true
	}
	for _, d := range devices {
		if audio.MatchesDevice(d.Name, current) {
			return d.Name, true
		}
	}
	return current, false
}

func micOptions(current string) ([]huh.Option[string], string, error) {
	devices, err := audio.ListCaptureDevices()
	if err != nil {
		return nil, "", err
	}
	opts := microphoneOptions(devices)
	selected, available := selectedMicrophone(current, devices)
	if !available {
		opts = append(opts, huh.NewOption(current+" (configured, unavailable)", current))
	}
	return opts, selected, nil
}

// askMicrophone returns the chosen default input device name ("" = system
// default). current pre-selects the existing value.
func askMicrophone(current string) (string, error) {
	opts, choice, err := micOptions(current)
	if err != nil {
		return "", err
	}
	opts = append(opts, huh.NewOption("← Back", pickBack))
	if err := huh.NewForm(huh.NewGroup(
		optionSelect(
			"Microphone",
			"Used by every hotkey unless the hotkey sets its own microphone.",
			opts,
			&choice,
		),
	)).WithTheme(microphoneTheme).Run(); err != nil {
		return "", cancelable(err)
	}
	if choice == pickBack {
		return "", errCancelStep
	}
	return choice, nil
}

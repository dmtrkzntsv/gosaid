package setup

import (
	"charm.land/huh/v2"

	"github.com/dmtrkzntsv/gosaid/internal/audio"
)

// micOptions builds the device picker: "System default" first, then every
// capture device (the current system default labeled).
func micOptions() ([]huh.Option[string], error) {
	devices, err := audio.ListCaptureDevices()
	if err != nil {
		return nil, err
	}
	opts := []huh.Option[string]{huh.NewOption("System default", "")}
	for _, d := range devices {
		label := d.Name
		if d.Default {
			label += " (system default)"
		}
		opts = append(opts, huh.NewOption(label, d.Name))
	}
	return opts, nil
}

// askMicrophone returns the chosen default input device name ("" = system
// default). current pre-selects the existing value.
func askMicrophone(current string) (string, error) {
	opts, err := micOptions()
	if err != nil {
		return "", err
	}
	opts = append(opts, huh.NewOption("← Back", pickBack))
	choice := current
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Microphone").
			Description("Used by every hotkey unless the hotkey sets its own microphone.").
			Options(opts...).
			Height(listHeight(len(opts))).
			Value(&choice),
	)).Run(); err != nil {
		return "", cancelable(err)
	}
	if choice == pickBack {
		return "", errCancelStep
	}
	return choice, nil
}

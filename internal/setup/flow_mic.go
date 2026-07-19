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

// runMicFlow selects the global default microphone.
func runMicFlow(s *Session) error {
	opts, err := micOptions()
	if err != nil {
		return err
	}
	choice := s.Cfg.Microphone
	opts = append(opts, huh.NewOption("← Back", pickBack))
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Microphone").
			Description("Used by every hotkey unless the hotkey sets its own microphone.").
			Options(opts...).
			Height(listHeight(len(opts))).
			Value(&choice),
	))
	if err := form.Run(); err != nil {
		return cancelable(err)
	}
	if choice == pickBack {
		return errCancelStep
	}
	if choice != s.Cfg.Microphone {
		SetDefaultMicrophone(s.Cfg, choice)
		s.Dirty = true
	}
	return nil
}

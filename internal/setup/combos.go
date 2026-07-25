package setup

import (
	"charm.land/huh/v2"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// SuggestedCombos is the curated key-combo list for the hotkey wizard. All
// entries parse with the hotkey package on every platform ("option" and
// "alt" are aliases everywhere).
var SuggestedCombos = []string{
	"option+left",
	"option+down",
	"option+right",
}

// shortcutOptions keeps configured suggestions visible so users can see why a
// familiar choice is unavailable. askCombo's validator prevents reusing them.
func shortcutOptions(configured map[string]config.Hotkey) []huh.Option[string] {
	opts := make([]huh.Option[string], 0, len(SuggestedCombos)+1)
	for _, combo := range SuggestedCombos {
		label := combo
		if _, exists := configured[combo]; exists {
			label = combo + " ✓"
		}
		opts = append(opts, huh.NewOption(label, combo))
	}
	return append(opts, huh.NewOption("Type your own…", pickTypeOwn))
}

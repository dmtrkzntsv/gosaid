package setup

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"charm.land/huh/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// The three cancel helpers form a round trip: a form abort becomes the
// internal sentinel (cancelable), a manager loop drops it (absorbCancel), and
// a caller with nowhere to go turns it back into an abort (uncancel).
func TestCancelHelpers(t *testing.T) {
	other := errors.New("disk on fire")
	wrappedAbort := fmt.Errorf("combo prompt: %w", huh.ErrUserAborted)

	cases := []struct {
		name string
		fn   func(error) error
		in   error
		want error
	}{
		{"cancelable maps abort to sentinel", cancelable, huh.ErrUserAborted, errCancelStep},
		{"cancelable maps wrapped abort", cancelable, wrappedAbort, errCancelStep},
		{"cancelable passes other errors", cancelable, other, other},
		{"cancelable passes nil", cancelable, nil, nil},

		{"absorbCancel drops the sentinel", absorbCancel, errCancelStep, nil},
		{"absorbCancel passes aborts through", absorbCancel, huh.ErrUserAborted, huh.ErrUserAborted},
		{"absorbCancel passes other errors", absorbCancel, other, other},
		{"absorbCancel passes nil", absorbCancel, nil, nil},

		{"uncancel restores the abort", uncancel, errCancelStep, huh.ErrUserAborted},
		{"uncancel passes other errors", uncancel, other, other},
		{"uncancel passes nil", uncancel, nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.fn(c.in); !errors.Is(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

// Back options carry sentinel values that several selects write straight into
// HotkeyAnswers fields. BuildHotkey must never see one — every wizard step
// returns errCancelStep before UpsertHotkey — so guard the shape of the
// sentinels themselves: they must be impossible to confuse with real input.
func TestPickSentinelsAreNotValidInput(t *testing.T) {
	for _, s := range []string{pickBack, pickTypeOwn} {
		if s == "" || s[0] != 0 {
			t.Errorf("sentinel %q must start with NUL so terminal input can't produce it", s)
		}
		// Belt and braces: a leaked sentinel must also fail config validation
		// rather than persist, so check it isn't mistakable for a language code.
		if config.IsValidLanguage(s) {
			t.Errorf("sentinel %q must not be a valid language code", s)
		}
	}
}

// huh subtracts a field's rendered title and description from the height it
// is given, so a list sized to its option count collapses — in the worst case
// to one visible row, which is what shipped before this helper existed.
func TestListHeightLeavesRoomForChrome(t *testing.T) {
	for _, n := range []int{1, 2, 3, 5} {
		if got := listHeight(n); got <= n {
			t.Errorf("listHeight(%d) = %d, must exceed the option count to "+
				"survive huh subtracting the title and description", n, got)
		}
	}
	// Long lists (the ~36-language picker) must stay scrollable rather than
	// demanding more rows than a terminal has.
	if got := listHeight(200); got > 20 {
		t.Errorf("listHeight(200) = %d, want a capped, scrollable height", got)
	}
	// The cap must still be usable, not collapse back to a sliver.
	if got := listHeight(200); got < 8 {
		t.Errorf("listHeight(200) = %d, too small to browse", got)
	}
}

func TestOptionSelectLeavesOneBlankLineBeforeOptions(t *testing.T) {
	opts := []huh.Option[string]{huh.NewOption("First option", "first")}
	tests := []struct {
		name        string
		description string
		wantPrefix  string
	}{
		{
			name:        "after description",
			description: "Helpful text.",
			wantPrefix:  "Header\nHelpful text.\n\n",
		},
		{
			name:       "after header when there is no description",
			wantPrefix: "Header\n\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := ""
			sel := optionSelect("Header", tt.description, opts, &value)
			sel.WithWidth(80)
			lines := strings.Split(ansi.Strip(sel.View()), "\n")
			for i := range lines {
				lines[i] = strings.TrimSpace(lines[i])
			}
			view := strings.Join(lines, "\n")
			if !strings.HasPrefix(view, tt.wantPrefix) {
				t.Fatalf("view = %q, want prefix %q", view, tt.wantPrefix)
			}
		})
	}
}

// The sentinel must never reach Run's error printing: uncancel(absorbCancel(x))
// is what a topic flow and the hub both end up applying.
func TestSentinelNeverEscapesToRun(t *testing.T) {
	if got := uncancel(absorbCancel(errCancelStep)); got != nil {
		t.Errorf("a manager that absorbs its cancel must end clean, got %v", got)
	}
	if got := uncancel(errCancelStep); !errors.Is(got, huh.ErrUserAborted) {
		t.Errorf("an unabsorbed cancel must surface as an abort, got %v", got)
	}
	if errors.Is(uncancel(errCancelStep), errCancelStep) {
		t.Error("errCancelStep must not survive uncancel — Run has no rule for it")
	}
}

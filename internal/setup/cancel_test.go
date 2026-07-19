package setup

import (
	"errors"
	"fmt"
	"testing"

	"charm.land/huh/v2"

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
	for _, s := range []string{pickAdd, pickBack, pickTypeOwn} {
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

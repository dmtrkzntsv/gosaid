package setup

import (
	"fmt"
	"io"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// shortcutInput is one form field with two focus targets: the text input and
// a Cancel row rendered directly beneath it. Keeping them in one huh.Field
// lets Enter submit from the input while Down moves to Cancel.
type shortcutInput struct {
	input         *huh.Input
	description   string
	cancelFocused bool
	canceled      bool
}

func newShortcutInput(value *string, description string) *shortcutInput {
	return &shortcutInput{
		description: description,
		input: huh.NewInput().
			Title("Shortcut").
			Description(description).
			Placeholder("ctrl+alt+space").
			Value(value),
	}
}

func (s *shortcutInput) Init() tea.Cmd { return s.input.Init() }

func (s *shortcutInput) Update(msg tea.Msg) (huh.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "down":
			if !s.cancelFocused {
				s.cancelFocused = true
				return s, s.input.Blur()
			}
			return s, nil
		case "up":
			if s.cancelFocused {
				s.cancelFocused = false
				return s, s.input.Focus()
			}
		case "enter":
			if s.cancelFocused {
				s.canceled = true
				return s, func() tea.Msg { return huh.NextField() }
			}
		}
		if s.cancelFocused {
			return s, nil
		}
	}

	_, cmd := s.input.Update(msg)
	return s, cmd
}

func (s *shortcutInput) View() string {
	cancel := "  Cancel"
	if s.cancelFocused {
		cancel = "> Cancel"
	}
	return s.input.View() + "\n" + cancel
}

func (s *shortcutInput) Blur() tea.Cmd {
	s.cancelFocused = false
	return s.input.Blur()
}

func (s *shortcutInput) Focus() tea.Cmd {
	s.cancelFocused = false
	return s.input.Focus()
}

func (s *shortcutInput) Error() error                    { return s.input.Error() }
func (s *shortcutInput) Skip() bool                      { return false }
func (s *shortcutInput) Zoom() bool                      { return false }
func (s *shortcutInput) KeyBinds() []key.Binding         { return s.input.KeyBinds() }
func (s *shortcutInput) GetKey() string                  { return s.input.GetKey() }
func (s *shortcutInput) GetValue() any                   { return s.input.GetValue() }
func (s *shortcutInput) Run() error                      { return huh.Run(s) }
func (s *shortcutInput) WithTheme(t huh.Theme) huh.Field { s.input.WithTheme(t); return s }
func (s *shortcutInput) WithKeyMap(k *huh.KeyMap) huh.Field {
	s.input.WithKeyMap(k)
	return s
}
func (s *shortcutInput) WithWidth(width int) huh.Field {
	s.input.WithWidth(width)
	return s
}
func (s *shortcutInput) WithHeight(height int) huh.Field {
	if height > 1 {
		s.input.WithHeight(height - 1)
	}
	return s
}
func (s *shortcutInput) WithPosition(position huh.FieldPosition) huh.Field {
	s.input.WithPosition(position)
	return s
}

func (s *shortcutInput) RunAccessible(w io.Writer, r io.Reader) error {
	fmt.Fprintln(w, s.description)
	return s.input.RunAccessible(w, r)
}

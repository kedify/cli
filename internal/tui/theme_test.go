package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCyanHuhThemeStylesFocusedConfirmation(t *testing.T) {
	theme := cyanHuhTheme()

	if got := theme.Focused.Title.GetForeground(); got != tuiCyan {
		t.Fatalf("focused title color = %v, want %v", got, tuiCyan)
	}
	if got := theme.Focused.FocusedButton.GetBackground(); got != tuiCyan {
		t.Fatalf("focused button background = %v, want %v", got, tuiCyan)
	}
	if got := theme.Help.ShortKey.GetForeground(); got != tuiCyan {
		t.Fatalf("help key color = %v, want %v", got, tuiCyan)
	}
}

func TestCyanConfirmUsesHuhKeyBindings(t *testing.T) {
	value := false
	confirm := newCyanConfirm("Continue?", "", &value)

	_, _ = confirm.Update(tea.KeyMsg{Type: tea.KeyRight})
	if !value {
		t.Fatal("right arrow did not select Yes")
	}

	_, _ = confirm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if value {
		t.Fatal("n did not select No")
	}

	_, _ = confirm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if !value {
		t.Fatal("y did not select Yes")
	}
}

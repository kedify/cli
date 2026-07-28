package tui

import (
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

var (
	tuiCyan  = lipgloss.Color("86")
	tuiMuted = lipgloss.Color("241")
	tuiError = lipgloss.Color("196")
)

func cyanHuhTheme() *huh.Theme {
	theme := huh.ThemeBase()

	theme.Focused.Base = theme.Focused.Base.BorderForeground(tuiCyan)
	theme.Focused.Card = theme.Focused.Base
	theme.Focused.Title = theme.Focused.Title.Foreground(tuiCyan).Bold(true)
	theme.Focused.Description = theme.Focused.Description.Foreground(tuiMuted)
	theme.Focused.ErrorIndicator = theme.Focused.ErrorIndicator.Foreground(tuiError)
	theme.Focused.ErrorMessage = theme.Focused.ErrorMessage.Foreground(tuiError)
	theme.Focused.FocusedButton = theme.Focused.FocusedButton.
		Foreground(lipgloss.Color("0")).
		Background(tuiCyan).
		Bold(true)
	theme.Focused.BlurredButton = theme.Focused.BlurredButton.
		Foreground(tuiCyan).
		Background(lipgloss.NoColor{})

	theme.Blurred = theme.Focused
	theme.Blurred.Base = theme.Blurred.Base.BorderStyle(lipgloss.HiddenBorder())
	theme.Blurred.Card = theme.Blurred.Base

	theme.Group.Title = theme.Focused.Title
	theme.Group.Description = theme.Focused.Description
	theme.Help.ShortKey = theme.Help.ShortKey.Foreground(tuiCyan)
	theme.Help.ShortDesc = theme.Help.ShortDesc.Foreground(tuiMuted)
	theme.Help.FullKey = theme.Help.FullKey.Foreground(tuiCyan)
	theme.Help.FullDesc = theme.Help.FullDesc.Foreground(tuiMuted)

	return theme
}

func cyanHuhKeyMap() *huh.KeyMap {
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit.SetKeys("ctrl+c", "esc")
	keyMap.Quit.SetHelp("esc", "cancel")
	return keyMap
}

func newCyanConfirm(title, description string, value *bool) *huh.Confirm {
	confirm := huh.NewConfirm().
		Title(title).
		Description(description).
		Affirmative("Yes").
		Negative("No").
		Value(value).
		WithButtonAlignment(lipgloss.Left)
	confirm.WithTheme(cyanHuhTheme())
	confirm.WithKeyMap(cyanHuhKeyMap())
	confirm.WithPosition(huh.FieldPosition{})
	confirm.Focus()
	return confirm
}

package tui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

var (
	promptTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(tuiCyan)
	promptHintStyle     = lipgloss.NewStyle().Foreground(tuiMuted)
	promptCursorStyle   = lipgloss.NewStyle().Reverse(true)
	promptSelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(tuiCyan)
	promptErrorStyle    = lipgloss.NewStyle().Foreground(tuiError)
)

type selectionModel struct {
	title   string
	hint    string
	options []string
	cursor  int
	choice  int
	done    bool
	quit    bool
}

type textPromptModel struct {
	title    string
	hint     string
	value    []rune
	cursor   int
	validate func(string) error
	err      string
	done     bool
	quit     bool
}

func Select(stdin io.Reader, stdout, stderr io.Writer, title, hint string, options []string) (int, error) {
	file, err := interactiveFile(stdin)
	if err != nil {
		return 0, err
	}
	if len(options) == 0 {
		return 0, errors.New("no options available")
	}

	model := selectionModel{
		title:   title,
		hint:    hint,
		options: options,
		choice:  -1,
	}
	result, err := tea.NewProgram(model, tea.WithInput(file), tea.WithOutput(promptOutput(stdout, stderr))).Run()
	if err != nil {
		return 0, fmt.Errorf("run selection prompt: %w", err)
	}
	finalModel, ok := result.(selectionModel)
	if !ok {
		return 0, errors.New("unexpected selection prompt state")
	}
	if finalModel.quit {
		return 0, errors.New("selection canceled")
	}
	if finalModel.choice < 0 {
		return 0, errors.New("no option selected")
	}
	return finalModel.choice, nil
}

func PromptText(
	stdin io.Reader,
	stdout, stderr io.Writer,
	title, hint, initial string,
	validate func(string) error,
) (string, error) {
	file, err := interactiveFile(stdin)
	if err != nil {
		return "", err
	}

	value := []rune(initial)
	model := textPromptModel{
		title:    title,
		hint:     hint,
		value:    value,
		cursor:   len(value),
		validate: validate,
	}
	result, err := tea.NewProgram(model, tea.WithInput(file), tea.WithOutput(promptOutput(stdout, stderr))).Run()
	if err != nil {
		return "", fmt.Errorf("run text prompt: %w", err)
	}
	finalModel, ok := result.(textPromptModel)
	if !ok {
		return "", errors.New("unexpected text prompt state")
	}
	if finalModel.quit {
		return "", errors.New("input canceled")
	}
	return strings.TrimSpace(string(finalModel.value)), nil
}

func Confirm(stdin io.Reader, stdout, stderr io.Writer, title, hint string, defaultYes bool) (bool, error) {
	file, err := interactiveFile(stdin)
	if err != nil {
		return false, err
	}

	value := defaultYes
	confirm := newCyanConfirm(title, hint, &value)
	form := huh.NewForm(huh.NewGroup(confirm)).
		WithTheme(cyanHuhTheme()).
		WithKeyMap(cyanHuhKeyMap()).
		WithInput(file).
		WithOutput(promptOutput(stdout, stderr)).
		WithShowHelp(true)
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return false, errors.New("confirmation canceled")
		}
		return false, fmt.Errorf("run confirmation prompt: %w", err)
	}
	if form.State != huh.StateCompleted {
		return false, errors.New("confirmation canceled")
	}
	return value, nil
}

func RequireInteractive(stdin io.Reader) error {
	_, err := interactiveFile(stdin)
	return err
}

func interactiveFile(stdin io.Reader) (*os.File, error) {
	file, ok := stdin.(*os.File)
	if !ok || !isInteractive(file) {
		return nil, errors.New("metrics requires an interactive terminal")
	}
	return file, nil
}

func (m selectionModel) Init() tea.Cmd {
	return nil
}

func (m selectionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.quit = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case "enter":
			m.choice = m.cursor
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m selectionModel) View() string {
	if m.done {
		return "\n"
	}

	var output strings.Builder
	output.WriteString(promptTitleStyle.Render(m.title))
	output.WriteString("\n")
	if m.hint != "" {
		output.WriteString(promptHintStyle.Render(m.hint))
		output.WriteString("\n")
	}
	output.WriteString("\n")
	for index, option := range m.options {
		prefix := "  "
		style := lipgloss.NewStyle()
		if index == m.cursor {
			prefix = "› "
			style = promptSelectedStyle
		}
		output.WriteString(style.Render(prefix + option))
		output.WriteString("\n")
	}
	output.WriteString("\n")
	output.WriteString(promptHintStyle.Render("Use ↑/↓ or j/k, Enter to select, Esc to cancel."))
	output.WriteString("\n")
	return output.String()
}

func (m textPromptModel) Init() tea.Cmd {
	return nil
}

func (m textPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.quit = true
			return m, tea.Quit
		case tea.KeyEnter:
			value := strings.TrimSpace(string(m.value))
			if value == "" {
				m.err = "A value is required."
				return m, nil
			}
			if m.validate != nil {
				if err := m.validate(value); err != nil {
					m.err = err.Error()
					return m, nil
				}
			}
			m.done = true
			return m, tea.Quit
		case tea.KeyLeft:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyRight:
			if m.cursor < len(m.value) {
				m.cursor++
			}
		case tea.KeyHome, tea.KeyCtrlA:
			m.cursor = 0
		case tea.KeyEnd, tea.KeyCtrlE:
			m.cursor = len(m.value)
		case tea.KeyBackspace:
			if m.cursor > 0 {
				m.value = append(m.value[:m.cursor-1], m.value[m.cursor:]...)
				m.cursor--
			}
		case tea.KeyDelete:
			if m.cursor < len(m.value) {
				m.value = append(m.value[:m.cursor], m.value[m.cursor+1:]...)
			}
		default:
			if msg.Type == tea.KeyRunes {
				m.value = insertRunes(m.value, m.cursor, msg.Runes)
				m.cursor += len(msg.Runes)
			}
		}
		m.err = ""
	}
	return m, nil
}

func (m textPromptModel) View() string {
	if m.done {
		return "\n"
	}

	var output strings.Builder
	output.WriteString(promptTitleStyle.Render(m.title))
	output.WriteString("\n")
	if m.hint != "" {
		output.WriteString(promptHintStyle.Render(m.hint))
		output.WriteString("\n")
	}
	output.WriteString("\n")

	before := string(m.value[:m.cursor])
	after := string(m.value[m.cursor:])
	cursor := " "
	if after != "" {
		r, size := utf8.DecodeRuneInString(after)
		cursor = string(r)
		after = after[size:]
	}
	output.WriteString(before)
	output.WriteString(promptCursorStyle.Render(cursor))
	output.WriteString(after)
	output.WriteString("\n")
	output.WriteString(promptHintStyle.Render("Enter to continue, Esc to cancel."))
	if m.err != "" {
		output.WriteString("\n")
		output.WriteString(promptErrorStyle.Render(m.err))
	}
	output.WriteString("\n")
	return output.String()
}

func insertRunes(value []rune, index int, runes []rune) []rune {
	if len(runes) == 0 {
		return value
	}
	value = append(value, make([]rune, len(runes))...)
	copy(value[index+len(runes):], value[index:len(value)-len(runes)])
	copy(value[index:], runes)
	return value
}

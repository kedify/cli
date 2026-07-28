package tui

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"
)

type yamlEditorModel struct {
	editor textarea.Model
	err    string
	width  int
	height int
	done   bool
	quit   bool
}

func EditYAML(
	stdin io.Reader,
	stdout, stderr io.Writer,
	value string,
) (string, error) {
	file, err := interactiveFile(stdin)
	if err != nil {
		return "", err
	}

	model := newYAMLEditorModel(value)
	result, err := tea.NewProgram(
		model,
		tea.WithInput(file),
		tea.WithOutput(promptOutput(stdout, stderr)),
	).Run()
	if err != nil {
		return "", fmt.Errorf("run YAML editor: %w", err)
	}

	finalModel, ok := result.(yamlEditorModel)
	if !ok {
		return "", errors.New("unexpected YAML editor state")
	}
	if finalModel.quit {
		return "", errors.New("YAML editing canceled")
	}
	if !finalModel.done {
		return "", errors.New("YAML editing did not finish")
	}
	return finalModel.editor.Value(), nil
}

func newYAMLEditorModel(value string) yamlEditorModel {
	editor := textarea.New()
	editor.SetValue(value)
	editor.ShowLineNumbers = true
	editor.CharLimit = 0
	editor.SetWidth(98)
	editor.SetHeight(22)
	editor.FocusedStyle.CursorLine = lipgloss.NewStyle().Background(lipgloss.Color("235"))
	editor.FocusedStyle.CursorLineNumber = lipgloss.NewStyle().
		Foreground(lipgloss.Color("86")).
		Background(lipgloss.Color("235"))
	editor.FocusedStyle.LineNumber = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	editor.FocusedStyle.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	_ = editor.Focus()

	return yamlEditorModel{
		editor: editor,
		width:  100,
		height: 30,
	}
}

func (m yamlEditorModel) Init() tea.Cmd {
	return m.editor.Focus()
}

func (m yamlEditorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeEditor()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.quit = true
			return m, tea.Quit
		case "ctrl+s":
			if err := validateYAML(m.editor.Value()); err != nil {
				m.err = err.Error()
				return m, nil
			}
			m.done = true
			m.err = ""
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	m.err = ""
	return m, cmd
}

func (m yamlEditorModel) View() string {
	if m.done {
		return "\n"
	}

	var output strings.Builder
	output.WriteString(metricsTitleStyle.Render("Edit generated YAML"))
	output.WriteString("\n")
	output.WriteString(metricsHintStyle.Render("Line numbers are for display only and are not included in the output."))
	output.WriteString("\n\n")
	output.WriteString(m.editor.View())
	if m.err != "" {
		output.WriteString("\n")
		output.WriteString(metricsErrorStyle.Render(m.err))
	}
	output.WriteString("\n\n")
	output.WriteString(metricsHintStyle.Render("Ctrl+S save and continue • Esc cancel"))
	output.WriteString("\n")
	return output.String()
}

func (m *yamlEditorModel) resizeEditor() {
	m.editor.SetWidth(max(20, m.width-2))
	m.editor.SetHeight(max(5, m.height-7))
}

func validateYAML(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("YAML cannot be empty")
	}

	decoder := yaml.NewDecoder(strings.NewReader(value))
	documents := 0
	for {
		var document any
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("invalid YAML: %w", err)
		}
		if document != nil {
			documents++
		}
	}
	if documents == 0 {
		return errors.New("YAML must contain at least one document")
	}
	return nil
}

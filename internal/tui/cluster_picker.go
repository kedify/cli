package tui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Bold(true)
	metaStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

type clusterOption struct {
	Name   string
	ID     string
	Status string
	Data   map[string]any
}

type clusterPickerModel struct {
	options []clusterOption
	cursor  int
	choice  *clusterOption
	quit    bool
}

func SelectClusterOrFail(stdin io.Reader, stdout io.Writer, clusters []map[string]any) (map[string]any, error) {
	file, ok := stdin.(*os.File)
	if !ok || !isInteractive(file) {
		return nil, errors.New("cluster name is required when not running interactively")
	}

	options := make([]clusterOption, 0, len(clusters))
	for _, cluster := range clusters {
		options = append(options, clusterOption{
			Name:   clusterName(cluster),
			ID:     clusterValue(cluster, "id"),
			Status: clusterValue(cluster, "agentStatus"),
			Data:   cluster,
		})
	}

	if len(options) == 0 {
		return nil, errors.New("no clusters available")
	}

	model := clusterPickerModel{options: options}
	program := tea.NewProgram(model, tea.WithInput(file), tea.WithOutput(stdout))
	result, err := program.Run()
	if err != nil {
		return nil, fmt.Errorf("run cluster picker: %w", err)
	}

	finalModel, ok := result.(clusterPickerModel)
	if !ok {
		return nil, errors.New("unexpected cluster picker state")
	}

	if finalModel.quit {
		return nil, errors.New("cluster selection canceled")
	}
	if finalModel.choice == nil {
		return nil, errors.New("no cluster selected")
	}

	return finalModel.choice.Data, nil
}

func (m clusterPickerModel) Init() tea.Cmd {
	return nil
}

func (m clusterPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			choice := m.options[m.cursor]
			m.choice = &choice
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m clusterPickerModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Select a Cluster"))
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("Use ↑/↓ or j/k, Enter to select, Esc to cancel."))
	b.WriteString("\n\n")

	for i, option := range m.options {
		prefix := "  "
		lineStyle := lipgloss.NewStyle()
		if i == m.cursor {
			prefix = "› "
			lineStyle = selectedStyle
		}

		b.WriteString(lineStyle.Render(prefix + option.Name))
		if option.Status != "" || option.ID != "" {
			metaParts := make([]string, 0, 2)
			if option.Status != "" {
				metaParts = append(metaParts, option.Status)
			}
			if option.ID != "" {
				metaParts = append(metaParts, option.ID)
			}
			b.WriteString("\n")
			b.WriteString(metaStyle.Render("  " + strings.Join(metaParts, " • ")))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func clusterName(cluster map[string]any) string {
	if name := clusterValue(cluster, "name"); name != "" {
		return name
	}
	if id := clusterValue(cluster, "id"); id != "" {
		return id
	}
	return "<unnamed cluster>"
}

func clusterValue(cluster map[string]any, key string) string {
	value, ok := cluster[key]
	if !ok {
		return ""
	}

	text, ok := value.(string)
	if !ok {
		return ""
	}

	return text
}

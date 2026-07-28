package tui

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

type resourceCreationModel struct {
	viewport     viewport.Model
	confirmation *huh.Confirm
	answer       *bool
	context      string
	namespace    string
	confirmed    bool
	done         bool
	quit         bool
	width        int
	height       int
}

func ConfirmResourceCreation(
	stdin io.Reader,
	stdout, stderr io.Writer,
	manifest, currentContext, namespace string,
) (bool, error) {
	file, err := interactiveFile(stdin)
	if err != nil {
		return false, err
	}

	model := newResourceCreationModel(manifest, currentContext, namespace)
	result, err := tea.NewProgram(
		model,
		tea.WithInput(file),
		tea.WithOutput(promptOutput(stdout, stderr)),
	).Run()
	if err != nil {
		return false, fmt.Errorf("run resource creation confirmation: %w", err)
	}

	finalModel, ok := result.(resourceCreationModel)
	if !ok {
		return false, errors.New("unexpected resource creation confirmation state")
	}
	if finalModel.quit {
		return false, errors.New("resource creation confirmation canceled")
	}
	if !finalModel.done {
		return false, errors.New("resource creation confirmation did not finish")
	}
	return finalModel.confirmed, nil
}

func newResourceCreationModel(manifest, currentContext, namespace string) resourceCreationModel {
	yamlViewport := viewport.New(96, 14)
	yamlViewport.Style = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("238")).
		Padding(0, 1)
	yamlViewport.SetContent(manifest)

	answer := false
	confirmation := newCyanConfirm(
		"Are you sure you want to create these resources in k8s cluster?",
		"",
		&answer,
	)
	confirmation.WithWidth(98)

	return resourceCreationModel{
		viewport:     yamlViewport,
		confirmation: confirmation,
		answer:       &answer,
		context:      currentContext,
		namespace:    namespace,
		width:        100,
		height:       30,
	}
}

func (m resourceCreationModel) Init() tea.Cmd {
	return nil
}

func (m resourceCreationModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViewport()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.quit = true
			return m, tea.Quit
		case "up", "down", "j", "k", "pgup", "pgdown", "home", "end":
			break
		default:
			updated, cmd := m.confirmation.Update(msg)
			m.confirmation = updated.(*huh.Confirm)
			switch msg.String() {
			case "y", "Y", "n", "N", "enter":
				m.confirmed = *m.answer
				m.done = true
				return m, tea.Quit
			}
			return m, cmd
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m resourceCreationModel) View() string {
	if m.done {
		return "\n"
	}

	var output strings.Builder
	output.WriteString(metricsTitleStyle.Render("Review Kubernetes resources"))
	output.WriteString("\n\n")
	output.WriteString("Kubernetes context: ")
	output.WriteString(metricsQueryStyle.Render(m.context))
	output.WriteString("\n")
	output.WriteString("Target namespace:  ")
	output.WriteString(metricsQueryStyle.Render(m.namespace))
	output.WriteString("\n\n")
	output.WriteString(metricsHintStyle.Render("YAML to create:"))
	output.WriteString("\n")
	output.WriteString(m.viewport.View())
	if m.viewport.TotalLineCount() > m.viewport.VisibleLineCount() {
		output.WriteString("\n")
		output.WriteString(metricsHintStyle.Render(fmt.Sprintf(
			"YAML scroll: %3.0f%%",
			m.viewport.ScrollPercent()*100,
		)))
	}
	output.WriteString("\n\n")
	output.WriteString(m.confirmation.View())
	output.WriteString("\n")
	output.WriteString(metricsHintStyle.Render("↑/↓/PgUp/PgDn review YAML • ←/→ choose • y/n answer • Enter confirm • Esc cancel"))
	output.WriteString("\n")
	return output.String()
}

func (m *resourceCreationModel) resizeViewport() {
	contentWidth := max(30, m.width-2)
	m.viewport.Width = contentWidth
	m.viewport.Height = max(5, m.height-14)
	m.confirmation.WithWidth(contentWidth)
}

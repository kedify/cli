package tui

import (
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type operationDoneMsg[T any] struct {
	result T
	err    error
}

type operationSpinnerModel[T any] struct {
	spinner   spinner.Model
	message   string
	operation func() (T, error)
	result    T
	err       error
	done      bool
}

func RunWithSpinner[T any](output io.Writer, message string, operation func() (T, error)) (T, error) {
	if output == nil || !isInteractiveWriter(output) {
		if output != nil {
			_, _ = fmt.Fprintln(output, message)
		}
		return operation()
	}

	model := newOperationSpinnerModel(message, operation)
	result, err := tea.NewProgram(
		model,
		tea.WithInput(nil),
		tea.WithOutput(output),
	).Run()
	if err != nil {
		var zero T
		return zero, fmt.Errorf("run progress spinner: %w", err)
	}

	finalModel, ok := result.(operationSpinnerModel[T])
	if !ok {
		var zero T
		return zero, fmt.Errorf("unexpected progress spinner state")
	}
	return finalModel.result, finalModel.err
}

func newOperationSpinnerModel[T any](
	message string,
	operation func() (T, error),
) operationSpinnerModel[T] {
	return operationSpinnerModel[T]{
		spinner: spinner.New(
			spinner.WithSpinner(spinner.MiniDot),
			spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("86"))),
		),
		message:   message,
		operation: operation,
	}
}

func (m operationSpinnerModel[T]) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			result, err := m.operation()
			return operationDoneMsg[T]{result: result, err: err}
		},
	)
}

func (m operationSpinnerModel[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case operationDoneMsg[T]:
		m.result = msg.result
		m.err = msg.err
		m.done = true
		return m, tea.Quit
	default:
		return m, nil
	}
}

func (m operationSpinnerModel[T]) View() string {
	if m.done {
		return ""
	}
	return m.spinner.View() + " " + m.message
}

func isInteractiveWriter(output io.Writer) bool {
	file, ok := output.(*os.File)
	return ok && isInteractive(file)
}

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

const apiKeysURL = "https://dashboard.dev.kedify.io/api-keys" // #nosec G101 -- public dashboard URL, not a credential

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	promptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	hintStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

type loginModel struct {
	token string
	err   string
	done  bool
	quit  bool
}

func ReadSecretOrPipe(stdin io.Reader, stdout io.Writer, stderr io.Writer) (string, error) {
	if file, ok := stdin.(*os.File); ok && isInteractive(file) {
		model := loginModel{}
		program := tea.NewProgram(model, tea.WithInput(file), tea.WithOutput(promptOutput(stdout, stderr)))
		result, err := program.Run()
		if err != nil {
			return "", fmt.Errorf("run login prompt: %w", err)
		}

		finalModel, ok := result.(loginModel)
		if !ok {
			return "", errors.New("unexpected login prompt state")
		}

		if finalModel.quit {
			return "", errors.New("login canceled")
		}
		if finalModel.token == "" {
			return "", errors.New("no token provided")
		}

		return finalModel.token, nil
	}

	data, err := io.ReadAll(stdin)
	if err != nil {
		return "", fmt.Errorf("read token from stdin: %w", err)
	}

	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("no token provided on stdin, generate one at %s", apiKeysURL)
	}

	return token, nil
}

func promptOutput(stdout io.Writer, stderr io.Writer) io.Writer {
	if stderr != nil {
		return stderr
	}

	return stdout
}

func (m loginModel) Init() tea.Cmd {
	return nil
}

func (m loginModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.quit = true
			return m, tea.Quit
		case tea.KeyEnter:
			if strings.TrimSpace(m.token) == "" {
				m.err = "Please paste a token."
				return m, nil
			}
			m.done = true
			return m, tea.Quit
		case tea.KeyBackspace, tea.KeyDelete:
			if len(m.token) > 0 {
				m.token = m.token[:len(m.token)-1]
			}
			m.err = ""
		default:
			if msg.Type == tea.KeyRunes {
				m.token += string(msg.Runes)
				m.err = ""
			}
		}
	}

	return m, nil
}

func (m loginModel) View() string {
	if m.done {
		return "\n"
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("Kedify Login"))
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("Generate a token at " + apiKeysURL))
	b.WriteString("\n\n")
	b.WriteString(promptStyle.Render("Paste Kedify token: "))
	b.WriteString(strings.Repeat("•", len(m.token)))
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("Press Enter to save, Esc to cancel."))

	if m.err != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(m.err))
	}

	b.WriteString("\n")
	return b.String()
}

func isInteractive(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}

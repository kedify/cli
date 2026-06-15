package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kedify/cli/internal/config"
	"golang.org/x/term"
)

const apiKeysURL = "https://dashboard.dev.kedify.io/api-keys"

// LoginCmd stores a Kedify API token in the local user configuration.
type LoginCmd struct{}

// Run executes the login command.
func (c *LoginCmd) Run(app *Context) error {
	token, err := readToken(app.Stdin, app.Stderr)
	if err != nil {
		return err
	}

	if err := config.WriteCredentials(config.Credentials{Token: token}); err != nil {
		return err
	}

	_, err = fmt.Fprintln(app.Stdout, "Credentials stored in ~/.config/kedify/credentials.json")
	return err
}

func readToken(stdin *os.File, stderr io.Writer) (string, error) {
	if isInteractiveInput(stdin) {
		if _, err := fmt.Fprintf(stderr, "Generate a Kedify token at %s\nPaste Kedify token and press Enter: ", apiKeysURL); err != nil {
			return "", fmt.Errorf("write prompt: %w", err)
		}

		line, err := term.ReadPassword(int(stdin.Fd()))
		if err != nil {
			return "", fmt.Errorf("read token from terminal: %w", err)
		}

		token := strings.TrimSpace(string(line))
		if token == "" {
			return "", errors.New("no token provided")
		}

		if _, err := fmt.Fprintln(stderr); err != nil {
			return "", fmt.Errorf("finish prompt: %w", err)
		}

		return token, nil
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

func isInteractiveInput(stdin *os.File) bool {
	info, err := stdin.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}

package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"github.com/kedify/cli/internal/config"
)

const apiKeysURL = "https://dashboard.dev.kedify.io/api-keys"

type LoginCmd struct{}

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

		restore, err := disableTerminalEcho(stdin)
		if err != nil {
			return "", fmt.Errorf("disable terminal echo: %w", err)
		}
		defer restore()

		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read token from terminal: %w", err)
		}

		token := strings.TrimSpace(line)
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

func disableTerminalEcho(file *os.File) (func(), error) {
	fd := file.Fd()

	termios, err := getTermios(fd)
	if err != nil {
		return nil, err
	}

	original := *termios
	updated := *termios
	updated.Lflag &^= syscall.ECHO

	if err := setTermios(fd, &updated); err != nil {
		return nil, err
	}

	return func() {
		_ = setTermios(fd, &original)
	}, nil
}

func getTermios(fd uintptr) (*syscall.Termios, error) {
	termios := &syscall.Termios{}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TCGETS), uintptr(unsafe.Pointer(termios)))
	if errno != 0 {
		return nil, errno
	}

	return termios, nil
}

func setTermios(fd uintptr, termios *syscall.Termios) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(termios)))
	if errno != 0 {
		return errno
	}

	return nil
}

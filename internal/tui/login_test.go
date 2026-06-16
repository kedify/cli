package tui

import (
	"bytes"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestReadSecretOrPipeReadsPipedInput(t *testing.T) {
	token, err := ReadSecretOrPipe(strings.NewReader("token-from-pipe\n"), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("ReadSecretOrPipe() error = %v", err)
	}
	if token != "token-from-pipe" {
		t.Fatalf("token = %q, want %q", token, "token-from-pipe")
	}
}

func TestLoginModelEnterWithoutTokenShowsError(t *testing.T) {
	model := loginModel{}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(loginModel)
	if got.err == "" {
		t.Fatal("expected validation error, got none")
	}
}

func TestLoginModelAcceptsTypedToken(t *testing.T) {
	model := loginModel{}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("abc")})
	got := updated.(loginModel)
	if got.token != "abc" {
		t.Fatalf("token = %q, want %q", got.token, "abc")
	}
}

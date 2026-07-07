package auth

import (
	"bytes"
	"io"
	"strings"
	"testing"

	clictx "github.com/kedify/cli/internal/cli/context"
	"github.com/kedify/cli/internal/service"
)

type fakeCredentialsStore struct {
	creds    service.Credentials
	readErr  error
	writeErr error
	wrote    service.Credentials
}

func (f *fakeCredentialsStore) ReadCredentials() (service.Credentials, error) {
	if f.readErr != nil {
		return service.Credentials{}, f.readErr
	}
	return f.creds, nil
}

func (f *fakeCredentialsStore) WriteCredentials(creds service.Credentials) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.wrote = creds
	return nil
}

func TestLoginCmdRunStoresCredentials(t *testing.T) {
	store := &fakeCredentialsStore{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ctx := &clictx.Context{
		Stdin:       bytes.NewBuffer(nil),
		Stdout:      stdout,
		Stderr:      stderr,
		Credentials: store,
		ReadSecret: func(_ io.Reader, _ io.Writer, _ io.Writer) (string, error) {
			return "secret-token", nil
		},
	}

	if err := (&LoginCmd{}).Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if store.wrote.Token != "secret-token" {
		t.Fatalf("stored token = %q, want %q", store.wrote.Token, "secret-token")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Credentials stored.") {
		t.Fatalf("stderr = %q, want confirmation message", stderr.String())
	}
}

func TestLoginCmdRunUsesExplicitToken(t *testing.T) {
	store := &fakeCredentialsStore{}
	ctx := &clictx.Context{
		Stdin:       bytes.NewBuffer(nil),
		Stdout:      &bytes.Buffer{},
		Stderr:      &bytes.Buffer{},
		Credentials: store,
		ReadSecret: func(_ io.Reader, _ io.Writer, _ io.Writer) (string, error) {
			t.Fatal("ReadSecret should not be called when token is provided explicitly")
			return "", nil
		},
	}

	if err := (&LoginCmd{Token: "arg-token"}).Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if store.wrote.Token != "arg-token" {
		t.Fatalf("stored token = %q, want %q", store.wrote.Token, "arg-token")
	}
}

func TestLoginCmdRunIgnoresWhitespaceExplicitToken(t *testing.T) {
	store := &fakeCredentialsStore{}
	ctx := &clictx.Context{
		Stdin:       bytes.NewBuffer(nil),
		Stdout:      &bytes.Buffer{},
		Stderr:      &bytes.Buffer{},
		Token:       "env-token",
		Credentials: store,
		ReadSecret: func(_ io.Reader, _ io.Writer, _ io.Writer) (string, error) {
			t.Fatal("ReadSecret should not be called when trimmed context token is available")
			return "", nil
		},
	}

	if err := (&LoginCmd{Token: "   "}).Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if store.wrote.Token != "env-token" {
		t.Fatalf("stored token = %q, want %q", store.wrote.Token, "env-token")
	}
}

func TestLoginCmdRunIgnoresWhitespaceContextToken(t *testing.T) {
	store := &fakeCredentialsStore{}
	ctx := &clictx.Context{
		Stdin:       bytes.NewBuffer(nil),
		Stdout:      &bytes.Buffer{},
		Stderr:      &bytes.Buffer{},
		Token:       "   ",
		Credentials: store,
		ReadSecret: func(_ io.Reader, _ io.Writer, _ io.Writer) (string, error) {
			return "secret-token", nil
		},
	}

	if err := (&LoginCmd{}).Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if store.wrote.Token != "secret-token" {
		t.Fatalf("stored token = %q, want %q", store.wrote.Token, "secret-token")
	}
}

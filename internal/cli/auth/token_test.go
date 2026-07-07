package auth

import (
	"bytes"
	"testing"

	clictx "github.com/kedify/cli/internal/cli/context"
	"github.com/kedify/cli/internal/service"
)

type fakeTokenCredentialsStore struct {
	creds service.Credentials
}

func (f *fakeTokenCredentialsStore) ReadCredentials() (service.Credentials, error) {
	return f.creds, nil
}

func (f *fakeTokenCredentialsStore) WriteCredentials(service.Credentials) error {
	return nil
}

func TestAuthTokenCmdRunPrintsStoredToken(t *testing.T) {
	store := &fakeTokenCredentialsStore{creds: service.Credentials{Token: "stored-token"}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ctx := &clictx.Context{
		Stdin:       bytes.NewBuffer(nil),
		Stdout:      stdout,
		Stderr:      stderr,
		Credentials: store,
	}

	if err := (&AuthTokenCmd{}).Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if stdout.String() != "stored-token\n" {
		t.Fatalf("stdout = %q, want token with newline", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestAuthTokenCmdRunPrefersContextToken(t *testing.T) {
	store := &fakeTokenCredentialsStore{creds: service.Credentials{Token: "stored-token"}}
	stdout := &bytes.Buffer{}
	ctx := &clictx.Context{
		Stdin:       bytes.NewBuffer(nil),
		Stdout:      stdout,
		Stderr:      &bytes.Buffer{},
		Token:       "override-token",
		Credentials: store,
	}

	if err := (&AuthTokenCmd{}).Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if stdout.String() != "override-token\n" {
		t.Fatalf("stdout = %q, want override token with newline", stdout.String())
	}
}

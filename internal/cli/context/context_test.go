package context

import (
	"testing"

	"github.com/kedify/cli/internal/service"
)

type fakeCredentialsStore struct {
	creds   service.Credentials
	readErr error
}

func (f *fakeCredentialsStore) ReadCredentials() (service.Credentials, error) {
	if f.readErr != nil {
		return service.Credentials{}, f.readErr
	}
	return f.creds, nil
}

func (f *fakeCredentialsStore) WriteCredentials(service.Credentials) error {
	return nil
}

func TestResolveTokenFallsBackToCredentials(t *testing.T) {
	ctx := &Context{
		Credentials: &fakeCredentialsStore{creds: service.Credentials{Token: "stored-token"}},
	}

	token, err := ResolveToken(ctx)
	if err != nil {
		t.Fatalf("ResolveToken() error = %v", err)
	}
	if token != "stored-token" {
		t.Fatalf("token = %q, want %q", token, "stored-token")
	}
}

func TestResolveTokenPrefersContextToken(t *testing.T) {
	ctx := &Context{
		Token:       "override-token",
		Credentials: &fakeCredentialsStore{creds: service.Credentials{Token: "stored-token"}},
	}

	token, err := ResolveToken(ctx)
	if err != nil {
		t.Fatalf("ResolveToken() error = %v", err)
	}
	if token != "override-token" {
		t.Fatalf("token = %q, want %q", token, "override-token")
	}
}

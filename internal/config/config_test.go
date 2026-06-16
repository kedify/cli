package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestReadCredentialsPrefersKeyring(t *testing.T) {
	origGet := keyringGet
	t.Cleanup(func() {
		keyringGet = origGet
	})

	keyringGet = func(service, user string) (string, error) {
		return "keyring-token", nil
	}

	creds, err := ReadCredentials()
	if err != nil {
		t.Fatalf("ReadCredentials() error = %v", err)
	}
	if creds.Token != "keyring-token" {
		t.Fatalf("token = %q, want %q", creds.Token, "keyring-token")
	}
}

func TestReadCredentialsFallsBackToFileWhenKeyringUnavailable(t *testing.T) {
	origGet := keyringGet
	t.Cleanup(func() {
		keyringGet = origGet
	})

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	keyringGet = func(service, user string) (string, error) {
		return "", keyring.ErrUnsupportedPlatform
	}

	filePath := filepath.Join(tmpHome, credentialsDir, credentialsFile)
	if err := os.MkdirAll(filepath.Dir(filePath), fileModeDir); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filePath, []byte("{\"token\":\"file-token\"}\n"), fileModeCreds); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	creds, err := ReadCredentials()
	if err != nil {
		t.Fatalf("ReadCredentials() error = %v", err)
	}
	if creds.Token != "file-token" {
		t.Fatalf("token = %q, want %q", creds.Token, "file-token")
	}
}

func TestWriteCredentialsFallsBackToFileWhenKeyringUnavailable(t *testing.T) {
	origSet := keyringSet
	t.Cleanup(func() {
		keyringSet = origSet
	})

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	keyringSet = func(service, user, password string) error {
		return keyring.ErrUnsupportedPlatform
	}

	if err := WriteCredentials(Credentials{Token: "file-token"}); err != nil {
		t.Fatalf("WriteCredentials() error = %v", err)
	}

	creds, err := readCredentialsFile()
	if err != nil {
		t.Fatalf("readCredentialsFile() error = %v", err)
	}
	if creds.Token != "file-token" {
		t.Fatalf("token = %q, want %q", creds.Token, "file-token")
	}
}

func TestWriteCredentialsReturnsKeyringErrorWhenStoreIsAvailableButFails(t *testing.T) {
	origSet := keyringSet
	t.Cleanup(func() {
		keyringSet = origSet
	})

	keyringSet = func(service, user, password string) error {
		return errors.New("permission denied")
	}

	err := WriteCredentials(Credentials{Token: "token"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

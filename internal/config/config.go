package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
)

const (
	credentialsDir  = ".config/kedify"
	credentialsFile = "credentials.json"
	fileModeDir     = 0o700
	fileModeCreds   = 0o600
	keyringService  = "io.kedify.cli"
	keyringUser     = "default"
)

type Credentials struct {
	Token string `json:"token"`
}

var (
	keyringSet = keyring.Set
	keyringGet = keyring.Get
)

func WriteCredentials(creds Credentials) error {
	err := keyringSet(keyringService, keyringUser, creds.Token)
	if err == nil {
		return nil
	}
	if !isKeyringUnavailable(err) {
		return fmt.Errorf("write credentials to keyring: %w", err)
	}

	return writeCredentialsFile(creds)
}

func ReadCredentials() (Credentials, error) {
	token, err := keyringGet(keyringService, keyringUser)
	switch {
	case err == nil:
		token = strings.TrimSpace(token)
		if token == "" {
			return Credentials{}, errors.New("keyring entry does not contain a token")
		}
		return Credentials{Token: token}, nil
	case errors.Is(err, keyring.ErrNotFound), isKeyringUnavailable(err):
		return readCredentialsFile()
	default:
		return Credentials{}, fmt.Errorf("read credentials from keyring: %w", err)
	}
}

func APIURLFromEnv() string {
	return strings.TrimSpace(os.Getenv("KEDIFY_API_URL"))
}

func writeCredentialsFile(creds Credentials) error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), fileModeDir); err != nil {
		return fmt.Errorf("create credentials directory: %w", err)
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}

	data = append(data, '\n')
	if err := os.WriteFile(path, data, fileModeCreds); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}

	return nil
}

func readCredentialsFile() (Credentials, error) {
	path, err := credentialsPath()
	if err != nil {
		return Credentials{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Credentials{}, errors.New("credentials not found, run `kedify login` first")
		}
		return Credentials{}, fmt.Errorf("read credentials: %w", err)
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return Credentials{}, fmt.Errorf("parse credentials: %w", err)
	}

	if strings.TrimSpace(creds.Token) == "" {
		return Credentials{}, errors.New("credentials file does not contain a token")
	}

	return creds, nil
}

func credentialsPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	return filepath.Join(homeDir, credentialsDir, credentialsFile), nil
}

func isKeyringUnavailable(err error) bool {
	if errors.Is(err, keyring.ErrUnsupportedPlatform) {
		return true
	}

	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"dbus",
		"secret service",
		"org.freedesktop.secrets",
		"keyring is not available",
		"credential manager is not available",
		"keychain is not available",
		"not supported",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}

	return false
}

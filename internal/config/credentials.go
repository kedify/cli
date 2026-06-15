package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	credentialsDir  = ".config/kedify"
	credentialsFile = "credentials.json"
	fileModeDir     = 0o700
	fileModeCreds   = 0o600
)

type Credentials struct {
	Token string `json:"token"`
}

func WriteCredentials(creds Credentials) error {
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

func ReadCredentials() (Credentials, error) {
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

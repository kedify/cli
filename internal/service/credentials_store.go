package service

import (
	"github.com/kedify/cli/internal/config"
)

type CredentialsStore interface {
	ReadCredentials() (Credentials, error)
	WriteCredentials(Credentials) error
}

type Credentials struct {
	Token string
}

type ConfigStore struct{}

func (ConfigStore) ReadCredentials() (Credentials, error) {
	creds, err := config.ReadCredentials()
	if err != nil {
		return Credentials{}, err
	}

	return Credentials{Token: creds.Token}, nil
}

func (ConfigStore) WriteCredentials(creds Credentials) error {
	return config.WriteCredentials(config.Credentials{Token: creds.Token})
}

package cli

import "github.com/kedify/cli/internal/config"

type configStore struct{}

func (configStore) ReadCredentials() (credentials, error) {
	creds, err := config.ReadCredentials()
	if err != nil {
		return credentials{}, err
	}

	return credentials{Token: creds.Token}, nil
}

func (configStore) WriteCredentials(creds credentials) error {
	return config.WriteCredentials(config.Credentials{Token: creds.Token})
}

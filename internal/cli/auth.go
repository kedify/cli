package cli

import "strings"

func resolveToken(ctx *context) (string, error) {
	if token := strings.TrimSpace(ctx.token); token != "" {
		return token, nil
	}

	creds, err := ctx.credentials.ReadCredentials()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(creds.Token), nil
}

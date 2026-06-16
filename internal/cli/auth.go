package cli

func resolveToken(ctx *context) (string, error) {
	if ctx.token != "" {
		return ctx.token, nil
	}

	creds, err := ctx.credentials.ReadCredentials()
	if err != nil {
		return "", err
	}

	return creds.Token, nil
}

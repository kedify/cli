package cli

import (
	"fmt"
	"strings"
)

type LoginCmd struct {
	Token string `arg:"" optional:"" name:"token" help:"Token to store without prompting."`
}

func (c *LoginCmd) Run(ctx *context) error {
	token := strings.TrimSpace(c.Token)
	if token == "" {
		token = strings.TrimSpace(ctx.token)
	}
	if token == "" {
		var err error
		token, err = ctx.readSecret(ctx.stdin, ctx.stdout, ctx.stderr)
		if err != nil {
			return err
		}
	}

	if err := ctx.credentials.WriteCredentials(credentials{Token: token}); err != nil {
		return err
	}

	_, err := fmt.Fprintln(ctx.stdout, "Credentials stored.")
	return err
}

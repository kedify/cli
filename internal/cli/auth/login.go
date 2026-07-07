package auth

import (
	"fmt"
	"strings"

	clictx "github.com/kedify/cli/internal/cli/context"
	"github.com/kedify/cli/internal/service"
)

type LoginCmd struct {
	Token string `arg:"" optional:"" name:"token" help:"Token to store without prompting."`
}

func (c *LoginCmd) Run(ctx *clictx.Context) error {
	token := strings.TrimSpace(c.Token)
	if token == "" {
		token = strings.TrimSpace(ctx.Token)
	}
	if token == "" {
		var err error
		token, err = ctx.ReadSecret(ctx.Stdin, ctx.Stdout, ctx.Stderr)
		if err != nil {
			return err
		}
	}

	if err := ctx.Credentials.WriteCredentials(service.Credentials{Token: token}); err != nil {
		return err
	}

	_, err := fmt.Fprintln(ctx.Stderr, "Credentials stored.")
	return err
}

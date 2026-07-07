package auth

import (
	"fmt"

	clictx "github.com/kedify/cli/internal/cli/context"
)

type AuthTokenCmd struct{}

func (c *AuthTokenCmd) Run(ctx *clictx.Context) error {
	token, err := clictx.ResolveToken(ctx)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(ctx.Stdout, token)
	return err
}

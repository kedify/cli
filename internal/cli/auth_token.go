package cli

import "fmt"

type AuthTokenCmd struct{}

func (c *AuthTokenCmd) Run(ctx *context) error {
	token, err := resolveToken(ctx)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(ctx.stdout, token)
	return err
}

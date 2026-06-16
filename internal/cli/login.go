package cli

import (
	"fmt"
	"io"

	"github.com/kedify/cli/internal/config"
	"github.com/kedify/cli/internal/tui"
)

func runLogin(ctx *context, args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "--help", "-h", "help":
			writeLoginHelp(ctx.stdout)
			return nil
		default:
			return fmt.Errorf("unexpected arguments: %v", args)
		}
	}

	token, err := tui.ReadSecretOrPipe(ctx.stdin, ctx.stdout, ctx.stderr)
	if err != nil {
		return err
	}

	if err := config.WriteCredentials(config.Credentials{Token: token}); err != nil {
		return err
	}

	_, err = fmt.Fprintln(ctx.stdout, "Credentials stored in ~/.config/kedify/credentials.json")
	return err
}

func writeLoginHelp(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage: kedify login

Read an auth token from stdin and store it locally.
Generate a token at https://dashboard.dev.kedify.io/api-keys.
`)
}

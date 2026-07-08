package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/alecthomas/kong"

	"github.com/kedify/cli/internal/api"
	"github.com/kedify/cli/internal/cli/apply"
	"github.com/kedify/cli/internal/cli/auth"
	clictx "github.com/kedify/cli/internal/cli/context"
	"github.com/kedify/cli/internal/cli/delete"
	"github.com/kedify/cli/internal/cli/get"
	"github.com/kedify/cli/internal/cli/list"
	clierrors "github.com/kedify/cli/internal/errors"
	"github.com/kedify/cli/internal/output"
	"github.com/kedify/cli/internal/service"
	"github.com/kedify/cli/internal/tui"
)

type CLI struct {
	APIURL string    `name:"apiurl" help:"Base URL for the Kedify API." default:"https://api.dev.kedify.io/v1" env:"KEDIFY_API_URL"`
	Token  string    `name:"token" help:"Kedify API token." env:"KEDIFY_TOKEN"`
	Auth   AuthCmd   `cmd:"" help:"Authentication helpers."`
	Apply  ApplyCmd  `cmd:"" help:"Apply Kedify recommendations."`
	Delete DeleteCmd `cmd:"" help:"Delete Kedify resources."`
	Get    GetCmd    `cmd:"" help:"Get Kedify resources."`
	List   ListCmd   `cmd:"" help:"List Kedify resources."`
}

type AuthCmd struct {
	Login auth.LoginCmd     `cmd:"" help:"Read an auth token from stdin and store it locally. Generate a token at https://dashboard.dev.kedify.io/api-keys."`
	Token auth.AuthTokenCmd `cmd:"" help:"Print the auth token."`
}

type GetCmd struct {
	Cluster get.GetClusterCmd `cmd:"" help:"Get a cluster by name or id. If no name is provided, an interactive picker is shown."`
}

type ApplyCmd struct {
	Recommendations apply.ApplyRecommendationsCmd `cmd:"" help:"Apply recommendations to a Helm values file."`
}

type DeleteCmd struct {
	Cluster delete.DeleteClusterCmd `cmd:"" help:"Delete a cluster by name or id. If no name is provided, an interactive picker is shown."`
}

type ListCmd struct {
	Clusters        list.ListClustersCmd        `cmd:"" help:"List clusters."`
	Recommendations list.ListRecommendationsCmd `cmd:"" help:"List recommendations for a cluster id."`
}

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	app := &clictx.Context{
		Stdin:         stdin,
		Stdout:        stdout,
		Stderr:        stderr,
		Client:        api.NewClient(),
		Credentials:   service.ConfigStore{},
		ReadSecret:    tui.ReadSecretOrPipe,
		SelectCluster: tui.SelectClusterOrFail,
		WriteOutput:   output.WriteOutput,
	}

	var cli CLI
	parser, err := kong.New(
		&cli,
		kong.Name("kedify"),
		kong.Description("Kedify command line interface."),
		kong.Writers(stdout, stderr),
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "kedify: error: %v\n", err)
		return 1
	}

	kctx, err := parser.Parse(args)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "kedify: error: %v\n", err)
		return 1
	}

	app.APIURL = cli.APIURL
	app.Token = cli.Token
	if err := kctx.Run(app); err != nil {
		var cmdErr *clierrors.CommandResultError
		if errors.As(err, &cmdErr) {
			if cmdErr.Payload != nil {
				if writeErr := app.WriteOutput(stdout, cmdErr.Payload, "json"); writeErr != nil {
					_, _ = fmt.Fprintf(stderr, "kedify: error: %v\n", writeErr)
					return 1
				}
			}
			return cmdErr.ExitCode
		}
		_, _ = fmt.Fprintf(stderr, "kedify: error: %v\n", err)
		return 1
	}

	return 0
}

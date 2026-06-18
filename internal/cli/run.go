package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/alecthomas/kong"
	"github.com/kedify/cli/internal/api"
	"github.com/kedify/cli/internal/tui"
)

type CLI struct {
	APIURL string   `name:"apiurl" help:"Base URL for the Kedify API." default:"https://api.dev.kedify.io/v1" env:"KEDIFY_API_URL"`
	Token  string   `name:"token" help:"Kedify API token." env:"KEDIFY_TOKEN"`
	Auth   AuthCmd  `cmd:"" help:"Authentication helpers."`
	Apply  ApplyCmd `cmd:"" help:"Apply Kedify recommendations."`
	Get    GetCmd   `cmd:"" help:"Get Kedify resources."`
	List   ListCmd  `cmd:"" help:"List Kedify resources."`
}

type AuthCmd struct {
	Login LoginCmd     `cmd:"" help:"Read an auth token from stdin and store it locally. Generate a token at https://dashboard.dev.kedify.io/api-keys."`
	Token AuthTokenCmd `cmd:"" help:"Print the auth token."`
}

type GetCmd struct {
	Cluster GetClusterCmd `cmd:"" help:"Get a cluster by name or id. If no name is provided, an interactive picker is shown."`
}

type ApplyCmd struct {
	Recommendations ApplyRecommendationsCmd `cmd:"" help:"Apply recommendations to a Helm values file."`
}

type ListCmd struct {
	Clusters        ListClustersCmd        `cmd:"" help:"List clusters."`
	Recommendations ListRecommendationsCmd `cmd:"" help:"List recommendations for a cluster id."`
}

type ListClustersCmd struct {
	Output string `name:"output" short:"o" help:"Output format." enum:"text,json,yaml" default:"text"`
}

type credentialsStore interface {
	ReadCredentials() (credentials, error)
	WriteCredentials(credentials) error
}

type clusterService interface {
	ListClusters(apiURL, token string) ([]map[string]any, error)
	GetCluster(apiURL, token, clusterID string) (map[string]any, error)
	GetRecommendations(apiURL, token, clusterID string) (any, error)
}

type credentials struct {
	Token string
}

type context struct {
	stdin         io.Reader
	stdout        io.Writer
	stderr        io.Writer
	apiURL        string
	token         string
	client        clusterService
	credentials   credentialsStore
	readSecret    func(io.Reader, io.Writer, io.Writer) (string, error)
	selectCluster func(io.Reader, io.Writer, io.Writer, []map[string]any) (map[string]any, error)
	writeOutput   func(io.Writer, any, string) error
}

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	app := &context{
		stdin:         stdin,
		stdout:        stdout,
		stderr:        stderr,
		client:        api.NewClient(),
		credentials:   configStore{},
		readSecret:    tui.ReadSecretOrPipe,
		selectCluster: tui.SelectClusterOrFail,
		writeOutput:   writeOutput,
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

	app.apiURL = cli.APIURL
	app.token = cli.Token
	if err := kctx.Run(app); err != nil {
		var cmdErr *commandResultError
		if errors.As(err, &cmdErr) {
			if cmdErr.payload != nil {
				if writeErr := app.writeOutput(stdout, cmdErr.payload, "json"); writeErr != nil {
					_, _ = fmt.Fprintf(stderr, "kedify: error: %v\n", writeErr)
					return 1
				}
			}
			return cmdErr.exitCode
		}
		_, _ = fmt.Fprintf(stderr, "kedify: error: %v\n", err)
		return 1
	}

	return 0
}

type commandResultError struct {
	exitCode int
	payload  any
}

func (e *commandResultError) Error() string {
	return "command failed"
}

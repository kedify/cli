// Package cli wires the Kedify command tree and its runtime dependencies.
package cli

import (
	"net/http"
	"os"
	"time"

	"github.com/alecthomas/kong"
	"github.com/kedify/cli/internal/api"
)

const httpRequestLimit = 30 * time.Second

type Root struct {
	APIURL string   `help:"Base URL for the Kedify API." default:"https://api.dev.kedify.io/v1" env:"KEDIFY_API_URL"`
	Login  LoginCmd `cmd:"" help:"Read an auth token from stdin and store it locally. Generate a token at https://dashboard.dev.kedify.io/api-keys."`
	List   ListCmd  `cmd:"" help:"List Kedify resources."`
}

// Context carries shared runtime dependencies for CLI commands.
type Context struct {
	APIURL string
	Stdout *os.File
	Stderr *os.File
	Stdin  *os.File
	Client *api.Client
}

// Run parses CLI arguments, executes the selected command, and returns a process exit code.
func Run() int {
	var root Root
	parser := kong.Parse(
		&root,
		kong.Name("kedify"),
		kong.Description("Kedify command line interface."),
	)

	httpClient := &http.Client{Timeout: httpRequestLimit}
	err := parser.Run(&Context{
		APIURL: root.APIURL,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Stdin:  os.Stdin,
		Client: api.NewClient(httpClient),
	})
	parser.FatalIfErrorf(err)

	return 0
}

package cli

import (
	"github.com/kedify/cli/internal/config"
	"github.com/kedify/cli/internal/output"
)

// ListClustersCmd lists clusters visible to the authenticated user.
type ListClustersCmd struct {
	Output string `help:"Output format." short:"o" enum:"json,yaml" default:"json"`
}

// Run executes the list clusters command.
func (c *ListClustersCmd) Run(app *Context) error {
	creds, err := config.ReadCredentials()
	if err != nil {
		return err
	}

	clusters, err := app.Client.ListClusters(app.APIURL, creds.Token)
	if err != nil {
		return err
	}

	return output.Write(app.Stdout, clusters, c.Output)
}

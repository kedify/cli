package list

import (
	clictx "github.com/kedify/cli/internal/cli/context"
)

type ListClustersCmd struct {
	Output string `name:"output" short:"o" help:"Output format." enum:"text,json,yaml" default:"text"`
}

func (c *ListClustersCmd) Run(ctx *clictx.Context) error {
	token, err := clictx.ResolveToken(ctx)
	if err != nil {
		return err
	}

	clusters, err := ctx.Client.ListClusters(ctx.APIURL, token)
	if err != nil {
		return err
	}

	return ctx.WriteOutput(ctx.Stdout, clusters, c.Output)
}

package list

import (
	clictx "github.com/kedify/cli/internal/cli/context"
)

type ListRecommendationsCmd struct {
	ClusterID string `arg:"" name:"cluster-id" help:"Cluster id."`
	Output    string `name:"output" short:"o" help:"Output format." enum:"text,json,yaml" default:"text"`
}

func (c *ListRecommendationsCmd) Run(ctx *clictx.Context) error {
	token, err := clictx.ResolveToken(ctx)
	if err != nil {
		return err
	}

	recommendations, err := ctx.Client.GetRecommendations(ctx.APIURL, token, c.ClusterID)
	if err != nil {
		return err
	}

	return ctx.WriteOutput(ctx.Stdout, recommendations, c.Output)
}
